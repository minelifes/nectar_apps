// Package logstore keeps the container logs of apps on the panel's disk, so they survive pod restarts and
// deploys and can be searched later (PLAN.md: persistent logs).
//
// Layout: <dir>/<app id>/<YYYYMMDDHH>.log for the current hour, gzip-compressed to .log.gz once the hour is
// over. Each line is "<unix nanos>\t<pod>\t<text>". Retention: days and a size cap per app.
package logstore

import (
	"bufio"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Line is one stored log line.
type Line struct {
	Pod  string    `json:"pod"`
	Time time.Time `json:"t"`
	Line string    `json:"line"`
}

// Store writes and searches the log files.
type Store struct {
	Dir string

	mu    sync.Mutex
	open  map[int64]*segment // current segment per app
	nowFn func() time.Time
}

type segment struct {
	hour string
	f    *os.File
	w    *bufio.Writer
}

// New creates the store directory.
func New(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return &Store{Dir: dir, open: map[int64]*segment{}}, nil
}

func (s *Store) now() time.Time {
	if s.nowFn != nil {
		return s.nowFn()
	}
	return time.Now()
}

func hourKey(t time.Time) string { return t.UTC().Format("2006010215") }

func (s *Store) appDir(app int64) string { return filepath.Join(s.Dir, strconv.FormatInt(app, 10)) }

// Append stores lines of an app (in the hour segment of each line's time, normally the current one).
func (s *Store) Append(app int64, lines []Line) error {
	if len(lines) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, l := range lines {
		seg, err := s.segmentFor(app, hourKey(l.Time))
		if err != nil {
			return err
		}
		text := strings.ReplaceAll(strings.ReplaceAll(l.Line, "\n", " "), "\r", "")
		if _, err := fmt.Fprintf(seg.w, "%d\t%s\t%s\n", l.Time.UnixNano(), l.Pod, text); err != nil {
			return err
		}
	}
	return nil
}

// segmentFor returns the open segment for app and hour (closing an older one). Lines of an older hour go
// to the open (current) segment.
func (s *Store) segmentFor(app int64, hour string) (*segment, error) {
	if cur := hourKey(s.now()); hour < cur {
		hour = cur // late lines (e.g. read again after a restart) never reopen a finished hour being compressed
	}
	if seg := s.open[app]; seg != nil {
		if seg.hour == hour || hour < seg.hour {
			return seg, nil
		}
		s.closeSeg(seg)
		delete(s.open, app)
	}
	if err := os.MkdirAll(s.appDir(app), 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(s.appDir(app), hour+".log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	seg := &segment{hour: hour, f: f, w: bufio.NewWriterSize(f, 32<<10)}
	s.open[app] = seg
	return seg, nil
}

func (s *Store) closeSeg(seg *segment) {
	_ = seg.w.Flush()
	_ = seg.f.Close()
}

// Flush writes buffered lines to disk.
func (s *Store) Flush() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, seg := range s.open {
		_ = seg.w.Flush()
	}
}

// Close flushes and closes every open file.
func (s *Store) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for app, seg := range s.open {
		s.closeSeg(seg)
		delete(s.open, app)
	}
}

// Maintain compresses finished hours and applies the retention: segments older than days are deleted, and
// the oldest ones go first while an app uses more than maxBytes.
func (s *Store) Maintain(days int, maxBytes int64) error {
	current := hourKey(s.now())
	cutoff := hourKey(s.now().Add(-time.Duration(days) * 24 * time.Hour))
	s.mu.Lock()
	for app, seg := range s.open { // close segments of finished hours
		if seg.hour < current {
			s.closeSeg(seg)
			delete(s.open, app)
		}
	}
	s.mu.Unlock()
	apps, err := os.ReadDir(s.Dir)
	if err != nil {
		return err
	}
	var errs []error
	for _, a := range apps {
		if !a.IsDir() {
			continue
		}
		dir := filepath.Join(s.Dir, a.Name())
		segs, err := segments(dir)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		var total int64
		for i := len(segs) - 1; i >= 0; i-- { // newest first
			sg := segs[i]
			if sg.hour < cutoff {
				errs = append(errs, os.Remove(sg.path))
				continue
			}
			if !sg.gz && sg.hour < current {
				if err := compress(sg.path); err != nil {
					errs = append(errs, err)
				} else if st, err := os.Stat(sg.path + ".gz"); err == nil {
					sg.size = st.Size()
					sg.path += ".gz"
				}
			}
			total += sg.size
			if maxBytes > 0 && total > maxBytes && sg.hour < current {
				errs = append(errs, os.Remove(sg.path))
			}
		}
		if left, _ := os.ReadDir(dir); len(left) == 0 {
			_ = os.Remove(dir)
		}
	}
	return errors.Join(errs...)
}

// DeleteApp removes the logs of a deleted app.
func (s *Store) DeleteApp(app int64) error {
	s.mu.Lock()
	if seg := s.open[app]; seg != nil {
		s.closeSeg(seg)
		delete(s.open, app)
	}
	s.mu.Unlock()
	return os.RemoveAll(s.appDir(app))
}

// Size returns the bytes used by an app's logs.
func (s *Store) Size(app int64) int64 {
	segs, _ := segments(s.appDir(app))
	var n int64
	for _, sg := range segs {
		n += sg.size
	}
	return n
}

type segFile struct {
	hour string
	path string
	gz   bool
	size int64
}

// segments lists an app's segment files, oldest first (a plain and a .gz file of the same hour can both
// exist briefly; both are read).
func segments(dir string) ([]segFile, error) {
	ents, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []segFile
	for _, e := range ents {
		name := e.Name()
		hour, gz := "", false
		switch {
		case strings.HasSuffix(name, ".log.gz"):
			hour, gz = strings.TrimSuffix(name, ".log.gz"), true
		case strings.HasSuffix(name, ".log"):
			hour = strings.TrimSuffix(name, ".log")
		default:
			continue
		}
		if len(hour) != 10 {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, segFile{hour: hour, path: filepath.Join(dir, name), gz: gz, size: info.Size()})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].hour < out[j].hour || (out[i].hour == out[j].hour && out[i].gz && !out[j].gz)
	})
	return out, nil
}

