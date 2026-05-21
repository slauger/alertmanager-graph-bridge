# Quality Criteria

These are the quality criteria the project is held to. Every change is expected
to keep all of them satisfied; CI enforces the automatable ones.

## QC1 - Correctness

- All CI checks pass on every push and pull request.
- The full test suite passes under the Go race detector (`go test -race`).
- The build is reproducible (`-trimpath`, pinned Go toolchain in `go.mod`).

## QC2 - Test coverage

- Total statement coverage stays at or above 80% (enforced by
  `hack/check-coverage.sh`); it currently sits well above 95%.
- Tests are table-driven and live next to the code they cover.
- Untrusted input parsing (the Alertmanager webhook) has a fuzz test
  (`FuzzParse`).
- A full end-to-end test wires the real server against mocked Microsoft Graph
  endpoints.

## QC3 - Resilience

- A panic in any HTTP handler is recovered and turned into a `500` response;
  the process never crashes on a bad request.
- All I/O has timeouts: HTTP read/read-header/write/idle timeouts, a bounded
  send timeout for Graph calls, and a bounded request body size.
- Malformed webhook payloads and malformed `email_to` labels are handled
  gracefully instead of failing a whole batch.
- Shutdown is graceful and bounded by a configurable grace period.

## QC4 - Security

- Static security analysis (`gosec`, via `golangci-lint`) passes; the single
  documented suppression is reviewed.
- Dependency vulnerability scanning (`govulncheck`) passes.
- Secrets (client secret, bearer token) are never written to logs.
- Incoming bearer tokens are compared in constant time.
- E-mail bodies are rendered with `html/template`, escaping attacker-influenced
  alert annotations.

## QC5 - Observability

- Prometheus metrics cover webhook traffic, request latency, mails sent, send
  errors by reason, send latency and recovered panics.
- An `agb_build_info` metric exposes the build and Go versions.
- Logging is structured (`slog`) and every webhook outcome is logged.

## QC6 - Operability

- Liveness (`/healthz`) and readiness (`/readyz`) probes are provided.
- Configuration is validated at startup; invalid configuration fails fast.
- Configuration is documented and supported through both a YAML file and
  environment variables.

## QC7 - Maintainability

- Commits follow the Conventional Commits specification.
- Linting (`golangci-lint`), formatting (`gofmt`), shell linting
  (`shellcheck`), container linting (`hadolint`) and a Unicode lint all pass.
- Dependencies are kept current by Renovate; releases are automated by
  semantic-release.
