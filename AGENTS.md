# Pushovergate

## Product contract

- `README.md` is the sole product and operator contract. Keep it in sync when behavior changes.
- Keep one hand-maintained contract. OpenAPI or generated reference documentation is explicitly excluded.
- Work only within explicitly agreed scope and acceptance criteria. Do not invent future ideas or implement adjacent ideas early.

## Engineering principles

Apply Jane Street-style correctness, KISS, and convention over configuration: keep one coherent, opinionated path, a small public surface, and few operator decisions.

- Make invalid states hard to represent. Validate untrusted input at the boundary; use domain types when they enforce a real invariant.
- Keep the functional core separate from effects. Data transformation should not perform network, clock, or logging work. Keep those dependencies explicit at the edges.
- Prefer immutable values and visible data flow. Give mutable state one clear owner.
- Treat readability as part of correctness. Prefer direct code, precise names, exhaustive decisions, and small reviewable changes.
- Prefer conventions and strong defaults over configuration. Expose configuration only for secrets (`API_TOKEN`, `PUSHOVER_APP_TOKEN`, `PUSHOVER_USER_KEY`). The listen port (`:8080`) and upstream Pushover URL (`https://api.pushover.net/1/messages.json`) are hardcoded conventions.
- Implement as a single shallow Go package in `main.go` with tests in `main_test.go`. No interfaces. Tests inject an `*http.Client` and an internal upstream URL field directly.
- Standard library only (`net/http`, `encoding/json`, `crypto/sha256`, `crypto/subtle`, `log/slog`, `net/url`, `os`, `os/signal`, `syscall`, `time`, `unicode/utf8`). Zero external runtime dependencies. Let `http.ServeMux` provide 404 and 405 responses.
- Measure before optimizing. Keep complexity local and minimal.
- Design effects so behavior remains deterministic and independently testable.

## Dependencies

- Use the latest stable Go release unless a documented project requirement justifies an exception. Standard library only for application code.
- Pin external container images by immutable digest (e.g. `image@sha256:...`).
- Reference GitHub Actions by full 40-character commit SHA with version comments.
- No `tools/go.mod`, `golangci-lint`, `govulncheck`, `markdownlint` configuration, Compose, local hooks, or release tooling.

## Change discipline

- Keep each change set focused on one accepted package and present its diff for review.
- During review, revise the diff without running validation unless the user asks. Validate only after approval, then present any resulting fixes.
- After final acceptance, create exactly one local commit. Pushes, pull requests, tags, releases, and image publication require separate instructions.

## Repo-local skills

- The repository uses the following repo-local skills, installed and updated separately via `npx skills update -p -y`:
  - `golang-code-style`
  - `golang-naming`
  - `golang-context`
  - `golang-error-handling`
  - `golang-security`
  - `golang-testing`
  - `golang-continuous-integration`
- Treat `.agents/skills` and `skills-lock.json` as CLI-owned.

## Validation

- Local validation runs via `just test` (`docker build --target test .`) and `just ci` (`docker build --target test .` followed by `docker build .`).
- Run Go tooling through Docker, never directly on the host.
- Dockerfile contains only `test`, `build`, and `runtime` stages with no module download or cache mounts.
- After changing the root file layout, run `just show_dockerignore` and exclude files the image build does not need.

## Git output

- For automated inspection, use `git --no-pager diff --staged`, `git --no-pager diff --stat`, and `git --no-pager show --stat`.
- Do not override `GIT_PAGER` globally.
