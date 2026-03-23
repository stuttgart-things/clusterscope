# PR Preview

clusterscope can show an interactive visual diff of what a pull request would change in the GitOps cluster graph — added, removed, and modified resources with field-level change details.

## Local testing with the dashboard

The fastest way to try PR preview locally:

```bash
# Point clusterscope at your clusters directory
# Use the parent of the cluster dirs so they appear in the sidebar
clusterscope -serve :8080 -root ./clusters/labul/vsphere

# Open http://localhost:8080
```

In the sidebar, fill in the **PR Preview** form:

- **Repository URL**: `https://github.com/stuttgart-things/stuttgart-things`
- **PR #**: `1919`
- **Cluster**: `movie-scripts`

Click **Preview Diff**. Clusterscope will:

1. Shallow-clone the repo at `refs/pull/1919/head`
2. Also clone the base branch (`main`) for comparison
3. Recursively discover cluster directories in both clones
4. Diff the two graphs and render the result

The diff view shows:

- Color-coded nodes: green (added), red (removed), orange (modified), dimmed (unchanged)
- Before / After / Diff view toggle
- Filter toggles for each diff kind
- Click any node for field-level change details
- Changes panel listing all modifications

Results are cached for 5 minutes — subsequent requests for the same repo+ref are instant.

## CLI diff mode

Compare two local directories directly:

```bash
# Markdown output (default) — for PR comments
clusterscope -diff \
  -base-dir ./clusters/labul/vsphere/movie-scripts \
  -head-dir ./pr-branch/clusters/labul/vsphere/movie-scripts

# JSON output — for CI pipelines
clusterscope -diff \
  -base-dir ./clusters/labul/vsphere/movie-scripts \
  -head-dir ./pr-branch/clusters/labul/vsphere/movie-scripts \
  -format json

# HTML output — interactive diff visualization
clusterscope -diff \
  -base-dir ./clusters/labul/vsphere/movie-scripts \
  -head-dir ./pr-branch/clusters/labul/vsphere/movie-scripts \
  -format html -out diff.html
```

### Example markdown output

```markdown
## Clusterscope Diff: movie-scripts

**-4** removed | 16 unchanged

### Removed
- ~~`claim-machinery-api`~~ (apps)
- ~~`machinery`~~ (apps)
- ~~`tekton`~~ (apps)
- ~~`tekton-dashboard-httproute`~~ (apps)

### Edge Changes
- - ~~`flux-apps` → `claim-machinery-api`~~ (watches)
- - ~~`flux-apps` → `machinery`~~ (watches)
- - ~~`flux-apps` → `tekton-dashboard-httproute`~~ (watches)
- - ~~`flux-apps` → `tekton`~~ (watches)
- - ~~`tekton` → `tekton-dashboard-httproute`~~ (dependsOn)
```

## GitHub Actions integration

Post the diff as a PR comment automatically:

```yaml
- name: Checkout base
  uses: actions/checkout@v4
  with:
    ref: ${{ github.event.pull_request.base.sha }}
    path: base

- name: Checkout head
  uses: actions/checkout@v4
  with:
    ref: ${{ github.event.pull_request.head.sha }}
    path: head

- name: GitOps Preview
  run: |
    clusterscope -diff \
      -base-dir base/clusters/labul/vsphere/movie-scripts \
      -head-dir head/clusters/labul/vsphere/movie-scripts \
      -format markdown > preview.md

- name: Comment on PR
  run: gh pr comment ${{ github.event.number }} --body-file preview.md
  env:
    GH_TOKEN: ${{ github.token }}
```

## On-demand preview API

The dashboard's PR preview form uses these endpoints:

| Endpoint | Description |
|---|---|
| `GET /preview/<cluster>?repo=<url>&pr=<N>` | HTML diff visualization |
| `GET /api/preview/<cluster>?repo=<url>&pr=<N>` | JSON diff data |
| `GET /preview/<cluster>?repo=<url>&ref=<ref>` | Preview any git ref (branch, tag) |
| `GET /api/preview/<cluster>?repo=<url>&ref=<ref>&base=<branch>` | Override base branch (default: `main`) |

### Query parameters

| Param | Description |
|---|---|
| `repo` | Git repository URL (required for on-demand mode) |
| `pr` | Pull request number — constructs `refs/pull/N/head` |
| `ref` | Git ref to preview (alternative to `pr`) |
| `base` | Base branch for comparison (default: `main`) |

### Private repositories

On-demand preview inherits the git auth from the environment. If your local git can fetch the repo, PR preview will work:

```bash
# These all work if configured:
# - SSH keys (~/.ssh/*)
# - Git credential helpers (gh auth, git-credential-manager)
# - GH_TOKEN environment variable
# - .netrc
```

In Kubernetes, mount a git credential secret or set `GH_TOKEN` as a pod env var.

## Sidecar-based preview

For persistent preview of a specific PR (e.g., in a staging environment), use the git-sync sidecar approach instead of on-demand cloning:

```bash
clusterscope -serve :8080 -root /data/base -preview-root /data/head
```

See [KCL Deployment](kcl.md) for the `previewEnabled` configuration that deploys a second git-sync sidecar targeting a PR ref.

## Diff flags reference

| Flag | Default | Description |
|---|---|---|
| `-diff` | `false` | Enable diff mode |
| `-base-dir` | — | Base cluster directory (required with `-diff`) |
| `-head-dir` | — | Head cluster directory (required with `-diff`) |
| `-format` | `markdown` | Output format: `markdown`, `json`, `html` |
| `-out` | stdout | Output file path |
| `-tech` | `flux` | Technology: `flux` or `argocd` |
