// Package sso signs users in with an external identity provider: GitHub (OAuth), Google and any OpenID
// Connect provider (Authentik, Keycloak, Zitadel, Okta, Entra ID …) with the authorization code flow and
// PKCE. It only finds out who the user is (a verified email, a name and, for OIDC, groups); the panel
// decides whether that person may sign in and with which roles.
package sso

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
)

// Provider kinds.
const (
	GitHub = "github"
	Google = "google"
	OIDC   = "oidc"
)

// Provider is one configured identity provider.
type Provider struct {
	ID           string `json:"id"`   // a-z0-9-, used in URLs
	Kind         string `json:"kind"` // github | google | oidc
	Name         string `json:"name"` // button label, e.g. "Company SSO"
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret,omitempty"` // sealed in storage, never returned
	Issuer       string `json:"issuer,omitempty"`        // oidc: https://auth.example.com/application/o/nectar/
	// AllowedDomains limits sign-in to emails of these domains (empty = any verified email; only users that
	// already exist can sign in unless AutoCreate is on).
	AllowedDomains []string `json:"allowed_domains"`
	// AutoCreate creates a user on the first sign-in (needs AllowedDomains, so a public provider like
	// GitHub cannot open the panel to everyone).
	AutoCreate bool `json:"auto_create"`
	// DefaultRole is the role (builtin key or id) given to auto-created users, everywhere.
	DefaultRole string `json:"default_role"`
	// GroupRoles maps OIDC groups to roles (builtin key or id), applied at every sign-in of auto-created
	// users — e.g. {"nectar-admins": "admin", "developers": "developer"}.
	GroupRoles map[string]string `json:"group_roles,omitempty"`
	Enabled    bool              `json:"enabled"`
}

var idRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,28}[a-z0-9])?$`)

// Validate checks a provider's settings.
func (p *Provider) Validate() error {
	p.Name, p.ClientID, p.Issuer = strings.TrimSpace(p.Name), strings.TrimSpace(p.ClientID), strings.TrimRight(strings.TrimSpace(p.Issuer), "/")
	switch {
	case !idRe.MatchString(p.ID):
		return errors.New("id must be 1–30 lowercase letters, digits or dashes")
	case !slices.Contains([]string{GitHub, Google, OIDC}, p.Kind):
		return errors.New("kind must be github, google or oidc")
	case p.ClientID == "":
		return errors.New("client_id is required")
	case p.Kind == OIDC:
		u, err := url.Parse(p.Issuer)
		if err != nil || u.Scheme != "https" || u.Host == "" {
			return errors.New("issuer must be the provider's https URL (its /.well-known/openid-configuration is read)")
		}
	}
	if p.Name == "" {
		p.Name = map[string]string{GitHub: "GitHub", Google: "Google", OIDC: "Single sign-on"}[p.Kind]
	}
	for i, d := range p.AllowedDomains {
		d = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(d), "@"))
		if d == "" || strings.ContainsAny(d, " /@") {
			return fmt.Errorf("allowed domain %q is not a domain", d)
		}
		p.AllowedDomains[i] = d
	}
	if p.AutoCreate && len(p.AllowedDomains) == 0 {
		return errors.New("auto_create needs allowed_domains (otherwise anyone with an account at the provider could sign in)")
	}
	return nil
}

// AllowsEmail reports whether an email's domain is allowed.
func (p Provider) AllowsEmail(email string) bool {
	if len(p.AllowedDomains) == 0 {
		return true
	}
	_, domain, ok := strings.Cut(strings.ToLower(email), "@")
	return ok && slices.Contains(p.AllowedDomains, domain)
}

// Identity is who signed in.
type Identity struct {
	Email  string
	Name   string
	Groups []string
}

// endpoints of a provider.
type endpoints struct {
	auth, token, userinfo string
	scopes                []string
}

// Client talks to providers (HTTP is replaceable in tests).
type Client struct {
	HTTP *http.Client

	mu        sync.Mutex
	discovery map[string]endpoints // issuer → endpoints (cached)
}

func (c *Client) http() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 15 * time.Second}
}

func (c *Client) endpoints(ctx context.Context, p Provider) (endpoints, error) {
	switch p.Kind {
	case GitHub:
		return endpoints{"https://github.com/login/oauth/authorize", "https://github.com/login/oauth/access_token", "https://api.github.com/user",
			[]string{"read:user", "user:email"}}, nil
	case Google:
		return endpoints{"https://accounts.google.com/o/oauth2/v2/auth", "https://oauth2.googleapis.com/token", "https://openidconnect.googleapis.com/v1/userinfo",
			[]string{"openid", "email", "profile"}}, nil
	}
	c.mu.Lock()
	if e, ok := c.discovery[p.Issuer]; ok {
		c.mu.Unlock()
		return e, nil
	}
	c.mu.Unlock()
	var doc struct {
		Authorization string `json:"authorization_endpoint"`
		Token         string `json:"token_endpoint"`
		Userinfo      string `json:"userinfo_endpoint"`
	}
	if err := c.getJSON(ctx, p.Issuer+"/.well-known/openid-configuration", "", &doc); err != nil {
		return endpoints{}, fmt.Errorf("OIDC discovery: %w", err)
	}
	if doc.Authorization == "" || doc.Token == "" || doc.Userinfo == "" {
		return endpoints{}, errors.New("OIDC discovery: the provider does not list authorization, token and userinfo endpoints")
	}
	e := endpoints{doc.Authorization, doc.Token, doc.Userinfo, []string{"openid", "email", "profile", "groups"}}
	c.mu.Lock()
	if c.discovery == nil {
		c.discovery = map[string]endpoints{}
	}
	c.discovery[p.Issuer] = e
	c.mu.Unlock()
	return e, nil
}

// Flow is what the panel remembers between start and callback (in a short-lived cookie).
type Flow struct {
	Provider string `json:"p"`
	State    string `json:"s"`
	Verifier string `json:"v"`
	Next     string `json:"n"` // panel path to open afterwards
	Expires  int64  `json:"e"`
}

func randomString(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// Start returns the provider's authorization URL and the flow to remember.
func (c *Client) Start(ctx context.Context, p Provider, redirectURI, next string) (string, Flow, error) {
	e, err := c.endpoints(ctx, p)
	if err != nil {
		return "", Flow{}, err
	}
	f := Flow{Provider: p.ID, State: randomString(24), Verifier: randomString(48), Next: next, Expires: time.Now().Add(10 * time.Minute).Unix()}
	sum := sha256.Sum256([]byte(f.Verifier))
	q := url.Values{
		"response_type": {"code"}, "client_id": {p.ClientID}, "redirect_uri": {redirectURI},
		"scope": {strings.Join(e.scopes, " ")}, "state": {f.State},
		"code_challenge": {base64.RawURLEncoding.EncodeToString(sum[:])}, "code_challenge_method": {"S256"},
	}
	if p.Kind == Google {
		q.Set("prompt", "select_account")
	}
	sep := "?"
	if strings.Contains(e.auth, "?") {
		sep = "&"
	}
	return e.auth + sep + q.Encode(), f, nil
}

// Finish exchanges the code and returns the verified identity.
func (c *Client) Finish(ctx context.Context, p Provider, f Flow, redirectURI, code string) (Identity, error) {
	e, err := c.endpoints(ctx, p)
	if err != nil {
		return Identity{}, err
	}
	form := url.Values{
		"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {redirectURI},
		"client_id": {p.ClientID}, "client_secret": {p.ClientSecret}, "code_verifier": {f.Verifier},
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, e.token, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := c.http().Do(req)
	if err != nil {
		return Identity{}, fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()
	var tok struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
		Description string `json:"error_description"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&tok); err != nil || tok.AccessToken == "" {
		msg := tok.Description
		if msg == "" {
			msg = tok.Error
		}
		if msg == "" {
			msg = resp.Status
		}
		return Identity{}, fmt.Errorf("the provider refused the sign-in: %s", msg)
	}
	if p.Kind == GitHub {
		return c.github(ctx, e, tok.AccessToken)
	}
	var info struct {
		Email    string `json:"email"`
		Verified *bool  `json:"email_verified"`
		Name     string `json:"name"`
		Username string `json:"preferred_username"`
		Groups   []any  `json:"groups"`
	}
	if err := c.getJSON(ctx, e.userinfo, tok.AccessToken, &info); err != nil {
		return Identity{}, fmt.Errorf("userinfo: %w", err)
	}
	if info.Email == "" {
		return Identity{}, errors.New("the provider did not share an email address (allow the email scope)")
	}
	if info.Verified != nil && !*info.Verified {
		return Identity{}, errors.New("the email address is not verified at the provider")
	}
	id := Identity{Email: strings.ToLower(info.Email), Name: info.Name}
	if id.Name == "" {
		id.Name = info.Username
	}
	for _, g := range info.Groups {
		if s, ok := g.(string); ok {
			id.Groups = append(id.Groups, s)
		}
	}
	return id, nil
}

func (c *Client) github(ctx context.Context, e endpoints, token string) (Identity, error) {
	var user struct {
		Login string `json:"login"`
		Name  string `json:"name"`
	}
	if err := c.getJSON(ctx, e.userinfo, token, &user); err != nil {
		return Identity{}, fmt.Errorf("GitHub user: %w", err)
	}
	var emails []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}
	if err := c.getJSON(ctx, strings.TrimSuffix(e.userinfo, "/user")+"/user/emails", token, &emails); err != nil {
		return Identity{}, fmt.Errorf("GitHub emails: %w", err)
	}
	for _, em := range emails {
		if em.Primary && em.Verified {
			name := user.Name
			if name == "" {
				name = user.Login
			}
			return Identity{Email: strings.ToLower(em.Email), Name: name}, nil
		}
	}
	return Identity{}, errors.New("your GitHub account has no verified primary email")
}

func (c *Client) getJSON(ctx context.Context, u, token string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := c.http().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s answered %s", u, resp.Status)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(out)
}
