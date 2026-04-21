# AMGI Configuration Reference

This document is the user-facing reference for writing an AMGI config file.
It walks through every top-level section, every field, and the gotchas that
catch operators most often.

- For the system design (how AMGI processes an event internally), see [architecture.md](architecture.md).
- For the canonical schema definition, see [internal/schema/schema.json](../internal/schema/schema.json) — auto-generated from Go structs in [internal/config/config.go](../internal/config/config.go).
- For copy-paste starter configs, see [examples/](../examples/).

---

## Table of contents

1. [Top-level structure](#1-top-level-structure)
2. [`version`](#2-version)
3. [`filters`](#3-filters)
4. [`webhook_server`](#4-webhook_server)
5. [`github`](#5-github)
6. [`marvin`](#6-marvin)
7. [Environment variables](#7-environment-variables)
8. [Examples](#8-examples)
9. [Operational notes](#9-operational-notes)
10. [What's planned](#10-whats-planned)

---

## 1. Top-level structure

A minimal AMGI config has five possible top-level keys. Three are required; two are conditional:

```yaml
version: "1"           # REQUIRED — schema version
filters: {...}          # optional — global filter rules
webhook_server: {...}   # required when any owner uses mode: webhook
github: {...}           # REQUIRED — which owners/repos to watch
marvin: {...}           # REQUIRED — Marvin destinations + task templates
```

Config files are validated at startup. Invalid shapes (wrong types, missing required fields, extra unknown keys) are rejected loudly with a pointer to the offending location.

---

## 2. `version`

```yaml
version: "1"
```

Always the string `"1"` (quoted — YAML parsers would otherwise interpret `1` as an integer). This is the config schema version. Only `"1"` is accepted today; a future schema-breaking change will bump this number, so your config doesn't silently work under a new AMGI version.

---

## 3. `filters`

Filters decide which GitHub issues and pull requests become Marvin tasks. Events that don't match are discarded silently (logged at info level).

```yaml
filters:
  issues:          # filter rules for GitHub issues
    labels:
      in: [bug, enhancement]
    assignees:
      exists: true
  pull_requests:   # filter rules for pull requests
    branches:
      in: [main, "release/.*"]
    reviewers:
      exists: true
```

Filters are **optional** at every level. If omitted entirely, AMGI creates a task for every event (subject to per-owner action allowlists and idempotency).

### 3.1 Operators

Four operators, Kubernetes-inspired:

| Operator       | Semantics                                                   | Value type       |
| -------------- | ----------------------------------------------------------- | ---------------- |
| `in`           | At least one field value must match an entry in the list   | array of strings |
| `notIn`        | No field value may match any entry in the list             | array of strings |
| `exists`       | Field has at least one value (`true`) or none (`false`)    | boolean          |
| `doesNotExist` | Inverse of `exists`                                         | boolean          |

Within a field, operators can be combined:

```yaml
labels:
  in: [bug, enhancement]
  notIn: [wontfix]
  exists: true
```

All operators ANDed together — the above means "labels must include `bug` or `enhancement`, must NOT include `wontfix`, and must have at least one label."

Across fields (e.g. `labels` AND `assignees`), all conditions ANDed too. There is no `any`/`all` wrapper; all conditions must pass.

### 3.2 Multi-value vs single-value fields

The value type matters for the `in`/`notIn` operators:

- **Multi-value fields** (`labels`, `assignees`, `reviewers`) — compared by **exact string match**, case-sensitive. `in: [bug]` matches a label named exactly `"bug"`, not `"bugs"` or `"Bug"`.
- **Single-value fields** (`title`, `author`, `branch`) — entries in `in`/`notIn` are **Go regular expressions**. `title.in: ["^\\[WIP\\]"]` matches titles starting with `[WIP]`.

This split exists because labels/assignees are naturally enumerable sets, while titles and branch names benefit from pattern matching.

### 3.3 Hierarchical resolution

Filters can be specified at three levels. Resolution is **most-specific-wins, no merging**:

```
┌──────────────────────────────────┐
│  repo has filters?     → use repo's, STOP        │
│         │ no                                     │
│         ▼                                        │
│  owner has filters?    → use owner's, STOP       │
│         │ no                                     │
│         ▼                                        │
│  global has filters?   → use global's, STOP      │
│         │ no                                     │
│         ▼                                        │
│  no filters defined   → match everything         │
└──────────────────────────────────────────────────┘
```

**There is no merging between levels.** If a repo has `filters:` defined, the owner's and global filters are ignored entirely for that repo. This is deliberate — merging semantics are subtle (union? intersection? field-by-field?) and each choice surprises users differently.

### 3.4 Issue filter fields

| Field       | Type         | Notes                                   |
| ----------- | ------------ | --------------------------------------- |
| `labels`    | multi-value  | exact-match label names                |
| `assignees` | multi-value  | exact-match GitHub logins              |
| `author`    | single-value | regex against issue creator's login    |
| `title`     | single-value | regex against issue title              |

### 3.5 Pull request filter fields

| Field       | Type         | Notes                                         |
| ----------- | ------------ | --------------------------------------------- |
| `labels`    | multi-value  | exact-match label names                       |
| `branches`  | single-value | regex against target branch (e.g. `main`)     |
| `reviewers` | multi-value  | exact-match requested-reviewer GitHub logins  |
| `assignees` | multi-value  | exact-match GitHub logins                     |
| `author`    | single-value | regex against PR author's login               |
| `title`     | single-value | regex against PR title                        |

---

## 4. `webhook_server`

Configures AMGI's HTTP listener for GitHub webhooks. Required when any owner uses `mode: webhook`.

```yaml
webhook_server:
  port: 8080                   # optional — default 8080
  path: /webhooks/github       # optional — default /webhooks/github
```

| Field  | Type    | Default             | Notes                                                                        |
| ------ | ------- | ------------------- | ---------------------------------------------------------------------------- |
| `port` | int     | `8080`              | 1-65535. TCP port AMGI listens on.                                          |
| `path` | string  | `/webhooks/github`  | Must start with `/`. Must match GitHub's webhook "Payload URL" path exactly.|

For polling-only deployments, omit `webhook_server` entirely — AMGI won't start the HTTP server.

---

## 5. `github`

Lists the GitHub owners and repositories AMGI watches. Each owner picks its own mode (webhook or polling).

```yaml
github:
  owners:
    - name: acme-corp
      mode: webhook
      marvin_config_id: default
      actions:
        issues: [opened, assigned]
        pull_requests: [review_requested]
      repositories:
        - name: api
        - name: web
```

### 5.1 `owners`

Array of owner entries. Each entry has:

| Field                      | Required | Notes                                                                      |
| -------------------------- | -------- | -------------------------------------------------------------------------- |
| `name`                     | yes      | GitHub owner login (user or organization).                                |
| `mode`                     | yes      | `webhook` or `polling`.                                                   |
| `marvin_config_id`         | yes      | Default Marvin config for repos under this owner; see section 5.5.       |
| `repositories`             | yes      | Non-empty list of repos.                                                  |
| `actions`                  | no       | Which webhook event actions create tasks; webhook mode only. See 5.4.    |
| `polling_interval_seconds` | no       | Seconds between polls; polling mode only. Minimum `60`.                  |
| `filters`                  | no       | Owner-level filter override (replaces global for this owner).            |

### 5.2 `mode`

```yaml
mode: webhook   # or: polling
```

- **`webhook`** — GitHub POSTs events to AMGI's HTTP endpoint in real time. Lowest latency. Requires a publicly reachable URL (direct, or via a tunnel like cloudflared).
- **`polling`** — AMGI calls GitHub's REST API on a fixed interval. No inbound HTTP needed. Works behind firewalls.

Both modes can coexist for different owners in the same config.

### 5.3 `polling_interval_seconds`

```yaml
polling_interval_seconds: 300    # 5 minutes
```

Seconds between poll cycles for this owner. **Minimum is 60.** The floor guards against accidentally exceeding GitHub's 5,000 req/hour rate limit.

Sensible values:

- **60-120s** — active high-traffic repos, small number of repos
- **300s (5 min)** — default for most single-user setups
- **900-1800s (15-30 min)** — quiet personal repos

At `T` seconds per tick across `N` repos, you consume roughly `N × 7200 / T` GitHub API calls per hour (two calls per tick: issues + PRs). Stay well under 4,000 to leave headroom for retries and bursts.

Webhook mode ignores this field.

### 5.4 `actions` (webhook mode only)

Restricts which GitHub webhook action types create Marvin tasks.

```yaml
actions:
  issues: [opened, assigned]
  pull_requests: [review_requested, assigned]
```

**Supported action values:**

- `issues`: `opened`, `assigned`
- `pull_requests`: `review_requested`, `assigned`

**Defaults (when omitted):**

- `issues`: `[opened, assigned]`
- `pull_requests`: `[review_requested, assigned]`

Polling mode doesn't receive action events — it fetches current state. AMGI treats every polled item as equivalent to `opened`. The `actions` field is ignored under polling mode.

### 5.5 `marvin_config_id` resolution

Each event must route to exactly one Marvin config. AMGI looks up the config ID with this precedence:

```
repo-level marvin_config_id set?   → use that
         │ no
         ▼
owner-level marvin_config_id       → use that (always required per owner)
```

Example:

```yaml
github:
  owners:
    - name: acme
      marvin_config_id: team-issues      # default for all repos
      repositories:
        - name: backend                   # uses team-issues
        - name: design
          marvin_config_id: design-reviews # overrides to design-reviews for this repo
```

Every owner MUST set `marvin_config_id`. Repositories may override it.

### 5.6 Owner-level and repo-level `filters`

Filter resolution runs exactly the same as global filters (section 3.3) — most specific wins, no merging.

```yaml
github:
  owners:
    - name: acme
      filters:                            # owner-level — replaces global
        issues:
          labels:
            in: [bug]
      repositories:
        - name: backend                   # inherits owner's filters
        - name: design
          filters:                        # repo-level — replaces owner + global
            issues:
              labels:
                in: [bug, design-review]
```

### 5.7 `repositories`

```yaml
repositories:
  - name: backend
  - name: frontend
    marvin_config_id: frontend-reviews   # optional per-repo override
    actions:                              # optional per-repo override (webhook only)
      pull_requests: [review_requested]
    filters: {...}                        # optional per-repo filter override
```

The repo `name` is **without the owner prefix** — i.e., `backend`, not `acme-corp/backend`. The owner is taken from the enclosing owner entry.

---

## 6. `marvin`

Defines the Marvin destinations (categories, labels, templates). Each `config` entry is referenced by `marvin_config_id` elsewhere in the file.

```yaml
marvin:
  configs:
    - id: team-issues
      list_name: "Customer Issues"
      label_names: [github, needs-triage]
      task:
        title_template: "{{.Type}} #{{.Number}}: {{.Title}}"
        note_template: |
          **Author:** {{.Author}}
          **Link:** {{.URL}}

          {{.Body}}
```

### 6.1 `configs`

Array of Marvin config entries. Each must have a unique `id`.

### 6.2 `list_name`

The Marvin category or project title that owns the created task. **Case-insensitive exact match.** AMGI resolves this name to a Marvin `_id` at startup by calling `GET /api/categories` once.

```yaml
list_name: "Customer Issues"    # matches a Marvin category titled exactly "Customer Issues"
```

- **Omit** `list_name` to land the task in Marvin's Inbox (AMGI omits the `parentId` field in the `addTask` request → Marvin's default behavior).
- Resolution is **case-insensitive**, so `list_name: "customer issues"` also works. This is a convenience; Marvin prevents duplicate titles so there's no ambiguity.
- If the name doesn't resolve at startup, AMGI **refuses to start** with a clear error naming the missing category and listing available titles. This is fail-fast by design — a typo shouldn't silently produce ghost tasks.

Name resolution happens **at startup** (once) and on cache miss (if you add a new category in Marvin after AMGI started). See [section 9.1](#91-name-resolution-lifecycle) for the full cache story.

> **Why not IDs?** Marvin uses opaque strings for `_id` (mixed formats: UUID4, 32-char hex, short alphanumeric). Configs full of those strings are unreadable and non-portable across accounts. Name-based routing is AMGI's intended interface — there is no `list_id` or `label_ids` field.

### 6.3 `label_names`

Array of Marvin label titles to attach to every task created with this config. Same case-insensitive exact-match rules as `list_name`, same fail-fast validation at startup.

```yaml
label_names: [github, needs-triage]
```

- Empty or omitted → task has no labels attached.
- An empty string inside the array is silently skipped.
- If any name doesn't resolve, AMGI refuses to start.

### 6.4 `auto_complete`

```yaml
auto_complete: false    # optional — default true
```

Controls Marvin's title autocomplete. Marvin parses certain characters in the title (`#`, `@`, `+`) as operators for category, label, and scheduling shortcuts. When true (default), the operators are processed and stripped from the title.

**Set to `false` when your title template could contain those characters literally.** GitHub issues commonly contain `#42` (issue number references) or `@username` (user mentions). Without `auto_complete: false`, Marvin will interpret those as operators — e.g., `#42` becomes a lookup for a category named "42" (and then silently ignored if no such category exists).

General guidance:
- Rendering GitHub content verbatim in titles → `auto_complete: false`.
- Using template variables carefully to avoid `#`/`@`/`+` → default (true) is fine.

### 6.5 `task` templates

```yaml
task:
  title_template: "{{.Type}} #{{.Number}}: {{.Title}}"
  note_template: |
    **Author:** {{.Author}}
    **Link:** {{.URL}}

    {{.Body}}
```

- **`title_template`** (required) — Go `text/template` rendering the task title.
- **`note_template`** (required) — Go `text/template` rendering the task note (body).

Both render from the normalized event (section 6.6).

### 6.6 Template variables

The following variables are available in `title_template` and `note_template`:

| Variable     | Type       | Notes                                                                        |
| ------------ | ---------- | ---------------------------------------------------------------------------- |
| `.Owner`     | string     | Owner login (user or organization).                                          |
| `.Repo`      | string     | Repository name (no owner prefix).                                           |
| `.Number`    | int        | Issue or PR number.                                                          |
| `.Type`      | string     | `issue` or `pull_request`.                                                  |
| `.Title`     | string     | GitHub issue/PR title.                                                       |
| `.Body`      | string     | GitHub issue/PR body (may be empty).                                        |
| `.State`     | string     | `open` or `closed`.                                                         |
| `.Action`    | string     | Webhook action (`opened`, `assigned`, …). Polling treats all as `opened`.   |
| `.Labels`    | []string   | Slice of label names. See slice-rendering note below.                       |
| `.Assignees` | []string   | Slice of assignee logins. Same note.                                         |
| `.Author`    | string     | GitHub login of the creator.                                                 |
| `.Branch`    | string     | Target branch (PR only; empty for issues).                                  |
| `.Reviewers` | []string   | Requested-reviewer logins (PR only; empty for issues).                      |
| `.URL`       | string     | GitHub `html_url` for the issue or PR.                                      |

**Slice-rendering gotcha:** `.Labels`, `.Assignees`, and `.Reviewers` are slices. Writing `{{.Labels}}` bare produces Go's default `[a b c]` format — probably not what you want. Use a range loop:

```
**Labels:** {{range .Labels}}`{{.}}` {{end}}
```

### 6.7 Optional task fields

Any of these can go under `task:` to set Marvin-side task properties. All are optional.

| Field             | Type    | Notes                                                                |
| ----------------- | ------- | -------------------------------------------------------------------- |
| `day`             | string  | Schedule date, `YYYY-MM-DD`.                                        |
| `due_date`        | string  | Due date, `YYYY-MM-DD`.                                             |
| `start_date`      | string  | Start date, `YYYY-MM-DD`. Sent via title operator if `auto_complete`. |
| `end_date`        | string  | End date, `YYYY-MM-DD`. Sent via title operator if `auto_complete`.   |
| `planned_week`    | string  | Monday of planned week, `YYYY-MM-DD`.                               |
| `planned_month`   | string  | Planned month, `YYYY-MM`.                                           |
| `time_estimate_ms`| int     | Duration estimate in milliseconds (e.g., `1800000` for 30 min).    |
| `priority`        | int     | `0`=none, `1`=yellow, `2`=orange, `3`=red.                         |
| `frog`            | int     | `0`=none, `1`=normal, `2`=baby, `3`=monster (eatThatFrog strategy). |
| `is_reward`       | bool    | `true` for a reward-style task.                                     |
| `reward_points`   | float   | Reward points to attach.                                             |
| `section`         | string  | `Morning`/`Afternoon`/`Evening` or a custom section name.           |
| `review_date`     | string  | `YYYY-MM-DD` for Marvin review strategy.                            |

These map directly onto Marvin's `/api/addTask` body fields. See [Marvin's API docs](https://github.com/amazingmarvin/MarvinAPI/wiki/Marvin-API#create-a-task) for semantics.

---

## 7. Environment variables

Secrets are never read from the config file. Always pass via environment variables.

| Variable                  | Required when           | Purpose                                            |
| ------------------------- | ----------------------- | -------------------------------------------------- |
| `MARVIN_API_TOKEN`        | always                  | Create Marvin tasks; fetch categories/labels.      |
| `GITHUB_TOKEN`            | any owner is `polling`  | PAT for GitHub REST API (`repo` or `public_repo`). |
| `GITHUB_WEBHOOK_SECRET`   | any owner is `webhook`  | HMAC-SHA256 signature validation.                  |
| `CONFIG_PATH`             | optional                | Path to the config file. Default `/etc/amgi/config.yaml`. |
| `AMGI_DB_PATH`            | optional                | SQLite path. Default `/etc/amgi/amgi.db`.          |

AMGI fails fast at startup if a required secret is missing. No silent fallback to empty strings.

---

## 8. Examples

Three starter configs in [`examples/`](../examples/), each representing a distinct use case:

- **[`examples/project-manager.yaml`](../examples/project-manager.yaml)** — SaaS PM watching backend + frontend for customer-impact bugs + PR review requests. Webhook mode.
- **[`examples/software-engineer.yaml`](../examples/software-engineer.yaml)** — Individual contributor watching team + personal repos. Polling mode with two different intervals.
- **[`examples/homemaker.yaml`](../examples/homemaker.yaml)** — Non-dev use case: home-task tracking via a private GitHub repo. Demonstrates that AMGI works beyond code workflows.

Pick the closest match, adapt owner/repo names, and update the Marvin `list_name` and `label_names` to values that exist in your Marvin account.

---

## 9. Operational notes

### 9.1 Name resolution lifecycle

When AMGI starts, it makes two `GET` calls to Marvin — one to `/api/categories`, one to `/api/labels` — and caches the results in memory. The cache maps case-insensitive titles to Marvin `_id` values.

```
┌─────────────────────────────────────────────────────────┐
│  AMGI start                                              │
│       │                                                 │
│       ▼                                                 │
│  Initialize:                                            │
│    GET /api/categories + GET /api/labels                │
│    (2 API calls, ~3 sec spaced by the reads rate limit) │
│    Validate every list_name/label_names in config       │
│       │                                                 │
│       ▼                                                 │
│  Steady state (hours → weeks):                          │
│    AddTask resolves from cache → ZERO API reads         │
│       │                                                 │
│       ▼                                                 │
│  Cache miss (e.g., you added a new Marvin label today): │
│    Refresh the cache once, retry the lookup              │
│    (1 extra API read, ~200ms)                           │
└─────────────────────────────────────────────────────────┘
```

The cache is **not persisted to disk**. Every AMGI restart re-fetches. This is intentional — restart is the natural way to pick up large-scale changes in Marvin.

### 9.2 When to restart AMGI

Restart AMGI when:

1. **You delete a label or category in Marvin** that AMGI was using. The cache still has the stale `_id`; Marvin silently ignores it at `addTask` time (task lands in Inbox without the expected label). Cache-miss refresh doesn't help because the cached entry is a hit, not a miss. **This is the only gotcha where a restart is required for correctness.**
2. **Config changes.** AMGI reads the config file at startup only.
3. **Environment variable changes** (e.g., token rotation).

You do NOT need to restart when:
- You add a new Marvin label or category. AMGI will refresh the cache on first reference.
- You rename a Marvin label (as long as AMGI references the new name in config). First reference to the new name triggers a refresh.

### 9.3 Marvin reachability at startup

AMGI makes 2 Marvin API calls before accepting GitHub events. If Marvin is unreachable or returns errors at startup, AMGI refuses to start. In practice:

- **Marvin outage** → AMGI won't start. New deploys and restarts fail until Marvin is back.
- **Token rotation without config reload** → AMGI won't start with the old token; rotate, restart with new token.
- **Rate limit exceeded at startup** → very unlikely (only 2 reads); but if it happens, AMGI fails with the Marvin 429 error and exits.

This is a known tradeoff — the price for fail-fast validation of config references at startup.

### 9.4 Rate limits

AMGI respects Marvin's published rate limits:

| Limit                | Behavior                                                                      |
| -------------------- | ----------------------------------------------------------------------------- |
| 1 write per second   | `perSecond.Wait` before every `addTask`.                                      |
| 1440 writes per day  | Custom fixed-window counter; `DailyBudgetExceededError` pushes events to retry. |
| 1 read per 3 seconds | `readsLimiter.Wait` before every `/api/categories` and `/api/labels` fetch.  |

For GitHub, the polling client reacts to rate limit headers and backs off per `X-RateLimit-Reset`. See [architecture.md](architecture.md#rate-limits-and-error-handling) for the full model.

---

## 10. What's planned

Features in [`docs/Roadmap.md`](Roadmap.md) that affect configuration:

- **Layer 3 semantic validation** — cross-field reference checks (e.g., `marvin_config_id` must match an existing `marvin.configs[].id`), uniqueness constraints. Currently surfaces at runtime; will become startup checks.
- **`amgi validate` CLI** — standalone validation command for CI integration. Currently validation runs at AMGI startup only.
- **Database retention** — `github_artifacts` grows unbounded; a future config option will cap or prune old rows.

No config-file breaking changes are currently planned.
