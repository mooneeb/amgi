# AMGI — Amazing Marvin + GitHub Integration

[![CI](https://github.com/mooneeb/amgi/actions/workflows/ci.yml/badge.svg)](https://github.com/mooneeb/amgi/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

**Create [Amazing Marvin](https://amazingmarvin.com/) tasks from GitHub issues and pull requests.** Self-hosted, config-driven, container-ready.

> **Status:** Pre-1.0 (alpha). Core features are in place; see [Roadmap](docs/Roadmap.md) for planned additions.

---

## What it does

```
┌──────────┐    webhook or poll     ┌──────┐   filter    ┌──────────┐   addTask   ┌────────┐
│  GitHub  │ ─────────────────────► │ AMGI │ ──────────► │ dedupe/  │ ──────────► │ Marvin │
│          │                        │      │             │ retry    │             │        │
└──────────┘                        └──────┘             └──────────┘             └────────┘
```

You describe *which* issues and PRs you care about (labels, assignees, branches, authors) in a YAML config. AMGI watches GitHub, filters matching events, and creates a Marvin task for each new item. Idempotency keeps it from duplicating; a retry queue handles transient failures.

## Features

- **Webhook and polling modes**, configurable per GitHub owner. Use webhooks for real time; polling when you can't expose a public URL.
- **Three-level filter hierarchy** (repo → owner → global) with Kubernetes-style operators: `in`, `notIn`, `exists`, `doesNotExist`.
- **Idempotency by `{owner}/{repo}#{number}`** — one task per issue/PR regardless of duplicate deliveries.
- **Rate-limit-aware**: backs off and retries on GitHub rate limits; enforces Marvin's 1/sec + 1440/day caps with a fixed-window daily counter.
- **Retry pipeline** for transient Marvin failures, with permanent-error classification (400/401/404 → give up; 429/5xx → retry up to 3 times).
- **Graceful shutdown** on SIGINT/SIGTERM — stops accepting new work, waits for in-flight operations to complete (10s grace for HTTP), exits clean.
- **Single static binary** (17 MB distroless Docker image). No runtime dependencies.

See [`docs/architecture.md`](docs/architecture.md) for the full design.

## Quickstart

### Prerequisites

- Go 1.26+ if building from source, or Docker if using the container image.
- A GitHub personal access token ([classic](https://github.com/settings/tokens) with `repo` or `public_repo`, or a fine-grained PAT with Issues + Pull requests read).
- A Marvin API token — [how to get one](https://app.amazingmarvin.com/pre?api).

### Config

Pick an [example](examples/) that matches your use case (project-manager, software-engineer, or homemaker) and adjust the owner/repo names and Marvin `list_name`/`label_names` to match your setup:

```yaml
version: "1"
github:
  owners:
    - name: your-github-username
      mode: polling
      marvin_config_id: default
      polling_interval_seconds: 300
      repositories:
        - name: your-repo
marvin:
  configs:
    - id: default
      task:
        title_template: "{{.Type}} #{{.Number}}: {{.Title}}"
        note_template: "{{.URL}}\n\n{{.Body}}"
```

### Run from source

```bash
go build -o amgi ./cmd/amgi

export GITHUB_TOKEN="ghp_..."
export MARVIN_API_TOKEN="..."
export GITHUB_WEBHOOK_SECRET="dev-secret"    # required for webhook mode
export CONFIG_PATH="$(pwd)/config.yaml"
export AMGI_DB_PATH="/var/lib/amgi/amgi.db"

./amgi
```

### Run with Docker

For single-host self-hosting, use Docker Compose — see [`docs/deploy-docker.md`](docs/deploy-docker.md) for the full setup runbook.

For a one-liner alternative (no compose file):

```bash
docker run --rm \
  -e GITHUB_TOKEN -e MARVIN_API_TOKEN -e GITHUB_WEBHOOK_SECRET \
  -v $(pwd)/config.yaml:/etc/amgi/config.yaml:ro \
  -v amgi-data:/var/lib/amgi \
  -e AMGI_DB_PATH=/var/lib/amgi/amgi.db \
  -p 8080:8080 \
  ghcr.io/mooneeb/amgi:latest
```

The webhook server listens on port 8080 by default; the `-p` flag maps it to your host. For polling-only deployments you can omit `-p`.

## Webhook setup (if using `mode: webhook`)

Configure a webhook in your GitHub repo or organization:

- **Payload URL:** `https://your-host:8080/webhooks/github` (or whatever `webhook_server.path` you set).
- **Content type:** `application/json`.
- **Secret:** any random string — put the same value in the `GITHUB_WEBHOOK_SECRET` env var.
- **SSL verification:** enable.
- **Events:** "Issues" and "Pull requests" (at minimum).

AMGI validates webhook signatures with HMAC-SHA256. Invalid signatures are rejected with HTTP 401.

## Configuration reference

- **User-facing reference:** [`docs/configuration.md`](docs/configuration.md) — every field, operator semantics, template variables, worked examples.
- **Canonical schema:** [`internal/schema/schema.json`](internal/schema/schema.json) (auto-generated from [Go structs](internal/config/config.go)).
- **Starter configs:** [`examples/project-manager.yaml`](examples/project-manager.yaml), [`examples/software-engineer.yaml`](examples/software-engineer.yaml), [`examples/homemaker.yaml`](examples/homemaker.yaml).

## Environment variables

| Variable                  | Required when        | Purpose                                                |
|---------------------------|----------------------|--------------------------------------------------------|
| `GITHUB_TOKEN`            | any owner is polling | PAT for GitHub REST API                                |
| `GITHUB_WEBHOOK_SECRET`   | any owner is webhook | Verify webhook signatures (HMAC-SHA256)                |
| `MARVIN_API_TOKEN`        | always               | Create Marvin tasks                                    |
| `CONFIG_PATH`             | optional             | Path to YAML config. Default: `/etc/amgi/config.yaml`  |
| `AMGI_DB_PATH`            | optional             | SQLite path. Default: `/var/lib/amgi/amgi.db`          |

## Deployment notes

- **Single-writer model.** AMGI assumes one process per SQLite file. Do not run multiple replicas against the same DB path.
- **Mount the DB on a persistent volume.** The idempotency store survives restarts; losing it means you may re-create already-created tasks.
- **Rate-limit budget.** Marvin allows 1440 task creations per UTC day. If you watch very high-traffic repos, factor that in.
- **Logs are JSON via `log/slog`.** Feed them into any structured log aggregator.

## Project principles

AMGI's design is guided by five principles:

- **Reliable sync** — Matching GitHub events become Marvin tasks consistently; the idempotency store ensures exactly one task per item, even across restarts and duplicate deliveries.
- **Config-driven** — All behaviour (which events matter, where they land, how tasks are titled) lives in YAML, not code. Teams adapt AMGI without forking.
- **Self-hosted and simple** — One binary, one container image, minimal external dependencies. Drop it into whatever infrastructure you already run.
- **Real-time or periodic** — Webhook mode for low latency, polling mode when webhooks aren't feasible (firewalls, no public URL). Per-owner choice.
- **Transparent and auditable** — Structured logs (JSON via `log/slog`), local state in SQLite. What synced and why is always inspectable.

## Contributing

Contributions are welcome — bug fixes, new filter operators, documentation, test coverage. See [CONTRIBUTING.md](CONTRIBUTING.md) for dev setup, commit conventions, and the PR process.

## Security

Report vulnerabilities privately. See [SECURITY.md](SECURITY.md).

## Code of Conduct

This project follows the [Contributor Covenant](CODE_OF_CONDUCT.md).

## License

[MIT](LICENSE). Copyright © 2025-2026 Muhammad Mooneeb Hussain.
