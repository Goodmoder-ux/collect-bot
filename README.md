# Group Fund Bot

[![CI](https://github.com/Goodmoder-ux/collect-bot/actions/workflows/ci.yml/badge.svg)](https://github.com/Goodmoder-ux/collect-bot/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

> Telegram-бот для керування груповими зборами та автоматичного зіставлення платежів Monobank.

A Telegram bot for running group collections (class funds, gifts, trips). It watches a Monobank jar
and automatically matches incoming payments to students by the surname in the payment comment.

- Students register with `/start` by picking their name from the roster, and the admin approves them.
- The admin creates collections (`/newcollection`) with an amount and a deadline. Students are
  told about each new collection right away.
- Payments to the jar are matched automatically when the amount equals an active collection and
  the comment contains exactly one student's surname. Everything else lands in `/unmatched` for
  manual review, and the admin is notified about it.
- Daily reminders go to debtors. The admin is notified about overdue collections and gets reports
  and CSV exports (`/status`, `/report`, `/export`).
- A collection closes automatically once everyone has paid.

The bot uses outbound connections only (Telegram long polling and the Monobank API), so it needs
no open ports, domain or TLS certificate. A home server behind NAT works fine.

## Quick start (Docker)

You need Docker with the Compose plugin and the four values described in
[Getting your tokens](#getting-your-tokens).

```bash
mkdir collect-bot && cd collect-bot
curl -fsSLO https://raw.githubusercontent.com/Goodmoder-ux/collect-bot/main/compose.yaml
curl -fsSL -o .env https://raw.githubusercontent.com/Goodmoder-ux/collect-bot/main/.env.example
chmod 600 .env && nano .env          # fill in the required values

mkdir data && sudo chown 65532:65532 data   # the container runs as an unprivileged user
docker compose up -d
docker compose ps                      # STATUS should become "healthy" within a minute
```

Then add your students to the roster (one full name per line, e.g. `Петренко Іван`):

```bash
docker compose exec -T bot /group-fund-bot import-students - < students.txt
```

Students can now open the bot, send `/start` and pick their name. You approve each request in Telegram.

## Configuration

All configuration is read from environment variables (the `.env` file). See
[`.env.example`](.env.example) for the full annotated list.

| Variable | Required | Default | Description |
|---|---|---|---|
| `TELEGRAM_TOKEN` | yes | | Bot token from @BotFather |
| `MONOBANK_TOKEN` | yes | | Personal Monobank API token |
| `JAR_ACCOUNT_ID` | yes | | ID of the jar to watch |
| `JAR_URL` | yes | | Public jar link shown to students |
| `ADMIN_TELEGRAM_ID` | yes | | Numeric Telegram ID of the admin |
| `BOT_VERSION` | | `latest` | Image tag used by `compose.yaml` |
| `DB_PATH` | | `/data/app.db` | SQLite database path |
| `TZ` | | `Europe/Kyiv` | Timezone for deadlines and reminders |
| `REMINDER_HOUR` | | `15` | Local hour of the daily reminder run |
| `POLL_INTERVAL` | | `70s` | Monobank polling interval (≥ 60s) |
| `LOG_LEVEL` | | `info` | `debug`, `info`, `warn`, `error` |
| `LOG_FORMAT` | | `text` | `text` or `json` |
| `HEALTH_PORT` | | `8080` | Localhost health endpoint port, `0` disables it |

If the configuration is invalid, the bot prints every problem at once and exits.

## Getting your tokens

1. **Telegram bot token:** message [@BotFather](https://t.me/BotFather), send `/newbot` and copy the token.
2. **Admin Telegram ID:** message [@userinfobot](https://t.me/userinfobot) and copy your numeric ID.
3. **Monobank token:** open <https://api.monobank.ua/>, log in with the Monobank app and copy the
   personal token. It grants read access to your statements, so treat it like a password.
4. **Jar account ID:** list your jars with the token:

   ```bash
   curl -s -H "X-Token: <MONOBANK_TOKEN>" https://api.monobank.ua/personal/client-info
   ```

   In the `jars` array, copy the `id` of the jar you collect into. `JAR_URL` is that jar's public
   `https://send.monobank.ua/jar/...` link.

## Operations

### Logs and health

```bash
docker compose logs -f bot
docker compose ps        # shows (healthy) / (unhealthy)
```

The container is healthy when the database answers and the last successful Monobank poll was
recent. The bot keeps running through Monobank or network outages and retries with backoff. After
downtime it resumes from where it stopped (up to Monobank's 31-day limit), so payments made while
it was offline are not lost.

### Upgrading

```bash
# optionally set BOT_VERSION=x.y.z in .env first
docker compose pull
docker compose up -d
```

Database migrations run automatically on startup. Read the [changelog](CHANGELOG.md) before
upgrading across minor versions.

### Backup and restore

Backups are taken online and the bot keeps running:

```bash
docker compose exec bot /group-fund-bot backup /data/backups/app-$(date +%F).db
```

The file appears in `./data/backups/` on the host. Copy it somewhere off the machine as well.

To restore:

```bash
docker compose stop bot
cp data/backups/app-YYYY-MM-DD.db data/app.db
rm -f data/app.db-wal data/app.db-shm
docker compose start bot
```

For a home-server setup with automated nightly backups, see
[docs/deploy-home-server.md](docs/deploy-home-server.md).

### CLI

The binary (and the image entrypoint) supports:

```text
group-fund-bot [run]                       start the bot (default)
group-fund-bot backup <file.db>            online backup of the database
group-fund-bot import-students <file|->    add names to the roster ("-" = stdin)
group-fund-bot healthcheck                 exit 0 if the running bot is healthy
group-fund-bot version                     print the version
```

## Development

Requires Go (see `go.mod` for the version).

```bash
cp .env.example .env   # fill in test bot credentials
make test              # go test -race ./...
make lint              # go vet + golangci-lint
make run               # go run ./cmd/bot  (database in ./data/app.db)
```

To build and run the image locally:

```bash
docker compose -f compose.yaml -f compose.dev.yaml up --build
```

Project layout:

```text
cmd/bot/                 entrypoint and CLI subcommands
internal/config/         environment configuration
internal/domain/         models and repository interfaces
internal/service/        payment matching, Monobank poller, reminders
internal/telegram/       bot handlers and admin wizards
internal/monobank/       Monobank API client
internal/repository/sqlite/  SQLite repositories
internal/health/         health endpoint
migrations/              SQL migrations, embedded into the binary
```

Releases: pushing a `vX.Y.Z` tag runs the tests, publishes a multi-arch image
(`linux/amd64`, `linux/arm64`) to `ghcr.io/goodmoder-ux/collect-bot` and creates a GitHub release.

See [CONTRIBUTING.md](CONTRIBUTING.md) to contribute and [SECURITY.md](SECURITY.md) to report
vulnerabilities.

## License

[MIT](LICENSE)
