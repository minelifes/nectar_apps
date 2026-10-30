package api

import (
	"errors"
	"fmt"
	"net/http"
	"path"
	"regexp"
	"strings"

	"nectar_apps/internal/deploy"
	"nectar_apps/internal/store"
)

// ------------------------------------------------------- scale / restart --

func (s *Server) opErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, deploy.ErrNotDeployed):
		writeError(w, http.StatusConflict, err.Error())
	default:
		s.deployErr(w, err)
	}
}

func (s *Server) scaleApp(w http.ResponseWriter, r *http.Request, p *principal, app store.App, proj store.Project) {
	if !s.needKube(w) {
		return
	}
	var in struct {
		Replicas *int `json:"replicas"`
		Force    bool `json:"force"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Replicas == nil || *in.Replicas < 0 || *in.Replicas > 20 {
		writeError(w, http.StatusBadRequest, "replicas must be 0–20")
		return
	}
	s.doScale(w, r, p, app, proj, *in.Replicas, in.Force, "app.scale")
}

func (s *Server) doScale(w http.ResponseWriter, r *http.Request, p *principal, app store.App, proj store.Project, replicas int, force bool, action string) {
	if replicas > app.Config.Replicas && !force {
		check := app
		check.Config.Replicas = replicas
		if err := s.deployer.CheckCapacity(r.Context(), check); err != nil {
			s.deployErr(w, err)
			return
		}
	}
	if err := s.deployer.Scale(r.Context(), app, proj, replicas); err != nil {
		s.opErr(w, err)
		return
	}
	s.audit(r, p, action, proj.Slug+"/"+app.Slug, map[string]int{"from": app.Config.Replicas, "to": replicas})
	app, _ = s.store.GetApp(r.Context(), app.ID)
	writeJSON(w, http.StatusOK, app)
}

func (s *Server) stopApp(w http.ResponseWriter, r *http.Request, p *principal, app store.App, proj store.Project) {
	if !s.needKube(w) {
		return
	}
	if app.Config.Replicas == 0 {
		writeJSON(w, http.StatusOK, app)
		return
	}
	s.doScale(w, r, p, app, proj, 0, true, "app.stop")
}

func (s *Server) startApp(w http.ResponseWriter, r *http.Request, p *principal, app store.App, proj store.Project) {
	if !s.needKube(w) {
		return
	}
	var in struct {
		Force bool `json:"force"`
	}
	if r.ContentLength != 0 && !decode(w, r, &in) {
		return
	}
	n := app.Config.Replicas
	if n == 0 {
		n = max(app.Config.ResumeReplicas, 1)
	}
	s.doScale(w, r, p, app, proj, n, in.Force, "app.start")
}

func (s *Server) restartApp(w http.ResponseWriter, r *http.Request, p *principal, app store.App, proj store.Project) {
	if !s.needKube(w) {
		return
	}
	if err := s.deployer.Restart(r.Context(), app, proj); err != nil {
		s.opErr(w, err)
		return
	}
	s.audit(r, p, "app.restart", proj.Slug+"/"+app.Slug, nil)
	w.WriteHeader(http.StatusNoContent)
}

// ----------------------------------------------------------------- volumes --

var volumeNameRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,28}[a-z0-9])?$`)

// reservedMounts cannot hold volumes or config files.
var reservedMounts = []string{"/", "/proc", "/sys", "/dev", "/etc", "/bin", "/sbin", "/lib", "/usr", "/var/run", "/run"}

func validMountPath(p string) string {
	if !strings.HasPrefix(p, "/") || path.Clean(p) != p || strings.ContainsAny(p, "\n\r\x00:") || len(p) > 256 {
		return "must be a clean absolute path, e.g. /data"
	}
	for _, r := range reservedMounts {
		if p == r {
			return "cannot be " + r
		}
	}
	return ""
}

var storageClassRe = regexp.MustCompile(`^[a-z0-9]([-a-z0-9.]{0,61}[a-z0-9])?$`)

func validateVolumes(vs []store.Volume, old []store.Volume) string {
	if len(vs) > 10 {
		return "at most 10 volumes"
	}
	names, mounts := map[string]bool{}, map[string]bool{}
	prev := map[string]int64{}
	prevVol := map[string]store.Volume{}
	for _, v := range old {
		prev[v.Name] = v.SizeMB
		prevVol[v.Name] = v
	}
	for _, v := range vs {
		if v.StorageClass != "" && !storageClassRe.MatchString(v.StorageClass) {
			return fmt.Sprintf("volume %s: invalid storage class %q", v.Name, v.StorageClass)
		}
		if was, ok := prevVol[v.Name]; ok && (was.StorageClass != v.StorageClass || was.Shared != v.Shared) {
			return "volume " + v.Name + ": the storage class and sharing of an existing volume cannot change — add a new volume and copy the data"
		}
		if !volumeNameRe.MatchString(v.Name) {
			return fmt.Sprintf("volume name %q: use 1–30 lowercase letters, digits and dashes", v.Name)
		}
		if names[v.Name] {
			return "duplicate volume " + v.Name
		}
		names[v.Name] = true
		if msg := validMountPath(v.MountPath); msg != "" {
			return "volume " + v.Name + ": mount path " + msg
		}
		if mounts[v.MountPath] {
			return "two volumes use " + v.MountPath
		}
		mounts[v.MountPath] = true
		if v.SizeMB < 64 || v.SizeMB > 1<<20 {
			return "volume " + v.Name + ": size must be 64 MiB – 1 TiB"
		}
		if was, ok := prev[v.Name]; ok && v.SizeMB < was {
			return "volume " + v.Name + ": volumes can only grow"
		}
	}
	return ""
}

func (s *Server) listVolumes(w http.ResponseWriter, r *http.Request, _ *principal, app store.App, proj store.Project) {
	if !s.needKube(w) {
		return
	}
	vols, err := s.deployer.Volumes(r.Context(), app, proj)
	if err != nil {
		writeError(w, http.StatusBadGateway, "kubernetes: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, vols)
}

func (s *Server) deleteVolume(w http.ResponseWriter, r *http.Request, p *principal, app store.App, proj store.Project) {
	if !s.needKube(w) {
		return
	}
	name := r.PathValue("name")
	if !volumeNameRe.MatchString(name) {
		writeError(w, http.StatusNotFound, "volume not found")
		return
	}
	if err := s.deployer.DeleteVolume(r.Context(), app, proj, name); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	s.audit(r, p, "volume.delete", proj.Slug+"/"+app.Slug, map[string]string{"volume": name})
	w.WriteHeader(http.StatusNoContent)
}

// resizeVolumes grows the PVCs of volumes whose size went up (best effort; the size is recorded anyway).
func (s *Server) resizeVolumes(r *http.Request, app store.App, proj store.Project, old []store.Volume) []string {
	if s.deployer == nil {
		return nil
	}
	prev := map[string]int64{}
	for _, v := range old {
		prev[v.Name] = v.SizeMB
	}
	var notes []string
	for _, v := range app.Config.Volumes {
		if was, ok := prev[v.Name]; !ok || v.SizeMB <= was {
			continue
		}
		applied, err := s.deployer.ResizeVolume(r.Context(), app, proj, v.Name, v.SizeMB)
		switch {
		case err != nil:
			notes = append(notes, v.Name+": "+err.Error())
		case !applied:
			notes = append(notes, v.Name+": the new size is recorded; this storage class cannot grow existing volumes (k3s local-path does not limit sizes)")
		}
	}
	return notes
}

// ------------------------------------------------------------ config files --

type fileItem struct {
	Path    string  `json:"path"`
	Content *string `json:"content,omitempty"`
	Secret  bool    `json:"secret"`
	Size    int     `json:"size"`
}

func (s *Server) getFiles(w http.ResponseWriter, r *http.Request, p *principal, app store.App, proj store.Project) {
	files, err := s.store.ListFiles(r.Context(), app.ID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	reveal := r.URL.Query().Get("reveal") == "1"
	if reveal && !p.eff.Can("env.secrets.view", &app.ProjectID) {
		writeError(w, http.StatusForbidden, "missing permission: env.secrets.view")
		return
	}
	out := make([]fileItem, 0, len(files))
	for _, f := range files {
		pt, err := s.envBox.Open(f.ContentEnc)
		if err != nil {
			s.serverError(w, r, err)
			return
		}
		it := fileItem{Path: f.Path, Secret: f.IsSecret, Size: len(pt)}
		if !f.IsSecret || reveal {
			it.Content = &pt
		}
		out = append(out, it)
	}
	if reveal {
		s.audit(r, p, "files.reveal", proj.Slug+"/"+app.Slug, nil)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) putFiles(w http.ResponseWriter, r *http.Request, p *principal, app store.App, proj store.Project) {
	var in []fileItem
	if !decode(w, r, &in) {
		return
	}
	if len(in) > 32 {
		writeError(w, http.StatusBadRequest, "at most 32 files")
		return
	}
	current, err := s.store.ListFiles(r.Context(), app.ID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	existing := map[string]store.AppFile{}
	for _, f := range current {
		existing[f.Path] = f
	}
	rt, _ := s.catalog.Get(app.RuntimeID)
	taken := map[string]bool{}
	for _, f := range rt.ConfigFiles {
		taken[f.Path] = true
	}
	for _, v := range app.Config.Volumes {
		taken[v.MountPath] = true
	}
	seen := map[string]bool{}
	total := 0
	var files []store.AppFile
	for _, it := range in {
		it.Path = strings.TrimSpace(it.Path)
		if msg := validMountPath(it.Path); msg != "" {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("file %q: path %s", it.Path, msg))
			return
		}
		if seen[it.Path] {
			writeError(w, http.StatusBadRequest, "duplicate file "+it.Path)
			return
		}
		seen[it.Path] = true
		if taken[it.Path] {
			writeError(w, http.StatusBadRequest, it.Path+" is used by the runtime or a volume")
			return
		}
		var enc string
		switch {
		case it.Content != nil:
			if len(*it.Content) > 256<<10 {
				writeError(w, http.StatusBadRequest, it.Path+": too large (max 256 KiB)")
				return
			}
			total += len(*it.Content)
			if enc, err = s.envBox.Seal(*it.Content); err != nil {
				s.serverError(w, r, err)
				return
			}
		case existing[it.Path].ContentEnc != "":
			enc = existing[it.Path].ContentEnc // content omitted → keep (secret files are not sent back)
			if pt, err := s.envBox.Open(enc); err == nil {
				total += len(pt)
			}
		default:
			writeError(w, http.StatusBadRequest, it.Path+": content is required")
			return
		}
		files = append(files, store.AppFile{Path: it.Path, ContentEnc: enc, IsSecret: it.Secret})
	}
	if total > 900<<10 {
		writeError(w, http.StatusBadRequest, "files are too large together (max 900 KiB)")
		return
	}
	if err := s.store.ReplaceFiles(r.Context(), app.ID, files); err != nil {
		s.serverError(w, r, err)
		return
	}
	paths := make([]string, 0, len(files))
	for _, f := range files {
		paths = append(paths, f.Path)
	}
	s.audit(r, p, "files.update", proj.Slug+"/"+app.Slug, map[string]any{"paths": paths})
	w.WriteHeader(http.StatusNoContent)
}
