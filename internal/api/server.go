// Package api implements the HTTP API described in api/openapi.yaml.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"nectar_apps/internal/adopt"
	"nectar_apps/internal/alerts"
	"nectar_apps/internal/artifacts"
	"nectar_apps/internal/authz"
	"nectar_apps/internal/backup"
	"nectar_apps/internal/build"
	"nectar_apps/internal/config"
	"nectar_apps/internal/deploy"
	"nectar_apps/internal/gateway"
	"nectar_apps/internal/housekeep"
	"nectar_apps/internal/kube"
	"nectar_apps/internal/logstore"
	"nectar_apps/internal/mail"
	"nectar_apps/internal/push"
	"nectar_apps/internal/registry"
	"nectar_apps/internal/routing"
	"nectar_apps/internal/runtimes"
	"nectar_apps/internal/secretbox"
	"nectar_apps/internal/sso"
	"nectar_apps/internal/store"
	"nectar_apps/internal/update"
)

// Session lifetimes.
const (
	webSessionTTL    = 7 * 24 * time.Hour // sliding
	mobileAccessTTL  = 15 * time.Minute
	mobileRefreshTTL = 30 * 24 * time.Hour // sliding on refresh
	sessionCookie    = "nectar_session"
)

// Deps are the dependencies of the API server.
type Deps struct {
	Config    config.Config
	Store     *store.Store
	Kube      *kube.Client      // nil when Kubernetes is not configured
	Deployer  *deploy.Deployer  // nil when Kubernetes is not configured
	Catalog   *runtimes.Catalog // runtime catalog
	Artifacts *artifacts.Store  // artifact files
	// ArtifactKey signs artifact download URLs (rekey.ArtifactKey); nil = the master key (tests)
	ArtifactKey []byte
	EnvBox      *secretbox.Box  // encrypts env values
	Gateway     *gateway.Client // nil when no gateway is configured
	Syncer      *routing.Syncer // domain → gateway route sync (created if nil)
	// IngressSync mirrors Kubernetes Ingress objects into the gateway (nil = disabled).
	IngressSync *adopt.IngressSync
	// Cluster overrides Kube for the adopt-mode endpoints (tests use a fake); default Kube.
	Cluster adopt.Kube
	Log     *slog.Logger
	Web     http.Handler // embedded UI (may be nil)
	// AlertBox encrypts alert channel configs (default EnvBox); AlertSender delivers test messages.
	AlertBox    *secretbox.Box
	AlertSender *alerts.Sender
	// Keeper runs retention and disk cleanup (created from the other dependencies if nil).
	Keeper *housekeep.Keeper
	// Backups runs backup and restore Jobs (nil without Kubernetes).
	Backups *backup.Manager
	// MFABox encrypts TOTP secrets (default EnvBox).
	MFABox *secretbox.Box
	// Mailer sends emails (created from Store and EnvBox if nil).
	Mailer *mail.Mailer
	// Updates checks for and installs panel, gateway and k3s updates (nil = not available).
	Updates *update.Manager
	// Builder runs Git builds (nil without Kubernetes); Registry is the built-in registry (disabled when nil).
	Builder  *build.Builder
	Registry *registry.Registry
	// RegistryBox encrypts registry logins (default EnvBox).
	RegistryBox *secretbox.Box
	// LogStore keeps app log history (nil = off).
	LogStore *logstore.Store
}

// Server holds the API dependencies.
type Server struct {
	cfg       config.Config
	store     *store.Store
	kube      *kube.Client
	deployer  *deploy.Deployer
	catalog   *runtimes.Catalog
	artifacts *artifacts.Store
	artKey    []byte // artifact URL signing key
	envBox    *secretbox.Box
	gateway   *gateway.Client
	syncer    *routing.Syncer
	ingSync   *adopt.IngressSync
	cluster   adopt.Kube
	log       *slog.Logger
	limiter   *loginLimiter
	web       http.Handler

	alertBox    *secretbox.Box
	alertSender *alerts.Sender
	keeper      *housekeep.Keeper
	backups     *backup.Manager
	mfaBox      *secretbox.Box
	mailer      *mail.Mailer
	updates     *update.Manager
	builder     *build.Builder
	registry    *registry.Registry
	registryBox *secretbox.Box
	mfa         mfaState
	access      allowlist
	logs        *logstore.Store
	mux         *http.ServeMux
	ssoBox      *secretbox.Box
	pushBox     *secretbox.Box // Firebase service account key
	pushClient  *push.Client
	ssoClient   *sso.Client
	sso         ssoFlows
	passkeys    passkeyFlows
}

