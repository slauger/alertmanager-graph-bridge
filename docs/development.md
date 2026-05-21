# Development

## Requirements

- Go 1.26 or newer.
- `helm` (for the chart), `golangci-lint`, `shellcheck` and `hadolint` for the
  full set of checks.

## Layout

```
cmd/alertmanager-graph-bridge   process entry point
internal/config                 configuration loading
internal/alertmanager           webhook payload parsing
internal/mail                   grouping and HTML rendering
internal/graph                  Microsoft Graph client
internal/server                 HTTP handlers and metrics
charts/alertmanager-graph-bridge Helm chart
images/alertmanager-graph-bridge Containerfile
hack                            CI helper scripts
```

## Common tasks

```bash
make build          # compile the binary into bin/
make run            # run locally with config.yaml
make test           # run all tests with the race detector
make cover          # run tests and enforce 80% coverage
make lint           # golangci-lint
make vet            # go vet
make vulncheck      # govulncheck
make unicode-lint   # detect suspicious Unicode characters
make helm-lint      # lint the Helm chart
make helm-unittest  # run Helm chart unit tests
make ci             # run the full check suite
```

## Testing strategy

- Table-driven unit tests live next to the code they cover.
- The Microsoft Graph token endpoint and the `sendMail` endpoint are mocked
  with `httptest.Server`, so no network or real Azure tenant is needed.
- `internal/server/e2e_test.go` wires the full server against mocked Graph
  endpoints and drives real webhook requests through it, covering bearer
  auth, recipient splitting, retries, Graph errors and metrics.
- Total statement coverage is kept at or above 80%.

## Conventional commits

Commit messages follow the
[Conventional Commits](https://www.conventionalcommits.org/) specification
(`feat:`, `fix:`, `docs:`, `chore:`, `BREAKING CHANGE:`). Releases and the
changelog are generated from them by semantic-release.
