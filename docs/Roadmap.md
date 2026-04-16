# AMGI Roadmap

Goals and non-goals for Amazing Marvin GitHub Integration. These define what we are building toward and what we are explicitly not targeting.

## Goals (targets to achieve)

- **Reliable sync from GitHub to Marvin** — Issues and pull requests that match the configured filters are created as Marvin tasks consistently, with no duplicate tasks for the same GitHub event.
- **Config-driven behaviour** — All rules (which events matter, where they land in Marvin, how tasks are titled and labelled) are defined in configuration, not code, so teams can adapt AMGI without forking.
- **Self-hosted and simple to run** — One deployable unit, minimal external dependencies, and clear documentation so a team can run AMGI where they already host their tools.
- **Support both real-time and periodic sync** — Teams can choose webhook-based delivery for low latency or polling where webhooks are not feasible (e.g. locked-down networks).
- **Transparent and auditable** — Logging and local state make it possible to see what was synced and why, and to debug missed or duplicate tasks.

## Non-goals (out of scope)

- **Full project-management replacement** — AMGI is a bridge between GitHub and Marvin, not a PM tool. We do not aim to replace Marvin, GitHub Projects, or similar.
- **GitHub-only or Marvin-only workflows** — The product assumes both GitHub and Marvin are in use. We do not target users who use only one of the two.
- **Zero-infrastructure deployment** — Running entirely inside GitHub Actions (or similar) with no long-lived process is out of scope. AMGI is a service that runs continuously.
- **Multi-tenant or hosted offering** — The design is for self-hosted, single-tenant use. A hosted SaaS version is not a current target.
- **Bidirectional sync** — We sync GitHub → Marvin only. Marvin task updates do not update GitHub issues or PRs.

## Future considerations

Items we may revisit later. Not committed; included for visibility.

- **Track older issues** — AMGI only processes items new or updated after it starts. A future feature could allow a one-time backfill of existing open issues/PRs (e.g. config option or CLI command) for teams that want historical items in Marvin.
- **Retention** — `github_artifacts` grows unbounded. A future feature could cap or prune processed rows (e.g. by age or count) to limit storage. Trade-off: deleting idempotency records risks duplicate Marvin tasks if the same issue is seen again.
- **Schema migrations** — The database file persists across deployments. When AMGI is updated (new image, new binary), the schema must be updated to match. Run migrations on startup: a `schema_version` table tracks the current version; migration scripts (e.g. `001_initial.sql`, `002_add_column.sql`) are embedded and run in order; only needed when the schema changes.
- **High availability (HA)** — Today AMGI runs as a single replica (SQLite single-writer). A future option could support multiple replicas for failover, e.g. by adding PostgreSQL as an alternative backend. Would require schema compatibility and shared state.
- **Multi-tenancy (per-owner webhook paths)** — Today we use a single webhook endpoint for all owners. A future option could allow different paths per owner (e.g. `/webhooks/acme`, `/webhooks/other-owner`) for isolation or routing. Most production environments use a single path and route by payload; this would add complexity and is deferred.