// New creates the API server.
func New(d Deps) *Server {
	if d.Syncer == nil {
		var gw routing.Gateway
		if d.Gateway != nil {
			gw = d.Gateway
		}
		d.Syncer = routing.New(d.Store, gw, d.Log)
	}
	if d.Cluster == nil && d.Kube != nil {
		d.Cluster = d.Kube
	}
	if d.AlertBox == nil {
		d.AlertBox = d.EnvBox
	}
	if d.MFABox == nil {
		d.MFABox = d.EnvBox
	}
	if d.RegistryBox == nil {
		d.RegistryBox = d.EnvBox
	}
	if d.Mailer == nil {
		d.Mailer = &mail.Mailer{Store: d.Store, Box: d.EnvBox, Sender: d.AlertSender}
	}
	if d.AlertSender == nil {
		d.AlertSender = &alerts.Sender{}
	}
	if d.Keeper == nil {
		d.Keeper = &housekeep.Keeper{Store: d.Store, Artifacts: d.Artifacts, DataDir: d.Config.DataDir, DBPath: d.Config.DBPath(),
			Namespace: d.Config.SystemNamespace, Log: d.Log}
		if d.Kube != nil {
			d.Keeper.Kube = d.Kube
		}
	}
	ssoBox, _ := secretbox.New(d.Config.MasterKey, "sso")
	pushBox, _ := secretbox.New(d.Config.MasterKey, "push")
	srv := &Server{ssoBox: ssoBox, pushBox: pushBox, pushClient: &push.Client{}, ssoClient: &sso.Client{}, cfg: d.Config, store: d.Store, kube: d.Kube, deployer: d.Deployer, catalog: d.Catalog,
		artifacts: d.Artifacts, artKey: d.ArtifactKey, envBox: d.EnvBox, gateway: d.Gateway, syncer: d.Syncer, ingSync: d.IngressSync,
		cluster: d.Cluster, log: d.Log, limiter: newLoginLimiter(10, 15*time.Minute), web: d.Web,
		alertBox: d.AlertBox, alertSender: d.AlertSender, keeper: d.Keeper, backups: d.Backups, mfaBox: d.MFABox, mailer: d.Mailer, updates: d.Updates,
		builder: d.Builder, registry: d.Registry, registryBox: d.RegistryBox, logs: d.LogStore}
	if d.Builder != nil {
		d.Builder.OnFinish = srv.previewBuildFinished // commit status + the newest commit of a preview
	}
	return srv
}

// SyncBuiltinRoles writes the built-in roles from the authz catalog into the database.
func (s *Server) SyncBuiltinRoles(ctx context.Context) error {
	for _, r := range authz.BuiltinRoles() {
		if err := s.store.SyncBuiltinRole(ctx, r.Key, r.Name, r.Description, r.Permissions); err != nil {
			return fmt.Errorf("sync role %s: %w", r.Key, err)
		}
	}
	return nil
}

