package api

import (
	"errors"
	"io"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"

	"nectar_apps/internal/deploy"
	"nectar_apps/internal/store"
	"nectar_apps/internal/volfiles"
)

// Volume file browser: GET/PUT/DELETE /api/apps/{id}/volumes/{name}/files… (volume.manage). The files are
// reached through a helper pod that mounts the volume (internal/volfiles).

const maxUpload = 64 << 20

func (s *Server) volumeSession(w http.ResponseWriter, r *http.Request, app store.App, proj store.Project) (*volfiles.Session, bool) {
	if !s.needKube(w) {
		return nil, false
	}
	name := r.PathValue("name")
	found := false
	vols, err := s.deployer.Volumes(r.Context(), app, proj)
	if err != nil {
		writeError(w, http.StatusBadGateway, "kubernetes: "+err.Error())
		return nil, false
	}
	for _, v := range vols {
		if v.Name == name && v.Phase != "" {
			found = true
		}
	}
	if !found {
		writeError(w, http.StatusNotFound, "volume not found (it is created with the next deploy)")
		return nil, false
	}
	b := &volfiles.Browser{Kube: s.kube, Image: deploy.InitImage}
	sess, err := b.Open(r.Context(), proj.Namespace, deploy.VolumeClaimName(app.Slug, name))
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return nil, false
	}
	return sess, true
}

// filePath reads and checks ?path= (or a JSON field).
func filePath(w http.ResponseWriter, p string) (string, bool) {
	c := volfiles.Clean(p)
	if c == "" {
		writeError(w, http.StatusBadRequest, "invalid path")
		return "", false
	}
	return c, true
}

func (s *Server) fileError(w http.ResponseWriter, err error) {
	if errors.Is(err, volfiles.ErrNotFound) {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeError(w, http.StatusBadGateway, err.Error())
}

// rel turns /data/x back into /x for answers and audit entries.
func rel(p string) string {
	if p == volfiles.Root {
		return "/"
	}
	return strings.TrimPrefix(p, volfiles.Root)
}

func (s *Server) listFiles(w http.ResponseWriter, r *http.Request, _ *principal, app store.App, proj store.Project) {
	dir, ok := filePath(w, r.URL.Query().Get("path"))
	if !ok {
		return
	}
	sess, ok := s.volumeSession(w, r, app, proj)
	if !ok {
		return
	}
	list, err := sess.List(r.Context(), dir)
	if err != nil {
		s.fileError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"path": rel(dir), "entries": list})
}

func (s *Server) downloadFile(w http.ResponseWriter, r *http.Request, p *principal, app store.App, proj store.Project) {
	file, ok := filePath(w, r.URL.Query().Get("path"))
	if !ok {
		return
	}
	sess, ok := s.volumeSession(w, r, app, proj)
	if !ok {
		return
	}
	data, err := sess.Read(r.Context(), file, volfiles.MaxRead)
	if err != nil {
		s.fileError(w, err)
		return
	}
	s.audit(r, p, "volume.file_read", proj.Slug+"/"+app.Slug, map[string]string{"volume": r.PathValue("name"), "path": rel(file)})
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": path.Base(file)}))
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(data)
}

