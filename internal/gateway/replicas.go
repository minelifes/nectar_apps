package gateway

import (
	"context"
	"time"
)

// FeatureReplicas: the gateway can run as edge replicas (PRIMARY_URL) and lists them (GET /api/replicas).
const FeatureReplicas = "replicas"

// Replica is an edge replica as the primary gateway saw it.
type Replica struct {
	Node     string    `json:"node"`
	Address  string    `json:"address"`
	Version  string    `json:"version"`
	LastSeen time.Time `json:"last_seen"`
	Error    string    `json:"error,omitempty"`
}

// Replicas lists the replicas that pulled from the primary in the last hour.
func (c *Client) Replicas(ctx context.Context) ([]Replica, error) {
	var out struct {
		Replicas []Replica `json:"replicas"`
	}
	err := c.do(ctx, "GET", "/api/replicas", nil, &out)
	return out.Replicas, err
}