// Handler returns the root HTTP handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// public
	mux.HandleFunc("GET /api/health", s.health)
	mux.HandleFunc("GET /api/version", s.version)
	mux.HandleFunc("GET /api/setup", s.setupStatus)
	mux.HandleFunc("POST /api/setup", s.setup)
	mux.HandleFunc("POST /api/auth/login", s.login)
	mux.HandleFunc("POST /api/auth/refresh", s.refresh)
	mux.HandleFunc("POST /api/auth/mfa", s.loginMFA)
	mux.HandleFunc("POST /api/auth/forgot", s.forgotPassword)
	mux.HandleFunc("GET /api/auth/token", s.checkUserToken)
	mux.HandleFunc("POST /api/auth/reset", s.resetPassword)

	// authenticated
	mux.Handle("POST /api/auth/logout", s.authed(s.logout))
	mux.Handle("GET /api/me", s.authed(s.me))
	mux.Handle("GET /api/me/permissions", s.authed(s.myPermissions))
	mux.Handle("GET /api/me/sessions", s.authed(s.mySessions))
	mux.Handle("GET /api/me/passkeys", s.authed(s.listPasskeys))
	mux.Handle("POST /api/me/passkeys/options", s.authed(s.passkeyOptions))
	mux.Handle("POST /api/me/passkeys", s.authed(s.addPasskey))
	mux.Handle("PATCH /api/me/passkeys/{pk}", s.authed(s.renamePasskey))
	mux.Handle("DELETE /api/me/passkeys/{pk}", s.authed(s.deletePasskey))
	mux.Handle("GET /api/me/push-devices", s.authed(s.listMyPushDevices))
	mux.Handle("PUT /api/me/push-devices", s.authed(s.registerPushDevice))
	mux.Handle("POST /api/me/push-devices/unregister", s.authed(s.unregisterPushDevice))
	mux.Handle("DELETE /api/me/push-devices/{id}", s.authed(s.deleteMyPushDevice))
	mux.Handle("GET /api/push/config", s.authed(s.pushConfig))
	mux.Handle("DELETE /api/me/sessions/{id}", s.interactive(s.deleteMySession))
	mux.Handle("GET /api/me/tokens", s.authed(s.listMyTokens))
	mux.Handle("POST /api/me/tokens", s.interactive(s.createMyToken))
	mux.Handle("DELETE /api/me/tokens/{tid}", s.authed(s.deleteMyToken))
	mux.Handle("GET /api/tokens", s.can("token.manage", s.listAllTokens))
	mux.Handle("DELETE /api/tokens/{tid}", s.can("token.manage", s.deleteAnyToken))
	mux.Handle("GET /api/me/mfa", s.authed(s.mfaStatus))
	mux.Handle("POST /api/me/mfa/setup", s.interactive(s.mfaSetup))
	mux.Handle("POST /api/me/mfa/enable", s.interactive(s.mfaEnable))
	mux.Handle("POST /api/me/mfa/disable", s.interactive(s.mfaDisable))
	mux.Handle("POST /api/me/mfa/recovery-codes", s.interactive(s.mfaRecoveryCodes))
	mux.Handle("DELETE /api/users/{id}/mfa", s.can("user.manage", s.resetUserMFA))
	mux.Handle("POST /api/users/{id}/invite", s.can("user.manage", s.inviteUser))
	mux.Handle("GET /api/settings/smtp", s.can("settings.manage", s.getSMTP))
	mux.Handle("PUT /api/settings/smtp", s.can("settings.manage", s.putSMTP))
	mux.Handle("POST /api/settings/smtp/test", s.can("settings.manage", s.testSMTP))
	mux.Handle("GET /api/settings/access", s.can("settings.manage", s.getAccess))
	mux.Handle("PUT /api/settings/access", s.can("settings.manage", s.putAccess))

	mux.Handle("GET /api/users", s.can("user.manage", s.listUsers))
	mux.Handle("POST /api/users", s.can("user.manage", s.createUser))
	mux.Handle("PATCH /api/users/{id}", s.can("user.manage", s.updateUser))
	mux.Handle("DELETE /api/users/{id}", s.can("user.manage", s.deleteUser))
	mux.Handle("GET /api/users/{id}/bindings", s.can("user.manage", s.getBindings))
	mux.Handle("PUT /api/users/{id}/bindings", s.can("user.manage", s.putBindings))

	mux.Handle("GET /api/permissions", s.authed(s.permissionCatalog))
	mux.Handle("GET /api/roles", s.authed(s.listRoles))
	mux.Handle("POST /api/roles", s.can("role.manage", s.createRole))
	mux.Handle("PUT /api/roles/{id}", s.can("role.manage", s.updateRole))
	mux.Handle("GET /api/users/directory", s.authed(s.userDirectory))
	mux.Handle("GET /api/projects/{pid}/members", s.inProject("project.view", s.projectMembers))
	mux.Handle("PUT /api/projects/{pid}/members/{uid}", s.inProject("project.members", s.putProjectMember))
	mux.Handle("DELETE /api/projects/{pid}/members/{uid}", s.inProject("project.members", s.deleteProjectMember))
	mux.Handle("DELETE /api/roles/{id}", s.can("role.manage", s.deleteRole))

	mux.Handle("GET /api/server/nodes", s.can("server.view", s.listNodes))
	mux.Handle("GET /api/audit", s.can("audit.view", s.listAudit))
	mux.Handle("GET /api/server/metrics", s.can("server.view", s.serverMetrics))
	mux.Handle("GET /api/server/join", s.can("server.manage", s.joinInfo))
	mux.Handle("POST /api/server/nodes/{name}/{action}", s.can("server.manage", s.nodeAction))
	mux.Handle("GET /api/server/edge", s.can("server.view", s.edgeStatus))
	mux.Handle("DELETE /api/server/nodes/{name}", s.can("server.manage", s.deleteNode))
	mux.Handle("GET /api/server/events", s.can("server.view", s.serverEvents))
	mux.Handle("GET /api/server/disk", s.can("server.view", s.serverDisk))
	mux.Handle("POST /api/server/cleanup", s.can("server.manage", s.serverCleanup))
	mux.Handle("POST /api/server/image-cleanup", s.can("server.manage", s.pruneImages))
	mux.Handle("PUT /api/settings/housekeeping", s.can("settings.manage", s.putHousekeeping))

	// M5: backups
	mux.Handle("GET /api/backup/targets", s.can("backup.manage", s.listTargets))
	mux.Handle("POST /api/backup/targets", s.can("backup.manage", s.createTarget))
	mux.Handle("PUT /api/backup/targets/{tid}", s.can("backup.manage", s.updateTarget))
	mux.Handle("DELETE /api/backup/targets/{tid}", s.can("backup.manage", s.deleteTarget))
	mux.Handle("POST /api/backup/targets/{tid}/check", s.can("backup.manage", s.checkTarget))
	mux.Handle("GET /api/backup/targets/{tid}/recovery-kit", s.can("backup.manage", s.recoveryKit))
	// Git builds and registries (M7)
	mux.Handle("GET /api/apps/{id}/source", s.onApp("app.view", s.getSource))
	mux.Handle("PUT /api/apps/{id}/source", s.onApp("app.edit", s.putSource))
	mux.Handle("DELETE /api/apps/{id}/source", s.onApp("app.edit", s.deleteSource))
	mux.Handle("POST /api/apps/{id}/source/webhook-secret", s.onApp("app.edit", s.rotateWebhookSecret))
	mux.Handle("GET /api/apps/{id}/builds", s.onApp("app.view", s.listBuilds))
	mux.Handle("GET /api/apps/{id}/previews", s.onApp("app.view", s.listPreviews))
	mux.Handle("POST /api/apps/{id}/builds", s.onApp("app.deploy", s.startBuild))
	mux.Handle("GET /api/apps/{id}/builds/{n}", s.onApp("app.logs", s.getBuild))
	mux.Handle("POST /api/apps/{id}/builds/{n}/cancel", s.onApp("app.deploy", s.cancelBuild))
	mux.HandleFunc("POST /api/hooks/git/{app}", s.gitWebhook)
	mux.Handle("GET /api/settings/registries", s.can("settings.manage", s.getRegistries))
	mux.Handle("PUT /api/settings/registries", s.can("settings.manage", s.putRegistry))
	mux.Handle("DELETE /api/settings/registries/{rid}", s.can("settings.manage", s.deleteRegistry))
	mux.Handle("GET /api/updates", s.can("server.view", s.getUpdates))
	mux.Handle("PUT /api/updates/settings", s.can("server.manage", s.putUpdateSettings))
	mux.Handle("POST /api/updates/check", s.can("server.manage", s.checkUpdates))
	mux.Handle("POST /api/updates/plan", s.can("server.manage", s.planUpdate))
	mux.Handle("POST /api/updates/apply", s.can("server.manage", s.applyUpdate))
	mux.Handle("POST /api/updates/rollback", s.can("server.manage", s.rollbackUpdate))
	mux.Handle("GET /api/updates/runs", s.can("server.view", s.listUpdateRuns))
	mux.Handle("GET /api/updates/runs/{rid}", s.can("server.view", s.getUpdateRun))
	mux.Handle("DELETE /api/updates/bad/{key}", s.can("server.manage", s.clearBadRelease))
	mux.Handle("POST /api/updates/k3s", s.can("server.manage", s.applyK3s))
	mux.Handle("GET /api/backup/plans", s.can("backup.manage", s.listPlans))
	mux.Handle("POST /api/backup/plans", s.can("backup.manage", s.createPlan))
	mux.Handle("PUT /api/backup/plans/{bid}", s.can("backup.manage", s.updatePlan))
	mux.Handle("DELETE /api/backup/plans/{bid}", s.can("backup.manage", s.deletePlan))
	mux.Handle("POST /api/backup/plans/{bid}/run", s.can("backup.manage", s.runPlan))
	mux.Handle("GET /api/backup/plans/{bid}/snapshots", s.can("backup.manage", s.planSnapshots))
	mux.Handle("POST /api/backup/plans/{bid}/restore", s.can("backup.restore", s.restorePlan))
	mux.Handle("GET /api/backup/runs", s.can("backup.manage", s.listRuns))

	// M4: alerts
	mux.Handle("GET /api/alerts", s.can("server.view", s.listAlerts))
	mux.Handle("POST /api/alerts/read", s.can("server.view", s.readAlerts))
	mux.Handle("DELETE /api/alerts/read/{id}", s.can("server.view", s.unreadAlert))
	mux.Handle("GET /api/alerts/rules", s.can("server.view", s.getAlertRules))
	mux.Handle("PUT /api/alerts/rules", s.can("settings.manage", s.putAlertRules))
	mux.Handle("GET /api/alerts/channels", s.can("settings.manage", s.listChannels))
	mux.Handle("POST /api/alerts/channels", s.can("settings.manage", s.createChannel))
	mux.Handle("PUT /api/alerts/channels/{cid}", s.can("settings.manage", s.updateChannel))
	mux.Handle("DELETE /api/alerts/channels/{cid}", s.can("settings.manage", s.deleteChannel))
	mux.Handle("POST /api/alerts/channels/{cid}/test", s.can("settings.manage", s.testChannel))

	// M1: projects, apps, artifacts, deploys
	mux.Handle("GET /api/runtimes", s.authed(s.listRuntimes))
	mux.Handle("GET /api/projects", s.authed(s.listProjects))
	mux.Handle("POST /api/projects", s.can("project.create", s.createProject))
	mux.Handle("GET /api/projects/{pid}", s.inProject("project.view", s.getProject))
	mux.Handle("PATCH /api/projects/{pid}", s.inProject("project.edit", s.updateProject))
	mux.Handle("DELETE /api/projects/{pid}", s.inProject("project.delete", s.deleteProject))
	mux.Handle("GET /api/projects/{pid}/apps", s.inProject("app.view", s.listApps))
	mux.Handle("POST /api/projects/{pid}/apps", s.inProject("app.create", s.createApp))
	mux.Handle("POST /api/projects/{pid}/apply", s.inProject("app.create", s.applyProject))
	mux.Handle("GET /api/projects/{pid}/export", s.inProject("project.view", s.exportProject))
	mux.Handle("POST /api/projects/{pid}/templates/{tid}", s.inProject("app.create", s.installTemplate))
	mux.Handle("POST /api/projects/{pid}/import/compose", s.inProject("app.create", s.importCompose))
	mux.Handle("GET /api/templates", s.authed(s.listTemplates))
	mux.Handle("GET /api/settings/sso", s.can("settings.manage", s.getSSO))
	mux.Handle("PUT /api/settings/sso", s.can("settings.manage", s.putSSO))
	mux.Handle("GET /api/settings/push", s.can("settings.manage", s.getPushSettings))
	mux.Handle("PUT /api/settings/push", s.can("settings.manage", s.putPushSettings))
	mux.Handle("POST /api/settings/push/test", s.can("settings.manage", s.testPush))
	mux.HandleFunc("GET /api/auth/sso", s.listSSO)
	mux.HandleFunc("POST /api/auth/passkey/options", s.passkeyLoginOptions)
	mux.HandleFunc("POST /api/auth/passkey", s.passkeyLogin)
	mux.HandleFunc("GET /api/auth/sso/{id}/start", s.startSSO)
	mux.HandleFunc("GET /api/auth/sso/{id}/callback", s.callbackSSO)
	mux.Handle("GET /api/projects/{pid}/env-groups", s.inProject("project.view", s.listEnvGroups))
	mux.Handle("PUT /api/projects/{pid}/env-groups/{name}", s.inProject("env.edit", s.putEnvGroup))
	mux.Handle("DELETE /api/projects/{pid}/env-groups/{name}", s.inProject("env.edit", s.deleteEnvGroup))
	mux.Handle("PUT /api/apps/{id}/env-groups", s.onApp("env.edit", s.putAppEnvGroups))
	mux.Handle("POST /api/apps/{id}/clone", s.onApp("app.view", s.cloneApp))
	mux.Handle("POST /api/apps/{id}/promote", s.onApp("app.view", s.promoteApp))
	mux.Handle("POST /api/projects/{pid}/clone", s.inProject("project.view", s.cloneProject))

	mux.Handle("GET /api/apps/{id}", s.onApp("app.view", s.getApp))
	mux.Handle("PATCH /api/apps/{id}", s.onApp("app.edit", s.updateApp))
	mux.Handle("DELETE /api/apps/{id}", s.onApp("app.delete", s.deleteApp))
	mux.Handle("GET /api/apps/{id}/status", s.onApp("app.view", s.appStatus))
	mux.Handle("GET /api/apps/{id}/usage", s.onApp("app.metrics", s.appUsage))
	mux.Handle("GET /api/apps/{id}/events", s.onApp("app.view", s.appEvents))
	mux.Handle("GET /api/apps/{id}/alerts", s.onApp("app.view", s.appAlerts))
	mux.Handle("GET /api/apps/{id}/pods", s.onApp("app.logs", s.appPods))
	mux.Handle("GET /api/apps/{id}/logs", s.onApp("app.logs", s.appLogs))
	mux.Handle("GET /api/apps/{id}/logs/ws", s.onApp("app.logs", s.appLogsWS))
	mux.Handle("GET /api/apps/{id}/logs/history", s.onApp("app.logs", s.appLogHistory))
	mux.Handle("GET /api/apps/{id}/exec/ws", s.onApp("app.exec", s.appExecWS))
	mux.Handle("GET /api/apps/{id}/env", s.onApp("env.view", s.getEnv))
	mux.Handle("PUT /api/apps/{id}/env", s.onApp("env.edit", s.putEnv))
	mux.Handle("GET /api/apps/{id}/artifacts", s.onApp("app.view", s.listArtifacts))
	mux.Handle("POST /api/apps/{id}/artifacts", s.onApp("app.deploy", s.uploadArtifact))
	mux.Handle("POST /api/apps/{id}/uploads", s.onApp("app.deploy", s.tusCreate))
	mux.HandleFunc("OPTIONS /api/uploads", s.tusOptions)
	mux.Handle("HEAD /api/uploads/{uid}", s.authed(s.tusHead))
	mux.Handle("PATCH /api/uploads/{uid}", s.authed(s.tusPatch))
	mux.Handle("DELETE /api/uploads/{uid}", s.authed(s.tusDelete))
	mux.Handle("POST /api/apps/{id}/deploy", s.onApp("app.deploy", s.deployApp))
	mux.Handle("GET /api/apps/{id}/releases", s.onApp("app.view", s.listReleases))
	mux.Handle("POST /api/apps/{id}/releases/{n}/rollback", s.onApp("app.rollback", s.rollback))
	mux.Handle("POST /api/apps/{id}/run", s.onApp("app.deploy", s.runTask))
	mux.Handle("POST /api/apps/{id}/scale", s.onApp("app.restart", s.scaleApp))
	mux.Handle("POST /api/apps/{id}/start", s.onApp("app.restart", s.startApp))
	mux.Handle("POST /api/apps/{id}/stop", s.onApp("app.restart", s.stopApp))
	mux.Handle("POST /api/apps/{id}/restart", s.onApp("app.restart", s.restartApp))
	mux.Handle("GET /api/apps/{id}/volumes", s.onApp("app.view", s.listVolumes))
	mux.Handle("DELETE /api/apps/{id}/volumes/{name}", s.onApp("volume.manage", s.deleteVolume))
	mux.Handle("GET /api/apps/{id}/volumes/{name}/files", s.onApp("volume.manage", s.listFiles))
	mux.Handle("GET /api/apps/{id}/volumes/{name}/files/content", s.onApp("volume.manage", s.downloadFile))
	mux.Handle("PUT /api/apps/{id}/volumes/{name}/files/content", s.onApp("volume.manage", s.uploadFile))
	mux.Handle("POST /api/apps/{id}/volumes/{name}/files/content", s.onApp("volume.manage", s.uploadFile)) // clients that stream with POST
	mux.Handle("POST /api/apps/{id}/volumes/{name}/files", s.onApp("volume.manage", s.fileAction))
	mux.Handle("POST /api/apps/{id}/volumes/{name}/files/close", s.onApp("volume.manage", s.closeFiles))
	mux.Handle("GET /api/server/volumes", s.can("server.view", s.serverVolumes))
	mux.Handle("GET /api/server/storage-classes", s.authed(s.storageClasses))
	mux.Handle("GET /api/apps/{id}/addons", s.onApp("app.view", s.appAddons))
	mux.Handle("GET /api/addon-kinds", s.authed(s.addonCatalog))
	mux.Handle("GET /api/projects/{pid}/addons", s.inProject("app.view", s.listAddons))
	mux.Handle("POST /api/projects/{pid}/addons", s.inProject("app.create", s.createAddon))
	mux.Handle("GET /api/addons/{aid}", s.onAddon("app.view", s.getAddon))
	mux.Handle("PATCH /api/addons/{aid}", s.onAddon("app.edit", s.updateAddon))
	mux.Handle("DELETE /api/addons/{aid}", s.onAddon("app.delete", s.deleteAddon))
	mux.Handle("POST /api/addons/{aid}/stop", s.onAddon("app.restart", s.setAddonStopped(true)))
	mux.Handle("POST /api/addons/{aid}/start", s.onAddon("app.restart", s.setAddonStopped(false)))
	mux.Handle("POST /api/addons/{aid}/restart", s.onAddon("app.restart", s.restartAddon))
	mux.Handle("PUT /api/addons/{aid}/shared", s.onAddon("app.view", s.shareAddon)) // + env.edit, checked inside
	mux.Handle("GET /api/addons/{aid}/credentials", s.onAddon("env.secrets.view", s.addonCredentials))
	mux.Handle("POST /api/addons/{aid}/links", s.onAddon("app.view", s.linkAddon))
	mux.Handle("DELETE /api/addons/{aid}/links/{app}", s.onAddon("app.view", s.unlinkAddon))
	// databases get the same observe endpoints as apps (same permissions)
	mux.Handle("GET /api/addons/{aid}/usage", s.onAddon("app.metrics", s.addonUsage))
	mux.Handle("GET /api/addons/{aid}/events", s.onAddon("app.view", s.addonEvents))
	mux.Handle("GET /api/addons/{aid}/pods", s.onAddon("app.logs", s.addonPods))
	mux.Handle("GET /api/addons/{aid}/logs", s.onAddon("app.logs", s.addonLogs))
	mux.Handle("GET /api/addons/{aid}/logs/ws", s.onAddon("app.logs", s.addonLogsWS))
	mux.Handle("GET /api/addons/{aid}/exec/ws", s.onAddon("app.exec", s.addonExecWS))
	mux.Handle("GET /api/apps/{id}/files", s.onApp("env.view", s.getFiles))
	mux.Handle("PUT /api/apps/{id}/files", s.onApp("env.edit", s.putFiles))

	// M2: domains and settings
	mux.Handle("GET /api/onboarding", s.can("settings.manage", s.getOnboarding))
	mux.Handle("PUT /api/onboarding", s.can("settings.manage", s.putOnboarding))
	mux.Handle("POST /api/settings/dns-check", s.can("settings.manage", s.dnsCheckBase))
	mux.Handle("GET /api/projects/{pid}/activity", s.inProject("project.view", s.projectActivity))
	mux.Handle("GET /api/settings", s.authed(s.getSettings))
	mux.Handle("PUT /api/settings", s.can("settings.manage", s.putSettings))
	mux.Handle("GET /api/apps/{id}/domains", s.onApp("domain.view", s.listDomains))
	mux.Handle("POST /api/apps/{id}/domains", s.onApp("domain.manage", s.createDomain))
	mux.Handle("PATCH /api/apps/{id}/domains/{did}", s.onApp("domain.manage", s.updateDomain))
	mux.Handle("DELETE /api/apps/{id}/domains/{did}", s.onApp("domain.manage", s.deleteDomain))
	mux.Handle("POST /api/apps/{id}/domains/{did}/check", s.onApp("domain.view", s.checkDomain))
	mux.Handle("POST /api/apps/{id}/domains/{did}/certificate", s.onApp("domain.tls", s.issueDomainCert))
	mux.Handle("POST /api/apps/{id}/domains/{did}/dns", s.onApp("domain.manage", s.createDomainDNS))
	mux.Handle("GET /api/domains", s.authed(s.listAllDomains))
	mux.Handle("GET /api/apps/{id}/metrics", s.onApp("app.metrics", s.appMetrics))
	mux.Handle("GET /api/apps/{id}/access-log", s.onApp("app.logs", s.appAccessLog))

	// gateway (headless): observability and runtime settings
	mux.Handle("GET /api/gateway/metrics", s.can("server.view", s.relay("/api/metrics", "minutes", "domain", "series")))
	mux.Handle("GET /api/gateway/access-log", s.can("server.view", s.relay("/api/access-log", "domain", "status", "ip", "limit")))
	mux.Handle("GET /api/gateway/health", s.can("server.view", s.relay("/api/health")))
	mux.Handle("GET /api/gateway/blocked", s.can("server.view", s.relay("/api/blocked")))
	mux.Handle("DELETE /api/gateway/blocked/{ip}", s.can("server.manage", s.unblockIP))
	mux.Handle("POST /api/gateway/blocked", s.can("server.manage", s.blockIP))
	mux.Handle("GET /api/gateway/routes", s.can("server.view", s.relay("/api/routes", "managed_by")))
	mux.Handle("GET /api/gateway/settings", s.can("settings.manage", s.relay("/api/settings")))
	mux.Handle("PUT /api/gateway/settings", s.can("settings.manage", s.putGatewaySettings))
	mux.Handle("GET /api/gateway/certificates", s.can("server.view", s.listCertificates))
	mux.Handle("POST /api/gateway/certificates", s.can("settings.manage", s.createCertificate))
	mux.Handle("POST /api/gateway/certificates/{cid}/renew", s.can("settings.manage", s.certificateAction))
	mux.Handle("DELETE /api/gateway/certificates/{cid}", s.can("settings.manage", s.certificateAction))

	// adopt mode: migration state, external namespaces
	mux.Handle("GET /api/adopt", s.can("settings.manage", s.adoptStatus))
	mux.Handle("DELETE /api/adopt/fallback-hits", s.can("settings.manage", s.resetFallbackHits))
	mux.Handle("GET /api/external/namespaces", s.can("server.view", s.externalNamespaces))
	mux.Handle("POST /api/external/namespaces/{ns}/adopt", s.can("project.create", s.adoptNamespace))

	// artifact downloads by app pods (signed URLs, no session)
	mux.HandleFunc("GET /internal/artifacts/{sha}", s.internalArtifact)

	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "no such endpoint")
	})
	if s.web != nil {
		mux.Handle("/", s.web)
	}

	s.mux = mux // blueprints apply themselves through the API (internal calls)
	var h http.Handler = securityHeaders(s.ipAllowlist(mux))
	if s.cfg.Dev {
		h = devCORS(h)
	}
	return s.recoverer(s.logRequests(h))
}

