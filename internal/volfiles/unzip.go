package volfiles

import (
	"archive/tar"
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"strconv"
	"strings"
	"time"
)

// Limits for a .zip unpacked into a volume (the volume's free space is checked as well).
const (
	MaxUnzipEntries = 200_000
	MaxUnzipBytes   = 1 << 40 // 1 TiB unpacked: a sanity bound; free space is the real limit
)

// ZipPlan is a checked zip archive, ready to be streamed into a volume as a tar.
type ZipPlan struct {
	entries []planEntry
	Files   int   // regular files
	Dirs    int   // folders (including parents the archive does not list)
	Skipped int   // symlinks and special entries
	Bytes   int64 // unpacked size of the files
	TarSize int64 // exact size of the tar stream WriteTar produces
}

type planEntry struct {
	hdr  *tar.Header
	file *zip.File // nil for folders
}

// PlanZip checks every entry of a zip archive without unpacking it: names must stay inside the target
// folder (no absolute paths, no ".."), symlinks and other special entries are skipped (a link could point
// outside the volume), macOS "__MACOSX" and ".DS_Store" are left out. Missing parent folders are added.
func PlanZip(zr *zip.Reader) (*ZipPlan, error) {
	if len(zr.File) > MaxUnzipEntries {
		return nil, fmt.Errorf("the archive has more than %d entries", MaxUnzipEntries)
	}
	p := &ZipPlan{}
	dirs := map[string]bool{}
	addDirs := func(name string, mod time.Time) {
		for _, d := range parents(name) {
			if !dirs[d] {
				dirs[d] = true
				p.entries = append(p.entries, planEntry{hdr: &tar.Header{Typeflag: tar.TypeDir, Name: d + "/", Mode: 0o755, ModTime: mod}})
				p.Dirs++
			}
		}
	}
	for _, f := range zr.File {
		name, ok := zipEntryName(f.Name)
		if !ok {
			return nil, fmt.Errorf("unsafe path in the archive: %q", f.Name)
		}
		if name == "" || name == "__MACOSX" || strings.HasPrefix(name, "__MACOSX/") || path.Base(name) == ".DS_Store" {
			continue
		}
		mod := f.Modified
		if mod.IsZero() || mod.Before(time.Unix(0, 0)) {
			mod = time.Now()
		}
		mode := f.Mode()
		switch {
		case mode.IsDir():
			addDirs(name, mod)
			continue
		case !mode.IsRegular():
			p.Skipped++
			continue
		}
		if dir := path.Dir(name); dir != "." {
			addDirs(dir, mod)
		}
		if f.UncompressedSize64 > MaxUnzipBytes || p.Bytes+int64(f.UncompressedSize64) > MaxUnzipBytes {
			return nil, fmt.Errorf("the unpacked files are larger than %d GB", MaxUnzipBytes>>30)
		}
		perm := int64(0o644)
		if mode.Perm()&0o111 != 0 {
			perm = 0o755 // keep scripts executable
		}
		p.Bytes += int64(f.UncompressedSize64)
		p.Files++
		p.entries = append(p.entries, planEntry{file: f,
			hdr: &tar.Header{Typeflag: tar.TypeReg, Name: name, Mode: perm, Size: int64(f.UncompressedSize64), ModTime: mod}})
	}
	// exact tar size: headers (with long-name records) + content padded to 512 + the 1024-byte trailer
	for _, e := range p.entries {
		var c countWriter
		if err := tar.NewWriter(&c).WriteHeader(e.hdr); err != nil {
			return nil, fmt.Errorf("%s: %w", e.hdr.Name, err)
		}
		p.TarSize += c.n + (e.hdr.Size+511)/512*512
	}
	p.TarSize += 1024
	return p, nil
}

// Entries is the number of files and folders that will be created or replaced.
func (p *ZipPlan) Entries() int { return p.Files + p.Dirs }

// WriteTar streams the archive's files as a tar to w (exactly TarSize bytes). A file whose content does
// not match the archive's directory (size or checksum) stops it with an error.
func (p *ZipPlan) WriteTar(w io.Writer) error {
	tw := tar.NewWriter(w)
	for _, e := range p.entries {
		if err := tw.WriteHeader(e.hdr); err != nil {
			return err
		}
		if e.file == nil {
			continue
		}
		rc, err := e.file.Open()
		if err != nil {
			return fmt.Errorf("%s: %w", e.file.Name, err)
		}
		n, err := io.Copy(tw, rc) // tar refuses more than the declared size; zip checks the checksum at the end
		rc.Close()
		if err != nil {
			return fmt.Errorf("%s: %w", e.file.Name, err)
		}
		if n != e.hdr.Size {
			return fmt.Errorf("%s: size does not match the archive", e.file.Name)
		}
	}
	return tw.Close()
}

type countWriter struct{ n int64 }

func (c *countWriter) Write(b []byte) (int, error) { c.n += int64(len(b)); return len(b), nil }

