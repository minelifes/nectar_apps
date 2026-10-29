// Package detect inspects uploaded artifacts to prefill the New App wizard: which runtime and
// version fits, which port and health check to use, and whether the file is safe to deploy.
package detect

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"debug/elf"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
)

// Result is what we learned about an artifact.
type Result struct {
	// Type is jar | elf | zip | tar | unknown.
	Type string `json:"type"`
	// Runtime and Version are suggestions for the wizard ("" if unsure).
	Runtime string `json:"runtime,omitempty"`
	Version string `json:"version,omitempty"`
	// Port / HealthPath are suggestions (0 / "" if unknown).
	Port       int    `json:"port,omitempty"`
	HealthPath string `json:"health_path,omitempty"`

	Java    *Java    `json:"java,omitempty"`
	ELF     *ELF     `json:"elf,omitempty"`
	Archive *Archive `json:"archive,omitempty"`

	Warnings []string `json:"warnings,omitempty"`
}

// Java describes a JAR.
type Java struct {
	BytecodeMajor     int    `json:"bytecode_major"`
	MinJava           int    `json:"min_java"`
	MainClass         string `json:"main_class,omitempty"`
	SpringBoot        bool   `json:"spring_boot"`
	SpringBootVersion string `json:"spring_boot_version,omitempty"`
}

// ELF describes a native executable.
type ELF struct {
	Arch   string `json:"arch"`             // amd64 | arm64 | 386 | arm | other
	Libc   string `json:"libc"`             // glibc | musl | static
	Interp string `json:"interp,omitempty"` // dynamic loader path
}

// Archive describes a zip or tar.gz.
type Archive struct {
	Format      string   `json:"format"` // zip | tar.gz
	Entries     int      `json:"entries"`
	StripPrefix string   `json:"strip_prefix,omitempty"` // single top-level folder that will be unwrapped
	Markers     []string `json:"markers,omitempty"`      // e.g. package.json, requirements.txt, index.html
}

// ErrUnsafeArchive is returned for archives with absolute or parent-relative paths (zip-slip).
var ErrUnsafeArchive = errors.New("archive contains unsafe paths (absolute or ../)")

const maxEntries = 200_000

// File inspects the file at p. filename is the original upload name (used for hints only).
func File(p, filename string) (Result, error) {
	f, err := os.Open(p)
	if err != nil {
		return Result{}, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return Result{}, err
	}
	head := make([]byte, 4)
	n, _ := io.ReadFull(f, head)
	head = head[:n]

	switch {
	case bytes.HasPrefix(head, []byte("\x7fELF")):
		return inspectELF(f)
	case bytes.HasPrefix(head, []byte("PK\x03\x04")) || bytes.HasPrefix(head, []byte("PK\x05\x06")):
		zr, err := zip.NewReader(f, st.Size())
		if err != nil {
			return Result{Type: "unknown"}, fmt.Errorf("invalid zip: %w", err)
		}
		if isJar(zr, filename) {
			return inspectJar(zr)
		}
		return inspectZip(zr)
	case bytes.HasPrefix(head, []byte{0x1f, 0x8b}):
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return Result{}, err
		}
		return inspectTarGz(f)
	}
	return Result{Type: "unknown", Warnings: []string{"unrecognized file type; it can still be deployed as a native binary or with a custom image"}}, nil
}

// ------------------------------------------------------------------ java --

func isJar(zr *zip.Reader, filename string) bool {
	if strings.HasSuffix(strings.ToLower(filename), ".jar") {
		return true
	}
	for _, f := range zr.File {
		if f.Name == "META-INF/MANIFEST.MF" {
			return true
		}
	}
	return false
}