// devCORS lets the Flutter dev server (`flutter run -d chrome`, a random localhost port) call the API in
// development mode. Only localhost origins, only bearer tokens (no cookies / credentials), never in production.
func devCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if o := r.Header.Get("Origin"); o != "" && strings.HasPrefix(r.URL.Path, "/api/") && isLocalOrigin(o) {
			h := w.Header()
			h.Set("Access-Control-Allow-Origin", o)
			h.Add("Vary", "Origin")
			if r.Method == http.MethodOptions {
				h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
				h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Tus-Resumable, Upload-Length, Upload-Offset, Upload-Metadata")
				h.Set("Access-Control-Max-Age", "600")
				w.WriteHeader(http.StatusNoContent)
				return
			}
			h.Set("Access-Control-Expose-Headers", "Location, Upload-Offset, Upload-Length")
		}
		next.ServeHTTP(w, r)
	})
}

func isLocalOrigin(o string) bool {
	u, err := url.Parse(o)
	if err != nil || u.Scheme != "http" {
		return false
	}
	switch u.Hostname() {
	case "localhost", "127.0.0.1", "::1":
		return true
	}
	return false
}

// ---------------------------------------------------------------- principal --

type principal struct {
	user    store.User
	session *store.Session  // nil for API tokens
	token   *store.APIToken // set for API tokens
	eff     authz.Effective
}

