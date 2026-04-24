# AMGI — Kubernetes Deployment

Kubernetes is the right deployment target for multi-host clusters, GitOps workflows, and environments that already standardize on K8s primitives. This guide covers setup, verification, operations, and troubleshooting. For broader project context see the [README](../../README.md); for configuration details see [configuration.md](../../docs/configuration.md). If you want a simpler single-host deployment, see [deploy/docker/README.md](../docker/README.md) instead.

## Prerequisites

- **`kubectl` and cluster access** with permission to create ConfigMaps, Secrets, PersistentVolumeClaims, Deployments, Services, and (optionally) Ingresses in your target namespace. Verify with `kubectl auth can-i create deployments`.
- **Kustomize** — built into `kubectl` 1.14+ (`kubectl apply -k`). No separate install.
- **An Ingress controller running in the cluster** (webhook mode only) — nginx-ingress, Traefik, AWS ALB, cloud provider default, etc. If you don't know what's installed, run `kubectl get ingressclass` to see available classes.
- **An Amazing Marvin account** with API access and at least one category to land tasks in. The categories and labels referenced in your `config.yaml` must already exist in Marvin before AMGI starts.
- **A TLS secret for your Ingress hostname** (webhook mode only) — either pre-created (`kubectl create secret tls amgi-tls --cert=... --key=...`) or provisioned by cert-manager.

## Quickstart

Every step assumes your working directory is `deploy/kubernetes/`.

### 1. Get the manifests

```bash
git clone https://github.com/mooneeb/amgi.git
cd amgi/deploy/kubernetes
```

### 2. Customize `configmap.yaml`

Edit the `config.yaml` content inside the ConfigMap to match your GitHub owners, repos, and Marvin configs. See [configuration.md](../../docs/configuration.md) for the schema and [examples/](../../examples/) for persona-based starters.

### 3. Customize `secret.yaml`

Replace each `REPLACE_ME` with your real value:

- `MARVIN_API_TOKEN` — always required.
- `GITHUB_WEBHOOK_SECRET` — required only if your config has owners in webhook mode.
- `GITHUB_TOKEN` — required only if your config has owners in polling mode.

Unused tokens can be left as empty strings. See [.env.example](../docker/.env.example) for per-variable details (the conditional requirements are identical across deployment modes).

For production, consider managing the Secret externally (external-secrets-operator, HashiCorp Vault, sealed-secrets, etc.) rather than committing real values. The shipped `secret.yaml` is a starter; swap it for your secret-management tool of choice.

### 4. Customize `ingress.yaml` (webhook mode only)

Four things to replace before applying:

- **`ingressClassName: nginx`** → your actual controller (`traefik`, `alb`, cluster-default).
- **`rules[0].host: amgi.example.com`** → your real hostname. DNS must resolve to the ingress controller's load balancer.
- **`rules[0].http.paths[0].path: /webhooks/github`** → match whatever `webhook_server.path` you set in your `config.yaml` (defaults to `/webhooks/github`).
- **`tls[0].secretName: amgi-tls`** → a Secret of type `kubernetes.io/tls` in the same namespace, either manually created or issued by cert-manager.

**Polling-only deployments:** delete `ingress.yaml` and remove the `- ingress.yaml` line from `kustomization.yaml`. The cluster-internal ClusterIP Service is still needed but no external entry point is required.

### 5. Apply

```bash
kubectl apply -k .
```

One command applies everything in dependency order. Expected output:

```
configmap/amgi-config created
secret/amgi-secrets created
persistentvolumeclaim/amgi-data created
deployment.apps/amgi created
service/amgi created
ingress.networking.k8s.io/amgi created
```

If you're deploying to a specific namespace, add `-n <namespace>`:

```bash
kubectl apply -k . -n amgi
```

### 6. Watch the rollout

```bash
kubectl rollout status deployment/amgi
```

Returns when the pod is Ready (or times out with a clear error). Follow with:

```bash
kubectl logs -f deployment/amgi
```

You should see JSON log lines like:

```json
{"level":"INFO","msg":"config loaded successfully"}
{"level":"INFO","msg":"marvin client initialized (categories + labels cached; config references validated)"}
{"level":"INFO","msg":"starting webhook server","path":"/webhooks/github","port":8080}
```

Once `marvin client initialized` appears and whichever mode-specific readiness signal applies (`starting webhook server` for webhook mode, `polled successfully` for polling mode), AMGI is up.

## Design decisions

The shipped manifests reflect a few deliberate choices that aren't obvious from the YAML alone. Rationale for each:

### Deployment + PVC, not StatefulSet

AMGI uses a single replica and a persistent SQLite file. StatefulSet is designed for workloads needing ordered pod identity (`pod-0`, `pod-1`, ...) and stable per-pod DNS names — typical for replicated databases and distributed consensus. AMGI has neither concern. A Deployment with an attached PVC provides exactly what we need: one pod at a time, the PVC holding state across pod replacement.

