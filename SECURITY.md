# Security Policy

## Reporting a vulnerability

If you believe you've found a security issue in AMGI, please report it privately. Do **not** open a public issue.

**Preferred channels**, in order:

1. **GitHub private vulnerability reporting** — go to the Security tab on the repository and click *Report a vulnerability*.
2. **Email** — `mooneeb.hussain@gmail.com`. Include "AMGI security" in the subject line.

When reporting, please include:

- A clear description of the issue and its impact.
- Steps to reproduce, or proof-of-concept code if applicable.
- Affected versions (commit SHAs if on HEAD).
- Any suggested remediation ideas (welcome but not required).

## What's in scope

- The core binary (`cmd/amgi`) and all `internal/` packages.
- The Docker image's security posture (not the base image itself — report distroless issues upstream to Google).
- Credential handling: webhook signature verification, API token transport, env var usage, SQLite file permissions.
- Any issue that could allow unauthenticated task creation, data exfiltration, or resource exhaustion from a remote attacker.

## What's out of scope

- Vulnerabilities in third-party dependencies where AMGI's usage is correct (please report those upstream).
- Issues requiring local access to an already-compromised host.
- Denial-of-service via traffic volume at the HTTP ingress — this is expected behavior of a single-process daemon; deploy behind a rate-limiting proxy in production.
- Social engineering, phishing, or other non-technical vectors.

## Response

This project is maintained part-time by a single person. Reasonable-effort response targets:

- **Acknowledgement:** within 3 business days.
- **Initial assessment:** within 7 business days.
- **Fix and disclosure:** target within 30 days for high-severity issues; longer for lower-severity ones, with the reporter kept in the loop.

If you haven't heard back within the acknowledgement window, please send a polite ping.

## Supported versions

AMGI is pre-1.0. Only the `main` branch receives security updates. Once tagged releases exist, this section will document which release lines are maintained.

| Version | Supported          |
|---------|--------------------|
| `main`  | :white_check_mark: |

## Disclosure

Once a fix is available, we'll coordinate a disclosure window with the reporter. Credit will be given in the release notes unless the reporter prefers anonymity.

Thank you for helping keep AMGI and its users safe.