func (s *Server) uploadFile(w http.ResponseWriter, r *http.Request, p *principal, app store.App, proj store.Project) {
	file, ok := filePath(w, r.URL.Query().Get("path"))
	if !ok {
		return
	}
	if file == volfiles.Root {
		writeError(w, http.StatusBadRequest, "path must name a file")
		return
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxUpload))
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "files up to 64 MB can be uploaded here")
		return
	}
	sess, ok := s.volumeSession(w, r, app, proj)
	if !ok {
		return
	}
	if err := sess.Write(r.Context(), file, data); err != nil {
		s.fileError(w, err)
		return
	}
	s.audit(r, p, "volume.file_write", proj.Slug+"/"+app.Slug, map[string]any{"volume": r.PathValue("name"), "path": rel(file), "bytes": len(data)})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) fileAction(w http.ResponseWriter, r *http.Request, p *principal, app store.App, proj store.Project) {
	var in struct {
		Action string `json:"action"` // mkdir | rename | delete
		Path   string `json:"path"`
		To     string `json:"to"`
	}
	if !decode(w, r, &in) {
		return
	}
	target, ok := filePath(w, in.Path)
	if !ok {
		return
	}
	to := ""
	if in.Action == "rename" {
		if to, ok = filePath(w, in.To); !ok {
			return
		}
	}
	if (in.Action == "delete" || in.Action == "rename") && target == volfiles.Root {
		writeError(w, http.StatusBadRequest, "the volume itself cannot be changed here")
		return
	}
	sess, ok := s.volumeSession(w, r, app, proj)
	if !ok {
		return
	}
	var err error
	switch in.Action {
	case "mkdir":
		err = sess.Mkdir(r.Context(), target)
	case "rename":
		err = sess.Rename(r.Context(), target, to)
	case "delete":
		err = sess.Remove(r.Context(), target)
	default:
		writeError(w, http.StatusBadRequest, "action must be mkdir, rename or delete")
		return
	}
	if err != nil {
		s.fileError(w, err)
		return
	}
	s.audit(r, p, "volume.file_"+in.Action, proj.Slug+"/"+app.Slug, map[string]string{"volume": r.PathValue("name"), "path": rel(target), "to": rel(to)})
	w.WriteHeader(http.StatusNoContent)
}

// closeFiles stops the volume's file browser pod (POST …/files/close, when the dialog closes).
func (s *Server) closeFiles(w http.ResponseWriter, r *http.Request, _ *principal, app store.App, proj store.Project) {
	if !s.needKube(w) {
		return
	}
	b := &volfiles.Browser{Kube: s.kube, Image: deploy.InitImage}
	if err := b.Close(r.Context(), proj.Namespace, deploy.VolumeClaimName(app.Slug, r.PathValue("name"))); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// serverVolumes: GET /api/server/volumes — every volume with its usage (server.view).
func (s *Server) serverVolumes(w http.ResponseWriter, r *http.Request, _ *principal) {
	if !s.needKube(w) {
		return
	}
	vols, err := s.deployer.AllVolumes(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, "kubernetes: "+err.Error())
		return
	}
	if vols == nil {
		vols = []deploy.VolumeUse{}
	}
	writeJSON(w, http.StatusOK, vols)
}

// storageClasses: GET /api/server/storage-classes — what volumes can use.
func (s *Server) storageClasses(w http.ResponseWriter, r *http.Request, _ *principal) {
	if !s.needKube(w) {
		return
	}
	list, err := s.kube.List(r.Context(), "StorageClass", "", "")
	if err != nil {
		writeError(w, http.StatusBadGateway, "kubernetes: "+err.Error())
		return
	}
	out := []map[string]any{}
	for _, sc := range list {
		name := strOf(sc, "metadata", "name")
		prov := strOf(sc, "provisioner")
		ann := mapOf(sc, "metadata", "annotations")
		out = append(out, map[string]any{
			"name":        name,
			"provisioner": prov,
			"default":     ann["storageclass.kubernetes.io/is-default-class"] == "true",
			"expandable":  sc["allowVolumeExpansion"] == true,
			// ReadWriteMany: NFS and Longhorn (its share-manager) can mount a volume on several nodes
			"shared":     strings.Contains(prov, "nfs") || prov == "driver.longhorn.io",
			"replicated": prov == "driver.longhorn.io",
			"node_local": prov == "rancher.io/local-path",
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func strOf(o map[string]any, keys ...string) string {
	v := any(o)
	for _, k := range keys {
		m, ok := v.(map[string]any)
		if !ok {
			return ""
		}
		v = m[k]
	}
	s, _ := v.(string)
	return s
}

func mapOf(o map[string]any, keys ...string) map[string]any {
	v := any(o)
	for _, k := range keys {
		m, ok := v.(map[string]any)
		if !ok {
			return nil
		}
		v = m[k]
	}
	m, _ := v.(map[string]any)
	return m
}
