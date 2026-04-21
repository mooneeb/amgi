# Contributing to AMGI

Thanks for your interest. AMGI creates Amazing Marvin tasks from GitHub issues and pull requests, with config-driven filtering and idempotency. Contributions are welcome across the board: bug fixes, new filter operators, documentation improvements, test coverage, and thoughtful feature additions that fit the design.

This document covers how to get set up, how work is reviewed, and where to ask questions.

## Scope

AMGI's scope is deliberately narrow:

- **In scope:** creating Marvin tasks from GitHub issues and pull requests, config-driven filtering, idempotency and retry, webhook and polling modes, operator ergonomics.
- **Out of scope (for now):** non-GitHub sources, non-Marvin destinations, multi-tenancy. See [`docs/Roadmap.md`](docs/Roadmap.md) for the boundary between "planned later" and "not planned."

If you have an idea that stretches scope, open an issue to discuss it before sending a PR.

## Design context

Before proposing significant changes, read [`docs/architecture.md`](docs/architecture.md). It covers the pipeline, data flow, rate-limit strategy, idempotency model, and operational considerations. Most design questions have answers there.

## Development setup

You need Go 1.26 or newer. Clone and verify the build:

```bash
git clone https://github.com/mooneeb/amgi.git
cd amgi
go build ./...
go test -race ./...
```

### Prerequisites for running against real APIs

- **A Marvin API token** — [how to get one](https://app.amazingmarvin.com/pre?api). Required in every mode.
- **A config file.** Start from [`examples/config.yaml`](examples/config.yaml) and adapt it to your GitHub username/repo. The canonical schema is defined by the Go structs in [`internal/config/config.go`](internal/config/config.go); a JSON Schema generated from those structs lives at [`internal/schema/schema.json`](internal/schema/schema.json) and works with any YAML editor that supports JSON Schema for autocomplete.
- **Credentials specific to your chosen mode** — see the two sub-sections below.

### Common environment variables

Both modes share these:

```bash
export MARVIN_API_TOKEN="..."                # Marvin API token
export CONFIG_PATH="$(pwd)/your-config.yaml" # path to your config
export AMGI_DB_PATH="/tmp/amgi-dev.db"       # SQLite database path
```

### Running in polling mode

Polling calls GitHub's REST API periodically. No inbound HTTP needed; easiest for local dev.

1. **Create a GitHub personal access token** — classic PAT with `repo` or `public_repo` scope, or a fine-grained PAT with Issues + Pull requests read.
2. **Set the mode in your config:**
   ```yaml
   github:
     owners:
       - name: your-github-username
         mode: polling
         polling_interval_seconds: 60   # fast for dev; 300+ for production
         ...
   ```
3. **Export and run:**
   ```bash
   export GITHUB_TOKEN="ghp_..."
   go run ./cmd/amgi
   ```

### Running in webhook mode

Webhook mode receives real-time events from GitHub. Requires a publicly reachable HTTPS URL for your dev machine.

1. **Expose your dev port to the internet.** Pick one:
   - [cloudflared](https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/install-and-setup/tunnel-guide/local/) (recommended): `cloudflared tunnel --url http://localhost:8080`
   - [ngrok](https://ngrok.com/): `ngrok http 8080`
   - [smee.io](https://smee.io/) + `smee -u https://smee.io/xxx -P /webhooks/github -t http://localhost:8080/webhooks/github`
2. **Create a webhook on your GitHub repo** (Settings → Webhooks → Add webhook):
   - **Payload URL:** your public tunnel URL + `/webhooks/github`
   - **Content type:** `application/json`
   - **Secret:** any random string; remember it for step 4.
   - **Events:** Issues + Pull requests.
3. **Set the mode in your config:**
   ```yaml
   github:
     owners:
       - name: your-github-username
         mode: webhook
         ...
   webhook_server:
     port: 8080
     path: /webhooks/github
   ```
4. **Export and run:**
   ```bash
   export GITHUB_WEBHOOK_SECRET="the-secret-you-chose-in-step-2"
   go run ./cmd/amgi
   ```

### Container build and run

Once you've confirmed local setup works, verify the Docker image:

```bash
docker build -t amgi:dev .
docker run --rm \
  -e MARVIN_API_TOKEN \
  -e GITHUB_TOKEN \
  -e GITHUB_WEBHOOK_SECRET \
  -v $(pwd)/your-config.yaml:/etc/amgi/config.yaml:ro \
  -p 8080:8080 \
  amgi:dev
```

Pass only the env vars your chosen mode needs; `docker run` silently drops unset ones.

## Codebase conventions

- `internal/` packages are not importable outside the module. Everything is `internal/` today.
- Config types live in `internal/config/`; JSON schema is generated from Go structs by `internal/schema/`.
- Per-API clients live under `internal/<vendor>/` and return the canonical `*event.Event` at their boundary.
- The processor (`internal/processor/`) is the shared pipeline consumed by both the webhook handler and the poller.
- See [`docs/architecture.md`](docs/architecture.md) for full design rationale and [`docs/Roadmap.md`](docs/Roadmap.md) for planned work.

## Commit messages

Match the existing log style. Imperative present tense, no leading type prefix required (though `docs:`, `build:`, etc., are welcome):

```
add polling mode: per-(owner,repo) goroutines wired into shepherd main
fix retry pass to skip retry_count increment on budget-exhausted errors
docs: clarify polling sort semantics
```

Prefer one commit per logical change. If a PR naturally splits into multiple coherent commits, split them.

## Pull request process

1. **Open an issue first** for anything beyond a typo or a one-liner fix. This prevents wasted work on changes that don't fit the scope.
2. **Fork and branch** off `main`. Name branches after the change, not the author.
3. **Keep PRs focused.** If you find yourself fixing an unrelated bug in the same PR, please split it.
4. **Make CI pass.** `go build`, `go vet`, `go test -race` all need to go green (the workflow runs them automatically). Format with `gofmt`.
5. **Describe the change.** Use the PR template; a thoughtful summary and test plan go a long way.
6. **Update docs** if behavior changes. `docs/architecture.md` and the README are the primary surfaces.
7. **Request review.** One maintainer approval plus green CI is required to merge.

## Reporting bugs and requesting features

Use the issue templates:

- **Bug report:** include Go version, OS, config snippet (with secrets redacted), and logs.
- **Feature request:** describe the problem before the solution. Proposals grounded in a real use case land faster.

## Questions

Open a GitHub issue with a descriptive title. There's no separate discussion channel — the issue tracker is the single source of truth for everything: bugs, features, and questions. Please search existing issues (including closed ones) before opening a new one.

## Security

If you find a vulnerability, please do **not** open a public issue. See [`SECURITY.md`](SECURITY.md) for the private reporting process.

## Code of Conduct

This project follows the [Contributor Covenant](CODE_OF_CONDUCT.md). Be kind, be specific, be patient.

## License

By contributing, you agree that your contributions are licensed under the same [MIT License](LICENSE) that covers the project.