type ctxKey struct{}

func principalFrom(ctx context.Context) *principal {
	p, _ := ctx.Value(ctxKey{}).(*principal)
	return p
}

// authenticate resolves the caller from a bearer token or the session cookie.
func (s *Server) authenticate(r *http.Request) (*principal, error) {
	var (
		se  store.Session
		u   store.User
		err error
	)
	tok, ok := bearerToken(r)
	if !ok {
		tok, ok = wsToken(r)
	}
	if ok && strings.HasPrefix(tok, "nat_") {
		return s.tokenPrincipal(r.Context(), tok)
	}
	if ok {
		if !strings.HasPrefix(tok, "nxa_") {
			return nil, errUnauthorized
		}
		se, u, err = s.store.SessionByToken(r.Context(), hashToken(tok), time.Minute, 0)
	} else if c, cerr := r.Cookie(sessionCookie); cerr == nil && c.Value != "" {
		if !sameOrigin(r) {
			return nil, errForbiddenOrigin
		}
		se, u, err = s.store.SessionByToken(r.Context(), hashToken(c.Value), 5*time.Minute, webSessionTTL)
	} else {
		return nil, errUnauthorized
	}
	if errors.Is(err, store.ErrNotFound) {
		return nil, errUnauthorized
	}
	if err != nil {
		return nil, err
	}
	if u.Disabled {
		return nil, errUnauthorized
	}
	eff, err := s.effective(r.Context(), u)
	if err != nil {
		return nil, err
	}
	return &principal{user: u, session: &se, eff: eff}, nil
}

