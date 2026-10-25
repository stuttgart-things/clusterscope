# Serve Mode

clusterscope can run as a persistent HTTP dashboard server that watches your cluster directories for changes and serves interactive HTML profiles.

## Start the server

```bash
clusterscope -serve :8080 -root ./clusters
```

Open `http://localhost:8080` in your browser.

## How it works

1. On startup, the cluster directories below `-root` are discovered (flat or nested `clusters/<lab>/…/<cluster>`, see [Lab View](lab.md#cluster-discovery-and-labs)) and parsed
2. Technology is auto-detected per cluster (Flux vs ArgoCD)
3. A file watcher monitors `-root`, the cluster directories and the directories in between
4. Changes are debounced (500ms) and trigger a re-scan; removed directories disappear
5. The `/api/clusters` endpoint returns a JSON summary of all clusters
6. `/clusters/<name>` returns the HTML profile for a specific cluster

## Endpoints

| Path | Description |
|---|---|
| `/` | HTMX shell with cluster selector and PR preview form |
| `/api/clusters` | JSON list of cluster summaries |
| `/clusters/<name>` | HTML profile for cluster `<name>` |
| `/lab` | [Lab view](lab.md): all clusters per lab, desired vs. actual, findings |
| `/api/lab` | Lab view as JSON |
| `/endpoints` | [Endpoint inventory](endpoints.md) (live mode) |
| `/api/endpoints` | Endpoint inventory as JSON |
| `/api/targets` | Route URLs in Prometheus `http_sd` format |
| `/preview/<name>?repo=<url>&pr=<N>` | On-demand PR diff visualization |
| `/api/preview/<name>?repo=<url>&pr=<N>` | On-demand PR diff as JSON |
| `/health`, `/healthz` | Liveness probe (`/health` is what sthings-portal polls) |
| `/readyz` | Readiness probe |
| `/metrics` | Prometheus metrics |
| `/pdf/<name>` | PDF export via headless Chromium |

## Behind sthings-portal

`-base-path /clusterscope` (or `BASE_PATH`, KCL `config.basePath`) serves
the dashboard under that prefix. The portal forwards `/clusterscope/...`
unchanged, so every page and API answers under the prefix; `/health`,
`/healthz`, `/readyz`, `/metrics` and `/api/...` (Prometheus `/api/targets`,
in-cluster clients) also stay at the root. `/clusterscope` redirects to
`/clusterscope/`.

The server adds `<base href="<prefix>/">` to every page it hands out, and
the pages only use relative URLs (`fetch("api/clusters")`, `clusters/<name>`,
`lab`, `pdf/<name>`, …), so the HTML the CLI renders into standalone files
needs no prefix at all. The PDF export opens the cluster page under the
prefix as well.

Inside the portal's frame the dashboard drops its title and footer and shows
one compact line (the portal names the service). A request through the
portal (`X-Forwarded-Prefix`) links the portal's design tokens
(`/static/homerun2-tokens.css`), on which the dashboard's colours are built;
opened on its own it looks as before.

## Taskfile

```bash
task ui-serve ROOT=./clusters PORT=8080
task ui-serve-live ROOT=./clusters KUBECONFIG=~/.kube/prod
```

## PR preview in the dashboard

The sidebar includes a **PR Preview** form. Enter a repository URL, PR number, and cluster name to see an interactive diff visualization. See [PR Preview](preview.md) for details.

## Kubernetes deployment

See [KCL Deployment](kcl.md) for running clusterscope in a cluster with a git-sync sidecar that keeps `-root` synchronized from a Git repository.
