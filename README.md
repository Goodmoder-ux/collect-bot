# Group Fund Bot

Telegram-бот для керування груповими зборами та автоматичного зіставлення платежів Monobank.

## Production deployment with Docker Compose

### Prerequisites

- Docker Engine with the Compose plugin;
- a Telegram bot token;
- a Monobank API token and jar account ID;
- the administrator's Telegram ID.

### 1. Clone the repository

```bash
git clone https://github.com/Goodmoder-ux/collect-bot.git
cd collect-bot
```

### 2. Configure environment

Create the deployment environment file:

```bash
cp .env.example .env
```

Fill in the values in `.env`:

```dotenv
TELEGRAM_TOKEN=...
MONOBANK_TOKEN=...
JAR_ACCOUNT_ID=...
JAR_URL=https://send.monobank.ua/jar/...
ADMIN_TELEGRAM_ID=...
DB_PATH=/app/data/app.db
```

Do not commit `.env` or API tokens to the repository.

### 3. Build and start the bot

```bash
docker compose up -d --build
```

The SQLite database is persisted in `./data` on the host and mounted at `/app/data` in the container.

### 4. Check logs and service status

```bash
docker compose ps
docker compose logs -f bot
```

The container stops gracefully when Docker sends `SIGTERM`; background pollers and schedulers are cancelled before the SQLite connection is closed.

### 5. Back up the SQLite database

Create regular backups from the host while the bot is stopped or during a quiet period:

```bash
docker compose stop bot
cp data/app.db "data/app.db.$(date +%Y%m%d-%H%M%S).backup"
docker compose start bot
```

Keep backups outside the deployment directory as well, and periodically verify that a backup can be restored. Do not rely on the container filesystem: only the `./data:/app/data` volume is persistent.

## Local development

The application can also be run directly with Go:

```bash
go run ./cmd/bot
```

For local configuration, `DB_PATH` may point to a local file such as `./data/group-fund-bot.db`.
