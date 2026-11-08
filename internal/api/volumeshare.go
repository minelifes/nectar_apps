package api

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"nectar_apps/internal/deploy"
	"nectar_apps/internal/store"
)

// Shared volumes: an app mounts a volume of another app of the same project (Volume.From = "app/volume").
// Both see the same files. The owner keeps the disk: it alone creates, resizes, backs up and deletes it;
// it can't drop or delete the volume (or be deleted) while another app mounts it. A ReadWriteOnce disk
// is on one node, so the apps sharing it prefer that node (see deploy.sharedAffinity); with a
// ReadWriteMany volume ("Shared" + e.g. nfs) they can run anywhere.

// resolveSharedVolumes fills Claim, size, class and Shared of the volumes taken from another app, and
// checks them. selfSlug is the app being saved. Returns an error message or "".
func (s *Server) resolveSharedVolumes(ctx context.Context, projectID int64, selfSlug string, vols []store.Volume) string {
	var apps []store.App
	for i := range vols {
		v := &vols[i]
		if v.From == "" {
			if v.ReadOnly {
				return "volume " + v.Name + ": read-only is for volumes shared from another app"
			}
			continue
		}
		owner, name, ok := deploy.SharedFrom(*v)
		if !ok {
			return "volume " + v.Name + `: "from" must be "app/volume"`
		}
		if owner == selfSlug {
			return "volume " + v.Name + ": an app can't mount its own volume twice"
		}
		if apps == nil {
			var err error
			if apps, err = s.store.ListApps(ctx, projectID); err != nil {
				return "volume " + v.Name + ": " + err.Error()
			}
		}
		i := slices.IndexFunc(apps, func(a store.App) bool { return a.Slug == owner })
		if i < 0 {
			return "volume " + v.Name + ": there is no app " + owner + " in this project"
		}
		j := slices.IndexFunc(apps[i].Config.Volumes, func(ov store.Volume) bool { return ov.Name == name })
		if j < 0 {
			return fmt.Sprintf("volume %s: app %s has no volume %s", v.Name, owner, name)
		}
		ov := apps[i].Config.Volumes[j]
		if ov.From != "" {
			return fmt.Sprintf("volume %s: %s/%s is itself shared from %s — use that one", v.Name, owner, name, ov.From)
		}
		v.Claim = deploy.ClaimOf(owner, ov)
		v.SizeMB, v.StorageClass, v.Shared = ov.SizeMB, ov.StorageClass, ov.Shared
	}
	return ""
}

// volumeUsers lists the other apps of the project that mount volume `name` of app `owner` (in their
// settings or in their running release).
func (s *Server) volumeUsers(ctx context.Context, projectID int64, owner, name string) []string {
	apps, err := s.store.ListApps(ctx, projectID)
	if err != nil {
		return nil
	}
	ref := owner + "/" + name
	uses := func(vs []store.Volume) bool {
		return slices.ContainsFunc(vs, func(v store.Volume) bool { return v.From == ref })
	}
	var out []string
	for _, a := range apps {
		if a.Slug == owner {
			continue
		}
		used := uses(a.Config.Volumes)
		if !used {
			if live, err := s.store.LiveRelease(ctx, a.ID); err == nil {
				used = uses(live.Config.Volumes)
			}
		}
		if used {
			out = append(out, a.Slug)
		}
	}
	return out
}

// sharedVolumeProblem: a volume that other apps mount can't be removed from the owner's settings.
func (s *Server) sharedVolumeProblem(ctx context.Context, app store.App, old, cur []store.Volume) string {
	for _, v := range old {
		if v.From != "" || slices.ContainsFunc(cur, func(n store.Volume) bool { return n.Name == v.Name }) {
			continue
		}
		if users := s.volumeUsers(ctx, app.ProjectID, app.Slug, v.Name); len(users) > 0 {
			return fmt.Sprintf("volume %s is mounted by %s: remove it there first", v.Name, strings.Join(users, ", "))
		}
	}
	return ""
}

// syncSharedSizes records the owner's new volume sizes on the apps that mount them (display only).
func (s *Server) syncSharedSizes(ctx context.Context, owner store.App) {
	apps, err := s.store.ListApps(ctx, owner.ProjectID)
	if err != nil {
		return
	}
	size := map[string]int64{}
	for _, v := range owner.Config.Volumes {
		size[owner.Slug+"/"+v.Name] = v.SizeMB
	}
	for _, a := range apps {
		changed := false
		for i, v := range a.Config.Volumes {
			if mb, ok := size[v.From]; ok && v.SizeMB != mb {
				a.Config.Volumes[i].SizeMB = mb
				changed = true
			}
		}
		if changed {
			_, _ = s.store.SaveApp(ctx, a)
		}
	}
}

// ownVolumes turns volumes shared from other apps into empty volumes of the app itself (same name, path
// and size): for copies that must not touch the original's files (previews, other projects).
func ownVolumes(vs []store.Volume) ([]store.Volume, []string) {
	var notes []string
	out := slices.Clone(vs)
	for i, v := range out {
		if v.From == "" {
			continue
		}
		notes = append(notes, fmt.Sprintf("volume %s (shared from %s) became an empty volume of its own", v.Name, v.From))
		out[i].From, out[i].Claim, out[i].ReadOnly = "", "", false
	}
	return out, notes
}
