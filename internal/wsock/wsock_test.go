package wsock

import (
	"bufio"
	"crypto/rand"
	"encoding/binary"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// clientFrame builds a masked client frame.
func clientFrame(op byte, p []byte) []byte {
	b := []byte{0x80 | op, 0x80 | byte(len(p))}
	var m [4]byte
	_, _ = rand.Read(m[:])
	b = append(b, m[:]...)
	for i, c := range p {
		b = append(b, c^m[i%4])
	}
	return b
}

func readServerFrame(t *testing.T, br *bufio.Reader) (byte, []byte) {
	t.Helper()
	var h [2]byte
	if _, err := io.ReadFull(br, h[:]); err != nil {
		t.Fatal(err)
	}
	n := int(h[1] & 0x7F)
	if n == 126 {
		var b [2]byte
		io.ReadFull(br, b[:])
		n = int(binary.BigEndian.Uint16(b[:]))
	}
	p := make([]byte, n)
	io.ReadFull(br, p)
	return h[0] & 0x0F, p
}

func TestHandshakeStreamPingClose(t *testing.T) {
	done := make(chan error, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := Upgrade(w, r)
		if err != nil {
			done <- err
			return
		}
		go func() {
			c.WriteText([]byte(`{"line":"hello"}`))
			c.WriteText([]byte(strings.Repeat("x", 300))) // 16-bit length
		}()
		done <- c.ReadLoop(nil)
	}))
	defer srv.Close()

	conn, err := net.Dial("tcp", strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(5 * time.Second))
	io.WriteString(conn, "GET /ws HTTP/1.1\r\nHost: x\r\nUpgrade: websocket\r\nConnection: keep-alive, Upgrade\r\nSec-WebSocket-Version: 13\r\nSec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n\r\n")
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, nil)
	if err != nil || resp.StatusCode != 101 || resp.Header.Get("Sec-WebSocket-Accept") != "s3pPLMBiTxaQ9kYGzzhZRbK+xOo=" {
		t.Fatalf("handshake: %v %+v", err, resp)
	}
	if op, p := readServerFrame(t, br); op != opText || string(p) != `{"line":"hello"}` {
		t.Fatalf("frame 1: %d %q", op, p)
	}
	if op, p := readServerFrame(t, br); op != opText || len(p) != 300 {
		t.Fatalf("frame 2: %d %d", op, len(p))
	}
	conn.Write(clientFrame(opPing, []byte("hi")))
	if op, p := readServerFrame(t, br); op != opPong || string(p) != "hi" {
		t.Fatalf("pong: %d %q", op, p)
	}
	conn.Write(clientFrame(opClose, []byte{0x03, 0xE8}))
	if op, _ := readServerFrame(t, br); op != opClose {
		t.Fatalf("close echo: %d", op)
	}
	if err := <-done; err != io.EOF {
		t.Fatalf("read loop: %v", err)
	}
}

func TestNotUpgrade(t *testing.T) {
	rec := httptest.NewRecorder()
	if _, err := Upgrade(rec, httptest.NewRequest("GET", "/", nil)); err != ErrNotWebSocket {
		t.Fatalf("plain GET: %v", err)
	}
}
