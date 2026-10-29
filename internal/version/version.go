// Package version holds build information, injected with -ldflags at build time.
package version

var (
	// Version is the semantic version of this build (e.g. "0.1.0"), or "dev".
	Version = "dev"
	// Commit is the short git commit hash of this build.
	Commit = "none"
)

const (
	// APIVersion is the major version of the HTTP API served under /api.
	// It changes only on breaking API changes (clients and the updater check it).
	APIVersion = 1
	// MinClientVersion is the oldest Flutter client version that works with this server.
	MinClientVersion = "0.1.0"
	// MinGateway is the oldest nectar-gateway this panel works with (mirrored in .github/release-requires.json).
	MinGateway = "0.2.0"
)

// GatewayAPIs lists the gateway admin API versions this panel can talk to (mirrored in
// .github/release-requires.json as gateway_api; the updater never pairs the panel with another one).
var GatewayAPIs = []int{1}
