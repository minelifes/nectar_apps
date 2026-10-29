package version

import (
	"encoding/json"
	"os"
	"slices"
	"testing"
)

// The release metadata (read by the updaters of other installations) must match the constants.
func TestReleaseRequires(t *testing.T) {
	b, err := os.ReadFile("../../.github/release-requires.json")
	if err != nil {
		t.Fatal(err)
	}
	var req struct {
		GatewayAPI []int  `json:"gateway_api"`
		MinGateway string `json:"min_gateway"`
	}
	if err := json.Unmarshal(b, &req); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(req.GatewayAPI, GatewayAPIs) || req.MinGateway != MinGateway {
		t.Errorf(".github/release-requires.json %+v does not match GatewayAPIs %v / MinGateway %s", req, GatewayAPIs, MinGateway)
	}
}
