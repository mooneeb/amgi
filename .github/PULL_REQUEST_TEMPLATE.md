<!--
Thanks for the PR. A couple of notes to make review easier:
  - If this PR is for something non-trivial, please link an issue that
    established the scope/design before code was written.
  - Keep the PR focused on one coherent change. Unrelated fixes are
    welcome — in a separate PR.
-->

## Summary

<!-- One or two sentences on what this PR changes and why. -->

## Related issues

<!-- "Closes #123" or "Part of #456". Omit if none. -->

## How this was tested

<!--
A short test plan. For user-facing changes, describe what you did to
verify the behavior end-to-end. For refactors, note what you did to
confirm no regressions.
-->

## Checklist

- [ ] `go build ./...` and `go vet ./...` both pass locally
- [ ] `go test -race ./...` passes locally
- [ ] Documentation updated if behavior changed (`README.md`, `docs/architecture.md`, config schema)
- [ ] No secrets, personal config, or local state included in the diff
- [ ] Commits have meaningful messages and are logically separate

## Notes for reviewers

<!-- Anything reviewers should look at especially carefully. Optional. -->
