// Package archcheck_test enforces architecture import boundaries using go list.
// Run with: go test ./test/archcheck/...
package archcheck_test

import (
	"os/exec"
	"strings"
	"testing"
)

const module = "MRMI_Gateway"

// directImports returns the set of direct imports for pkg using go list.
func directImports(t *testing.T, pkg string) map[string]struct{} {
	t.Helper()
	out, err := exec.Command("go", "list", "-f", "{{join .Imports \",\"}}", pkg).Output()
	if err != nil {
		t.Fatalf("go list %s: %v", pkg, err)
	}
	result := make(map[string]struct{})
	for _, imp := range strings.Split(strings.TrimSpace(string(out)), ",") {
		if imp = strings.TrimSpace(imp); imp != "" {
			result[imp] = struct{}{}
		}
	}
	return result
}

// assertNoImports fails if imports contains any package matching a forbidden prefix or exact path.
func assertNoImports(t *testing.T, from string, imports map[string]struct{}, forbidden ...string) {
	t.Helper()
	for imp := range imports {
		for _, prefix := range forbidden {
			if imp == prefix || strings.HasPrefix(imp, prefix+"/") {
				t.Errorf("boundary violation: %s must not import %s (forbidden prefix: %q)", from, imp, prefix)
			}
		}
	}
}

// assertOnlyInternalImports fails if any MRMI_Gateway/* import is not in the allowed set.
// Stdlib and third-party imports are always permitted.
func assertOnlyInternalImports(t *testing.T, from string, imports map[string]struct{}, allowed ...string) {
	t.Helper()
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, a := range allowed {
		allowedSet[a] = struct{}{}
	}
	for imp := range imports {
		if !strings.HasPrefix(imp, module+"/") {
			continue
		}
		if _, ok := allowedSet[imp]; !ok {
			t.Errorf("boundary violation: %s must not import %s", from, imp)
		}
	}
}

// TestCoreBoundary guards internal/core from infrastructure and transport packages.
// core is the use-case layer: domain types only; no gRPC, no proto, no HTTP server,
// no transport adapter, no wiring layer.
func TestCoreBoundary(t *testing.T) {
	imports := directImports(t, module+"/internal/core")
	assertNoImports(t, "internal/core", imports,
		"google.golang.org/grpc",
		"google.golang.org/protobuf",
		module+"/internal/server",
		module+"/internal/transport",
		module+"/internal/app",
	)
}

// TestGRPCTransportBoundary guards internal/transport/grpc from the HTTP server
// and app wiring layers. The gRPC adapter handles translation and lifecycle only;
// it must not depend on the HTTP management surface or construction logic.
func TestGRPCTransportBoundary(t *testing.T) {
	imports := directImports(t, module+"/internal/transport/grpc")
	assertNoImports(t, "internal/transport/grpc", imports,
		module+"/internal/server",
		module+"/internal/app",
	)
}

// TestGatewayBinaryBoundary guards cmd/mrmi-gateway from accumulating business logic.
// The gateway entrypoint wires the process; it may only import the app wiring layer,
// config loader, and version — not domain or infrastructure packages directly.
func TestGatewayBinaryBoundary(t *testing.T) {
	imports := directImports(t, module+"/cmd/mrmi-gateway")
	assertOnlyInternalImports(t, "cmd/mrmi-gateway", imports,
		module+"/internal/app",
		module+"/internal/config",
		module+"/internal/version",
	)
}

// TestCLIBoundary guards cmd/mrmi from importing infrastructure packages.
// The operator CLI may use leaf domain packages (audit, identity) but must not
// reach into the wiring layer, HTTP server, gRPC transport, use-case core, or delivery.
func TestCLIBoundary(t *testing.T) {
	imports := directImports(t, module+"/cmd/mrmi")
	assertNoImports(t, "cmd/mrmi", imports,
		module+"/internal/app",
		module+"/internal/server",
		module+"/internal/transport",
		module+"/internal/core",
		module+"/internal/delivery",
	)
}
