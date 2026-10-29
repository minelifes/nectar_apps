package alerts

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/mail"
	"net/smtp"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Channel kinds.
const (
	Ntfy     = "ntfy"
	Telegram = "telegram"
	Email    = "email"
	Webhook  = "webhook"
)

// Kinds lists the supported channel kinds.
var Kinds = []string{Ntfy, Telegram, Email, Webhook}

// ChannelConfig is the configuration of a channel; which fields matter depends on the kind.
type ChannelConfig struct {
	// ntfy: server (default https://ntfy.sh), topic, optional access token
	// webhook: URL, optional secret (HMAC-SHA256 of the body in X-Nectar-Signature)
	URL   string `json:"url,omitempty"`
	Topic string `json:"topic,omitempty"`
	Token string `json:"token,omitempty"`

	// telegram
	BotToken string `json:"bot_token,omitempty"`
	ChatID   string `json:"chat_id,omitempty"`

	// email
	Host     string `json:"host,omitempty"`
	Port     int    `json:"port,omitempty"`
	Security string `json:"security,omitempty"` // starttls (default) | tls | none
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	From     string `json:"from,omitempty"`
	To       string `json:"to,omitempty"` // comma-separated
	// UsePanelSMTP sends through the panel's SMTP settings (only To is needed then).
	UsePanelSMTP bool `json:"use_panel_smtp,omitempty"`

	Secret string `json:"secret,omitempty"`
}

// SecretFields are the config fields never returned by the API.
var SecretFields = []string{"token", "bot_token", "password", "secret"}

// Mask replaces secrets with Masked (an empty secret stays empty).
func (c ChannelConfig) Mask() ChannelConfig {
	for _, p := range []*string{&c.Token, &c.BotToken, &c.Password, &c.Secret} {
		if *p != "" {
			*p = Masked
		}
	}
	return c
}

// Masked stands for an unchanged secret in API input and output.
const Masked = "••••••••"

// KeepSecrets copies secrets from old where the input still holds Masked.
func (c ChannelConfig) KeepSecrets(old ChannelConfig) ChannelConfig {
	keep := func(p *string, o string) {
		if *p == Masked {
			*p = o
		}
	}
	keep(&c.Token, old.Token)
	keep(&c.BotToken, old.BotToken)
	keep(&c.Password, old.Password)
	keep(&c.Secret, old.Secret)
	return c
}

// Validate checks the config of a channel kind and fills in defaults.
func (c *ChannelConfig) Validate(kind string) error {
	switch kind {
	case Ntfy:
		if c.URL == "" {
			c.URL = "https://ntfy.sh"
		}
		if err := httpURL(c.URL); err != nil {
			return err
		}
		c.Topic = strings.TrimSpace(c.Topic)
		if c.Topic == "" || strings.ContainsAny(c.Topic, "/?# ") {
			return errors.New("a topic name is required (letters, digits, - and _)")
		}
	case Telegram:
		if c.BotToken == "" || !strings.Contains(c.BotToken, ":") {
			return errors.New("the bot token from @BotFather looks like 123456:ABC…")
		}
		c.ChatID = strings.TrimSpace(c.ChatID)
		if c.ChatID == "" {
			return errors.New("a chat id is required (your user id, a group id or @channelname)")
		}
	case Email:
		if c.UsePanelSMTP {
			if len(c.recipients()) == 0 {
				return errors.New("at least one recipient is required")
			}
			for _, to := range c.recipients() {
				if _, err := mail.ParseAddress(to); err != nil {
					return fmt.Errorf("invalid recipient %q", to)
				}
			}
			return nil
		}
		c.Host = strings.TrimSpace(c.Host)
		if c.Host == "" {
			return errors.New("an SMTP host is required")
		}
		switch c.Security {
		case "":
			c.Security = "starttls"
		case "starttls", "tls", "none":
		default:
			return errors.New("security must be starttls, tls or none")
		}
		if c.Port == 0 {
			c.Port = map[string]int{"starttls": 587, "tls": 465, "none": 25}[c.Security]
		}
		if c.Port < 1 || c.Port > 65535 {
			return errors.New("invalid SMTP port")
		}
		if _, err := mail.ParseAddress(c.From); err != nil {
			return errors.New("invalid sender address")
		}
		if len(c.recipients()) == 0 {
			return errors.New("at least one recipient is required")
		}
		for _, to := range c.recipients() {
			if _, err := mail.ParseAddress(to); err != nil {
				return fmt.Errorf("invalid recipient %q", to)
			}
		}
	case Webhook:
		if err := httpURL(c.URL); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown channel kind %q", kind)
	}
	return nil
}

