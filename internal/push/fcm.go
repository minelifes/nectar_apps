// Package push sends native push notifications to the Nectar mobile and desktop apps through Firebase
// Cloud Messaging (HTTP v1 API). FCM delivers to Android directly and to iOS/macOS through APNs (upload the
// APNs key to the Firebase project). Optional: without a Firebase project the panel uses ntfy, Telegram,
// email or webhooks.
package push

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// ServiceAccount is the part of a Google service account key file FCM needs.
type ServiceAccount struct {
	Type        string `json:"type"`
	ProjectID   string `json:"project_id"`
	ClientEmail string `json:"client_email"`
	PrivateKey  string `json:"private_key"`
	TokenURI    string `json:"token_uri"`
}

// ParseServiceAccount reads a service account JSON key.
func ParseServiceAccount(b []byte) (ServiceAccount, *rsa.PrivateKey, error) {
	var sa ServiceAccount
	if err := json.Unmarshal(b, &sa); err != nil {
		return sa, nil, errors.New("not a service account key (JSON) file")
	}
	if sa.Type != "service_account" || sa.ClientEmail == "" || sa.PrivateKey == "" || sa.ProjectID == "" {
		return sa, nil, errors.New("the key file needs type service_account, project_id, client_email and private_key (Firebase console → Project settings → Service accounts → Generate new private key)")
	}
	block, _ := pem.Decode([]byte(sa.PrivateKey))
	if block == nil {
		return sa, nil, errors.New("private_key is not PEM")
	}
	k, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		if k1, err1 := x509.ParsePKCS1PrivateKey(block.Bytes); err1 == nil {
			k = k1
		} else {
			return sa, nil, fmt.Errorf("private_key: %w", err)
		}
	}
	rk, ok := k.(*rsa.PrivateKey)
	if !ok {
		return sa, nil, errors.New("private_key is not an RSA key")
	}
	if sa.TokenURI == "" {
		sa.TokenURI = "https://oauth2.googleapis.com/token"
	}
	return sa, rk, nil
}

// Message is one notification.
type Message struct {
	Title string
	Body  string
	Data  map[string]string // e.g. path: /apps/3 (opened when the notification is tapped)
}

// ErrUnregistered means the device token is no longer valid (app removed, token rotated): forget it.
var ErrUnregistered = errors.New("the device is no longer registered")

// Client sends through FCM. Endpoint and TokenURI are replaceable in tests.
type Client struct {
	HTTP     *http.Client
	Endpoint string // default https://fcm.googleapis.com

	mu      sync.Mutex
	token   string
	expires time.Time
	keyFor  string // client email the cached token belongs to
}

func (c *Client) http() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 20 * time.Second}
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

// accessToken returns an OAuth token for the service account (cached until shortly before expiry).
func (c *Client) accessToken(ctx context.Context, sa ServiceAccount, key *rsa.PrivateKey) (string, error) {
	c.mu.Lock()
	if c.keyFor == sa.ClientEmail && c.token != "" && time.Until(c.expires) > time.Minute {
		t := c.token
		c.mu.Unlock()
		return t, nil
	}
	c.mu.Unlock()
	now := time.Now()
	header, _ := json.Marshal(map[string]string{"alg": "RS256", "typ": "JWT"})
	claims, _ := json.Marshal(map[string]any{"iss": sa.ClientEmail, "scope": "https://www.googleapis.com/auth/firebase.messaging",
		"aud": sa.TokenURI, "iat": now.Unix(), "exp": now.Add(time.Hour).Unix()})
	unsigned := b64(header) + "." + b64(claims)
	sum := sha256.Sum256([]byte(unsigned))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	if err != nil {
		return "", err
	}
	form := url.Values{"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"}, "assertion": {unsigned + "." + b64(sig)}}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, sa.TokenURI, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.http().Do(req)
	if err != nil {
		return "", fmt.Errorf("google token: %w", err)
	}
	defer resp.Body.Close()
	var tok struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
		Error       string `json:"error_description"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&tok); err != nil || tok.AccessToken == "" {
		if tok.Error == "" {
			tok.Error = resp.Status
		}
		return "", fmt.Errorf("google token: %s", tok.Error)
	}
	c.mu.Lock()
	c.token, c.expires, c.keyFor = tok.AccessToken, now.Add(time.Duration(tok.ExpiresIn)*time.Second), sa.ClientEmail
	c.mu.Unlock()
	return tok.AccessToken, nil
}

// Send delivers m to one device token. ErrUnregistered means the token should be deleted.
func (c *Client) Send(ctx context.Context, sa ServiceAccount, key *rsa.PrivateKey, deviceToken string, m Message) error {
	at, err := c.accessToken(ctx, sa, key)
	if err != nil {
		return err
	}
	data := map[string]string{}
	for k, v := range m.Data {
		data[k] = v
	}
	body, _ := json.Marshal(map[string]any{"message": map[string]any{
		"token":        deviceToken,
		"notification": map[string]string{"title": m.Title, "body": m.Body},
		"data":         data,
		"android":      map[string]any{"priority": "high"},
		"apns":         map[string]any{"payload": map[string]any{"aps": map[string]any{"sound": "default"}}},
	}})
	endpoint := c.Endpoint
	if endpoint == "" {
		endpoint = "https://fcm.googleapis.com"
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, endpoint+"/v1/projects/"+url.PathEscape(sa.ProjectID)+"/messages:send", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+at)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http().Do(req)
	if err != nil {
		return fmt.Errorf("FCM: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return nil
	}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
	var e struct {
		Error struct {
			Message string `json:"message"`
			Status  string `json:"status"`
			Details []struct {
				ErrorCode string `json:"errorCode"`
			} `json:"details"`
		} `json:"error"`
	}
	_ = json.Unmarshal(raw, &e)
	for _, d := range e.Error.Details {
		if d.ErrorCode == "UNREGISTERED" {
			return ErrUnregistered
		}
	}
	if resp.StatusCode == http.StatusNotFound {
		return ErrUnregistered
	}
	msg := e.Error.Message
	if msg == "" {
		msg = strings.TrimSpace(string(raw))
	}
	return fmt.Errorf("FCM %s: %s", resp.Status, msg)
}
