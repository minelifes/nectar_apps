// Package config loads the panel configuration from environment variables.
package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Config is the runtime configuration of the panel.
type Config struct {
	// HTTPAddr is the listen address of the API + web UI (NECTAR_HTTP_ADDR, default ":8080").
	HTTPAddr string
	// DataDir holds the SQLite database and uploaded artifacts (NECTAR_DATA_DIR, default "/data").
	DataDir string
	// BaseDomain is the default domain for apps, e.g. "apps.example.com" (NECTAR_BASE_DOMAIN).
	BaseDomain string
	// GatewayURL is the nectar-gateway admin API (NECTAR_GATEWAY_URL).
	GatewayURL string
	// GatewayToken is the service token for the gateway admin API (NECTAR_GATEWAY_TOKEN).
	GatewayToken string
	// Routing selects how domains are served (NECTAR_ROUTING): "gateway" (nectar-gateway, default) or
	// "ingress" (standard Kubernetes Ingress objects for an existing controller — install strategy B).
	Routing string
	// IngressClass is the IngressClass for routing=ingress (NECTAR_INGRESS_CLASS, "" = cluster default).
	IngressClass string
	// IngressIssuer is the cert-manager ClusterIssuer for TLS with routing=ingress (NECTAR_INGRESS_ISSUER).
	IngressIssuer string
	// IngressSync lists the IngressClass names whose Ingress objects (and Traefik IngressRoutes / Gateway
	// API HTTPRoutes of IngressController) are synced to nectar-gateway routes (NECTAR_INGRESS_SYNC,
	// comma-separated, default "nectar"; "*" = all classes; "off" = disabled). Adopt mode sets the classes of
	// the controller it moved behind the gateway.
	IngressSync []string
	// IngressController is the controller those objects were written for (NECTAR_INGRESS_CONTROLLER:
	// traefik, ingress-nginx, …); it decides controller-specific defaults such as HTTPS redirects.
	IngressController string
	// MasterKey encrypts secrets at rest (NECTAR_MASTER_KEY, base64, at least 32 bytes).
	MasterKey []byte
	// SetupToken authorizes creating the first Owner (NECTAR_SETUP_TOKEN).
	SetupToken string
	// KubeAPI overrides the Kubernetes API URL, e.g. "http://127.0.0.1:8001" for `kubectl proxy`
	// (NECTAR_KUBE_API). Empty = in-cluster configuration.
	KubeAPI string
	// InternalURL is the panel URL reachable from app pods (artifact downloads)
	// (NECTAR_INTERNAL_URL, default http://nectar-apps.nectar-system.svc).
	InternalURL string
	// SystemNamespace is where the panel and gateway run (NECTAR_NAMESPACE, default nectar-system).
	SystemNamespace string
	// MaxArtifactBytes limits uploads (NECTAR_MAX_ARTIFACT_MB, default 2048).
	MaxArtifactBytes int64
	// Dev enables development mode: text logs, insecure cookies, a fixed dev master key (NECTAR_DEV=1).
	Dev bool
	// LogLevel is debug|info|warn|error (NECTAR_LOG_LEVEL, default info).
	LogLevel slog.Level
	// RegistryPassword enables the built-in image registry for Git builds (NECTAR_REGISTRY_PASSWORD, set by
	// install.sh --registry; "" = no registry). RegistryHost is the name nodes pull from (NECTAR_REGISTRY_HOST,
	// default registry.nectar.local), RegistryNodePort its NodePort (NECTAR_REGISTRY_NODEPORT, default 30500),
	// RegistrySizeGB its volume (NECTAR_REGISTRY_SIZE_GB, default 20).
	RegistryPassword string
	RegistryHost     string
	RegistryNodePort int
	RegistrySizeGB   int
}

// DBPath is the location of the SQLite database.
func (c Config) DBPath() string { return filepath.Join(c.DataDir, "nectar.db") }

// ArtifactsDir is where uploaded artifacts are stored.
func (c Config) ArtifactsDir() string { return filepath.Join(c.DataDir, "artifacts") }

