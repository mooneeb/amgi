# Contributing to AMGI

Thanks for your interest. AMGI is a small, focused project — a one-way sync from GitHub to Amazing Marvin. Contributions are welcome across the board: bug fixes, new filter operators, documentation improvements, test coverage, and thoughtful feature additions that fit the design.

This document covers how to get set up, how work is reviewed, and where to ask questions.

## Scope

AMGI's scope is deliberately narrow:

- **In scope:** GitHub → Marvin task creation, config-driven filtering, idempotency and retry, webhook and polling modes, operator ergonomics.
- **Out of scope (for now):** bi-directional sync, non-GitHub sources, non-Marvin destinations, multi-tenancy. See [`docs/Roadmap.md`](docs/Roadmap.md) for the boundary between "planned later" and "not planned."

If you have an idea that stretches scope, open an issue to discuss it before sending a PR.

## Design context

Before proposing significant changes, read [`docs/architecture.md`](docs/architecture.md). It covers the pipeline, data flow, rate-limit strategy, idempotency model, and operational considerations. Most design questions have answers there.

## Development setup

You need Go 1.26 or newer. Clone and build:

```bash
git clone https://github.com/mooneeb/amgi.git
cd amgi
go build ./...
go test -race ./...
```

To run locally against real APIs, you will need:

- A GitHub personal access token (`repo` or `public_repo` scope).
- A Marvin API token ([how to get one](https://app.amazingmarvin.com/pre?api)).
- A config file. Start from [`examples/config.yaml`](examples/config.yaml) and adapt to your GitHub username/repo.

```bash
export GITHUB_TOKEN="ghp_..."
export MARVIN_API_TOKEN="..."
export GITHUB_WEBHOOK_SECRET="dev-secret"       # required even in polling-only mode; see issue tracker
export CONFIG_PATH="$(pwd)/your-config.yaml"
export AMGI_DB_PATH="/tmp/amgi-dev.db"

go run ./cmd/amgi
```

Container build and run:

```bash
docker build -t amgi:dev .
docker run --rm \
  -e GITHUB_TOKEN -e MARVIN_API_TOKEN -e GITHUB_WEBHOOK_SECRET \
  -v $(pwd)/your-config.yaml:/etc/amgi/config.yaml:ro \
  -p 8080:8080 \
  amgi:dev
```

## How AMGI is built

The build order is spike → walking skeleton → harden → polish (see [architecture doc](docs/architecture.md#system-overview)). The project is currently transitioning from "walking skeleton" to "harden." Deferred hardening work is tracked in project internals and the [Roadmap](docs/Roadmap.md).

Key conventions:

- `internal/` packages are not importable outside the module. Everything is `internal/` today.
- Config types live in `internal/config/`; JSON schema is generated from Go structs by `internal/schema/`.
- Per-API clients live under `internal/<vendor>/` and return the canonical `*event.Event` at their boundary.
- The processor (`internal/processor/`) is the shared pipeline consumed by both the webhook handler and the poller.

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

Open a GitHub Discussion or a regular issue — there's no dedicated chat channel. Please search first to avoid duplicates.

## Security

If you find a vulnerability, please do **not** open a public issue. See [`SECURITY.md`](SECURITY.md) for the private reporting process.

## Code of Conduct

This project follows the [Contributor Covenant](CODE_OF_CONDUCT.md). Be kind, be specific, be patient.

## License

By contributing, you agree that your contributions are licensed under the same [MIT License](LICENSE) that covers the project.
