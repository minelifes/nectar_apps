package auth

import (
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// BasicAuthUser is a user in a basic-auth list. An empty Password keeps the user's current password.
type BasicAuthUser struct {
	User     string `json:"user"`
	Password string `json:"password,omitempty"`
}

// Htpasswd builds htpasswd text ("user:<bcrypt>" lines) for users. Users without a new password
// keep their hash from current. The format matches what nectar-gateway stores.
func Htpasswd(users []BasicAuthUser, current string) (string, error) {
	if len(users) > 50 {
		return "", errors.New("at most 50 basic-auth users")
	}
	old := map[string]string{}
	for _, line := range strings.Split(current, "\n") {
		if u, h, ok := strings.Cut(strings.TrimSpace(line), ":"); ok {
			old[u] = h
		}
	}
	var lines []string
	seen := map[string]bool{}
	for _, u := range users {
		name := strings.TrimSpace(u.User)
		if name == "" || len(name) > 64 || strings.ContainsAny(name, ": \t\n") {
			return "", fmt.Errorf("invalid user name %q", u.User)
		}
		if seen[name] {
			return "", fmt.Errorf("duplicate user %q", name)
		}
		seen[name] = true
		hash := old[name]
		if u.Password != "" {
			if len(u.Password) < 8 || len(u.Password) > 72 {
				return "", fmt.Errorf("password for %q must be 8–72 characters", name)
			}
			b, err := bcrypt.GenerateFromPassword([]byte(u.Password), bcrypt.DefaultCost)
			if err != nil {
				return "", err
			}
			hash = string(b)
		}
		if hash == "" {
			return "", fmt.Errorf("user %q needs a password", name)
		}
		lines = append(lines, name+":"+hash)
	}
	return strings.Join(lines, "\n"), nil
}