func compress(path string) error {
	in, err := os.Open(path)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := path + ".gz.tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	zw := gzip.NewWriter(out)
	if _, err := io.Copy(zw, in); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := zw.Close(); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path+".gz"); err != nil {
		return err
	}
	return os.Remove(path)
}

// Query selects lines.
type Query struct {
	From, To time.Time // To exclusive; zero = unbounded
	Text     string    // substring (case-insensitive) or, with Regex, a regular expression
	Regex    bool
	Pod      string
	Limit    int // default 500, at most 5000
}

// Result is a page of lines, newest first. More is set when older lines match too: ask again with
// To = Next.
type Result struct {
	Lines []Line    `json:"lines"`
	More  bool      `json:"more"`
	Next  time.Time `json:"next,omitempty"`
}

// Search returns matching lines of an app, newest first.
func (s *Store) Search(app int64, q Query) (Result, error) {
	if q.Limit <= 0 {
		q.Limit = 500
	}
	q.Limit = min(q.Limit, 5000)
	var re *regexp.Regexp
	if q.Regex && q.Text != "" {
		var err error
		if re, err = regexp.Compile(q.Text); err != nil {
			return Result{}, fmt.Errorf("invalid regular expression: %w", err)
		}
	}
	needle := strings.ToLower(q.Text)
	s.Flush()
	segs, err := segments(s.appDir(app))
	if err != nil {
		return Result{}, err
	}
	res := Result{Lines: []Line{}}
	fromHour, toHour := "", "9999999999"
	if !q.From.IsZero() {
		fromHour = hourKey(q.From)
	}
	if !q.To.IsZero() {
		toHour = hourKey(q.To)
	}
	for i := len(segs) - 1; i >= 0; i-- {
		sg := segs[i]
		if sg.hour < fromHour || sg.hour > toHour {
			continue
		}
		lines, err := readSegment(sg)
		if err != nil {
			continue // a damaged segment does not hide the others
		}
		sort.SliceStable(lines, func(a, b int) bool { return lines[a].Time.After(lines[b].Time) })
		for _, l := range lines {
			if (!q.To.IsZero() && !l.Time.Before(q.To)) || (!q.From.IsZero() && l.Time.Before(q.From)) {
				continue
			}
			if q.Pod != "" && l.Pod != q.Pod {
				continue
			}
			switch {
			case re != nil:
				if !re.MatchString(l.Line) {
					continue
				}
			case needle != "":
				if !strings.Contains(strings.ToLower(l.Line), needle) {
					continue
				}
			}
			if len(res.Lines) == q.Limit {
				res.More, res.Next = true, res.Lines[len(res.Lines)-1].Time
				return res, nil
			}
			res.Lines = append(res.Lines, l)
		}
	}
	return res, nil
}

func readSegment(sg segFile) ([]Line, error) {
	f, err := os.Open(sg.path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var r io.Reader = f
	if sg.gz {
		zr, err := gzip.NewReader(f)
		if err != nil {
			return nil, err
		}
		defer zr.Close()
		r = zr
	}
	var out []Line
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		parts := strings.SplitN(sc.Text(), "\t", 3)
		if len(parts) != 3 {
			continue
		}
		ns, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			continue
		}
		out = append(out, Line{Time: time.Unix(0, ns).UTC(), Pod: parts[1], Line: parts[2]})
	}
	return out, sc.Err()
}
