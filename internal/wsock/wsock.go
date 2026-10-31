// Package wsock is a minimal WebSocket (RFC 6455): the server side streams JSON to the UI (live logs, web
// terminal) and the client side talks to the Kubernetes exec API. No extensions, no fragmentation of
// outgoing frames (incoming fragments are reassembled). Kept in-house so the panel needs no extra module.
package wsock

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"crypto/tls"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const guid = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

const (
	opCont   = 0x0
	opText   = 0x1
	opBinary = 0x2
	opClose  = 0x8
	opPing   = 0x9
	opPong   = 0xA

	maxClientFrame = 64 << 10 // frames from browsers
	maxServerFrame = 4 << 20  // frames from servers we dial (exec output)
)

// Message types for ReadMessage and WriteMessage.
const (
	Text   = opText
	Binary = opBinary
)

// ErrNotWebSocket means the request is not a WebSocket handshake.
var ErrNotWebSocket = errors.New("not a websocket handshake")

// IsUpgrade reports whether r asks for a WebSocket.
func IsUpgrade(r *http.Request) bool {
	return headerHas(r.Header, "Connection", "upgrade") && strings.EqualFold(r.Header.Get("Upgrade"), "websocket")
}

func headerHas(h http.Header, name, token string) bool {
	for _, v := range h.Values(name) {
		for _, part := range strings.Split(v, ",") {
			if strings.EqualFold(strings.TrimSpace(part), token) {
				return true
			}
		}
	}
	return false
}

// Conn is an upgraded connection. WriteText is safe for concurrent use.
type Conn struct {
	c      net.Conn
	br     *bufio.Reader
	wmu    sync.Mutex
	client bool // we dialed: outgoing frames are masked, incoming ones are not
	closed bool // a close frame was sent (RFC 6455 §5.5.1: exactly one; browsers fail the connection on a second)
}

// ErrClosed is returned by writes after the close frame was sent.
var ErrClosed = errors.New("websocket: connection closed")

// Upgrade completes the handshake. The caller has authenticated and checked the Origin already.
func Upgrade(w http.ResponseWriter, r *http.Request) (*Conn, error) {
	if r.Method != http.MethodGet || !IsUpgrade(r) {
		return nil, ErrNotWebSocket
	}
	if r.Header.Get("Sec-WebSocket-Version") != "13" {
		w.Header().Set("Sec-WebSocket-Version", "13")
		http.Error(w, "unsupported websocket version", http.StatusUpgradeRequired)
		return nil, ErrNotWebSocket
	}
	key := r.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		http.Error(w, "missing Sec-WebSocket-Key", http.StatusBadRequest)
		return nil, ErrNotWebSocket
	}
	c, rw, err := http.NewResponseController(w).Hijack()
	if err != nil {
		return nil, err
	}
	sum := sha1.Sum([]byte(key + guid))
	resp := "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: " +
		base64.StdEncoding.EncodeToString(sum[:]) + "\r\n\r\n"
	_ = c.SetDeadline(time.Time{})
	if _, err := rw.WriteString(resp); err != nil {
		c.Close()
		return nil, err
	}
	if err := rw.Flush(); err != nil {
		c.Close()
		return nil, err
	}
	return &Conn{c: c, br: rw.Reader}, nil
}

// WriteText sends one text message.
func (c *Conn) WriteText(p []byte) error { return c.write(opText, p) }

// WriteMessage sends one Text or Binary message.
func (c *Conn) WriteMessage(op byte, p []byte) error { return c.write(op, p) }

// Ping sends a ping (keeps proxies from closing an idle stream).
func (c *Conn) Ping() error { return c.write(opPing, nil) }

func (c *Conn) write(op byte, p []byte) error {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	if c.closed {
		return ErrClosed
	}
	if op == opClose {
		c.closed = true
	}
	hdr := make([]byte, 2, 14)
	hdr[0] = 0x80 | op
	var maskBit byte
	if c.client {
		maskBit = 0x80
	}
	switch n := len(p); {
	case n < 126:
		hdr[1] = maskBit | byte(n)
	case n <= 0xFFFF:
		hdr[1] = maskBit | 126
		hdr = binary.BigEndian.AppendUint16(hdr, uint16(n))
	default:
		hdr[1] = maskBit | 127
		hdr = binary.BigEndian.AppendUint64(hdr, uint64(n))
	}
	if c.client {
		var mask [4]byte
		_, _ = rand.Read(mask[:])
		hdr = append(hdr, mask[:]...)
		q := make([]byte, len(p))
		for i := range p {
			q[i] = p[i] ^ mask[i%4]
		}
		p = q
	}
	_ = c.c.SetWriteDeadline(time.Now().Add(15 * time.Second))
	if _, err := c.c.Write(append(hdr, p...)); err != nil {
		return err
	}
	return nil
}

// ReadMessage returns the next data message (Text or Binary), answering pings and reassembling fragments.
// It returns io.EOF when the peer closes the connection.
func (c *Conn) ReadMessage() (byte, []byte, error) {
	var msgOp byte
	var buf []byte
	for {
		op, fin, payload, err := c.readFrameFin()
		if err != nil {
			return 0, nil, err
		}
		switch op {
		case opClose:
			_ = c.write(opClose, payload[:min(len(payload), 2)])
			return 0, nil, io.EOF
		case opPing:
			_ = c.write(opPong, payload)
			continue
		case opPong:
			continue
		case opCont:
			buf = append(buf, payload...)
		default:
			msgOp, buf = op, payload
		}
		if len(buf) > maxServerFrame {
			return 0, nil, errors.New("websocket: message too large")
		}
		if fin {
			return msgOp, buf, nil
		}
	}
}

