# AMGI Roadmap

AMGI creates Amazing Marvin tasks from GitHub issues and pull requests. This document covers what v1.0 ships, what's on the trajectory beyond, and what's deliberately out of scope. For design details see [architecture.md](architecture.md); for configuration see [configuration.md](configuration.md).

## 1. Where we are today (v1.0)

The v1.0 release covers end-to-end sync from GitHub to Marvin with:

- **Webhook and polling modes** — configurable per GitHub owner.
- **Filter engine** — three-level hierarchy (repo → owner → global), Kubernetes-style operators (`in`, `notIn`, `exists`, `doesNotExist`).
- **Idempotency** — keyed on `{owner}/{repo}#{number}`; one Marvin task per GitHub item regardless of duplicate deliveries.
- **Rate-limit compliance** — backs off and retries on GitHub rate limits; enforces Marvin's 1/sec and 1440/day limits with a fixed-window daily counter.
- **Retry pipeline** — transient failures sweep on a configurable interval (`retry_interval_seconds`, default 5 minutes); permanent-error classification for 400/401/404.
- **Marvin name resolution** — config uses human-readable `list_name` and `label_names`; resolved to Marvin `_id` values at startup, with on-miss refresh when Marvin is updated mid-run.
- **Graceful shutdown** — context-aware workers exit cleanly on SIGTERM/SIGINT.
- **Single static binary** — distroless container image, no runtime dependencies.

See [../README.md](../README.md) for the complete feature list and quickstart.

## 2. Post-1.0 trajectory

Work scoped for after the v1.0 release, grouped by theme.

### 2.1 Validation completeness

- **Layer 3 semantic config validation** — cross-field checks such as "`marvin_config_id` must match an existing `marvin.configs[].id`" or "`polling_interval_seconds` required when `mode: polling`" currently surface at runtime. Goal: fail fast at startup, return structured errors that the CLI wrapper can present cleanly.
- **`amgi validate` CLI subcommand** — the validation pipeline exists as a library. Wrap it in a CLI for CI integration (pre-merge config linting) and local developer iteration.

### 2.2 Feature scope

- **Activity notifications (task-per-event)** — v1.0 creates one Marvin task per issue/PR; further activity (comments, reviews, review comments) does not create additional tasks even when user response is expected. A future feature would redefine the unit of work from "item" to "actionable event." Requires: event-scoped idempotency keys (e.g. `{owner}/{repo}/{type}#{number}/{action-specific-id}`), GitHub list methods for `/issues/comments`, `/pulls/reviews`, `/pulls/comments`, extensions to the internal event model, webhook support for `issue_comment` / `pull_request_review` / `pull_request_review_comment`, and additional template variables for comment bodies and reviewer logins.
- **Historical backfill** — AMGI only processes items new or updated after it starts. A future option would allow a one-time sync of existing open items (config flag or CLI command) for teams adding AMGI to a repo with existing state.
- **Bidirectional sync** — today AMGI creates Marvin tasks from GitHub activity only; Marvin task updates do not propagate back to GitHub. A future feature could sync task state (completed, rescheduled, re-labeled) back to GitHub issues and pull requests. Requires a Marvin webhook (if/when available) or polling Marvin for task-state changes, a conflict model for concurrent edits, and a reverse idempotency design. Gated on signal from users that one-way flow is insufficient.

### 2.3 Operational maturity

- **Database retention** — `github_artifacts` grows unbounded. A future option would cap or prune processed rows (by age or row count) to limit storage. Tradeoff: deleting idempotency records risks duplicate Marvin tasks if the same GitHub item is seen again after its row is pruned.
- **Schema migrations** — the SQLite file persists across deployments. Schema changes in a new AMGI release need migrations to run on startup. Planned shape: a `schema_version` table, embedded migration scripts (`001_initial.sql`, `002_add_column.sql`), in-order execution. Only needed once the schema actually changes.
- **High availability** — today AMGI runs as a single replica (SQLite single-writer). Multiple replicas for failover would require a PostgreSQL backend option and shared-state coordination, with schema compatibility across backends.
- **Multi-tenant webhook paths** — today AMGI uses a single webhook endpoint for all owners. A future option could allow per-owner paths (`/webhooks/acme`, `/webhooks/other-owner`) for isolation or routing. Most production deployments route by payload; this adds complexity and is deferred until a concrete need surfaces.
- **Health/readiness endpoint** — AMGI has no `/healthz` or `/readyz` HTTP endpoint. Deployment manifests (compose, Kubernetes) can wire TCP probes on the webhook port today, but that's wrong for polling-only deployments (webhook server disabled). A dedicated endpoint would be mode-agnostic and support Kubernetes-style readiness + liveness semantics.
- **Opinionated resource sizing** — deployment manifests ship as starter templates with no resource limits set. Publishing a sizing guide — benchmarked against realistic workloads (webhook QPS, polling interval × repo count) — is post-1.0 work. Until then, operators size empirically.
- **Reverse-proxy / TLS reference manifests** — webhook deployments need TLS termination and HTTP routing in front of the AMGI container. Setup is environment-specific (Traefik, Caddy, nginx, Cloudflare Tunnel, etc.) and v1.0 leaves this to the operator. Shipping reference configurations (a docker-compose override for Traefik, a Kubernetes Ingress + cert-manager example) is post-1.0.

## 3. Explicitly out of scope

Positions AMGI has committed to, so users and contributors can calibrate expectations:

- **Full project-management replacement** — AMGI bridges GitHub and Marvin. It does not aim to replace Marvin, GitHub Projects, or similar.
- **GitHub-only or Marvin-only workflows** — AMGI assumes both systems are in use.
- **Zero-infrastructure deployment** — running entirely inside GitHub Actions (or similar) with no long-lived process is out of scope. AMGI is a service that runs continuously.
- **Multi-tenant or hosted offering** — AMGI targets self-hosted, single-tenant use. A hosted SaaS version is not a target.

## 4. Known limitations (v1.0)

Current-release gaps, cross-referenced to the post-1.0 work that closes each:

- **No Layer 3 semantic config validation** — invalid cross-field references (e.g., a `marvin_config_id` that points at no existing config) surface at runtime rather than startup. See [section 2.1](#21-validation-completeness).
- **No `amgi validate` CLI subcommand** — the validation library exists; the CLI wrapper does not. See [section 2.1](#21-validation-completeness).
- **Database rows are kept indefinitely** — no automatic retention or pruning of `github_artifacts`. A busy deployment will grow unbounded until the retention feature lands. See [section 2.3](#23-operational-maturity).
- **No health/readiness HTTP endpoint** — TCP probes on the webhook port work for webhook-mode deployments only; polling-only deployments have no liveness signal beyond process existence. See [section 2.3](#23-operational-maturity).
- **No published sizing guide** — deployment manifests ship as starter templates without resource limits; operators size empirically for v1.0. See [section 2.3](#23-operational-maturity).
- **No shipped reverse-proxy / TLS reference manifests** — webhook deployments need an external reverse proxy or tunnel for TLS termination; configuration is left to the operator for v1.0. See [section 2.3](#23-operational-maturity).
