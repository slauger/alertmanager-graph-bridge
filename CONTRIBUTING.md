# Contributing

Thanks for your interest in improving alertmanager-graph-bridge.

## Development setup

You need Go 1.26 or newer. The optional tooling (`golangci-lint`, `helm`,
`shellcheck`, `hadolint`) is required to run the full check suite locally.

```bash
git clone https://github.com/slauger/alertmanager-graph-bridge
cd alertmanager-graph-bridge
make test
```

## Workflow

1. Branch off `develop`.
2. Make your change with accompanying tests.
3. Run `make ci` and make sure every check passes.
4. Open a pull request against `develop`.

## Commit messages

Commits follow the [Conventional Commits](https://www.conventionalcommits.org/)
specification. The type prefix drives semantic-release:

- `feat:` - a new feature (minor release)
- `fix:` - a bug fix (patch release)
- `docs:`, `chore:`, `ci:`, `test:`, `refactor:` - no release
- `feat!:` or a `BREAKING CHANGE:` footer - a major release

## Quality bar

All changes must keep the [quality criteria](docs/quality.md) satisfied:
tests pass under the race detector, coverage stays at or above 80%, and
`golangci-lint`, `govulncheck`, `gosec`, `helm`, `hadolint`, `shellcheck` and
the Unicode lint all pass. CI enforces this.

## Reporting bugs

Open a GitHub issue with steps to reproduce, the expected and actual behaviour,
and the relevant logs (with secrets redacted).