// ReadLoop answers pings and returns when the client closes the connection or sends garbage.
// Data messages from the client are passed to onText (may be nil).
func (c *Conn) ReadLoop(onText func([]byte)) error {
	for {
		op, payload, err := c.readFrame()
		if err != nil {
			return err
		}
		switch op {
		case opClose:
			_ = c.write(opClose, payload[:min(len(payload), 2)])
			return io.EOF
		case opPing:
			_ = c.write(opPong, payload)
		case opPong:
		default:
			if onText != nil {
				onText(payload)
			}
		}
	}
}

func (c *Conn) readFrame() (byte, []byte, error) {
	op, _, p, err := c.readFrameFin()
	return op, p, err
}

func (c *Conn) readFrameFin() (byte, bool, []byte, error) {
	var h [2]byte
	if _, err := io.ReadFull(c.br, h[:]); err != nil {
		return 0, false, nil, err
	}
	op := h[0] & 0x0F
	fin := h[0]&0x80 != 0
	masked := h[1]&0x80 != 0
	n := uint64(h[1] & 0x7F)
	switch n {
	case 126:
		var b [2]byte
		if _, err := io.ReadFull(c.br, b[:]); err != nil {
			return 0, false, nil, err
		}
		n = uint64(binary.BigEndian.Uint16(b[:]))
	case 127:
		var b [8]byte
		if _, err := io.ReadFull(c.br, b[:]); err != nil {
			return 0, false, nil, err
		}
		n = binary.BigEndian.Uint64(b[:])
	}
	limit := uint64(maxClientFrame)
	if c.client {
		limit = maxServerFrame
		if masked {
			return 0, false, nil, errors.New("websocket: masked server frame")
		}
	} else if !masked {
		return 0, false, nil, errors.New("websocket: unmasked client frame")
	}
	if n > limit {
		return 0, false, nil, errors.New("websocket: frame too large")
	}
	var mask [4]byte
	if masked {
		if _, err := io.ReadFull(c.br, mask[:]); err != nil {
			return 0, false, nil, err
		}
	}
	p := make([]byte, n)
	if _, err := io.ReadFull(c.br, p); err != nil {
		return 0, false, nil, err
	}
	if masked {
		for i := range p {
			p[i] ^= mask[i%4]
		}
	}
	return op, fin, p, nil
}

// Close closes the connection (sending a normal-closure frame first).
func (c *Conn) Close() error {
	_ = c.write(opClose, []byte{0x03, 0xE8})
	return c.c.Close()
}

// Dial opens a client connection to a ws:// or wss:// URL (or http/https). header carries authorization and
// Sec-WebSocket-Protocol offers; the chosen subprotocol is returned.
func Dial(ctx context.Context, rawURL string, header http.Header, tlsCfg *tls.Config) (*Conn, string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, "", err
	}
	secure := u.Scheme == "wss" || u.Scheme == "https"
	host := u.Host
	if u.Port() == "" {
		if secure {
			host = net.JoinHostPort(u.Hostname(), "443")
		} else {
			host = net.JoinHostPort(u.Hostname(), "80")
		}
	}
	var d net.Dialer
	nc, err := d.DialContext(ctx, "tcp", host)
	if err != nil {
		return nil, "", err
	}
	if secure {
		cfg := &tls.Config{MinVersion: tls.VersionTLS12}
		if tlsCfg != nil {
			cfg = tlsCfg.Clone()
		}
		if cfg.ServerName == "" {
			cfg.ServerName = u.Hostname()
		}
		cfg.NextProtos = []string{"http/1.1"} // no HTTP/2: the upgrade needs HTTP/1.1
		tc := tls.Client(nc, cfg)
		if err := tc.HandshakeContext(ctx); err != nil {
			nc.Close()
			return nil, "", err
		}
		nc = tc
	}
	if dl, ok := ctx.Deadline(); ok {
		_ = nc.SetDeadline(dl)
	}
	var keyBytes [16]byte
	_, _ = rand.Read(keyBytes[:])
	key := base64.StdEncoding.EncodeToString(keyBytes[:])
	req := &http.Request{Method: http.MethodGet, URL: u, Host: u.Host, Header: http.Header{}, Proto: "HTTP/1.1", ProtoMajor: 1, ProtoMinor: 1}
	for k, v := range header {
		req.Header[k] = v
	}
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Sec-WebSocket-Version", "13")
	req.Header.Set("Sec-WebSocket-Key", key)
	if err := req.Write(nc); err != nil {
		nc.Close()
		return nil, "", err
	}
	br := bufio.NewReader(nc)
	resp, err := http.ReadResponse(br, req)
	if err != nil {
		nc.Close()
		return nil, "", err
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		nc.Close()
		return nil, "", &HandshakeError{Code: resp.StatusCode, Body: strings.TrimSpace(string(b))}
	}
	sum := sha1.Sum([]byte(key + guid))
	if resp.Header.Get("Sec-WebSocket-Accept") != base64.StdEncoding.EncodeToString(sum[:]) {
		nc.Close()
		return nil, "", errors.New("websocket: bad Sec-WebSocket-Accept")
	}
	_ = nc.SetDeadline(time.Time{})
	return &Conn{c: nc, br: br, client: true}, resp.Header.Get("Sec-WebSocket-Protocol"), nil
}

// HandshakeError is a refused upgrade (the body usually holds a Kubernetes Status).
type HandshakeError struct {
	Code int
	Body string
}

func (e *HandshakeError) Error() string {
	var st struct {
		Message string `json:"message"`
	}
	if json.Unmarshal([]byte(e.Body), &st) == nil && st.Message != "" {
		return fmt.Sprintf("websocket: %d %s", e.Code, st.Message)
	}
	return fmt.Sprintf("websocket: %d %s", e.Code, e.Body)
}
