package api

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Chunked .zip uploads into a volume: size limit, offsets, a resent chunk, unzip only when complete,
// cancel removes the file. (Unpacking itself is covered by volfiles.TestUnzipWithBusybox.)
func TestZipUploadChunks(t *testing.T) {
	e := newEnv(t)
	e.api.cfg.DataDir = t.TempDir()
	e.api.cfg.MaxZipBytes = 1 << 20
	o := e.owner()
	_, proj, _ := e.do(o, "POST", "/api/projects", map[string]any{"slug": "shop", "name": "Shop"})
	_, app, _ := e.do(o, "POST", "/api/projects/"+itoa(proj["id"].(float64))+"/apps", map[string]any{"slug": "web", "name": "Web",
		"runtime": "image", "config": map[string]any{"image": "nginx:1", "port": 80}})
	base := "/api/apps/" + itoa(app["id"].(float64)) + "/volumes/data/zip-uploads"

	if code, _, _ := e.do(o, "POST", base, map[string]any{"size": 2 << 20, "filename": "big.zip"}); code != 413 {
		t.Fatalf("over the limit: %d", code)
	}
	code, u, _ := e.do(o, "POST", base, map[string]any{"size": 10, "filename": "../site.zip"})
	if code != 201 || u["state"] != "uploading" || u["filename"] == "../site.zip" {
		t.Fatalf("create: %d %v", code, u)
	}
	id := u["id"].(string)
	file := filepath.Join(e.api.cfg.DataDir, "zip-uploads", id+".zip")

	chunk := func(offset string, body string) (int, string) {
		c, _, b := e.raw(o, "POST", base+"/"+id+"?offset="+offset, []byte(body), "Content-Type", "application/octet-stream")
		return c, string(b)
	}
	if c, b := chunk("3", "abc"); c != 409 || !strings.Contains(b, `"offset":0`) {
		t.Fatalf("wrong offset: %d %s", c, b)
	}
	if c, b := chunk("0", "hello"); c != 200 || !strings.Contains(b, `"offset":5`) {
		t.Fatalf("chunk 1: %d %s", c, b)
	}
	if code, _, _ := e.do(o, "POST", base+"/"+id+"/unzip", map[string]any{"path": "/"}); code != 409 {
		t.Fatalf("unzip before the end: %d", code)
	}
	if c, b := chunk("5", "world!!"); c != 400 || !strings.Contains(b, `"offset":5`) {
		t.Fatalf("past the declared size: %d %s", c, b)
	}
	if c, _ := chunk("5", "world"); c != 200 {
		t.Fatalf("chunk 2 resent: %d", c)
	}
	if got, _ := os.ReadFile(file); !bytes.Equal(got, []byte("helloworld")) {
		t.Fatalf("file: %q", got)
	}
	if _, st, _ := e.do(o, "GET", base+"/"+id, nil); st["offset"].(float64) != 10 || st["percent"].(float64) != 100 {
		t.Fatalf("status: %v", st)
	}
	// another app's volume can't see it
	_, app2, _ := e.do(o, "POST", "/api/projects/"+itoa(proj["id"].(float64))+"/apps", map[string]any{"slug": "api", "name": "API",
		"runtime": "image", "config": map[string]any{"image": "nginx:1", "port": 80}})
	if code, _, _ := e.do(o, "GET", "/api/apps/"+itoa(app2["id"].(float64))+"/volumes/data/zip-uploads/"+id, nil); code != 404 {
		t.Fatalf("other app: %d", code)
	}
	if code, _, _ := e.do(o, "DELETE", base+"/"+id, nil); code != 204 {
		t.Fatalf("cancel: %d", code)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatal("file left after cancel")
	}
	if code, _, _ := e.do(o, "GET", base+"/"+id, nil); code != 404 {
		t.Fatalf("after cancel: %d", code)
	}
}

func TestHumanBytes(t *testing.T) {
	for n, want := range map[int64]string{512: "512 B", 1536: "1.5 KB", 2 << 30: "2 GB", 10 << 40: "10 TB"} {
		if got := humanBytes(n); got != want {
			t.Fatalf("%d: %s", n, got)
		}
	}
}