// devMasterKey is used only with NECTAR_DEV=1 when no key is configured. Never use it in production.
var devMasterKey = []byte("nectar-dev-master-key-do-not-use-in-prod!")

// Load reads the configuration from the environment.
func Load() (Config, error) {
	c := Config{
		HTTPAddr:          env("NECTAR_HTTP_ADDR", ":8080"),
		DataDir:           env("NECTAR_DATA_DIR", "/data"),
		BaseDomain:        strings.ToLower(strings.TrimSpace(os.Getenv("NECTAR_BASE_DOMAIN"))),
		GatewayURL:        strings.TrimRight(os.Getenv("NECTAR_GATEWAY_URL"), "/"),
		GatewayToken:      os.Getenv("NECTAR_GATEWAY_TOKEN"),
		Routing:           env("NECTAR_ROUTING", "gateway"),
		IngressClass:      os.Getenv("NECTAR_INGRESS_CLASS"),
		IngressIssuer:     os.Getenv("NECTAR_INGRESS_ISSUER"),
		IngressController: os.Getenv("NECTAR_INGRESS_CONTROLLER"),
		SetupToken:        os.Getenv("NECTAR_SETUP_TOKEN"),
		KubeAPI:           strings.TrimRight(os.Getenv("NECTAR_KUBE_API"), "/"),
		Dev:               os.Getenv("NECTAR_DEV") == "1" || os.Getenv("NECTAR_DEV") == "true",
		InternalURL:       strings.TrimRight(env("NECTAR_INTERNAL_URL", "http://nectar-apps.nectar-system.svc"), "/"),
		SystemNamespace:   env("NECTAR_NAMESPACE", "nectar-system"),
		RegistryPassword:  os.Getenv("NECTAR_REGISTRY_PASSWORD"),
		RegistryHost:      env("NECTAR_REGISTRY_HOST", "registry.nectar.local"),
	}
	c.RegistryNodePort, _ = strconv.Atoi(env("NECTAR_REGISTRY_NODEPORT", "30500"))
	c.RegistrySizeGB, _ = strconv.Atoi(env("NECTAR_REGISTRY_SIZE_GB", "20"))
	mb, err := strconv.ParseInt(env("NECTAR_MAX_ARTIFACT_MB", "2048"), 10, 64)
	if err != nil || mb <= 0 {
		return c, errors.New("NECTAR_MAX_ARTIFACT_MB must be a positive number")
	}
	c.MaxArtifactBytes = mb << 20

	if v := strings.TrimSpace(env("NECTAR_INGRESS_SYNC", "nectar")); v != "off" && v != "" {
		for _, cl := range strings.Split(v, ",") {
			if cl = strings.TrimSpace(cl); cl != "" {
				c.IngressSync = append(c.IngressSync, cl)
			}
		}
	}
	if c.Routing != "gateway" && c.Routing != "ingress" {
		return c, errors.New("NECTAR_ROUTING must be gateway or ingress")
	}
	if err := c.LogLevel.UnmarshalText([]byte(env("NECTAR_LOG_LEVEL", "info"))); err != nil {
		return c, fmt.Errorf("NECTAR_LOG_LEVEL: %w", err)
	}

	key, err := parseMasterKey(os.Getenv("NECTAR_MASTER_KEY"))
	switch {
	case err != nil:
		return c, fmt.Errorf("NECTAR_MASTER_KEY: %w", err)
	case key == nil && c.Dev:
		key = devMasterKey
	case key == nil:
		return c, errors.New("NECTAR_MASTER_KEY is required (base64, 32+ bytes); install.sh generates it")
	}
	c.MasterKey = key
	return c, nil
}

func parseMasterKey(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	key, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		key, err = base64.RawStdEncoding.DecodeString(s)
	}
	if err != nil {
		return nil, errors.New("not valid base64")
	}
	if len(key) < 32 {
		return nil, fmt.Errorf("too short: %d bytes, need at least 32", len(key))
	}
	return key, nil
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
