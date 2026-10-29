package api

import (
	"bufio"
	"io"
	"net"
	"regexp"
	"strings"
	"testing"
	"time"
)

// fakeSMTP accepts messages and hands their data to the channel.
func fakeSMTP(t *testing.T) (port int, msgs chan string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	msgs = make(chan string, 10)
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				r := bufio.NewReader(c)
				say := func(s string) { io.WriteString(c, s+"\r\n") }
				say("220 test")
				var body strings.Builder
				data := false
				for {
					line, err := r.ReadString('\n')
					if err != nil {
						return
					}
					if data {
						if line == ".\r\n" {
							data = false
							msgs <- body.String()
							body.Reset()
							say("250 ok")
							continue
						}
						body.WriteString(line)
						continue
					}
					switch cmd := strings.ToUpper(strings.TrimSpace(line)); {
					case strings.HasPrefix(cmd, "EHLO"):
						say("250-test\r\n250 8BITMIME")
					case strings.HasPrefix(cmd, "DATA"):
						data = true
						say("354 go")
					case strings.HasPrefix(cmd, "QUIT"):
						say("221 bye")
						return
					default:
						say("250 ok")
					}
				}
			}(c)
		}
	}()
	return ln.Addr().(*net.TCPAddr).Port, msgs
}

func TestSMTPResetAndInvite(t *testing.T) {
	e := newEnv(t)
	o := e.owner()
	anon := e.client()

	// without SMTP: no reset emails; invitations return the link
	if code, _, _ := e.do(anon, "POST", "/api/auth/forgot", map[string]string{"email": "owner@x.io"}); code != 409 {
		t.Fatalf("forgot without smtp: %d", code)
	}
	code, inv, _ := e.do(o, "POST", "/api/users", map[string]any{"email": "dev@x.io", "name": "Dev", "invite": true})
	if code != 201 || inv["emailed"] != false || !strings.Contains(inv["invite_url"].(string), "/reset?token=nrst_") {
		t.Fatalf("invite: %d %v", code, inv)
	}
	tok := strings.SplitN(inv["invite_url"].(string), "token=", 2)[1]
	if code, v, _ := e.do(anon, "GET", "/api/auth/token?token="+tok, nil); code != 200 || v["purpose"] != "invite" || v["email"] != "dev@x.io" {
		t.Fatalf("token: %d %v", code, v)
	}
	if code, _, _ := e.do(anon, "POST", "/api/auth/login", map[string]string{"email": "dev@x.io", "password": "placeholder-not-used"}); code != 401 {
		t.Fatalf("invited user with the placeholder: %d", code)
	}
	if code, _, _ := e.do(anon, "POST", "/api/auth/reset", map[string]string{"token": tok, "password": "dev-password-1"}); code != 200 {
		t.Fatalf("accept invite: %d", code)
	}
	if code, _, _ := e.do(anon, "POST", "/api/auth/reset", map[string]string{"token": tok, "password": "dev-password-2"}); code != 404 {
		t.Fatalf("token reused: %d", code)
	}
	if code, _, _ := e.do(e.client(), "POST", "/api/auth/login", map[string]string{"email": "dev@x.io", "password": "dev-password-1"}); code != 200 {
		t.Fatalf("login after invite: %d", code)
	}

	// with SMTP
	port, msgs := fakeSMTP(t)
	if code, v, _ := e.do(o, "PUT", "/api/settings/smtp", map[string]any{"host": "127.0.0.1", "port": port, "security": "none",
		"from": "Nectar <panel@x.io>", "password": "smtp-pass", "username": "u", "panel_url": "https://panel.x.io/"}); code != 200 || v["configured"] != true {
		t.Fatalf("smtp: %d %v", code, v)
	}
	if _, v, _ := e.do(o, "GET", "/api/settings/smtp", nil); v["settings"].(map[string]any)["password"] != "••••••••" {
		t.Fatalf("password not masked: %v", v)
	}
	if code, _, _ := e.do(o, "POST", "/api/settings/smtp/test", map[string]string{"to": "me@x.io"}); code != 200 {
		t.Fatalf("test mail: %d", code)
	}
	if m := <-msgs; !strings.Contains(m, "Subject: Test from Nectar Apps") {
		t.Fatalf("test mail: %s", m)
	}
	if code, _, _ := e.do(anon, "POST", "/api/auth/forgot", map[string]string{"email": "nobody@x.io"}); code != 204 {
		t.Fatalf("forgot unknown: %d", code)
	}
	if code, _, _ := e.do(anon, "POST", "/api/auth/forgot", map[string]string{"email": "dev@x.io"}); code != 204 {
		t.Fatalf("forgot: %d", code)
	}
	var mail string
	select {
	case mail = <-msgs:
	case <-time.After(5 * time.Second):
		t.Fatal("no reset email")
	}
	link := regexp.MustCompile(`https://panel\.x\.io/reset\?token=(nrst_[A-Za-z0-9_-]+)`).FindStringSubmatch(mail)
	if link == nil || !strings.Contains(mail, "To: <dev@x.io>") {
		t.Fatalf("reset email: %s", mail)
	}
	if code, _, _ := e.do(anon, "POST", "/api/auth/reset", map[string]string{"token": link[1], "password": "short"}); code != 400 {
		t.Fatalf("weak password: %d", code)
	}
	if code, _, _ := e.do(anon, "POST", "/api/auth/reset", map[string]string{"token": link[1], "password": "dev-password-3"}); code != 200 {
		t.Fatalf("reset: %d", code)
	}
	if code, _, _ := e.do(e.client(), "POST", "/api/auth/login", map[string]string{"email": "dev@x.io", "password": "dev-password-3"}); code != 200 {
		t.Fatalf("login after reset: %d", code)
	}
}
