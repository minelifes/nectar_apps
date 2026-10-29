package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestMigrateIdempotent(t *testing.T) {
	s := openTestStore(t)
	if err := s.migrate(context.Background()); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
}

func TestFirstOwnerAndLastOwnerProtection(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)

	owner, err := s.CreateFirstOwner(ctx, " Admin@Example.com ", "Admin", "h")
	if err != nil || !owner.IsOwner || owner.Email != "admin@example.com" {
		t.Fatalf("CreateFirstOwner = %+v, %v", owner, err)
	}
	if _, err := s.CreateFirstOwner(ctx, "x@example.com", "X", "h"); !errors.Is(err, ErrConflict) {
		t.Fatalf("second first-owner: %v", err)
	}
	if _, err := s.CreateUser(ctx, "ADMIN@example.com", "dup", "h", false); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate email (case-insensitive): %v", err)
	}
	if u, err := s.GetUserByEmail(ctx, "ADMIN@EXAMPLE.COM"); err != nil || u.ID != owner.ID {
		t.Fatalf("lookup by email: %v", err)
	}

	yes := true
	if _, err := s.UpdateUser(ctx, owner.ID, UserUpdate{Disabled: &yes}); !errors.Is(err, ErrLastOwner) {
		t.Fatalf("disabling last owner: %v", err)
	}
	if err := s.DeleteUser(ctx, owner.ID); !errors.Is(err, ErrLastOwner) {
		t.Fatalf("deleting last owner: %v", err)
	}
	if err := s.SetOwner(ctx, owner.ID, false); !errors.Is(err, ErrLastOwner) {
		t.Fatalf("revoking last owner: %v", err)
	}

	second, _ := s.CreateUser(ctx, "two@example.com", "Two", "h", false)
	if err := s.SetOwner(ctx, second.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteUser(ctx, owner.ID); err != nil {
		t.Fatalf("delete with another owner present: %v", err)
	}
}

func TestSessions(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	u, _ := s.CreateUser(ctx, "a@example.com", "A", "h", false)

	now := time.Unix(1_800_000_000, 0)
	s.now = func() time.Time { return now }

	web, err := s.CreateSession(ctx, NewSession{UserID: u.ID, Kind: "web", TokenHash: "t1", TokenTTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	if _, got, err := s.SessionByToken(ctx, "t1", time.Minute, time.Hour); err != nil || got.ID != u.ID {
		t.Fatalf("SessionByToken: %v", err)
	}

	// sliding: 50 minutes later the session is extended by another hour
	now = now.Add(50 * time.Minute)
	se, _, err := s.SessionByToken(ctx, "t1", time.Minute, time.Hour)
	if err != nil || !se.TokenExpiresAt.Equal(now.Add(time.Hour).UTC()) {
		t.Fatalf("slide: %v %v", se.TokenExpiresAt, err)
	}
	// expired
	now = now.Add(2 * time.Hour)
	if _, _, err := s.SessionByToken(ctx, "t1", time.Minute, time.Hour); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expired session: %v", err)
	}

	// mobile rotation
	_, err = s.CreateSession(ctx, NewSession{UserID: u.ID, Kind: "mobile", TokenHash: "a1", TokenTTL: 15 * time.Minute,
		RefreshHash: "r1", RefreshTTL: 30 * 24 * time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Hour) // access expired, refresh valid
	if _, _, err := s.SessionByToken(ctx, "a1", time.Minute, 0); !errors.Is(err, ErrNotFound) {
		t.Fatal("old access token should be expired")
	}
	if _, _, err := s.RotateMobile(ctx, "r1", "a2", 15*time.Minute, 30*24*time.Hour); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.SessionByToken(ctx, "a2", time.Minute, 0); err != nil {
		t.Fatalf("rotated access token: %v", err)
	}

	list, _ := s.ListSessions(ctx, u.ID)
	if len(list) != 1 || list[0].Kind != "mobile" {
		t.Fatalf("ListSessions = %+v", list)
	}
	if err := s.DeleteSession(ctx, u.ID+1, list[0].ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("must not delete another user's session")
	}
	if err := s.PurgeExpiredSessions(ctx); err != nil {
		t.Fatal(err)
	}
	_ = web
}

func TestRolesAndBindings(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	u, _ := s.CreateUser(ctx, "a@example.com", "A", "h", false)

	if err := s.SyncBuiltinRole(ctx, "viewer", "Viewer", "ro", []string{"app.view"}); err != nil {
		t.Fatal(err)
	}
	if err := s.SyncBuiltinRole(ctx, "viewer", "Viewer", "ro", []string{"app.view", "app.logs"}); err != nil {
		t.Fatal(err)
	}
	roles, _ := s.ListRoles(ctx)
	if len(roles) != 1 || !roles[0].Builtin || len(roles[0].Permissions) != 2 {
		t.Fatalf("builtin sync: %+v", roles)
	}
	viewer := roles[0]
	if _, err := s.UpdateRole(ctx, viewer.ID, "x", "", nil); !errors.Is(err, ErrBuiltinRole) {
		t.Fatalf("update builtin: %v", err)
	}

	custom, err := s.CreateRole(ctx, "Ops", "", []string{"app.restart"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateRole(ctx, "ops", "", nil); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate role name: %v", err)
	}

	res, _ := s.DB().ExecContext(ctx, `INSERT INTO projects (slug, name, namespace, created_at, updated_at) VALUES ('shop','Shop','prj-shop',0,0)`)
	pid, _ := res.LastInsertId()

	bs, err := s.ReplaceBindings(ctx, u.ID, []BindingInput{
		{RoleID: viewer.ID}, {RoleID: viewer.ID}, // duplicate collapsed
		{RoleID: custom.ID, ProjectID: &pid},
	})
	if err != nil || len(bs) != 2 {
		t.Fatalf("ReplaceBindings = %+v, %v", bs, err)
	}
	if _, err := s.ReplaceBindings(ctx, u.ID, []BindingInput{{RoleID: 999}}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown role: %v", err)
	}
	if bs, _ := s.UserBindings(ctx, u.ID); len(bs) != 2 {
		t.Fatal("failed replace must roll back")
	}
	if err := s.DeleteRole(ctx, custom.ID); !errors.Is(err, ErrRoleInUse) {
		t.Fatalf("delete bound role: %v", err)
	}
	// deleting the project removes its bindings
	if _, err := s.DB().ExecContext(ctx, `DELETE FROM projects WHERE id = ?`, pid); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteRole(ctx, custom.ID); err != nil {
		t.Fatalf("delete unbound role: %v", err)
	}
}

func TestAuditAndSettings(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	for i := 0; i < 3; i++ {
		if err := s.Audit(ctx, nil, "cli", "test.action", "t", "1.2.3.4", map[string]int{"i": i}); err != nil {
			t.Fatal(err)
		}
	}
	got, _ := s.ListAudit(ctx, 2, 0)
	if len(got) != 2 || got[0].ID <= got[1].ID {
		t.Fatalf("ListAudit order/limit: %+v", got)
	}
	older, _ := s.ListAudit(ctx, 10, got[1].ID)
	if len(older) != 1 {
		t.Fatalf("pagination: %+v", older)
	}

	var v struct{ Domain string }
	if err := s.GetSetting(ctx, "base", &v); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	_ = s.SetSetting(ctx, "base", map[string]string{"Domain": "a.io"})
	_ = s.SetSetting(ctx, "base", map[string]string{"Domain": "b.io"})
	if err := s.GetSetting(ctx, "base", &v); err != nil || v.Domain != "b.io" {
		t.Fatalf("settings: %+v %v", v, err)
	}
}