### `strategy: Recreate`, not `RollingUpdate`

The default RollingUpdate strategy spins up a new pod before terminating the old one. With a ReadWriteOnce PVC, the new pod's attach request fails because the old pod still holds the volume, and the rollout hangs. `Recreate` terminates the old pod first, releasing the PVC, then spins up the replacement. Trade-off: a brief downtime window (typically under 10 seconds) during every deployment update. Acceptable for AMGI's workload; non-optional given the storage constraint.

### ClusterIP Service + Ingress, not NodePort or LoadBalancer

ClusterIP is the portable default — it works identically on every cluster flavor. External reachability is opinion territory: NodePort exposes on every node (often firewalled in production), LoadBalancer provisions cloud resources (cluster-specific, costs money), Ingress is the standard abstraction. Shipping ClusterIP + Ingress keeps the starter portable; operators choose their ingress class and controller via the Ingress object.

### `accessModes: ReadWriteOnce` on the PVC

SQLite is single-writer. ReadWriteOnce (one node at a time) matches the data model exactly. ReadWriteMany would allow multiple pods to mount the same volume — fine for read-only or shared-state workloads, wrong for a single-writer database. As a bonus, RWO is supported by virtually every StorageClass; RWX requires specific backends (NFS, CephFS, certain CSI drivers).

### No readiness or liveness probes

