package adopt

import (
	"bufio"
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Host is the server the installer runs on. Paths are absolute host paths ("/etc/nginx/nginx.conf").
// Tests use a temporary directory as the root and a fake command runner.
type Host struct {
	// Root is prepended to every path ("" = the real root).
	Root string
	// Runner executes commands (nil = os/exec). Output is stdout+stderr.
	Runner func(ctx context.Context, name string, args ...string) (string, error)
}

func (h Host) p(path string) string {
	if h.Root == "" {
		return path
	}
	return filepath.Join(h.Root, path)
}

// ReadFile reads a host file.
func (h Host) ReadFile(path string) ([]byte, error) { return os.ReadFile(h.p(path)) }

// WriteFile writes a host file atomically, keeping the mode of an existing file.
func (h Host) WriteFile(path string, data []byte) error {
	mode := os.FileMode(0o644)
	if st, err := os.Stat(h.p(path)); err == nil {
		mode = st.Mode().Perm()
	}
	if err := os.MkdirAll(filepath.Dir(h.p(path)), 0o755); err != nil {
		return err
	}
	tmp := h.p(path) + ".nectar-tmp"
	if err := os.WriteFile(tmp, data, mode); err != nil {
		return err
	}
	return os.Rename(tmp, h.p(path))
}

// Exists reports whether a host path exists.
func (h Host) Exists(path string) bool {
	_, err := os.Stat(h.p(path))
	return err == nil
}

// Glob matches host paths; results are host paths (without Root).
func (h Host) Glob(pattern string) []string {
	m, _ := filepath.Glob(h.p(pattern))
	out := make([]string, 0, len(m))
	for _, x := range m {
		if h.Root != "" {
			x = "/" + strings.TrimPrefix(strings.TrimPrefix(x, h.Root), "/")
		}
		out = append(out, x)
	}
	sort.Strings(out)
	return out
}

// Run executes a command on the host.
func (h Host) Run(ctx context.Context, name string, args ...string) (string, error) {
	if h.Runner != nil {
		return h.Runner(ctx, name, args...)
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	return string(out), err
}

// Has reports whether a command exists on the host PATH.
func (h Host) Has(name string) bool {
	if h.Runner != nil {
		_, err := h.Runner(context.Background(), "which", name)
		return err == nil
	}
	_, err := exec.LookPath(name)
	return err == nil
}

// Listener is a process listening on a TCP port.
type Listener struct {
	Port    int    `json:"port"`
	Address string `json:"address"` // "0.0.0.0", "127.0.0.1", "::"
	PID     int    `json:"pid,omitempty"`
	Process string `json:"process,omitempty"` // comm, e.g. "nginx"
	Command string `json:"command,omitempty"` // full command line
	Unit    string `json:"unit,omitempty"`    // systemd unit, if any
	InPod   bool   `json:"in_pod,omitempty"`  // a Kubernetes pod (hostNetwork / hostPort), not a host service
}

// Listeners returns the processes listening on the given TCP ports, read from /proc (no ss/netstat needed).
func (h Host) Listeners(ports ...int) []Listener {
	want := map[int]bool{}
	for _, p := range ports {
		want[p] = true
	}
	inodes := map[string]Listener{}
	for _, f := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		b, err := h.ReadFile(f)
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(bytes.NewReader(b))
		sc.Scan() // header
		for sc.Scan() {
			fields := strings.Fields(sc.Text())
			if len(fields) < 10 || fields[3] != "0A" { // 0A = LISTEN
				continue
			}
			addr, port, ok := parseProcAddr(fields[1])
			if !ok || !want[port] {
				continue
			}
			inodes[fields[9]] = Listener{Port: port, Address: addr}
		}
	}
	if len(inodes) == 0 {
		return nil
	}
	// map socket inodes to processes
	for _, fdDir := range h.Glob("/proc/[0-9]*/fd") {
		pidDir := filepath.Dir(fdDir)
		pid, _ := strconv.Atoi(filepath.Base(pidDir))
		ents, err := os.ReadDir(h.p(fdDir))
		if err != nil {
			continue
		}
		for _, e := range ents {
			link, err := os.Readlink(filepath.Join(h.p(fdDir), e.Name()))
			if err != nil || !strings.HasPrefix(link, "socket:[") {
				continue
			}
			ino := strings.TrimSuffix(strings.TrimPrefix(link, "socket:["), "]")
			l, ok := inodes[ino]
			if !ok || l.PID != 0 {
				continue
			}
			l.PID = pid
			comm, _ := h.ReadFile(pidDir + "/comm")
			l.Process = strings.TrimSpace(string(comm))
			cmd, _ := h.ReadFile(pidDir + "/cmdline")
			l.Command = strings.TrimSpace(strings.ReplaceAll(string(cmd), "\x00", " "))
			cg, _ := h.ReadFile(pidDir + "/cgroup")
			l.Unit = systemdUnit(string(cg))
			l.InPod = strings.Contains(string(cg), "kubepods")
			if l.InPod {
				l.Unit = ""
			}
			inodes[ino] = l
		}
	}
	out := make([]Listener, 0, len(inodes))
	seen := map[string]bool{}
	for _, l := range inodes {
		k := fmt.Sprintf("%d/%s/%d", l.Port, l.Address, l.PID)
		if !seen[k] {
			seen[k] = true
			out = append(out, l)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Port != out[j].Port {
			return out[i].Port < out[j].Port
		}
		return out[i].Address < out[j].Address
	})
	return out
}

// parseProcAddr decodes "0100007F:0050" (IPv4) or the 32-hex-digit IPv6 form.
func parseProcAddr(s string) (string, int, bool) {
	hexIP, hexPort, ok := strings.Cut(s, ":")
	if !ok {
		return "", 0, false
	}
	port, err := strconv.ParseUint(hexPort, 16, 16)
	if err != nil {
		return "", 0, false
	}
	raw, err := hex.DecodeString(hexIP)
	if err != nil || (len(raw) != 4 && len(raw) != 16) {
		return "", 0, false
	}
	// /proc stores each 32-bit word in host (little-endian) order
	ip := make(net.IP, len(raw))
	for w := 0; w < len(raw); w += 4 {
		ip[w], ip[w+1], ip[w+2], ip[w+3] = raw[w+3], raw[w+2], raw[w+1], raw[w]
	}
	return ip.String(), int(port), true
}

// systemdUnit extracts "nginx.service" from a /proc/<pid>/cgroup file.
func systemdUnit(cgroup string) string {
	for _, line := range strings.Split(cgroup, "\n") {
		for _, part := range strings.Split(line, "/") {
			if strings.HasSuffix(part, ".service") {
				return part
			}
		}
	}
	return ""
}

// ErrNotRoot is returned by host-changing commands without root privileges.
var ErrNotRoot = errors.New("this command changes the server and must run as root (sudo)")