// tokenPrincipal resolves an API token: the user's rights, limited to the token's role and project.
func (s *Server) tokenPrincipal(ctx context.Context, tok string) (*principal, error) {
	t, u, err := s.store.APITokenByHash(ctx, hashToken(tok))
	if errors.Is(err, store.ErrNotFound) {
		return nil, errUnauthorized
	}
	if err != nil {
		return nil, err
	}
	if u.Disabled {
		return nil, errUnauthorized
	}
	eff, err := s.effective(ctx, u)
	if err != nil {
		return nil, err
	}
	return &principal{user: u, token: &t, eff: tokenEffective(eff, t)}, nil
}

func tokenEffective(user authz.Effective, t store.APIToken) authz.Effective {
	switch {
	case t.RoleID != nil:
		return authz.Intersect(user, authz.Evaluate(false, []authz.Binding{{ProjectID: t.ProjectID, Permissions: t.RolePermissions}}))
	case t.ProjectID != nil:
		return authz.Restrict(user, *t.ProjectID)
	}
	return user
}

func (s *Server) effective(ctx context.Context, u store.User) (authz.Effective, error) {
	if u.IsOwner {
		return authz.Evaluate(true, nil), nil
	}
	bs, err := s.store.UserBindings(ctx, u.ID)
	if err != nil {
		return authz.Effective{}, err
	}
	in := make([]authz.Binding, len(bs))
	for i, b := range bs {
		in[i] = authz.Binding{ProjectID: b.ProjectID, Permissions: b.Permissions}
	}
	return authz.Evaluate(false, in), nil
}