func inspectJar(zr *zip.Reader) (Result, error) {
	res := Result{Type: "jar", Runtime: "java", Port: 8080}
	j := &Java{}
	res.Java = j

	manifest := map[string]string{}
	for _, f := range zr.File {
		if f.Name == "META-INF/MANIFEST.MF" {
			manifest = readManifest(f)
			break
		}
	}
	j.MainClass = manifest["Start-Class"]
	if j.MainClass == "" {
		j.MainClass = manifest["Main-Class"]
	}
	j.SpringBootVersion = manifest["Spring-Boot-Version"]
	j.SpringBoot = j.SpringBootVersion != "" || strings.HasPrefix(manifest["Main-Class"], "org.springframework.boot.loader")
	if j.SpringBoot {
		res.HealthPath = "/actuator/health"
	}
	if manifest["Main-Class"] == "" {
		res.Warnings = append(res.Warnings, "no Main-Class in MANIFEST.MF: this JAR may not be executable with java -jar")
	}

	// The highest bytecode version among the app's own classes decides the minimum Java.
	checked := 0
	for _, f := range zr.File {
		name := f.Name
		if !strings.HasSuffix(name, ".class") || strings.HasPrefix(name, "META-INF/versions/") ||
			strings.HasPrefix(name, "BOOT-INF/lib/") || strings.HasPrefix(name, "org/springframework/boot/loader/") {
			continue
		}
		if major := classMajor(f); major > j.BytecodeMajor {
			j.BytecodeMajor = major
		}
		if checked++; checked >= 500 {
			break
		}
	}
	if j.BytecodeMajor >= 45 {
		j.MinJava = j.BytecodeMajor - 44 // 52 → 8, 61 → 17, 65 → 21
		if j.BytecodeMajor < 52 {
			j.MinJava = 8 // anything older runs on 8
		}
		res.Version = javaLTS(j.MinJava)
	}
	return res, nil
}

func readManifest(f *zip.File) map[string]string {
	out := map[string]string{}
	rc, err := f.Open()
	if err != nil {
		return out
	}
	defer rc.Close()
	sc := bufio.NewScanner(io.LimitReader(rc, 1<<20))
	var key string
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		if strings.HasPrefix(line, " ") && key != "" { // continuation line
			out[key] += strings.TrimPrefix(line, " ")
			continue
		}
		if k, v, ok := strings.Cut(line, ":"); ok {
			key = strings.TrimSpace(k)
			out[key] = strings.TrimSpace(v)
		}
	}
	return out
}

func classMajor(f *zip.File) int {
	rc, err := f.Open()
	if err != nil {
		return 0
	}
	defer rc.Close()
	var hdr [8]byte
	if _, err := io.ReadFull(rc, hdr[:]); err != nil || binary.BigEndian.Uint32(hdr[:4]) != 0xCAFEBABE {
		return 0
	}
	return int(binary.BigEndian.Uint16(hdr[6:8]))
}

// javaLTS returns the smallest LTS release that can run code compiled for min.
func javaLTS(min int) string {
	for _, lts := range []int{8, 11, 17, 21, 25} {
		if min <= lts {
			return fmt.Sprint(lts)
		}
	}
	return fmt.Sprint(min)
}

// ------------------------------------------------------------------- elf --

func inspectELF(r io.ReaderAt) (Result, error) {
	ef, err := elf.NewFile(r)
	if err != nil {
		return Result{Type: "unknown"}, fmt.Errorf("invalid ELF file: %w", err)
	}
	defer ef.Close()
	info := &ELF{Libc: "static"}
	switch ef.Machine {
	case elf.EM_X86_64:
		info.Arch = "amd64"
	case elf.EM_AARCH64:
		info.Arch = "arm64"
	case elf.EM_386:
		info.Arch = "386"
	case elf.EM_ARM:
		info.Arch = "arm"
	default:
		info.Arch = strings.ToLower(strings.TrimPrefix(ef.Machine.String(), "EM_"))
	}
	for _, p := range ef.Progs {
		if p.Type != elf.PT_INTERP {
			continue
		}
		b, err := io.ReadAll(io.LimitReader(p.Open(), 4096))
		if err == nil {
			info.Interp = strings.TrimRight(string(b), "\x00")
		}
	}
	switch {
	case info.Interp == "":
		info.Libc = "static"
	case strings.Contains(info.Interp, "musl"):
		info.Libc = "musl"
	default:
		info.Libc = "glibc"
	}
	res := Result{Type: "elf", Runtime: "native", ELF: info, Port: 8080}
	switch info.Libc {
	case "musl":
		res.Version = "alpine"
	case "static":
		res.Version = "static"
	default:
		res.Version = "debian"
	}
	if ef.Type != elf.ET_EXEC && ef.Type != elf.ET_DYN {
		res.Warnings = append(res.Warnings, "this ELF file is not an executable")
	}
	if info.Arch != "amd64" && info.Arch != "arm64" {
		res.Warnings = append(res.Warnings, "unsupported CPU architecture "+info.Arch+" (servers run amd64 or arm64)")
	}
	return res, nil
}

