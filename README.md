# AMGI — Amazing Marvin ↔ GitHub Integration

[![CI](https://github.com/mooneeb/amgi/actions/workflows/ci.yml/badge.svg)](https://github.com/mooneeb/amgi/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

**One-way sync of GitHub issues and pull requests into [Amazing Marvin](https://amazingmarvin.com/) tasks.** Self-hosted, config-driven, container-ready.

> **Status:** Pre-1.0 (alpha). Feature-complete for the walking-skeleton scope; hardening is ongoing. See [Roadmap](docs/Roadmap.md).

---

## What it does

```
┌──────────┐    webhook or poll     ┌──────┐   filter    ┌──────────┐   addTask   ┌────────┐
│  GitHub  │ ─────────────────────► │ AMGI │ ──────────► │ dedupe/  │ ──────────► │ Marvin │
│          │                        │      │             │ retry    │             │        │
└──────────┘                        └──────┘             └──────────┘             └────────┘
```

You describe *which* issues and PRs you care about (labels, assignees, branches, authors) in a YAML config. AMGI watches GitHub, filters matching events, and creates a Marvin task for each new item. Idempotency keeps it from duplicating; a retry queue handles transient failures.

AMGI is **one-way** (GitHub → Marvin). Marvin is the source of truth for your task state; GitHub is the source of truth for the work.

## Features

- **Webhook and polling modes**, configurable per GitHub owner. Use webhooks for real time; polling when you can't expose a public URL.
- **Three-level filter hierarchy** (repo → owner → global) with Kubernetes-style operators: `in`, `notIn`, `exists`, `doesNotExist`.
- **Idempotency by `{owner}/{repo}#{number}`** — one task per issue/PR regardless of duplicate deliveries.
- **Rate-limit-aware**: respects GitHub's `X-RateLimit-Reset`; enforces Marvin's 1/sec + 1440/day caps with a fixed-window daily counter.
- **Retry pipeline** for transient Marvin failures, with permanent-error classification (400/401/404 → give up; 429/5xx → retry up to 3 times).
- **Graceful shutdown** on SIGINT/SIGTERM — drains in-flight work, persists state, exits clean.
- **Single static binary** (17 MB distroless Docker image). No runtime dependencies.

See [`docs/architecture.md`](docs/architecture.md) for the full design.

## Quickstart

### Prerequisites

- Go 1.26+ if building from source, or Docker if using the container image.
- A GitHub personal access token ([classic](https://github.com/settings/tokens) with `repo` or `public_repo`, or a fine-grained PAT with Issues + Pull requests read).
- A Marvin API token — [how to get one](https://app.amazingmarvin.com/pre?api).

### Config

Copy [`examples/config.yaml`](examples/config.yaml) and adjust the owner, repo, and Marvin template to match your setup:

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
export GITHUB_WEBHOOK_SECRET="dev-secret"    # required even in polling-only mode currently
export CONFIG_PATH="$(pwd)/config.yaml"
export AMGI_DB_PATH="/var/lib/amgi/amgi.db"

./amgi
```

### Run with Docker

```bash
docker build -t amgi .

docker run --rm \
  -e GITHUB_TOKEN -e MARVIN_API_TOKEN -e GITHUB_WEBHOOK_SECRET \
  -v $(pwd)/config.yaml:/etc/amgi/config.yaml:ro \
  -v amgi-data:/var/lib/amgi \
  -e AMGI_DB_PATH=/var/lib/amgi/amgi.db \
  -p 8080:8080 \
  amgi
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

- Full schema in [`internal/config/config.go`](internal/config/config.go) (Go structs with `jsonschema` tags).
- JSON Schema at [`internal/schema/schema.json`](internal/schema/schema.json) (auto-generated).
- Template variables and filter operators: [`docs/architecture.md`](docs/architecture.md#configuration).
- Commented example: [`examples/config.yaml`](examples/config.yaml).

## Environment variables

| Variable                  | Required when        | Purpose                                                |
|---------------------------|----------------------|--------------------------------------------------------|
| `GITHUB_TOKEN`            | any owner is polling | PAT for GitHub REST API                                |
| `GITHUB_WEBHOOK_SECRET`   | always (currently)   | Verify webhook signatures (HMAC-SHA256)                |
| `MARVIN_API_TOKEN`        | always               | Create Marvin tasks                                    |
| `CONFIG_PATH`             | optional             | Path to YAML config. Default: `/etc/amgi/config.yaml`  |
| `AMGI_DB_PATH`            | optional             | SQLite path. Default: `/etc/amgi/amgi.db`              |

## Deployment notes

- **Single-writer model.** AMGI assumes one process per SQLite file. Do not run multiple replicas against the same DB path.
- **Mount the DB on a persistent volume.** The idempotency store survives restarts; losing it means you may re-create already-created tasks.
- **Rate-limit budget.** Marvin allows 1440 task creations per UTC day. If you watch very high-traffic repos, factor that in.
- **Logs are JSON via `log/slog`.** Feed them into any structured log aggregator.

## Contributing

Contributions are welcome — bug fixes, new filter operators, documentation, test coverage. See [CONTRIBUTING.md](CONTRIBUTING.md) for dev setup, commit conventions, and the PR process.

## Security

Report vulnerabilities privately. See [SECURITY.md](SECURITY.md).

## Code of Conduct

This project follows the [Contributor Covenant](CODE_OF_CONDUCT.md).

## License

[MIT](LICENSE). Copyright © 2025-2026 Muhammad Mooneeb Hussain.