var (
	errUnauthorized    = errors.New("not logged in")
	errForbiddenOrigin = errors.New("cross-origin request rejected")
)

type handlerWithPrincipal func(w http.ResponseWriter, r *http.Request, p *principal)

// authed requires a logged-in caller.
func (s *Server) authed(h handlerWithPrincipal) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, err := s.authenticate(r)
		switch {
		case errors.Is(err, errUnauthorized):
			writeError(w, http.StatusUnauthorized, err.Error())
			return
		case errors.Is(err, errForbiddenOrigin):
			writeError(w, http.StatusForbidden, err.Error())
			return
		case err != nil:
			s.serverError(w, r, err)
			return
		}
		h(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, p)), p)
	})
}

// interactive requires a signed-in person: API tokens cannot change sessions, two-factor settings or tokens.
func (s *Server) interactive(h handlerWithPrincipal) http.Handler {
	return s.authed(func(w http.ResponseWriter, r *http.Request, p *principal) {
		if p.token != nil {
			writeError(w, http.StatusForbidden, "not allowed with an API token: sign in to the panel")
			return
		}
		h(w, r, p)
	})
}

// can requires a global permission.
func (s *Server) can(perm string, h handlerWithPrincipal) http.Handler {
	return s.authed(func(w http.ResponseWriter, r *http.Request, p *principal) {
		if !p.eff.Can(perm, nil) {
			writeError(w, http.StatusForbidden, "missing permission: "+perm)
			return
		}
		h(w, r, p)
	})
}

