# Installation

## Binary (pre-built)

Download the latest release from [GitHub Releases](https://github.com/stuttgart-things/clusterscope/releases):

```bash
# Linux amd64
curl -L https://github.com/stuttgart-things/clusterscope/releases/latest/download/clusterscope-linux-amd64 \
  -o /usr/local/bin/clusterscope && chmod +x /usr/local/bin/clusterscope
```

## Build from source

```bash
git clone https://github.com/stuttgart-things/clusterscope.git
cd clusterscope
go build -o clusterscope ./cmd/clusterscope
```

Requires **Go 1.22+**.

## Docker / Container

```bash
docker run --rm -v $(pwd)/clusters:/data \
  ghcr.io/stuttgart-things/clusterscope:latest \
  -serve :8080 -root /data
```

## Kubernetes (KCL)

See [KCL Deployment](kcl.md) for deploying clusterscope as a Kubernetes workload with git-sync sidecar.

## Task (via Taskfile)

If you use [Task](https://taskfile.dev):

```bash
task build-output-binary
# Binary written to /tmp/go/build/clusterscope
```
