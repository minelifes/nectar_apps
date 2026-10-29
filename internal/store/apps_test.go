package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestProjectsAppsReleases(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)

	p, err := s.CreateProject(ctx, "shop", "Shop", nil)
	if err != nil || p.Namespace != "prj-shop" || p.Quota != nil {
		t.Fatalf("CreateProject = %+v, %v", p, err)
	}
	if _, err := s.CreateProject(ctx, "shop", "Again", nil); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate slug: %v", err)
	}
	p, _ = s.UpdateProject(ctx, p.ID, nil, true, &Quota{CPUMillicores: 2000})
	if p.Quota == nil || p.Quota.CPUMillicores != 2000 {
		t.Fatalf("quota: %+v", p.Quota)
	}
	p, _ = s.UpdateProject(ctx, p.ID, nil, true, &Quota{}) // empty quota = unlimited
	if p.Quota != nil {
		t.Fatalf("empty quota should mean unlimited: %+v", p.Quota)
	}

	a, err := s.CreateApp(ctx, App{ProjectID: p.ID, Slug: "api", Name: "API", Kind: "web", RuntimeID: "java", RuntimeVersion: "21",
		Config: AppConfig{Port: 8080, Replicas: 1, Resources: Resources{CPUMillicores: 500, MemoryMB: 512}}})
	if err != nil || a.Status != "new" || a.Config.Port != 8080 {
		t.Fatalf("CreateApp = %+v, %v", a, err)
	}
	if _, err := s.CreateApp(ctx, App{ProjectID: p.ID, Slug: "api", Name: "x", Kind: "web", RuntimeID: "java"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate app slug: %v", err)
	}
	a.Config.Replicas = 3
	if a, _ = s.SaveApp(ctx, a); a.Config.Replicas != 3 {
		t.Fatal("SaveApp did not persist config")
	}
	if p, _ := s.GetProject(ctx, p.ID); p.Apps != 1 {
		t.Fatalf("project app count = %d", p.Apps)
	}

	// env
	if err := s.ReplaceEnv(ctx, a.ID, []EnvVar{{Key: "A", ValueEnc: "x"}, {Key: "B", ValueEnc: "y", IsSecret: true}}); err != nil {
		t.Fatal(err)
	}
	if err := s.ReplaceEnv(ctx, a.ID, []EnvVar{{Key: "A"}, {Key: "A"}}); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate env key: %v", err)
	}
	if env, _ := s.ListEnv(ctx, a.ID); len(env) != 2 || !env[1].IsSecret {
		t.Fatalf("env after failed replace = %+v", env)
	}

	// artifacts + releases
	uid := int64(1)
	_ = uid
	art, err := s.CreateArtifact(ctx, a.ID, "abc", "app.jar", 10, map[string]string{"type": "jar"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	r1, _ := s.CreateRelease(ctx, a.ID, &art.ID, "eclipse-temurin:21-jre", a.Config, nil)
	r2, _ := s.CreateRelease(ctx, a.ID, &art.ID, "eclipse-temurin:21-jre", a.Config, nil)
	if r1.Number != 1 || r2.Number != 2 || r1.Status != ReleasePending {
		t.Fatalf("release numbers %d %d", r1.Number, r2.Number)
	}
	if active, _ := s.HasActiveRelease(ctx, a.ID); !active {
		t.Fatal("expected an active release")
	}
	_ = s.SetReleaseStatus(ctx, r1.ID, ReleaseLive, "")
	_ = s.SetReleaseStatus(ctx, r2.ID, ReleaseLive, "")
	if r1, _ = s.GetRelease(ctx, r1.ID); r1.Status != ReleaseSuperseded || r1.FinishedAt == nil {
		t.Fatalf("r1 should be superseded: %+v", r1)
	}
	if live, _ := s.LiveRelease(ctx, a.ID); live.ID != r2.ID {
		t.Fatalf("live release = %d", live.ID)
	}
	if got, _ := s.GetReleaseByNumber(ctx, a.ID, 1); got.ID != r1.ID {
		t.Fatal("GetReleaseByNumber")
	}

	// deleting the project cascades
	if err := s.DeleteProject(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetApp(ctx, a.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("app should be gone: %v", err)
	}
	if ok, _ := s.ArtifactSHAExists(ctx, "abc"); ok {
		t.Fatal("artifact should be gone")
	}
}

func TestPruneArtifactsAndUploads(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	p, _ := s.CreateProject(ctx, "p", "P", nil)
	a, _ := s.CreateApp(ctx, App{ProjectID: p.ID, Slug: "a", Name: "A", Kind: "web", RuntimeID: "java"})
	var ids []int64
	for _, sha := range []string{"s0", "s1", "s2", "s0", "s3"} {
		art, _ := s.CreateArtifact(ctx, a.ID, sha, "f", 1, nil, nil)
		ids = append(ids, art.ID)
	}
	rel, _ := s.CreateRelease(ctx, a.ID, &ids[1], "img", AppConfig{}, nil) // s1 is in use
	_ = s.SetReleaseStatus(ctx, rel.ID, ReleaseLive, "")

	orphans, err := s.PruneArtifacts(ctx, 2) // keep the 2 newest (s0#4, s3) + in-use s1
	if err != nil {
		t.Fatal(err)
	}
	// deleted: s0#1 (s0 still referenced by #4) and s2 → only s2 is an orphan file
	if len(orphans) != 1 || orphans[0] != "s2" {
		t.Fatalf("orphans = %v", orphans)
	}
	left, _ := s.ListArtifacts(ctx, a.ID, 0)
	if len(left) != 3 {
		t.Fatalf("left = %d", len(left))
	}

	now := time.Unix(1_800_000_000, 0)
	s.now = func() time.Time { return now }
	if err := s.CreateUpload(ctx, Upload{ID: "u1", AppID: a.ID, Filename: "x", Size: 10}, time.Hour); err != nil {
		t.Fatal(err)
	}
	if err := s.SetUploadOffset(ctx, "u1", 0, 5); err != nil {
		t.Fatal(err)
	}
	if err := s.SetUploadOffset(ctx, "u1", 0, 7); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale offset accepted: %v", err)
	}
	if u, _ := s.GetUpload(ctx, "u1"); u.Offset != 5 {
		t.Fatalf("offset = %d", u.Offset)
	}
	now = now.Add(2 * time.Hour)
	if _, err := s.GetUpload(ctx, "u1"); !errors.Is(err, ErrNotFound) {
		t.Fatal("expired upload still visible")
	}
	if ids, _ := s.ExpiredUploads(ctx); len(ids) != 1 {
		t.Fatalf("expired = %v", ids)
	}
}

// Ids of deleted apps and add-ons are never handed out again (they tag backups and metric series),
// also when a project delete removes them by cascade.
func TestIDsAreNotReused(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	p, _ := s.CreateProject(ctx, "shop", "Shop", nil)
	mk := func(proj int64, slug string) int64 {
		a, err := s.CreateApp(ctx, App{ProjectID: proj, Slug: slug, Name: slug, Kind: "web", RuntimeID: "image"})
		if err != nil {
			t.Fatal(err)
		}
		return a.ID
	}
	a1, a2 := mk(p.ID, "a"), mk(p.ID, "b")
	if err := s.DeleteApp(ctx, a2); err != nil {
		t.Fatal(err)
	}
	if a3 := mk(p.ID, "c"); a3 <= a2 {
		t.Fatalf("app id %d reused (deleted %d)", a3, a2)
	}
	db, err := s.CreateAddon(ctx, Addon{ProjectID: p.ID, Slug: "db", Name: "DB", Kind: "postgres", Version: "16"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteAddon(ctx, db.ID); err != nil {
		t.Fatal(err)
	}
	if db2, _ := s.CreateAddon(ctx, Addon{ProjectID: p.ID, Slug: "db", Name: "DB", Kind: "postgres", Version: "16"}); db2.ID <= db.ID {
		t.Fatalf("add-on id %d reused", db2.ID)
	}
	// a whole project goes: its apps' ids stay used, their backup plans go
	q, _ := s.CreateProject(ctx, "tmp", "Tmp", nil)
	last := mk(q.ID, "x")
	tg, err := s.SaveBackupTarget(ctx, BackupTarget{Name: "disk", Kind: "local", Config: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveBackupPlan(ctx, BackupPlan{TargetID: tg.ID, Scope: "app", SubjectID: last, Schedule: "0 3 * * *", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteProject(ctx, q.ID); err != nil {
		t.Fatal(err)
	}
	if plans, _ := s.ListBackupPlans(ctx); len(plans) != 0 {
		t.Fatalf("plans of the deleted project's app are left: %+v", plans)
	}
	if next := mk(p.ID, "d"); next <= last || a1 != 1 {
		t.Fatalf("after a project delete: %d (last %d)", next, last)
	}
}
