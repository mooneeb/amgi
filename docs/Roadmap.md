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

- **Activity notifications (task-per-event, not task-per-item)** — v1 creates exactly one Marvin task per issue/PR (keyed by `{owner}/{repo}#{number}` — one task per item, regardless of further activity on that item). Further activity — new comments, PR reviews, review comments — does not create additional tasks, even though a user may need to respond to them. A future feature would redefine the unit of work from "item" to "actionable event," creating a Marvin task for each new comment, review, or review-comment. Requires: revising the idempotency model to be event-scoped (key shape like `{owner}/{repo}/{type}#{number}/{action-specific-id}`), adding GitHub list methods for `/issues/comments`, `/pulls/reviews`, `/pulls/comments`, extending the `event.Event` model for activity payloads, webhook support for `issue_comment`/`pull_request_review`/`pull_request_review_comment` events, and template variables for comment bodies and reviewer logins. Estimated ~2-3 days of focused work. Scoped out of v1 to keep the initial release focused.
- **Track older issues** — AMGI only processes items new or updated after it starts. A future feature could allow a one-time backfill of existing open issues/PRs (e.g. config option or CLI command) for teams that want historical items in Marvin.
- **Retention** — `github_artifacts` grows unbounded. A future feature could cap or prune processed rows (e.g. by age or count) to limit storage. Trade-off: deleting idempotency records risks duplicate Marvin tasks if the same issue is seen again.
- **Schema migrations** — The database file persists across deployments. When AMGI is updated (new image, new binary), the schema must be updated to match. Run migrations on startup: a `schema_version` table tracks the current version; migration scripts (e.g. `001_initial.sql`, `002_add_column.sql`) are embedded and run in order; only needed when the schema changes.
- **High availability (HA)** — Today AMGI runs as a single replica (SQLite single-writer). A future option could support multiple replicas for failover, e.g. by adding PostgreSQL as an alternative backend. Would require schema compatibility and shared state.
- **Multi-tenancy (per-owner webhook paths)** — Today we use a single webhook endpoint for all owners. A future option could allow different paths per owner (e.g. `/webhooks/acme`, `/webhooks/other-owner`) for isolation or routing. Most production environments use a single path and route by payload; this would add complexity and is deferred.
- **Configurable retry sweep interval** — The retry ticker runs every 60 seconds (hard-coded). A future option could expose this as a config field (e.g. `retry.interval_seconds`) for operators who want tighter or looser cadence. Not currently a bottleneck for any known workload.

## Known limitations

Behaviours that are documented here so external users know what to expect from the current release.

- **No Layer 3 semantic config validation.** The config parser enforces schema (Layer 1) and unmarshal (Layer 2), but not cross-field references like "`marvin_config_id` must match an entry in `marvin.configs[].id`" or "`polling_interval_seconds` required when `mode: polling`." Invalid references surface at runtime, not at startup.
- **`amgi validate` CLI subcommand is not yet implemented.** The validation pipeline exists as a library; a CLI wrapper is planned for CI integration.
- **Database rows are kept indefinitely.** There's no automatic retention or pruning of the `github_artifacts` table. A busy deployment will grow unbounded until a future retention feature lands.
