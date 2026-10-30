package logstore

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"nectar_apps/internal/kube"
)

func TestStoreSearchAndRetention(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 10, 30, 10, 0, 0, 0, time.UTC)
	now := base
	s.nowFn = func() time.Time { return now }
	for h := 0; h < 3; h++ {
		now = base.Add(time.Duration(h) * time.Hour)
		var lines []Line
		for i := 0; i < 10; i++ {
			pod := "web-a"
			if i%2 == 1 {
				pod = "web-b"
			}
			lines = append(lines, Line{Pod: pod, Time: now.Add(time.Duration(i) * time.Minute), Line: fmt.Sprintf("hour %d line %d %s", h, i, map[bool]string{true: "ERROR boom", false: "ok"}[i == 7])})
		}
		if err := s.Append(1, lines); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Maintain(7, 0); err != nil {
		t.Fatal(err)
	}
	files, _ := filepath.Glob(filepath.Join(dir, "1", "*"))
	gz := 0
	for _, f := range files {
		if strings.HasSuffix(f, ".log.gz") {
			gz++
		}
	}
	if gz != 2 || len(files) != 3 {
		t.Fatalf("after maintenance: %v", files)
	}

	// newest first, across plain and compressed segments
	res, err := s.Search(1, Query{Limit: 5})
	if err != nil || len(res.Lines) != 5 || !res.More || res.Lines[0].Line != "hour 2 line 9 ok" {
		t.Fatalf("page 1: %+v %v", res, err)
	}
	res2, _ := s.Search(1, Query{Limit: 100, To: res.Next})
	if len(res2.Lines) != 25 || res2.More || res2.Lines[len(res2.Lines)-1].Line != "hour 0 line 0 ok" {
		t.Fatalf("page 2: %d %v", len(res2.Lines), res2.More)
	}
	// text, regex, pod and time filters
	if r, _ := s.Search(1, Query{Text: "error"}); len(r.Lines) != 3 {
		t.Fatalf("text search: %d", len(r.Lines))
	}
	if r, _ := s.Search(1, Query{Text: `hour [01] line 7`, Regex: true}); len(r.Lines) != 2 {
		t.Fatalf("regex search: %d", len(r.Lines))
	}
	if _, err := s.Search(1, Query{Text: "(", Regex: true}); err == nil {
		t.Fatal("bad regex accepted")
	}
	if r, _ := s.Search(1, Query{Pod: "web-b", From: base.Add(time.Hour), To: base.Add(2 * time.Hour)}); len(r.Lines) != 5 {
		t.Fatalf("pod + range: %d", len(r.Lines))
	}

	// retention: 1 day later everything older than a day goes; the size cap drops the oldest first
	now = base.Add(27 * time.Hour)
	s.Append(1, []Line{{Pod: "web-a", Time: now, Line: "fresh"}})
	if err := s.Maintain(1, 0); err != nil {
		t.Fatal(err)
	}
	if r, _ := s.Search(1, Query{}); len(r.Lines) != 1 || r.Lines[0].Line != "fresh" {
		t.Fatalf("after retention: %+v", r.Lines)
	}
	if err := s.DeleteApp(1); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "1")); !os.IsNotExist(err) {
		t.Fatal("app logs not deleted")
	}
}

func TestSizeCap(t *testing.T) {
	s, _ := New(t.TempDir())
	base := time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC)
	now := base
	s.nowFn = func() time.Time { return now }
	big := strings.Repeat("x", 4000)
	for h := 0; h < 5; h++ {
		now = base.Add(time.Duration(h) * time.Hour)
		var lines []Line
		for i := 0; i < 100; i++ {
			lines = append(lines, Line{Pod: "p", Time: now.Add(time.Duration(i) * time.Second), Line: fmt.Sprintf("%d %d %s", h, i, big)})
		}
		s.Append(2, lines)
	}
	s.Flush()
	before := s.Size(2)
	if err := s.Maintain(30, 2<<10); err != nil { // tiny cap: only the current hour survives
		t.Fatal(err)
	}
	if after := s.Size(2); after >= before {
		t.Fatalf("size cap did nothing: %d → %d", before, after)
	}
	r, _ := s.Search(2, Query{Limit: 1000})
	if len(r.Lines) == 0 || !strings.HasPrefix(r.Lines[len(r.Lines)-1].Line, "4 ") {
		t.Fatalf("oldest hours should go first; oldest left: %.10q", r.Lines[len(r.Lines)-1].Line)
	}
}

// fakeKube serves pods and a log stream per pod.
type fakeKube struct {
	mu   sync.Mutex
	pods []kube.Object
	logs map[string]string
	hits map[string]int
}

func (f *fakeKube) List(context.Context, string, string, string) ([]kube.Object, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.pods, nil
}

func (f *fakeKube) Stream(_ context.Context, path string) (io.ReadCloser, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for pod, text := range f.logs {
		if strings.Contains(path, "/pods/"+pod+"/log") {
			f.hits[pod]++
			return io.NopCloser(strings.NewReader(text)), nil
		}
	}
	return nil, fmt.Errorf("no such pod")
}

func TestCollector(t *testing.T) {
	s, _ := New(t.TempDir())
	pod := func(name, app string) kube.Object {
		return kube.Object{"metadata": map[string]any{"name": name, "namespace": "prj-shop", "labels": map[string]any{labelAppID: app}},
			"status": map[string]any{"containerStatuses": []any{map[string]any{"state": map[string]any{"running": map[string]any{}}}}}}
	}
	fk := &fakeKube{pods: []kube.Object{pod("web-1", "3"), pod("api-1", "4")}, hits: map[string]int{}, logs: map[string]string{
		"web-1": "2026-10-30T10:00:00.000000001Z hello\n2026-10-30T10:00:01.5Z world\n",
		"api-1": "2026-10-30T10:00:02Z started\n",
	}}
	c := &Collector{Store: s, Kube: fk, Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Interval: 10 * time.Millisecond}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { c.Run(ctx); close(done) }()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		fk.mu.Lock()
		h := fk.hits["web-1"]
		fk.mu.Unlock()
		if h >= 3 { // reconnected a few times (each stream ends at once in the fake)
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	<-done
	r, _ := s.Search(3, Query{})
	if len(r.Lines) != 2 || r.Lines[0].Line != "world" || r.Lines[1].Line != "hello" || r.Lines[0].Pod != "web-1" {
		t.Fatalf("web logs (no duplicates after reconnects): %+v", r.Lines)
	}
	if r, _ := s.Search(4, Query{}); len(r.Lines) != 1 {
		t.Fatalf("api logs: %+v", r.Lines)
	}
	// the dedupe state survives a restart
	c2 := &Collector{Store: s, Kube: fk, Log: c.Log}
	c2.loadState()
	if c2.last["prj-shop/web-1"].IsZero() {
		t.Fatal("state not saved")
	}
}