// --------------------------------------------------------------- archives --

func inspectZip(zr *zip.Reader) (Result, error) {
	names := make([]string, 0, len(zr.File))
	for _, f := range zr.File {
		names = append(names, f.Name)
		if len(names) > maxEntries {
			return Result{Type: "zip"}, errors.New("archive has too many entries")
		}
	}
	return classifyArchive("zip", names)
}

func inspectTarGz(r io.Reader) (Result, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return Result{Type: "unknown"}, fmt.Errorf("invalid gzip: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	var names []string
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return Result{Type: "tar"}, fmt.Errorf("invalid tar: %w", err)
		}
		if h.Typeflag == tar.TypeSymlink || h.Typeflag == tar.TypeLink {
			if path.IsAbs(h.Linkname) || strings.Contains(h.Linkname, "..") {
				return Result{Type: "tar"}, ErrUnsafeArchive
			}
		}
		name := h.Name
		if h.Typeflag == tar.TypeDir && !strings.HasSuffix(name, "/") {
			name += "/"
		}
		names = append(names, name)
		if len(names) > maxEntries {
			return Result{Type: "tar"}, errors.New("archive has too many entries")
		}
	}
	return classifyArchive("tar.gz", names)
}

func classifyArchive(format string, raw []string) (Result, error) {
	var names []string
	for _, n := range raw {
		n = strings.TrimPrefix(n, "./")
		if n == "" || n == "." {
			continue
		}
		if path.IsAbs(n) || strings.HasPrefix(n, "../") || strings.Contains(n, "/../") || n == ".." || strings.Contains(n, "\\") {
			return Result{Type: typeOf(format)}, ErrUnsafeArchive
		}
		if strings.HasPrefix(n, "__MACOSX/") || path.Base(n) == ".DS_Store" {
			continue
		}
		names = append(names, n)
	}
	arc := &Archive{Format: format, Entries: len(names)}
	res := Result{Type: typeOf(format), Archive: arc}
	if len(names) == 0 {
		res.Warnings = append(res.Warnings, "archive is empty")
		return res, nil
	}

	// single top-level folder (e.g. dist/) → unwrap
	top := strings.SplitN(names[0], "/", 2)[0]
	single := strings.Contains(names[0], "/")
	for _, n := range names {
		if strings.SplitN(n, "/", 2)[0] != top || !strings.Contains(n, "/") && n != top {
			single = false
			break
		}
	}
	if single {
		arc.StripPrefix = top + "/"
	}
	root := map[string]bool{}
	hasPHP := false
	for _, n := range names {
		rel := strings.TrimPrefix(n, arc.StripPrefix)
		if !strings.Contains(strings.TrimSuffix(rel, "/"), "/") {
			root[rel] = true
		}
		if strings.HasSuffix(rel, ".php") {
			hasPHP = true
		}
	}
	for _, m := range []string{"package.json", "requirements.txt", "pyproject.toml", "index.php", "index.html", "Dockerfile"} {
		if root[m] {
			arc.Markers = append(arc.Markers, m)
		}
	}
	switch {
	case root["package.json"]:
		res.Runtime, res.Port = "node", 3000
	case root["requirements.txt"] || root["pyproject.toml"]:
		res.Runtime, res.Port = "python", 8000
		if !root["requirements.txt"] {
			res.Warnings = append(res.Warnings, "pyproject.toml found but no requirements.txt: dependencies will not be installed automatically")
		}
	case root["index.php"] || hasPHP:
		res.Runtime, res.Port, res.HealthPath = "php", 80, "/"
	case root["index.html"]:
		res.Runtime, res.Port, res.HealthPath = "static", 8080, "/"
	}
	if root["Dockerfile"] && res.Runtime == "" {
		res.Warnings = append(res.Warnings, "a Dockerfile was found: build it into an image and use the Docker image type (Git builds come later)")
	}
	return res, nil
}

func typeOf(format string) string {
	if format == "zip" {
		return "zip"
	}
	return "tar"
}
