<!--
Thanks for opening a PR! A few things that help the review go fast:

1. This PR must contain `Fixes #N` (or `Closes #N`) in the description,
   pointing at the issue it closes. Without it, the issue stays open and
   the project board doesn't move.
2. Branch from `qa`, not `main`. Open the PR against `qa`.
3. Keep the diff focused on one issue. Open a follow-up issue for the rest.
4. Run the relevant tests locally before pushing. CI does not run `go test`
   today, so please run `go test ./...` from `backend/` and from the repo
   root yourself.
5. Sign off your commits (`git commit -s`). See CONTRIBUTING.md for why.
-->

## What

<!-- One-paragraph summary of what this PR changes. -->

## Why

<!-- What problem does this fix? Link the issue with Fixes #N. -->

Fixes #

## How

<!-- Short description of the approach. Mention any non-obvious trade-offs. -->

## Testing

<!-- What did you run? Paste commands or short logs. -->

- [ ] `go test ./...` passes (backend and catalog)
- [ ] `npm test` passes (frontend)
- [ ] `gofmt` / `go vet ./...` clean
- [ ] `npm run build` succeeds
- [ ] Infra tests (if `infra/` changed):
  - [ ] `bash infra/tests/health-check-test.sh`
  - [ ] `bash .github/scripts/classify-release-test.sh`

## UI changes

<!-- Skip if N/A. -->

- [ ] Screenshots or screen recording attached
- [ ] Slot contract still valid (no new modal/menu without a slot)
- [ ] Tested in a real project workspace (not just `npm run dev`)

## Docs

<!-- Skip if N/A. -->

- [ ] `README.md` updated for user-facing behavior
- [ ] `docs/` updated for architecture / agent / app changes
- [ ] `CONTRIBUTING.md` updated if workflow changed
