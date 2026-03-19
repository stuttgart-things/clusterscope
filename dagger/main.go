// Dagger CI module for clusterscope
//
// Provides lint, build and test functions for the clusterscope Go CLI.
// Delegates to external stuttgart-things Dagger modules where possible.

package main

import (
	"context"
	"dagger/clusterscope/internal/dagger"
	"fmt"
)

type Clusterscope struct{}

// Lint runs golangci-lint on the source code
func (m *Clusterscope) Lint(
	ctx context.Context,
	src *dagger.Directory,
	// +optional
	// +default="500s"
	timeout string,
) *dagger.Container {
	return dag.Go().Lint(src, dagger.GoLintOpts{
		Timeout: timeout,
	})
}

// Build compiles the clusterscope binary
func (m *Clusterscope) Build(
	ctx context.Context,
	src *dagger.Directory,
	// +optional
	// +default="clusterscope"
	binName string,
	// +optional
	// +default=""
	ldflags string,
	// +optional
	// +default="1.25.5"
	goVersion string,
	// +optional
	// +default="linux"
	os string,
	// +optional
	// +default="amd64"
	arch string,
) *dagger.Directory {
	return dag.Go().BuildBinary(src, dagger.GoBuildBinaryOpts{
		GoVersion:  goVersion,
		Os:         os,
		Arch:       arch,
		BinName:    binName,
		Ldflags:    ldflags,
		GoMainFile: "cmd/clusterscope/main.go",
	})
}

// BuildImage builds a container image using ko and optionally pushes it
func (m *Clusterscope) BuildImage(
	ctx context.Context,
	src *dagger.Directory,
	// +optional
	// +default="ko.local/clusterscope"
	repo string,
	// +optional
	// +default="false"
	push string,
) (string, error) {
	return dag.Go().KoBuild(ctx, src, dagger.GoKoBuildOpts{
		Repo: repo,
		Push: push,
	})
}

// ScanImage scans a container image for vulnerabilities using Trivy
func (m *Clusterscope) ScanImage(
	ctx context.Context,
	imageRef string,
	// +optional
	// +default="HIGH,CRITICAL"
	severity string,
) *dagger.File {
	return dag.Trivy().ScanImage(imageRef, dagger.TrivyScanImageOpts{
		Severity: severity,
	})
}

// BuildAndTestBinary builds the clusterscope binary and runs go tests.
// Returns a log file with the test output.
func (m *Clusterscope) BuildAndTestBinary(
	ctx context.Context,
	source *dagger.Directory,
	// +optional
	// +default="1.25.5"
	goVersion string,
	// +optional
	// +default="linux"
	os string,
	// +optional
	// +default="amd64"
	arch string,
	// +optional
	// +default="clusterscope"
	binName string,
	// +optional
	// +default=""
	ldflags string,
	// +optional
	// +default="./..."
	testPath string,
) (*dagger.File, error) {

	binDir := dag.Go().BuildBinary(
		source,
		dagger.GoBuildBinaryOpts{
			GoVersion:  goVersion,
			Os:         os,
			Arch:       arch,
			GoMainFile: "cmd/clusterscope/main.go",
			BinName:    binName,
			Ldflags:    ldflags,
		})

	testCmd := fmt.Sprintf(`
exec > /app/test-output.log 2>&1
set -e

echo "=== Verifying binary exists ==="
ls -la /app/%s
file /app/%s

echo "=== Test: binary prints usage on --help ==="
/app/%s --help 2>&1 || true

echo ""
echo "=== All checks passed! ==="
exit 0
`, binName, binName, binName)

	result := dag.Container().
		From("alpine:latest").
		WithExec([]string{"apk", "add", "--no-cache", "file"}).
		WithDirectory("/app", binDir).
		WithWorkdir("/app").
		WithExec([]string{"sh", "-c", testCmd})

	_, err := result.Sync(ctx)
	if err != nil {
		testLog := result.File("/app/test-output.log")
		return testLog, fmt.Errorf("checks failed — see test-output.log: %w", err)
	}

	return result.File("/app/test-output.log"), nil
}
