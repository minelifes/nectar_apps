package api

import (
	"archive/zip"
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"nectar_apps/internal/store"
	"nectar_apps/internal/volfiles"
)

// Large .zip files into a volume (volume.manage):
//
//	POST   …/volumes/{name}/zip-uploads              {"size": N, "filename": "site.zip"} → {"id", "chunk_size"}
//	POST   …/volumes/{name}/zip-uploads/{uid}?offset= one chunk (≤ 32 MB) at that offset → {"offset"}
//	POST   …/volumes/{name}/zip-uploads/{uid}/unzip   {"path": "/site"} → starts unpacking (202)
//	GET    …/volumes/{name}/zip-uploads/{uid}         progress: state uploading | checking | unpacking | done | error
//	DELETE …/volumes/{name}/zip-uploads/{uid}         cancel (removes the upload)
//
// The zip is written to the panel's data folder chunk by chunk (small requests: no proxy body limit or
// timeout gets in the way, and a failed chunk is simply sent again). Unpacking runs in the background:
// the archive is checked (volfiles.PlanZip), the volume's free space compared with its unpacked size,
// then a tar stream goes into the helper pod (memory stays small whatever the size). The zip is deleted
// afterwards. Uploads live in memory: a panel restart drops them (the files are cleaned up at start).

const (
	zipChunkSize    = 8 << 20  // what the UI sends
	zipMaxChunk     = 32 << 20 // what the server accepts
	zipIdleTTL      = 6 * time.Hour
	zipFinishedTTL  = time.Hour
	zipMaxActive    = 4                // uploads in progress at the same time (all users)
	zipDiskMargin   = 512 << 20        // the panel's disk keeps this much free
	zipJobTimeout   = 6 * time.Hour    // unpacking gives up after this
	zipPodMinLeft   = 50 * time.Minute // a helper pod with less lifetime left is replaced first
	zipVolumeMargin = 64 << 20         // the volume keeps this much free (plus 2 %)
)

type zipUploads struct {
	mu      sync.Mutex
	byID    map[string]*zipUpload
	cleaned sync.Once
}

// zipUpload is one upload; the exported-style fields are its JSON status.
type zipUpload struct {
	mu       sync.Mutex
	ID       string    `json:"id"`
	Filename string    `json:"filename"`
	Size     int64     `json:"size"`
	Offset   int64     `json:"offset"`
	State    string    `json:"state"` // uploading | checking | unpacking | done | error
	Target   string    `json:"path,omitempty"`
	Sent     int64     `json:"sent,omitempty"`  // tar bytes streamed into the pod
	Total    int64     `json:"total,omitempty"` // tar size
	Files    int       `json:"files,omitempty"`
	Dirs     int       `json:"dirs,omitempty"`
	Skipped  int       `json:"skipped,omitempty"`
	Unpacked int64     `json:"unpacked_bytes,omitempty"`
	Message  string    `json:"message,omitempty"`
	Updated  time.Time `json:"updated"`

	appID  int64
	volume string
	file   string
	cancel context.CancelFunc
}

func (u *zipUpload) status() map[string]any {
	u.mu.Lock()
	defer u.mu.Unlock()
	pct := 0.0
	switch {
	case u.State == "uploading" && u.Size > 0:
		pct = float64(u.Offset) / float64(u.Size) * 100
	case u.State == "unpacking" && u.Total > 0:
		pct = float64(u.Sent) / float64(u.Total) * 100
	case u.State == "done":
		pct = 100
	}
	return map[string]any{"id": u.ID, "filename": u.Filename, "size": u.Size, "offset": u.Offset, "state": u.State,
		"path": u.Target, "sent": u.Sent, "total": u.Total, "files": u.Files, "dirs": u.Dirs, "skipped": u.Skipped,
		"unpacked_bytes": u.Unpacked, "message": u.Message, "percent": pct, "chunk_size": zipChunkSize}
}

func (s *Server) zipDir() string { return filepath.Join(s.cfg.DataDir, "zip-uploads") }

// sweepZips drops expired uploads (and, once, files left over from before a restart).
func (s *Server) sweepZips() {
	z := &s.zips
	z.cleaned.Do(func() {
		_ = os.RemoveAll(s.zipDir())
	})
	z.mu.Lock()
	defer z.mu.Unlock()
	if z.byID == nil {
		z.byID = map[string]*zipUpload{}
	}
	for id, u := range z.byID {
		u.mu.Lock()
		idle := time.Since(u.Updated)
		expired := (u.State == "uploading" && idle > zipIdleTTL) || ((u.State == "done" || u.State == "error") && idle > zipFinishedTTL)
		u.mu.Unlock()
		if expired {
			_ = os.Remove(u.file)
			delete(z.byID, id)
		}
	}
}

