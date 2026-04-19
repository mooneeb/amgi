---
name: Bug report
about: Something is not working as documented
title: "[bug] "
labels: [bug]
---

## What happened?

<!-- A clear, one-sentence description of the problem. -->

## What did you expect?

<!-- What behavior did you expect instead? -->

## Steps to reproduce

<!-- Minimal steps someone else can follow to hit the same bug. -->

1.
2.
3.

## Environment

- AMGI version or commit SHA:
- Go version (`go version`):
- OS + arch:
- Deployment (binary / Docker / K8s):

## Config

<!--
Paste the relevant portion of your config.yaml, with secrets redacted.
Use placeholder values for any `list_id`, `label_ids`, or identifying repo names
if you're concerned about privacy.
-->

```yaml

```

## Logs

<!--
Paste relevant log output. If the logs are long, please include the
startup lines AND the lines around the failure. slog output is preferred
over truncated screenshots.
-->

```

```

## Anything else

<!-- Network conditions, webhook vs polling mode, related issues, etc. -->
