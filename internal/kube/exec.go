package kube

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/url"
	"strings"

	"nectar_apps/internal/wsock"
)

// Exec stream channels of the Kubernetes WebSocket exec protocol (v4/v5.channel.k8s.io): every binary
// message starts with the channel byte.
const (
	ExecStdin  = 0
	ExecStdout = 1
	ExecStderr = 2
	ExecError  = 3 // a metav1.Status JSON when the command ends
	ExecResize = 4 // {"Width": cols, "Height": rows}
)

// Exec starts command in a container and returns the WebSocket of the exec stream. With tty the output
// comes on ExecStdout only (stderr is merged by the terminal).
func (c *Client) Exec(ctx context.Context, namespace, pod, container string, command []string, tty bool) (*wsock.Conn, error) {
	q := url.Values{"stdin": {"true"}, "stdout": {"true"}, "tty": {boolStr(tty)}, "stderr": {boolStr(!tty)}}
	if container != "" {
		q.Set("container", container)
	}
	for _, a := range command {
		q.Add("command", a)
	}
	u := c.base + "/api/v1/namespaces/" + url.PathEscape(namespace) + "/pods/" + url.PathEscape(pod) + "/exec?" + q.Encode()
	u = "ws" + strings.TrimPrefix(u, "http") // http→ws, https→wss
	h := http.Header{}
	h.Set("Sec-WebSocket-Protocol", "v5.channel.k8s.io, v4.channel.k8s.io")
	tok, err := c.bearer()
	if err != nil {
		return nil, err
	}
	if tok != "" {
		h.Set("Authorization", "Bearer "+tok)
	}
	var tlsCfg *tls.Config
	if tr, ok := c.http.Transport.(*http.Transport); ok {
		tlsCfg = tr.TLSClientConfig
	}
	conn, _, err := wsock.Dial(ctx, u, h, tlsCfg)
	return conn, err
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
