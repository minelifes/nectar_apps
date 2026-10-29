package detect

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func classBytes(major uint16) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint32(b, 0xCAFEBABE)
	binary.BigEndian.PutUint16(b[6:], major)
	return b
}

func writeZip(t *testing.T, name string, files map[string][]byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for n, c := range files {
		w, _ := zw.Create(n)
		w.Write(c)
	}
	zw.Close()
	os.WriteFile(p, buf.Bytes(), 0o600)
	return p
}

func TestSpringBootJar(t *testing.T) {
	p := writeZip(t, "app.jar", map[string][]byte{
		"META-INF/MANIFEST.MF":                    []byte("Manifest-Version: 1.0\r\nMain-Class: org.springframework.boot.loader.launch.JarLauncher\r\nStart-Class: com.example.Demo\r\n ExtraName\r\nSpring-Boot-Version: 3.5.0\r\n"),
		"BOOT-INF/classes/com/example/Demo.class": classBytes(61), // Java 17
		"BOOT-INF/lib/some-lib.jar":               []byte("x"),
		"org/springframework/boot/loader/X.class": classBytes(52),
		"META-INF/versions/21/Foo.class":          classBytes(65), // ignored
	})
	r, err := File(p, "app.jar")
	if err != nil {
		t.Fatal(err)
	}
	if r.Runtime != "java" || r.Version != "17" || r.Java.MinJava != 17 || !r.Java.SpringBoot ||
		r.Java.MainClass != "com.example.DemoExtraName" || r.HealthPath != "/actuator/health" {
		t.Fatalf("unexpected: %+v %+v", r, r.Java)
	}
}

func TestPlainJarPicksLTS(t *testing.T) {
	p := writeZip(t, "tool.jar", map[string][]byte{
		"META-INF/MANIFEST.MF": []byte("Main-Class: a.B\n"),
		"a/B.class":            classBytes(58), // Java 14 → needs 17 LTS
		"a/C.class":            classBytes(55),
	})
	r, _ := File(p, "tool.jar")
	if r.Version != "17" || r.Java.BytecodeMajor != 58 || r.Java.SpringBoot {
		t.Fatalf("unexpected: %+v %+v", r, r.Java)
	}
}

func TestArchives(t *testing.T) {
	cases := []struct {
		files   map[string][]byte
		runtime string
		strip   string
	}{
		{map[string][]byte{"dist/index.html": nil, "dist/assets/app.js": nil}, "static", "dist/"},
		{map[string][]byte{"package.json": nil, "server.js": nil}, "node", ""},
		{map[string][]byte{"app/requirements.txt": nil, "app/app.py": nil}, "python", "app/"},
		{map[string][]byte{"index.php": nil, "lib/x.php": nil}, "php", ""},
		{map[string][]byte{"__MACOSX/._x": nil, "site/index.html": nil}, "static", "site/"},
	}
	for i, c := range cases {
		r, err := File(writeZip(t, "a.zip", c.files), "a.zip")
		if err != nil || r.Runtime != c.runtime || r.Archive.StripPrefix != c.strip {
			t.Errorf("case %d: %+v %+v %v", i, r, r.Archive, err)
		}
	}
}

func TestZipSlipRejected(t *testing.T) {
	for _, bad := range []string{"../etc/passwd", "/abs", "a/../../x", "a\\b"} {
		_, err := File(writeZip(t, "a.zip", map[string][]byte{bad: nil}), "a.zip")
		if !errors.Is(err, ErrUnsafeArchive) {
			t.Errorf("%q: %v", bad, err)
		}
	}
}

func TestTarGz(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, n := range []string{"build/", "build/index.html"} {
		typ := byte(tar.TypeReg)
		if n[len(n)-1] == '/' {
			typ = tar.TypeDir
		}
		tw.WriteHeader(&tar.Header{Name: n, Typeflag: typ, Mode: 0o644})
	}
	tw.WriteHeader(&tar.Header{Name: "build/evil", Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd"})
	tw.Close()
	gz.Close()
	p := filepath.Join(t.TempDir(), "site.tgz")
	os.WriteFile(p, buf.Bytes(), 0o600)
	if _, err := File(p, "site.tgz"); !errors.Is(err, ErrUnsafeArchive) {
		t.Fatalf("absolute symlink accepted: %v", err)
	}
}

func TestELF(t *testing.T) {
	// the Go test binary itself is an ELF on Linux
	if runtime.GOOS != "linux" {
		t.Skip("ELF test runs on linux")
	}
	exe, _ := os.Executable()
	r, err := File(exe, "app")
	if err != nil || r.Type != "elf" || r.Runtime != "native" || r.ELF.Arch != runtime.GOARCH {
		t.Fatalf("unexpected: %+v %v", r, err)
	}
	// a static Go binary must map to the distroless static image
	if _, err := exec.LookPath("go"); err == nil {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc main(){}\n"), 0o600)
		os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module m\n\ngo 1.22\n"), 0o600)
		cmd := exec.Command("go", "build", "-o", "bin")
		cmd.Dir, cmd.Env = dir, append(os.Environ(), "CGO_ENABLED=0", "GOFLAGS=")
		if out, err := cmd.CombinedOutput(); err == nil {
			r, _ := File(filepath.Join(dir, "bin"), "bin")
			if r.ELF.Libc != "static" || r.Version != "static" {
				t.Errorf("static binary: %+v", r.ELF)
			}
		} else {
			t.Logf("skip static build: %s", out)
		}
	}
}

func TestUnknown(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.txt")
	os.WriteFile(p, []byte("hello"), 0o600)
	r, err := File(p, "x.txt")
	if err != nil || r.Type != "unknown" || len(r.Warnings) == 0 {
		t.Fatalf("%+v %v", r, err)
	}
}