func (c ChannelConfig) recipients() []string {
	var out []string
	for _, s := range strings.Split(c.To, ",") {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func httpURL(s string) error {
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return errors.New("a http(s) URL is required")
	}
	return nil
}

// Notification is what a channel sends.
type Notification struct {
	Title    string    `json:"title"`
	Message  string    `json:"message"`
	Severity string    `json:"severity"`
	State    string    `json:"state"` // firing | resolved | test
	Rule     string    `json:"rule,omitempty"`
	Subject  string    `json:"subject,omitempty"`
	Link     string    `json:"link,omitempty"`
	Time     time.Time `json:"time"`
}

// Sender delivers notifications. The zero value is ready to use.
type Sender struct {
	HTTP        *http.Client
	TelegramAPI string // default https://api.telegram.org
	// SMTPDial is used instead of net.Dial in tests.
	SMTPDial func(ctx context.Context, addr string) (net.Conn, error)
}

func (s *Sender) client() *http.Client {
	if s.HTTP != nil {
		return s.HTTP
	}
	return &http.Client{Timeout: 15 * time.Second}
}

// Send delivers n through a channel.
func (s *Sender) Send(ctx context.Context, kind string, c ChannelConfig, n Notification) error {
	switch kind {
	case Ntfy:
		return s.ntfy(ctx, c, n)
	case Telegram:
		return s.telegram(ctx, c, n)
	case Email:
		return s.email(ctx, c, n)
	case Webhook:
		return s.webhook(ctx, c, n)
	}
	return fmt.Errorf("unknown channel kind %q", kind)
}

func (s *Sender) post(req *http.Request) error {
	res, err := s.client().Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 2048))
	if res.StatusCode >= 300 {
		msg := strings.TrimSpace(string(body))
		var j struct {
			Description string `json:"description"` // telegram
			Error       string `json:"error"`       // ntfy
		}
		if json.Unmarshal(body, &j) == nil && (j.Description != "" || j.Error != "") {
			msg = j.Description + j.Error
		}
		return fmt.Errorf("HTTP %d: %s", res.StatusCode, msg)
	}
	return nil
}

func (s *Sender) ntfy(ctx context.Context, c ChannelConfig, n Notification) error {
	body := n.Message
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.URL, "/")+"/"+url.PathEscape(c.Topic), strings.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Title", headerSafe(n.Title))
	prio, tag := "default", "information_source"
	switch {
	case n.State == "resolved":
		prio, tag = "default", "white_check_mark"
	case n.Severity == Critical:
		prio, tag = "urgent", "rotating_light"
	case n.Severity == Warning:
		prio, tag = "high", "warning"
	}
	req.Header.Set("Priority", prio)
	req.Header.Set("Tags", tag)
	if n.Link != "" {
		req.Header.Set("Click", n.Link)
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	return s.post(req)
}

// headerSafe keeps titles valid as HTTP header values (ntfy reads UTF-8 titles, but no line breaks).
func headerSafe(s string) string {
	return strings.NewReplacer("\r", " ", "\n", " ").Replace(s)
}

