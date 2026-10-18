# Changelog

All notable changes to this project are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project uses
[Semantic Versioning](https://semver.org/).

## [Unreleased]

## [0.1.0] - 2026-10-18

### Added
- Published multi-arch Docker image at `ghcr.io/goodmoder-ux/collect-bot` with CI and tagged releases.
- `backup` subcommand for consistent online SQLite backups.
- `import-students` subcommand to load the student roster from a file or stdin.
- `healthcheck` subcommand and a localhost health endpoint; the image declares a `HEALTHCHECK`.
- `version` subcommand; the version is logged at startup.
- Optional settings: `TZ`, `REMINDER_HOUR`, `POLL_INTERVAL`, `LOG_LEVEL`, `LOG_FORMAT`, `HEALTH_PORT`.
- Structured logging with `log/slog`.
- MIT license, contribution and security guides, home-server deployment guide.

### Changed
- The Monobank poll cursor is stored in the database, so payments made while the bot was offline
  are picked up after a restart.
- Monobank errors and rate limits no longer stop the bot; polling retries with backoff.
- The daily reminder run happens today if the scheduled hour hasn't passed yet. Before, any
  restart skipped that day.
- A failed reminder to one student (e.g. they blocked the bot) no longer stops reminders to the others.
- Migrations and timezone data are embedded in the binary. `MIGRATIONS_PATH` is no longer used.
- The runtime image is distroless (`nonroot`, UID 65532). The database lives at `/data/app.db`
  (previously `/app/data/app.db`).
- SQLite runs in WAL mode.
- Configuration errors are all reported at once.

### Migration notes
- If you used the old `docker-compose.yml`, move your database from `./data/app.db` (mounted at
  `/app/data`) to the new mount: keep the `./data` directory, set `DB_PATH=/data/app.db` (or remove
  it from `.env`), switch to `compose.yaml` and run `sudo chown -R 65532:65532 data`.

[Unreleased]: https://github.com/Goodmoder-ux/collect-bot/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/Goodmoder-ux/collect-bot/releases/tag/v0.1.0