// audit records an action by p (never fails the request).
func (s *Server) audit(r *http.Request, p *principal, action, target string, payload any) {
	var uid *int64
	email := ""
	if p != nil {
		uid, email = &p.user.ID, p.user.Email
	}
	if err := s.store.Audit(r.Context(), uid, email, action, target, clientIP(r), payload); err != nil {
		s.log.Error("audit write failed", "action", action, "err", err)
	}
}

// ---------------------------------------------------------------- helpers --

func bearerToken(r *http.Request) (string, bool) {
	h := r.Header.Get("Authorization")
	if t, ok := strings.CutPrefix(h, "Bearer "); ok && t != "" {
		return strings.TrimSpace(t), true
	}
	return "", false
}

// sameOrigin protects cookie-authenticated unsafe requests against CSRF (in addition to SameSite=Strict).
func sameOrigin(r *http.Request) bool {
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	}
	if o := r.Header.Get("Origin"); o != "" {
		return o == "http://"+r.Host || o == "https://"+r.Host
	}
	if site := r.Header.Get("Sec-Fetch-Site"); site != "" {
		return site == "same-origin" || site == "none"
	}
	return true // non-browser client
}

// clientIP returns the caller's IP. X-Forwarded-For is trusted only from private/loopback peers
// (the gateway runs inside the cluster).
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	addr, err := netip.ParseAddr(host)
	if err == nil && (addr.IsPrivate() || addr.IsLoopback()) {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			first := strings.TrimSpace(strings.Split(xff, ",")[0])
			if _, err := netip.ParseAddr(first); err == nil {
				return first
			}
		}
	}
	return host
}

func pathID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id, err == nil && id > 0
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func (s *Server) serverError(w http.ResponseWriter, r *http.Request, err error) {
	s.log.Error("request failed", "method", r.Method, "path", r.URL.Path, "err", err)
	writeError(w, http.StatusInternalServerError, "internal error")
}

// decode reads a JSON body (max 1 MiB, unknown fields rejected).
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		msg := "invalid JSON body"
		if !errors.Is(err, io.EOF) {
			msg += ": " + err.Error()
		}
		writeError(w, http.StatusBadRequest, msg)
		return false
	}
	return true
}

// -------------------------------------------------------------- middleware --

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) { r.status = code; r.ResponseWriter.WriteHeader(code) }

func (r *statusRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		if r.URL.Path == "/api/health" && rec.status == http.StatusOK {
			return // probes every few seconds
		}
		s.log.Info("http", "method", r.Method, "path", r.URL.Path, "status", rec.status,
			"ms", time.Since(start).Milliseconds(), "ip", clientIP(r))
	})
}

func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				if v == http.ErrAbortHandler {
					panic(v)
				}
				s.log.Error("panic", "path", r.URL.Path, "panic", v, "stack", string(debug.Stack()))
				writeError(w, http.StatusInternalServerError, "internal error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "same-origin")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			h.Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}
