# Contributing

## Development setup

```bash
git clone https://github.com/stuttgart-things/clusterscope.git
cd clusterscope
go build ./...
go test ./...
```

Requires **Go 1.22+** and [Task](https://taskfile.dev).

## Available tasks

```bash
task --list
```

Key tasks:

| Task | Description |
|---|---|
| `task lint` | Run golangci-lint via Dagger |
| `task test` | Run Go unit tests with race detector via Dagger (same as CI) |
| `task build-test-binary` | Build binary + smoke test (`--help`) via Dagger |
| `task build-output-binary` | Build binary to `/tmp/go/build/` |
| `task build-scan-image-ko` | Build + push + scan container image |
| `task ui-serve ROOT=./clusters` | Run serve mode locally |
| `task ui-serve-live ROOT=./clusters` | Run serve mode with live enrichment |
| `task ui-static ROOT=./clusters` | Generate static HTML dashboard |
| `task render-manifests` | Render KCL Kubernetes manifests |

## Running tests locally

```bash
go test ./...
```

## Linting

```bash
task lint
# or directly:
dagger call -m ./dagger lint --src .
```

## Adding a new parser

1. Create `pkg/<technology>/parser.go`
2. Implement `ParseDir(dir string) (*graph.ClusterProfile, error)`
3. Populate `graph.Node` with `Type`, `Layer`, `Sub`, `Source`, `Path`, etc.
4. Add technology detection in `internal/scan/scan.go` → `DetectTech()`
5. Wire up in `cmd/clusterscope/main.go` and `internal/serve/serve.go`

## PR workflow

1. Create a feature branch: `git checkout -b feature/my-feature`
2. Implement + test
3. `go build ./... && task lint`
4. Push and open a PR targeting `main`
5. CI runs lint + build automatically

## Code style

- Standard Go formatting (`gofmt`)
- `golangci-lint` with project config
- No panics in library code — return errors
- Security: no credentials in HTML output, validate external inputs at boundaries

## Releases

Releases are cut with [release-please](https://github.com/googleapis/release-please) from the conventional-commit history (`feat:` → minor, `fix:` → patch while < 1.0):

1. Every push to `main` updates the release pull request (version bump + `CHANGELOG.md`).
2. Merging it tags `vX.Y.Z`, creates the GitHub release and dispatches `build-scan-image.yaml` at the tag.
3. The tag build publishes:
    - image `ghcr.io/stuttgart-things/clusterscope:vX.Y.Z`, plus `latest` on the same digest
    - kustomize bases `clusterscope-kustomize` and `clusterscope-kustomize-multi-repo` at `vX.Y.Z` (+ `latest`), **pinned to the image `vX.Y.Z`**
    - binaries (goreleaser) attached to the release

Pushes to `main` publish the image as `<short sha>` and `main`; `latest` only moves on a release. `task trigger-release [TAG=vX.Y.Z]` republishes an existing tag.