// zipEntryName cleans a zip entry name ("" = the root); false if it would leave the target folder.
func zipEntryName(n string) (string, bool) {
	n = strings.ReplaceAll(n, "\\", "/") // archives made on Windows
	if strings.HasPrefix(n, "/") || strings.Contains(n, "\x00") || (len(n) > 1 && n[1] == ':') {
		return "", false
	}
	for _, part := range strings.Split(n, "/") {
		if part == ".." {
			return "", false
		}
	}
	c := path.Clean(n)
	if c == "." {
		return "", true
	}
	return c, true
}

// parents returns a/b/c → a, a/b, a/b/c.
func parents(dir string) []string {
	var out []string
	parts := strings.Split(dir, "/")
	for i := range parts {
		out = append(out, strings.Join(parts[:i+1], "/"))
	}
	return out
}

// FreeBytes returns the free space of the volume (as the helper pod sees it).
func (s *Session) FreeBytes(ctx context.Context) (int64, error) {
	res, err := s.run(ctx, `df -kP "$1" | awk 'NR==2 {print $4}'`, nil, 4096, Root)
	if err != nil {
		return 0, err
	}
	kb, err := strconv.ParseInt(strings.TrimSpace(string(res.Stdout)), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("free space of the volume: unexpected df output %q", strings.TrimSpace(string(res.Stdout)))
	}
	return kb << 10, nil
}

// extractScript unpacks $2 bytes of tar from stdin into a temporary folder inside $1, gives everything
// the owner of $1, checks that no file would replace a folder (or the other way round), then moves the
// entries in (folders that exist already are merged). Nothing in $1 changes when a step before the move
// fails. "head -c" ends the input: the exec API cannot close stdin.
const extractScript = `[ -d "$1" ] || exit 44
o=$(stat -c %u:%g "$1"); t="$1/.nectar-unzip-$$"
mkdir "$t" || exit 3
if ! head -c "$2" | tar -x -f - -C "$t" 2>"$t.err"; then cat "$t.err" >&2; rm -rf "$t" "$t.err"; exit 3; fi
rm -f "$t.err"
chown -R "$o" "$t" || { rm -rf "$t"; exit 3; }
clash() {
  local f b n
  for f in "$1"/* "$1"/.[!.]* "$1"/..?*; do
    [ -e "$f" ] || [ -L "$f" ] || continue
    b=${f##*/}; n="$2/$b"
    if [ -d "$f" ]; then
      if [ -L "$n" ] || { [ -e "$n" ] && [ ! -d "$n" ]; }; then echo "${n#/data}: a file is in the way of a folder from the archive" >&2; return 1; fi
      if [ -d "$n" ]; then clash "$f" "$n" || return 1; fi
    elif [ -d "$n" ] && [ ! -L "$n" ]; then echo "${n#/data}: a folder is in the way of a file from the archive" >&2; return 1
    fi
  done
}
merge() {
  local f b n
  for f in "$1"/* "$1"/.[!.]* "$1"/..?*; do
    [ -e "$f" ] || [ -L "$f" ] || continue
    b=${f##*/}; n="$2/$b"
    if [ -d "$f" ] && [ -d "$n" ] && [ ! -L "$n" ]; then merge "$f" "$n" || return 1
    else mv -f "$f" "$n" || return 1
    fi
  done
}
clash "$t" "$1" || { rm -rf "$t"; exit 3; }
merge "$t" "$1" || { rm -rf "$t"; exit 3; }
rm -rf "$t"`

// Extract streams a tar of tarSize bytes from r into a folder of the volume (see extractScript). Existing
// files with the same names are replaced; nothing else is deleted.
func (s *Session) Extract(ctx context.Context, dir string, r io.Reader, tarSize int64) error {
	cmd := []string{"sh", "-c", extractScript, "sh", dir, strconv.FormatInt(tarSize, 10)}
	res, err := s.b.Kube.ExecStream(ctx, s.namespace, s.pod, "files", cmd, r, 64<<10)
	if err != nil {
		return err
	}
	switch res.ExitCode {
	case 0:
		return nil
	case 44:
		return ErrNotFound
	}
	msg := strings.TrimSpace(res.Stderr)
	if msg == "" {
		msg = "unpacking failed (exit code " + strconv.Itoa(res.ExitCode) + ")"
	}
	return errors.New(msg)
}

// Unzip streams a planned archive into dir; progress (if not nil) gets the tar bytes sent so far.
func (s *Session) Unzip(ctx context.Context, dir string, p *ZipPlan, progress func(sent int64)) error {
	pr, pw := io.Pipe()
	go func() {
		var w io.Writer = pw
		if progress != nil {
			w = &progressWriter{w: pw, fn: progress}
		}
		pw.CloseWithError(p.WriteTar(w))
	}()
	err := s.Extract(ctx, dir, pr, p.TarSize)
	pr.CloseWithError(errors.New("unpacking stopped")) // ends WriteTar if the pod stopped reading
	if err == nil && progress != nil {
		progress(p.TarSize)
	}
	return err
}

type progressWriter struct {
	w    io.Writer
	n    int64
	fn   func(int64)
	last time.Time
}

func (p *progressWriter) Write(b []byte) (int, error) {
	n, err := p.w.Write(b)
	p.n += int64(n)
	if n > 0 && time.Since(p.last) > 200*time.Millisecond {
		p.last = time.Now()
		p.fn(p.n)
	}
	return n, err
}
