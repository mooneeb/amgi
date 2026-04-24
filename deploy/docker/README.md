# AMGI — Docker Compose Deployment

Docker Compose is the simplest way to run AMGI on a single host — a home lab, a small VPS, or a developer laptop. This guide covers setup, verification, day-to-day operations, and common troubleshooting. For broader project context see the [README](../../README.md); for configuration details see [configuration.md](../../docs/configuration.md).

## Prerequisites

- **Docker Engine 20.10+ with Compose v2** — use the modern `docker compose` subcommand, not the legacy `docker-compose` binary (EOL since 2023). Verify with `docker compose version`.
- **An Amazing Marvin account** with API access and at least one category to land tasks in. AMGI resolves Marvin names (category names, label names) to internal IDs at startup — the categories and labels referenced in your `config.yaml` must already exist in Marvin.
- **GitHub access**, depending on your deployment mode:
  - **Webhook mode**: a publicly-reachable URL with TLS where GitHub can POST events. Setup is environment-specific (reverse proxy, tunnel, etc.) and out of scope for this doc.
  - **Polling mode**: a GitHub Personal Access Token with `repo` scope (private repos) or `public_repo` scope (public only).

## Quickstart

A 10-minute happy-path setup. Every step assumes your working directory is the one containing `docker-compose.yaml`.

### 1. Get the manifests

```bash
# Clone the repository (simplest)
git clone https://github.com/mooneeb/amgi.git
cd amgi/deploy/docker

# Alternative — download just the two deployment files
mkdir amgi && cd amgi
curl -O https://raw.githubusercontent.com/mooneeb/amgi/main/deploy/docker/docker-compose.yaml
curl -O https://raw.githubusercontent.com/mooneeb/amgi/main/deploy/docker/.env.example
```

### 2. Create `config.yaml`

AMGI needs a config file describing which GitHub owners/repos to sync and which Marvin configs to use. Copy one of the starters from [`examples/`](../../examples/) and adapt it:

```bash
cp ../../examples/minimal.yaml config.yaml
# Open config.yaml in your editor and adapt it to your GitHub
# owners/repos and Marvin configs.
```

**This file must exist before you launch.** The compose file bind-mounts `./config.yaml` into the container; if the file is missing when `docker compose up` runs, Docker creates an empty *directory* at that path and AMGI fails on read.

See [`docs/configuration.md`](../../docs/configuration.md) for the full schema.

### 3. Create `.env`

```bash
cp .env.example .env
# Open .env in your editor and fill in the values your mode requires.
```

The comments in `.env.example` document each variable, where to obtain it, and which mode needs it.

### 4. Launch

```bash
docker compose up -d
```

The `-d` flag detaches — the container runs in the background.

### 5. Watch for readiness

```bash
docker compose logs -f amgi
```

You should see JSON log lines like:

```json
{"level":"INFO","msg":"config loaded successfully"}
{"level":"INFO","msg":"marvin client initialized (categories + labels cached; config references validated)"}
{"level":"INFO","msg":"starting webhook server","path":"/webhooks/github","port":8080}
{"level":"INFO","msg":"polled successfully","owner":"...","repo":"...","issue count":0,"pull request count":0}
```

Once you see `marvin client initialized` and whichever mode-specific readiness signal applies (`starting webhook server` for webhook mode, `polled successfully` for polling mode), AMGI is up. Exit the tail with Ctrl+C — the container keeps running.

## Configuration

### `config.yaml`

AMGI's behavior is driven entirely by `config.yaml`. It defines which GitHub owners/repos to sync, which Marvin config (list + labels + task template) each owner maps to, and per-deployment options like polling intervals.

- **Full schema reference:** [`docs/configuration.md`](../../docs/configuration.md).
- **Starter configs:** [`examples/`](../../examples/) has persona-based starters.
- **Pick-up semantics:** AMGI reads `config.yaml` once at startup. Edit the file, then `docker compose restart amgi` to apply changes.

### Environment variables

Secrets and path overrides are passed to the container via environment variables, loaded from the `.env` file in the working directory. Compose auto-loads `.env` before the container starts and interpolates `${VAR}` references in the compose file.

See [`.env.example`](.env.example) for each variable's purpose, format, and source.

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

- **`marvin client initialized`** — AMGI has successfully authenticated to Marvin and resolved every `list_name` / `label_names` in your config to real Marvin IDs. If this line is missing, check `MARVIN_API_TOKEN` and confirm every name in config.yaml exists as a category/label in Marvin.
- **`starting webhook server`** — the HTTP server is bound and listening on port 8080. Missing means either no owner has `mode: webhook` configured or the server failed to bind (port collision?).
- **`polled successfully`** — polling is active for at least one owner.

### Checking Marvin

For each real GitHub activity you expect to sync:

1. Trigger the activity (create an issue, open a PR).
2. In webhook mode a task appears in Marvin within ~1-2 seconds. In polling mode, within one polling interval (default 60 seconds).
3. Verify the task lands in the correct Marvin category (per your config's `list_name`), with the correct labels attached.
4. Verify the task's title matches your `title_template` from config.yaml.

### Checking GitHub (webhook mode)

GitHub exposes a "Recent Deliveries" panel per webhook:

1. Navigate: Repo → Settings → Webhooks → (your AMGI webhook) → Recent Deliveries.
2. Each delivery shows a green ✓ (AMGI responded 2xx) or red ✗ (AMGI responded 4xx/5xx or the request timed out).
3. Click any delivery to inspect the full request body, the response body, and the response latency.

Green checks with 2xx responses confirm AMGI received and processed the webhook. Red X'es need diagnosis — see Troubleshooting.

## Operations

### Viewing logs

```bash
docker compose logs amgi              # all logs since the container started
docker compose logs -f amgi           # stream new logs as they arrive
docker compose logs --tail 100 amgi   # last 100 lines
docker compose logs --since 10m amgi  # last 10 minutes
```

Logs are JSON-formatted for compatibility with log aggregators (Loki, CloudWatch, Datadog). Pipe through `jq` for readable output:

```bash
docker compose logs -f amgi | jq -r '. | "\(.time) [\(.level)] \(.msg)"'
```

### Restarting the service

After editing `config.yaml`:

```bash
docker compose restart amgi
```

AMGI reads config at startup only — a restart is how it picks up changes. The SQLite state (idempotency records, polling cursors) is preserved in the named volume across restarts.

### Updating to a new image version

```bash
docker compose pull amgi   # download the newest image per the tag in compose
docker compose up -d       # recreate the container with the new image
```

The default image reference is `ghcr.io/mooneeb/amgi:latest`, which floats to the newest release. To pin a specific version:

```yaml
services:
  amgi:
    image: ghcr.io/mooneeb/amgi:v1.0.0   # pinned
```

### Stopping and removing

```bash
docker compose stop amgi   # stop the container, keep state
docker compose down        # stop + remove container and network
docker compose down -v     # also remove the named volume (ERASES SQLite state)
```

The `-v` flag is destructive — it deletes the idempotency database and polling cursors. Next startup behaves like a fresh install (AMGI does not backfill historical GitHub items — see [configuration.md](../../docs/configuration.md) for cursor semantics).

## Troubleshooting

### Container exits immediately with a config-validation error

Symptom: `docker compose ps` shows the amgi container as `Exited (1)`; logs contain an error like:

```
"failed to parse and validate config: schema validation failed\nmap[...polling_interval_seconds/minimum:30 should be at least 60 ...]"
```

The error path (`/github/owners/1/polling_interval_seconds/minimum`) is a JSON Schema breadcrumb pointing at the exact field that violates the schema — in this example, `github.owners[1].polling_interval_seconds` is below the 60-second minimum.

Fix the config and relaunch with `docker compose up -d`.

### `MARVIN_API_TOKEN is not set` on startup

Exit with a log line like:

```
"MARVIN_API_TOKEN is not set"
```

Causes, in descending likelihood:

1. `.env` doesn't exist, or isn't in the same directory as `docker-compose.yaml`. Compose only auto-loads `.env` from its working directory.
2. `.env` exists but `MARVIN_API_TOKEN=...` is missing or blank.
3. The `.env` line has trailing whitespace or quoting that breaks the value.

Diagnose with:

```bash
docker compose config   # shows the fully-resolved compose config, including env
```

The variable value (or its absence) appears in the output. Same approach for `GITHUB_WEBHOOK_SECRET` and `GITHUB_TOKEN`.

### Every webhook delivery shows as rejected (signature fail)

Symptom: GitHub's Recent Deliveries panel shows red ✗ for every delivery; AMGI logs show:

```json
{"level":"WARN","msg":"webhook signature mismatch"}
```

Cause: the `GITHUB_WEBHOOK_SECRET` in `.env` does not match the Secret field in GitHub's webhook config. HMAC-SHA256 signatures only match when both sides hold the exact same secret.

Fix:

1. Regenerate the secret: `openssl rand -hex 32`.
2. Update both `.env` (your side) and the GitHub webhook config (Repo → Settings → Webhooks → Edit → Secret).
3. `docker compose restart amgi` to reload `.env`.

### Tasks landing in the wrong Marvin category

AMGI resolves each GitHub event to a Marvin config via the `(owner, repo)` tuple — not owner name alone. Each `(owner, repo)` pair must map to exactly one Marvin config in `config.yaml`.

If you have multiple owner stanzas sharing the same owner name with different repos, AMGI's semantic validator enforces at startup that their repo sets are disjoint. If validation accepted your config but routing looks wrong, the stanza structure is likely ambiguous in practice — review how owners and their repositories are grouped.

### GitHub polling returns zero items despite new activity

Likely causes:

1. **Cursor semantics** — AMGI polls with `since=<last-cursor>`. On first run it uses `since=now`, so items created before AMGI started are invisible (no historical backfill by design). Subsequent polls use the last poll's completion time.
2. **Polling interval** — default is 60s. Activity created right after a poll completes waits for the next tick.
3. **Wrong PAT scope** — syncing private repos with a `public_repo`-only token returns empty results for private repos silently. Check token scopes.

### Marvin's daily API budget exhausted

Symptom:

```json
{"level":"WARN","msg":"daily budget exceeded","resets_at":"..."}
```

Marvin has a documented 1440/day `addTask` cap. AMGI enforces it with a fixed-window counter that resets at UTC midnight.

To avoid hitting the ceiling:

- Check polling intervals — aggressive polling across many repos multiplies API calls.
- Check filter scope — if every event creates a task, bursts of activity drain the budget. Consider action allowlists or label-based filters in `config.yaml`.
- Review failed-task retries — transient failures retry on a configurable interval; persistent failures can compound budget usage.