func (s *Server) zipUploadCreate(w http.ResponseWriter, r *http.Request, p *principal, app store.App, _ store.Project) {
	var in struct {
		Size     int64  `json:"size"`
		Filename string `json:"filename"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Size <= 0 {
		writeError(w, http.StatusBadRequest, "size is required")
		return
	}
	if in.Size > s.cfg.MaxZipBytes {
		writeError(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("archives up to %s can be unpacked here (NECTAR_MAX_ZIP_MB)", humanBytes(s.cfg.MaxZipBytes)))
		return
	}
	s.sweepZips()
	if err := os.MkdirAll(s.zipDir(), 0o700); err != nil {
		s.serverError(w, r, err)
		return
	}
	if free := freeDiskBytes(s.zipDir()); free >= 0 && free < in.Size+zipDiskMargin {
		writeError(w, http.StatusInsufficientStorage, fmt.Sprintf("the panel has %s of free disk; this archive needs %s while it is unpacked", humanBytes(free), humanBytes(in.Size+zipDiskMargin)))
		return
	}
	z := &s.zips
	z.mu.Lock()
	active := 0
	for _, u := range z.byID {
		u.mu.Lock()
		if u.State != "done" && u.State != "error" {
			active++
		}
		u.mu.Unlock()
	}
	if active >= zipMaxActive {
		z.mu.Unlock()
		writeError(w, http.StatusTooManyRequests, "too many archives are being uploaded right now: try again when one has finished")
		return
	}
	b := make([]byte, 18)
	_, _ = rand.Read(b)
	id := base64.RawURLEncoding.EncodeToString(b)
	u := &zipUpload{ID: id, Filename: cleanFilename(in.Filename), Size: in.Size, State: "uploading", Updated: time.Now(),
		appID: app.ID, volume: r.PathValue("name"), file: filepath.Join(s.zipDir(), id+".zip")}
	f, err := os.OpenFile(u.file, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		z.mu.Unlock()
		s.serverError(w, r, err)
		return
	}
	f.Close()
	z.byID[id] = u
	z.mu.Unlock()
	writeJSON(w, http.StatusCreated, u.status())
}

// zipUpload loads {uid} and checks it belongs to this app volume.
func (s *Server) zipUpload(w http.ResponseWriter, r *http.Request, app store.App) (*zipUpload, bool) {
	s.zips.mu.Lock()
	u := s.zips.byID[r.PathValue("uid")]
	s.zips.mu.Unlock()
	if u == nil || u.appID != app.ID || u.volume != r.PathValue("name") {
		writeError(w, http.StatusNotFound, "upload not found or expired")
		return nil, false
	}
	return u, true
}

func (s *Server) zipUploadStatus(w http.ResponseWriter, r *http.Request, _ *principal, app store.App, _ store.Project) {
	if u, ok := s.zipUpload(w, r, app); ok {
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, u.status())
	}
}

func (s *Server) zipUploadChunk(w http.ResponseWriter, r *http.Request, _ *principal, app store.App, _ store.Project) {
	u, ok := s.zipUpload(w, r, app)
	if !ok {
		return
	}
	offset, err := strconv.ParseInt(r.URL.Query().Get("offset"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "offset is required")
		return
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.State != "uploading" {
		writeError(w, http.StatusConflict, "the upload is complete")
		return
	}
	if offset != u.Offset {
		writeJSON(w, http.StatusConflict, map[string]any{"error": "offset does not match the server offset", "offset": u.Offset})
		return
	}
	f, err := os.OpenFile(u.file, os.O_WRONLY, 0)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	defer f.Close()
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		s.serverError(w, r, err)
		return
	}
	limit := min(int64(zipMaxChunk), u.Size-offset)
	n, err := io.Copy(f, io.LimitReader(r.Body, limit))
	if err == nil {
		// more than the declared size (or the chunk limit) is an error, not silently cut
		var one [1]byte
		if m, _ := r.Body.Read(one[:]); m > 0 {
			err = errors.New("chunk is too large (at most 32 MB, and not past the declared size)")
		}
	}
	if err != nil {
		_ = f.Truncate(offset) // the client sends the chunk again
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "chunk failed: " + err.Error(), "offset": offset})
		return
	}
	u.Offset = offset + n
	u.Updated = time.Now()
	writeJSON(w, http.StatusOK, map[string]any{"offset": u.Offset, "size": u.Size})
}

func (s *Server) zipUploadUnzip(w http.ResponseWriter, r *http.Request, p *principal, app store.App, proj store.Project) {
	u, ok := s.zipUpload(w, r, app)
	if !ok {
		return
	}
	var in struct {
		Path string `json:"path"`
	}
	if !decode(w, r, &in) {
		return
	}
	dir, ok := filePath(w, in.Path)
	if !ok {
		return
	}
	if !s.needKube(w) {
		return
	}
	u.mu.Lock()
	if u.State != "uploading" || u.Offset != u.Size {
		u.mu.Unlock()
		writeError(w, http.StatusConflict, "upload the whole archive first")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), zipJobTimeout)
	u.State, u.Target, u.cancel, u.Updated = "checking", rel(dir), cancel, time.Now()
	u.mu.Unlock()

	uid, email, ip := p.user.ID, p.user.Email, clientIP(r)
	target := proj.Slug + "/" + app.Slug
	go func() {
		defer cancel()
		err := s.runUnzip(ctx, u, app, proj, dir)
		u.mu.Lock()
		if err != nil {
			u.State, u.Message = "error", err.Error()
			if errors.Is(ctx.Err(), context.Canceled) {
				u.Message = "cancelled"
			}
		} else {
			u.State, u.Sent = "done", u.Total
		}
		u.Updated = time.Now()
		payload := map[string]any{"volume": u.volume, "path": u.Target, "file": u.Filename, "bytes": u.Size,
			"files": u.Files, "dirs": u.Dirs, "skipped": u.Skipped, "state": u.State, "message": u.Message}
		u.mu.Unlock()
		_ = os.Remove(u.file)
		if err := s.store.Audit(context.Background(), &uid, email, "volume.file_unzip", target, ip, payload); err != nil {
			s.log.Error("audit write failed", "action", "volume.file_unzip", "err", err)
		}
	}()
	writeJSON(w, http.StatusAccepted, u.status())
}

// runUnzip checks the archive, the volume's free space, then streams it into the folder.
func (s *Server) runUnzip(ctx context.Context, u *zipUpload, app store.App, proj store.Project, dir string) error {
	zr, err := zip.OpenReader(u.file)
	if err != nil {
		return errors.New("not a valid .zip file")
	}
	defer zr.Close()
	plan, err := volfiles.PlanZip(&zr.Reader)
	if err != nil {
		return err
	}
	u.mu.Lock()
	u.Files, u.Dirs, u.Skipped, u.Unpacked, u.Total = plan.Files, plan.Dirs, plan.Skipped, plan.Bytes, plan.TarSize
	u.mu.Unlock()

	sess, _, err := s.openVolume(ctx, app, proj, u.volume, zipPodMinLeft)
	if err != nil {
		return err
	}
	free, err := sess.FreeBytes(ctx)
	if err != nil {
		return err
	}
	if need := plan.Bytes + plan.Bytes/50 + zipVolumeMargin; free < need {
		return fmt.Errorf("the volume has %s free; unpacking needs about %s (make the volume bigger or free some space)", humanBytes(free), humanBytes(need))
	}
	u.mu.Lock()
	u.State, u.Updated = "unpacking", time.Now()
	u.mu.Unlock()
	return sess.Unzip(ctx, dir, plan, func(sent int64) {
		u.mu.Lock()
		u.Sent, u.Updated = sent, time.Now()
		u.mu.Unlock()
	})
}

func (s *Server) zipUploadDelete(w http.ResponseWriter, r *http.Request, _ *principal, app store.App, _ store.Project) {
	u, ok := s.zipUpload(w, r, app)
	if !ok {
		return
	}
	u.mu.Lock()
	if u.cancel != nil {
		u.cancel() // the job removes the file when it stops
	}
	u.mu.Unlock()
	s.zips.mu.Lock()
	delete(s.zips.byID, u.ID)
	s.zips.mu.Unlock()
	_ = os.Remove(u.file)
	w.WriteHeader(http.StatusNoContent)
}

// humanBytes formats a size for messages (1.5 GB).
func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return strconv.FormatInt(n, 10) + " B"
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return strings.TrimSuffix(strconv.FormatFloat(float64(n)/float64(div), 'f', 1, 64), ".0") + " " + string("KMGTPE"[exp]) + "B"
}
