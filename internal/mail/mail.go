// Package mail sends the panel's own emails (password resets, invitations, alert emails) through the SMTP
// server configured in the settings.
package mail

import (
	"context"
	"errors"
	"strings"
	"time"

	"nectar_apps/internal/alerts"
	"nectar_apps/internal/secretbox"
	"nectar_apps/internal/store"
)

// SettingsKey holds Settings in the settings table.
const SettingsKey = "smtp"

// Settings is the SMTP configuration. The password is stored sealed.
type Settings struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Security string `json:"security"` // starttls | tls | none
	Username string `json:"username"`
	Password string `json:"password,omitempty"` // sealed in the store, masked in the API
	From     string `json:"from"`
	// PanelURL is the public address of the panel, used for links in emails (e.g. https://panel.example.com).
	PanelURL string `json:"panel_url"`
}

// Configured reports whether emails can be sent.
func (s Settings) Configured() bool { return s.Host != "" && s.From != "" }

// ErrNotConfigured means no SMTP server is set up.
var ErrNotConfigured = errors.New("no SMTP server is configured (Settings → Email)")

// Mailer sends emails with the stored settings.
type Mailer struct {
	Store  *store.Store
	Box    *secretbox.Box
	Sender *alerts.Sender
}

// Load returns the settings with the password decrypted.
func (m *Mailer) Load(ctx context.Context) (Settings, error) {
	var s Settings
	if err := m.Store.GetSetting(ctx, SettingsKey, &s); err != nil && !errors.Is(err, store.ErrNotFound) {
		return s, err
	}
	if s.Password != "" {
		pt, err := m.Box.Open(s.Password)
		if err != nil {
			return s, err
		}
		s.Password = pt
	}
	return s, nil
}

// Save validates and stores settings (the password sealed).
func (m *Mailer) Save(ctx context.Context, s Settings) error {
	if s.Password != "" {
		sealed, err := m.Box.Seal(s.Password)
		if err != nil {
			return err
		}
		s.Password = sealed
	}
	return m.Store.SetSetting(ctx, SettingsKey, s)
}

// Channel turns the settings into an email channel config for recipients.
func (s Settings) Channel(to ...string) alerts.ChannelConfig {
	return alerts.ChannelConfig{Host: s.Host, Port: s.Port, Security: s.Security, Username: s.Username, Password: s.Password, From: s.From,
		To: strings.Join(to, ", ")}
}

// Validate checks the settings and fills in defaults.
func (s *Settings) Validate() error {
	s.PanelURL = strings.TrimRight(strings.TrimSpace(s.PanelURL), "/")
	if s.PanelURL != "" && !strings.HasPrefix(s.PanelURL, "https://") && !strings.HasPrefix(s.PanelURL, "http://") {
		return errors.New("the panel address must start with https://")
	}
	if s.Host == "" && s.From == "" {
		return nil // turned off
	}
	c := s.Channel("check@example.com")
	if err := c.Validate(alerts.Email); err != nil {
		return err
	}
	s.Port, s.Security = c.Port, c.Security
	return nil
}

// Send emails one message.
func (m *Mailer) Send(ctx context.Context, to, subject, body string) error {
	s, err := m.Load(ctx)
	if err != nil {
		return err
	}
	if !s.Configured() {
		return ErrNotConfigured
	}
	sender := m.Sender
	if sender == nil {
		sender = &alerts.Sender{}
	}
	return sender.Send(ctx, alerts.Email, s.Channel(to), alerts.Notification{Title: subject, Message: body, State: "mail", Time: time.Now()})
}
