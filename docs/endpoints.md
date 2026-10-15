# Endpoint Inventory

With `-live`, clusterscope collects the Gateway API objects of every reachable cluster and answers *"which URLs exist in this lab, where do they go, and do they work?"* — across all clusters under `-root`.

HTTPRoutes are read **live** from the clusters, not from Git: many routes are generated (Helm chart values, KCL modules, Flux `postBuild` substitution) and only exist after rendering.

## Usage

```bash
# inventory only (Gateway / HTTPRoute status)
clusterscope -serve :8080 -root ./clusters/labul/vsphere -live -kubeconfig-dir /kubeconfigs

# plus a one-shot reachability probe of every route URL on each refresh
clusterscope -serve :8080 -root ./clusters/labul/vsphere -live -kubeconfig-dir /kubeconfigs -probe
```

| Flag | Default | Description |
|---|---|---|
| `-probe` | `false` | Probe every HTTPRoute URL on each live refresh |
| `-probe-timeout` | `5s` | Per-URL probe timeout |

Open **Inventory → Endpoints** in the dashboard sidebar (or `/#endpoints`, standalone at `/endpoints`). Filters (text, cluster, *problems only*) are kept in the URL, so a filtered view can be shared: `/endpoints?cluster=platform-sthings&problems=1`.

## What is collected

Per cluster, `gateway.networking.k8s.io/v1`:

| Field | Source |
|---|---|
| hostnames, paths | `spec.hostnames`, `spec.rules[].matches[].path` |
| gateway | `spec.parentRefs` (`namespace/name[/section]`) |
| backends | `spec.rules[].backendRefs` (`[ns/]name:port`) |
| Accepted / ResolvedRefs | `status.parents[].conditions` — `True` only if **all** parents report `True` |
| owning app | `helm.toolkit.fluxcd.io/name` → `argocd.argoproj.io/tracking-id` → `kustomize.toolkit.fluxcd.io/name` → (Argo CD clusters) `app.kubernetes.io/instance` |
| Gateways | class, listeners, addresses, `Accepted` / `Programmed` |

Clusters without Gateway API CRDs are shown as such, not as an error.

### URLs

One URL per concrete hostname (wildcard hostnames are listed but not probed). The scheme comes from the parent Gateway's listener that matches the hostname (`HTTPS` → `https`, only `HTTP` → `http`, unknown → `https`). The root `/` is used when any rule matches `/`, otherwise the first path.

## Probe

With `-probe`, every URL is checked once per live refresh (concurrency 8):

| Stage | Failure means |
|---|---|
| `dns` | hostname does not resolve |
| `connect` | TCP connection failed |
| `tls` | handshake / certificate verification failed |
| `http` | status ≥ 500, or other HTTP error |

A status below 500 counts as OK — `401`, `403`, `404` and redirects (not followed) mean routing, certificate and backend work and the application answered. For HTTPS the leaf certificate's expiry is reported; the UI flags certificates expiring in less than 14 days.

While a cluster's API server is **unreachable**, its last known routes are kept (marked *stale*) and still probed — an unreachable API server does not mean the applications are down.

The probe is a dashboard snapshot. Continuous probing and alerting belong in Prometheus + blackbox-exporter — see `/api/targets`.

### Excluding a route

Some routes are not meant to answer all the time — an ACME HTTP-01 route, for example, whose solver only listens while a certificate is issued. Annotate them to skip the probe:

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: piezo-a-acme
  annotations:
    clusterscope.stuttgart-things.com/probe: "false"
```

The route and its URLs stay in the inventory (marked *not probed*), but they are not probed, raise no `endpoint-down` finding and are left out of `/api/targets` — so blackbox-exporter does not alert on them either. Any other value, or no annotation, probes as usual.

## API

### `GET /api/endpoints[?cluster=<name>]`

```json
{
  "clusters": [{"name": "platform-sthings", "reachability": "reachable", "gatewayAPI": true, "collectedAt": "…"}],
  "gateways": [{"cluster": "platform-sthings", "namespace": "default", "name": "platform-sthings-gateway", "class": "cilium", "listeners": […], "programmed": "True"}],
  "routes": [{
    "cluster": "platform-sthings", "namespace": "argocd", "name": "argocd-server",
    "hostnames": ["argocd.platform.example.com"], "urls": ["https://argocd.platform.example.com"],
    "parents": [{"gateway": "default/platform-sthings-gateway", "accepted": "True", "resolvedRefs": "True"}],
    "backends": ["argocd-server:443"], "app": "argo-cd-httproute",
    "accepted": "True", "resolvedRefs": "True",
    "probes": [{"url": "https://argocd.platform.example.com", "ok": true, "statusCode": 200, "latencyMs": 38, "certExpiry": "…"}]
  }],
  "probing": true
}
```

### `GET /api/targets[?cluster=<name>]`

Prometheus [`http_sd`](https://prometheus.io/docs/prometheus/latest/http_sd/) format, one target group per route URL:

```json
[{"targets": ["https://argocd.platform.example.com"], "labels": {"cluster": "platform-sthings", "namespace": "argocd", "route": "argocd-server", "app": "argo-cd-httproute"}}]
```

blackbox-exporter scrape config:

```yaml
- job_name: blackbox-httproutes
  metrics_path: /probe
  params:
    module: [http_2xx]
  http_sd_configs:
    - url: http://clusterscope.clusterscope.svc:8080/api/targets
  relabel_configs:
    - source_labels: [__address__]
      target_label: __param_target
    - source_labels: [__param_target]
      target_label: instance
    - target_label: __address__
      replacement: blackbox-exporter.monitoring.svc:9115
```

## Metrics

| Metric | Labels | Description |
|---|---|---|
| `clusterscope_httproute_accepted` | `cluster`, `namespace`, `route` | `1` if all parent Gateways accepted the route, else `0`. Removed while the cluster is unreachable. |

## RBAC

Read access on `gateway.networking.k8s.io` `gateways` and `httproutes` in every cluster (included in the KCL ClusterRole).