AMGI does not yet expose a `/healthz` or `/readyz` endpoint. A TCP probe on port 8080 would report "healthy" for webhook-mode pods but incorrectly fail polling-only deployments (the webhook server isn't bound when all owners are in polling mode). A dedicated health endpoint is planned — see [Roadmap](../../docs/Roadmap.md#23-operational-maturity) section 2.3. Until then, pod liveness falls back to process-crash triggering a restart, and readiness is implicit from startup completing.

### `resources.requests` only, no `resources.limits`

Requests tell the scheduler how to place the pod. Limits cap runtime usage — and set too tight, they cause OOM-kills that surface as "AMGI is randomly crashing." Setting correct limits requires benchmarking against your actual workload (webhook QPS, number of repos in polling mode, size of Marvin API responses). v1.0 ships requests only; operators tune limits based on measurement. See [Roadmap](../../docs/Roadmap.md#23-operational-maturity) section 2.3 for a planned sizing guide.

## Verifying the deployment

### Expected startup log sequence

A clean startup emits JSON logs in roughly this order:

```json
{"level":"INFO","msg":"config loaded successfully"}
{"level":"INFO","msg":"store created successfully"}
{"level":"INFO","msg":"marvin client created successfully"}
{"level":"INFO","msg":"marvin client initialized (categories + labels cached; config references validated)"}
{"level":"INFO","msg":"processor created successfully"}
{"level":"INFO","msg":"resolved retry sweep interval","interval":300000000000,"source":"default"}
{"level":"INFO","msg":"starting webhook server","path":"/webhooks/github","port":8080}
{"level":"INFO","msg":"resolved polling interval","owner":"...","interval":60000000000,"source":"config"}
```

Notable lines:

- **`marvin client initialized`** — AMGI authenticated to Marvin and resolved every name in your config to real Marvin IDs. Missing means `MARVIN_API_TOKEN` is wrong or a configured category/label doesn't exist in Marvin.
- **`starting webhook server`** — the HTTP server is listening on 8080. Missing if no owner has `mode: webhook`.
- **`polled successfully`** — polling is ticking for at least one owner.

### Checking Marvin

For each GitHub activity you expect to sync:

1. Trigger the activity (create an issue, open a PR).
2. Webhook mode: task appears within ~1-2 seconds. Polling mode: within one polling interval (default 60 seconds).
3. Verify category (per `list_name`) and labels (per `label_names`) match config.
4. Verify the title matches your `title_template`.

### Checking GitHub (webhook mode)

Repo → Settings → Webhooks → Recent Deliveries. Green ✓ = AMGI responded 2xx. Red ✗ = diagnosis needed (see Troubleshooting).

## Operations

### Viewing logs

```bash
kubectl logs deployment/amgi                  # current pod, all logs
kubectl logs -f deployment/amgi               # stream live
kubectl logs --tail 100 deployment/amgi       # last 100 lines
kubectl logs --since 10m deployment/amgi      # last 10 minutes
kubectl logs deployment/amgi --previous       # previous pod's logs (after restart)
```

Logs are JSON-formatted for log aggregators. Pipe through `jq` for readable output:

```bash
kubectl logs -f deployment/amgi | jq -r '. | "\(.time) [\(.level)] \(.msg)"'
```

### Restarting after config change

Editing the ConfigMap does NOT automatically restart the pod — AMGI reads config at startup only. After changing the ConfigMap (or the Secret), trigger a rollout:

```bash
kubectl apply -k .                         # re-apply changed manifests
kubectl rollout restart deployment/amgi    # force the pod to recreate
```

The new pod reads the updated ConfigMap and Secret on startup. SQLite state in the PVC survives.

### Updating to a new image version

The manifest defaults to `ghcr.io/mooneeb/amgi:latest`, which moves as new releases are tagged. To pull the latest:

```bash
kubectl rollout restart deployment/amgi
```

To pin a specific version:

```bash
kubectl set image deployment/amgi amgi=ghcr.io/mooneeb/amgi:v1.0.0
```

Or edit `deployment.yaml` to change the image tag and re-apply with `kubectl apply -k .`.

### Scaling considerations

**Do not scale `replicas` above 1.** AMGI's SQLite store is single-writer; a second pod would race the first for the PVC (which is ReadWriteOnce) or corrupt the database if storage somehow allowed multi-attach. Scaling AMGI horizontally requires a PostgreSQL backend option, which is Post-1.0 work — see [Roadmap](../../docs/Roadmap.md#23-operational-maturity) section 2.3.

### Tearing down

```bash
kubectl delete -k .           # removes Deployment, Service, Ingress, ConfigMap, Secret
```

**This preserves the PVC** — your SQLite data is safe. To also delete the PVC (and the underlying PersistentVolume, depending on reclaim policy):

```bash
kubectl delete pvc amgi-data
```

PVC deletion is destructive — idempotency records and polling cursors are gone. Next startup behaves like a fresh install (AMGI does not backfill historical GitHub items).

## Troubleshooting

### Pod stuck in Pending

```bash
kubectl describe pod -l app=amgi
```

The Events section at the bottom names the blocker. Common causes:

- **"0/N nodes are available: N Insufficient cpu/memory"** — no node can satisfy the resource requests. Reduce `resources.requests` in `deployment.yaml`, or add cluster capacity.
- **"persistentvolumeclaim amgi-data not found" or "not yet bound"** — the PVC itself is Pending. See "PVC stuck in Pending" below.
- **"0/N nodes are available: N node(s) had taints"** — add a toleration or remove the node taint.

### Pod in CrashLoopBackOff with config-validation errors

```bash
kubectl logs deployment/amgi --previous
```

If the log shows a JSON Schema error like `/github/owners/1/polling_interval_seconds/minimum:30 should be at least 60`, your ConfigMap's `config.yaml` is invalid per AMGI's schema. The path is a breadcrumb directly to the violating field. Edit `configmap.yaml`, re-apply, and restart the pod with `kubectl rollout restart deployment/amgi`.

### Pod in CrashLoopBackOff with "MARVIN_API_TOKEN is not set"

You left placeholder `REPLACE_ME` values in `secret.yaml` and applied without editing. Fix:

```bash
kubectl edit secret amgi-secrets           # replace each placeholder
kubectl rollout restart deployment/amgi
```

Same symptom and approach for missing `GITHUB_TOKEN` (polling mode) or `GITHUB_WEBHOOK_SECRET` (webhook mode).

### PVC stuck in Pending

```bash
kubectl describe pvc amgi-data
```

Likely causes:

- **"no persistent volumes available"** — your cluster has no default StorageClass, or the default doesn't support dynamic provisioning. Fix: set an explicit `storageClassName` in `pvc.yaml` to one that supports dynamic provisioning (`kubectl get storageclass` to list).
- **"failed to provision volume: access mode ReadWriteOnce not supported"** — rare; indicates a misconfigured StorageClass. Contact your cluster admin.

### Webhook deliveries fail before reaching AMGI

Symptom: GitHub's Recent Deliveries panel shows red ✗ but AMGI's logs show no webhook attempts — the request never made it. The Ingress is the likely culprit:

```bash
kubectl describe ingress amgi
kubectl get ingress amgi -o yaml
```

Check:

- **Address field populated?** A missing address means your Ingress controller hasn't processed the Ingress. Confirm the controller is running (`kubectl get pods -n <controller-namespace>`).
- **Hostname DNS resolves to the Ingress address?** `dig amgi.example.com` from outside the cluster should return the controller's IP.
- **TLS certificate valid for the hostname?** `openssl s_client -connect amgi.example.com:443 -servername amgi.example.com` shows the cert. Mismatched or expired certs make GitHub refuse the delivery.

### Deployment stuck in Progressing

Almost always: the strategy was changed from `Recreate` to `RollingUpdate` (e.g., by a kustomize overlay), and the new pod can't attach the RWO PVC while the old pod holds it. Revert:

```bash
kubectl rollout undo deployment/amgi       # revert to prior state
kubectl edit deployment amgi               # fix strategy.type back to Recreate
```

### Marvin's daily API budget exhausted

Symptom: AMGI logs include `{"level":"WARN","msg":"daily budget exceeded","resets_at":"..."}`.

Marvin allows 1440 addTask calls per UTC day; AMGI enforces this with a fixed-window counter. To avoid hitting the ceiling:

- Check polling intervals — aggressive polling across many repos multiplies API calls.
- Check filter scope — tight filters (label allowlists, action allowlists) cut traffic at the source.
- Review failed-task retries — persistent failures compound budget usage.
