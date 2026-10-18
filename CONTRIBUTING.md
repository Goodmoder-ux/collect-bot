# Contributing

Thanks for helping! Bug reports, fixes and improvements are welcome.

## Development setup

1. Install Go (version from `go.mod`) and, optionally, Docker and
   [golangci-lint](https://golangci-lint.run/).
2. Create a separate test bot with @BotFather. Never develop against the production bot or database.
3. `cp .env.example .env` and fill in the test credentials.
4. `make run`

## Before opening a pull request

- `make test` and `make lint` pass.
- New behaviour comes with tests. The service layer has fakes and an in-memory SQLite fixture
  (`internal/service/integration_test.go`).
- Schema changes go in a new numbered file in `migrations/`. Never edit a migration that has
  already been released.
- User-facing changes get a line under `[Unreleased]` in `CHANGELOG.md`.
- Commit messages follow [Conventional Commits](https://www.conventionalcommits.org/)
  (`feat(scope): ...`, `fix(scope): ...`).

## Releasing (maintainers)

1. Move the `[Unreleased]` entries in `CHANGELOG.md` under a new version heading.
2. `git tag vX.Y.Z && git push origin vX.Y.Z`
3. The release workflow publishes the image and creates the GitHub release.
