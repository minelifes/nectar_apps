package adopt

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"nectar_apps/internal/kube"
)

// Phases of an adoption.
const (
	PhaseDiscovered = "discovered"
	PhaseBackedUp   = "backed-up"
	PhaseShadow     = "shadow"   // installed on 18080/18443, routes imported
	PhaseVerified   = "verified" // shadow compared with the old proxy
	PhaseSoaking    = "soaking"  // cutover done, old proxy is the fallback
	PhaseFinalized  = "finalized"
	PhaseRolledBack = "rolled-back"
	PhaseKeep       = "keep" // strategy B: existing controller kept, no cutover
)

// Step is one change made to the server, recorded so that rollback can undo it.
type Step struct {
	Kind   string          `json:"kind"`   // see the step* constants
	Target string          `json:"target"` // object or file
	Before json.RawMessage `json:"before,omitempty"`
	Note   string          `json:"note,omitempty"`
	At     time.Time       `json:"at"`
}

// Step kinds.
const (
	stepHelmChartConfig = "helmchartconfig" // Target kube-system/traefik; Before = original object or null
	stepServiceType     = "service"         // Target ns/name; Before = original spec subset
	stepHostConfig      = "host-config"     // Target file; Before = path of the backup copy
	stepHostRebind      = "host-reload"     // Target unit (reload after config changes)
	stepHostStop        = "host-stop"       // Target unit (stopped standalone nectar_gateway)
	stepGatewayPorts    = "gateway-ports"   // Before = {"proxy": ":18080", "https": ":18443"}
	stepGatewaySettings = "gateway-settings"
	stepImportedRoutes  = "imported-routes" // Before = [ids] created in the gateway
)

// State is the persisted progress of an adoption (/var/lib/nectar/adopt/state.json and the
// nectar-adopt ConfigMap that the panel reads).
type State struct {
	Strategy  string        `json:"strategy"`
	Phase     string        `json:"phase"`
	BackupDir string        `json:"backup_dir,omitempty"`
	Journal   []Step        `json:"journal,omitempty"`
	Verify    *VerifyResult `json:"verify,omitempty"`
	Gate      *VerifyResult `json:"gate,omitempty"` // health gate after the cutover
	Fallback  struct {
		HTTP string `json:"http"`
		TLS  string `json:"tls"`
	} `json:"fallback"`
	ImportedRoutes []int64   `json:"imported_routes,omitempty"`
	ShadowAt       time.Time `json:"shadow_at,omitempty"`
	CutoverAt      time.Time `json:"cutover_at,omitempty"`
	FinalizedAt    time.Time `json:"finalized_at,omitempty"`
	RolledBackAt   time.Time `json:"rolled_back_at,omitempty"`
	UpdatedAt      time.Time `json:"updated_at"`
	History        []string  `json:"history,omitempty"` // human-readable log of the phases
}

// ConfigMapName is the ConfigMap in the system namespace that mirrors the adoption state for the panel.
const ConfigMapName = "nectar-adopt"

// restart begins a new adoption after a finished one (rolled back or finalized, e.g. after an uninstall
// and a new install): its journal, fallback and imported routes describe a cluster state that is gone,
// and a rollback of the new adoption must not replay them. A run in progress keeps everything.
func (s *State) restart() {
	if s.Phase != PhaseRolledBack && s.Phase != PhaseFinalized {
		return
	}
	*s = State{Phase: PhaseDiscovered, History: s.History}
}

func (e *Env) statePath() string { return filepath.Join(e.StateDir, "adopt", "state.json") }

// LoadState reads the state (a fresh state when none exists).
func (e *Env) LoadState() (*State, error) {
	b, err := e.Host.ReadFile(e.statePath())
	if os.IsNotExist(err) {
		return &State{Phase: PhaseDiscovered}, nil
	}
	if err != nil {
		return nil, err
	}
	var s State
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("adopt state %s: %w", e.statePath(), err)
	}
	return &s, nil
}

// SaveState writes the state file and mirrors it (plus the report) to the ConfigMap.
func (e *Env) SaveState(ctx context.Context, s *State, rep *Report) error {
	s.UpdatedAt = e.now()
	b, _ := json.MarshalIndent(s, "", "  ")
	if err := e.Host.WriteFile(e.statePath(), b); err != nil {
		return err
	}
	data := map[string]any{"state.json": string(b)}
	if rep != nil {
		rb, _ := json.Marshal(rep)
		md := rep.Markdown()
		if len(rb)+len(md) < 900<<10 { // ConfigMaps are limited to 1 MiB
			data["report.json"], data["report.md"] = string(rb), md
		}
		_ = e.Host.WriteFile(filepath.Join(e.StateDir, "adopt", "report.json"), rb)
		_ = e.Host.WriteFile(filepath.Join(e.StateDir, "adopt", "nectar-migration-report.md"), []byte(md))
	} else if old := e.configMap(ctx); old != nil {
		for _, k := range []string{"report.json", "report.md"} {
			if v, ok := old[k]; ok {
				data[k] = v
			}
		}
	}
	var ns kube.Object
	if err := e.Kube.GetObject(ctx, "Namespace", "", e.SystemNamespace, &ns); err != nil {
		return nil // not installed yet (discovery / backup): the file is enough
	}
	_, err := e.Kube.Apply(ctx, kube.Object{
		"apiVersion": "v1", "kind": "ConfigMap",
		"metadata": map[string]any{"name": ConfigMapName, "namespace": e.SystemNamespace,
			"labels": map[string]any{"app.kubernetes.io/part-of": "nectar", "app.kubernetes.io/managed-by": "nectar-apps"}},
		"data": data,
	})
	return err
}

func (e *Env) configMap(ctx context.Context) map[string]any {
	var cm kube.Object
	if e.Kube.GetObject(ctx, "ConfigMap", e.SystemNamespace, ConfigMapName, &cm) != nil {
		return nil
	}
	d, _ := cm["data"].(map[string]any)
	return d
}

func (s *State) record(e *Env, step Step) {
	step.At = e.now()
	s.Journal = append(s.Journal, step)
}

func (s *State) log(e *Env, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	s.History = append(s.History, e.now().Format(time.RFC3339)+" "+msg)
	e.logf("%s", msg)
}