func (s *Sender) telegram(ctx context.Context, c ChannelConfig, n Notification) error {
	api := s.TelegramAPI
	if api == "" {
		api = "https://api.telegram.org"
	}
	text := "<b>" + htmlEscape(n.Title) + "</b>\n" + htmlEscape(n.Message)
	if n.Link != "" {
		text += "\n" + `<a href="` + htmlEscape(n.Link) + `">Open in Nectar Apps</a>`
	}
	payload, _ := json.Marshal(map[string]any{"chat_id": c.ChatID, "text": text, "parse_mode": "HTML", "disable_web_page_preview": true})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, api+"/bot"+c.BotToken+"/sendMessage", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if err := s.post(req); err != nil {
		// never leak the bot token (it is part of the URL) into errors
		return errors.New(strings.ReplaceAll(err.Error(), c.BotToken, "<token>"))
	}
	return nil
}

func htmlEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;").Replace(s)
}

func (s *Sender) webhook(ctx context.Context, c ChannelConfig, n Notification) error {
	body, _ := json.Marshal(n)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.URL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "nectar-apps-alerts")
	if c.Secret != "" {
		m := hmac.New(sha256.New, []byte(c.Secret))
		m.Write(body)
		req.Header.Set("X-Nectar-Signature", "sha256="+hex.EncodeToString(m.Sum(nil)))
	}
	return s.post(req)
}

func (s *Sender) email(ctx context.Context, c ChannelConfig, n Notification) error {
	addr := net.JoinHostPort(c.Host, strconv.Itoa(c.Port))
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	dial := s.SMTPDial
	if dial == nil {
		dial = func(ctx context.Context, addr string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "tcp", addr)
		}
	}
	conn, err := dial(ctx, addr)
	if err != nil {
		return err
	}
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	}
	tlsCfg := &tls.Config{ServerName: c.Host, MinVersion: tls.VersionTLS12}
	if c.Security == "tls" {
		conn = tls.Client(conn, tlsCfg)
	}
	cl, err := smtp.NewClient(conn, c.Host)
	if err != nil {
		conn.Close()
		return err
	}
	defer cl.Close()
	if c.Security == "starttls" {
		if ok, _ := cl.Extension("STARTTLS"); !ok {
			return errors.New("the SMTP server does not offer STARTTLS")
		}
		if err := cl.StartTLS(tlsCfg); err != nil {
			return err
		}
	}
	if c.Username != "" {
		if ok, _ := cl.Extension("AUTH"); ok {
			if err := cl.Auth(smtp.PlainAuth("", c.Username, c.Password, c.Host)); err != nil {
				return err
			}
		}
	}
	from, _ := mail.ParseAddress(c.From)
	if err := cl.Mail(from.Address); err != nil {
		return err
	}
	var to []string
	for _, r := range c.recipients() {
		a, _ := mail.ParseAddress(r)
		if err := cl.Rcpt(a.Address); err != nil {
			return err
		}
		to = append(to, a.String())
	}
	w, err := cl.Data()
	if err != nil {
		return err
	}
	body := n.Message
	if n.Link != "" {
		body += "\n\n" + n.Link
	}
	msg := "From: " + from.String() + "\r\n" +
		"To: " + strings.Join(to, ", ") + "\r\n" +
		"Subject: " + mimeHeader(n.Title) + "\r\n" +
		"Date: " + n.Time.Format(time.RFC1123Z) + "\r\n" +
		"MIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\nContent-Transfer-Encoding: 8bit\r\n" +
		"X-Nectar-Alert: " + n.State + "\r\n\r\n" +
		strings.ReplaceAll(strings.ReplaceAll(body, "\r\n", "\n"), "\n", "\r\n") + "\r\n"
	if _, err := io.WriteString(w, msg); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return cl.Quit()
}

// mimeHeader encodes non-ASCII subjects (RFC 2047).
func mimeHeader(s string) string { return mime.BEncoding.Encode("utf-8", headerSafe(s)) }
