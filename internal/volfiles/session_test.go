package volfiles

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"nectar_apps/internal/kube"
)

// localExec runs the helper-pod scripts with a local busybox, mapping /data to a temp dir.
type localExec struct {
	root    string
	busybox string
}

func (l localExec) Apply(context.Context, kube.Object) (kube.Object, error)      { return nil, nil }
func (l localExec) GetObject(context.Context, string, string, string, any) error { return nil }
func (l localExec) Delete(context.Context, string, string, string) error         { return nil }

func (l localExec) ExecRun(ctx context.Context, _, _, _ string, command []string, stdin []byte, maxOut int) (kube.ExecResult, error) {
	args := make([]string, len(command))
	for i, a := range command {
		if strings.HasPrefix(a, Root) {
			a = l.root + strings.TrimPrefix(a, Root)
		}
		args[i] = a
	}
	cmd := exec.CommandContext(ctx, l.busybox, args...)
	cmd.Stdin = bytes.NewReader(stdin)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	res := kube.ExecResult{Stdout: out.Bytes(), Stderr: errb.String()}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		res.ExitCode = ee.ExitCode()
	} else if err != nil {
		return res, err
	}
	if out.Len() > maxOut {
		return res, kube.ErrOutputTooLarge
	}
	return res, nil
}

func TestSessionWithBusybox(t *testing.T) {
	bb, err := exec.LookPath("busybox")
	if err != nil {
		t.Skip("busybox not installed")
	}
	root := t.TempDir()
	s := &Session{b: &Browser{Kube: localExec{root: root, busybox: bb}}, namespace: "ns", pod: "p"}
	ctx := context.Background()
	p := func(rel string) string { return Clean(rel) }

	if err := s.Mkdir(ctx, p("logs/2026")); err != nil {
		t.Fatal(err)
	}
	if err := s.Write(ctx, p("logs/2026/a.txt"), []byte("hello\nworld\n")); err != nil {
		t.Fatal(err)
	}
	if err := s.Write(ctx, p("empty.txt"), nil); err != nil {
		t.Fatal(err)
	}
	if err := s.Write(ctx, p("space name.txt"), []byte("x")); err != nil {
		t.Fatal(err)
	}
	if err := s.Write(ctx, p("missing/dir/a.txt"), []byte("x")); !errors.Is(err, ErrNotFound) {
		t.Fatalf("write into a missing folder: %v", err)
	}
	os.WriteFile(filepath.Join(root, ".hidden"), []byte("h"), 0o600)
	os.Symlink("logs", filepath.Join(root, "current"))

	list, err := s.List(ctx, p("/"))
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, e := range list {
		names = append(names, e.Type+":"+e.Name)
	}
	if got := strings.Join(names, ","); got != "dir:logs,file:.hidden,link:current,file:empty.txt,file:space name.txt" {
		t.Fatalf("list: %s", got)
	}
	if _, err := s.List(ctx, p("nope")); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing folder: %v", err)
	}
	if empty, err := s.List(ctx, p("logs/2026/../2026")); err != nil || len(empty) != 1 || empty[0].Size != 12 {
		t.Fatalf("list subfolder: %+v %v", empty, err)
	}

	b, err := s.Read(ctx, p("logs/2026/a.txt"), 1<<20)
	if err != nil || string(b) != "hello\nworld\n" {
		t.Fatalf("read: %q %v", b, err)
	}
	if _, err := s.Read(ctx, p("logs/2026/a.txt"), 5); err == nil {
		t.Fatal("read past the limit")
	}
	if _, err := s.Read(ctx, p("logs"), 100); !errors.Is(err, ErrNotFound) {
		t.Fatalf("read a folder: %v", err)
	}
	// overwrite keeps the mode
	os.Chmod(filepath.Join(root, "logs/2026/a.txt"), 0o640)
	if err := s.Write(ctx, p("logs/2026/a.txt"), []byte("new")); err != nil {
		t.Fatal(err)
	}
	if st, _ := os.Stat(filepath.Join(root, "logs/2026/a.txt")); st.Mode().Perm() != 0o640 {
		t.Fatalf("mode after overwrite: %v", st.Mode())
	}
	if err := s.Rename(ctx, p("logs/2026/a.txt"), p("logs/b.txt")); err != nil {
		t.Fatal(err)
	}
	if err := s.Rename(ctx, p("logs/b.txt"), p("empty.txt")); err == nil {
		t.Fatal("rename over an existing file")
	}
	if err := s.Remove(ctx, p("logs")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "logs")); !os.IsNotExist(err) {
		t.Fatal("folder not removed")
	}
	if err := s.Remove(ctx, Root); err == nil {
		t.Fatal("removed the volume root")
	}
	if err := s.Mkdir(ctx, p("empty.txt")); err == nil {
		t.Fatal("mkdir over a file")
	}
}
