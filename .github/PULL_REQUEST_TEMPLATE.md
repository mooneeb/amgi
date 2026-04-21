<!--
Thanks for the PR. A few non-negotiables to make review work:
  - Every PR must link a related GitHub issue (see "Related issues"
    below). File one first if it doesn't exist yet — even a two-line
    issue is enough. PRs without a linked issue will be closed with
    a polite pointer to open one first.
  - Keep the PR focused on one coherent change. Unrelated fixes are
    welcome — in a separate PR.
-->

## Summary

<!-- One or two sentences on what this PR changes and why. -->

## Related issue (required)

<!--
Every PR must link a GitHub issue. Use "Closes #123" to auto-close
on merge, or "Part of #456" for PRs that address a portion of a
larger issue. If no issue exists yet, please file one first and
reference it here.
-->

Closes #

## How this was tested

<!--
A short test plan. For user-facing changes, describe what you did to
verify the behavior end-to-end. For refactors, note what you did to
confirm no regressions.
-->

## Checklist

- [ ] This PR links a GitHub issue (see "Related issue" above)
- [ ] `go build ./...` and `go vet ./...` both pass locally
- [ ] `go test -race ./...` passes locally
- [ ] Documentation updated if behavior changed (`README.md`, `docs/architecture.md`, config schema)
- [ ] No secrets, personal config, or local state included in the diff
- [ ] Commits have meaningful messages and are logically separate

## Notes for reviewers

<!-- Anything reviewers should look at especially carefully. Optional. -->
