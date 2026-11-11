# Changelog

All notable changes to this project are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project uses
[Semantic Versioning](https://semver.org/).

## [Unreleased]

## [0.2.0] - 2026-11-11

### Added
- `/status`, `/export` and `/closecollection` without an ID show the collections as buttons, so
  collection IDs never have to be typed or guessed. Passing an ID still works.
- Archive of closed collections (`/collections` → "Архів") with their status and CSV export.
- "Експорт CSV" button and the collected total in the collection status.
- Students are told about a new collection as soon as it is created, with the amount, the deadline
  and a button to the jar. Before, the first message was a reminder a day or two later.
- Students get a confirmation when their payment is matched automatically or recorded as cash.
- The admin is notified about every new unmatched payment, with a button to match it, and when a
  collection closes because everyone has paid.
- A student whose registration request is rejected is told so.
- More surname forms are recognised in payment comments, including feminine and genitive ones
  ("за Ковальської", "від Шевчука").
- The bank's description of a transfer is searched for the surname together with the payer's
  comment, and both are shown to the admin.
- A warning when a new collection has the same amount as an active one.

### Changed
- With several active collections of the same amount, a payment goes to the one the student has
  not paid yet with the nearest deadline. Before, every such payment needed manual matching.
- The admin's `/collections` is a single message with one button per collection.
- Buttons under the collection status are shorter and laid out in two rows so they are not cut off
  on phones.

### Removed
- `/match payment_id student_id collection_id`. Use the buttons in `/unmatched` instead.

### Fixed
- A repeated payment from a student who already paid is no longer recorded as a second payment
  for the same collection. It goes to `/unmatched` for the admin to decide.
- A payment that is already matched can no longer be matched again by a second tap, and a student
  cannot be given two payments for one collection.
- Matching a payment or approving a registration no longer reports an error when the student
  cannot be notified (for example, they blocked the bot).
- The bot now replies when a command or button fails instead of staying silent, and inline
  buttons no longer keep spinning after an error. A handler panic no longer crashes the bot.

### Migration notes
- The database gets a rule that a student has at most one payment per collection. If an earlier
  version recorded duplicates, the earliest payment of each pair stays and the others reappear
  in `/unmatched`. Nothing is deleted. Take a backup before upgrading, as usual.

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

[Unreleased]: https://github.com/Goodmoder-ux/collect-bot/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/Goodmoder-ux/collect-bot/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/Goodmoder-ux/collect-bot/releases/tag/v0.1.0
