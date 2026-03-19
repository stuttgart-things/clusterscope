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
| `task build-test-binary` | Build + test via Dagger |
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
