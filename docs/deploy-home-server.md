# Deploying on a home server

This guide sets up the bot on a Linux home server (x86_64 or arm64) with Docker. It includes pinned
versions, nightly backups and simple upgrades. The bot only makes outbound connections, so you
don't need port forwarding, a domain or TLS.

## 1. Install

```bash
sudo mkdir -p /opt/collect-bot && sudo chown "$USER" /opt/collect-bot
cd /opt/collect-bot

curl -fsSLO https://raw.githubusercontent.com/Goodmoder-ux/collect-bot/main/compose.yaml
curl -fsSL -o .env https://raw.githubusercontent.com/Goodmoder-ux/collect-bot/main/.env.example
chmod 600 .env

mkdir -p data/backups
sudo chown -R 65532:65532 data   # UID of the unprivileged user inside the container
```

Edit `.env`. Fill in the required values (see the README section "Getting your tokens"), and
**pin a version** so upgrades only happen when you choose:

```dotenv
BOT_VERSION=0.1.0
```

Start it:

```bash
docker compose up -d
docker compose ps          # wait for (healthy)
docker compose logs -f bot
```

Import the roster once:

```bash
docker compose exec -T bot /group-fund-bot import-students - < students.txt
```

`restart: unless-stopped` brings the bot back after reboots as long as the Docker service is
enabled (`sudo systemctl enable --now docker`).

## 2. Nightly backups

Create `/opt/collect-bot/backup.sh`:

```sh
#!/bin/sh
set -eu
cd /opt/collect-bot
docker compose exec -T bot /group-fund-bot backup "/data/backups/app-$(date +%F-%H%M).db"
# keep two weeks of local backups
find data/backups -name 'app-*.db' -mtime +14 -delete
```

```bash
chmod +x /opt/collect-bot/backup.sh
echo '30 3 * * * root /opt/collect-bot/backup.sh >> /var/log/collect-bot-backup.log 2>&1' \
  | sudo tee /etc/cron.d/collect-bot-backup
```

A backup that sits on the same disk is not a real backup. Copy `data/backups/` off the machine,
for example with rsync to a NAS or with [restic](https://restic.net/) to cloud storage:

```sh
rsync -a /opt/collect-bot/data/backups/ nas:/backups/collect-bot/
```

Occasionally check that a backup can actually be restored (see "Backup and restore" in the README).

## 3. Upgrading

1. Read the [changelog](../CHANGELOG.md) for the new version.
2. Take a backup: `./backup.sh`.
3. Set `BOT_VERSION=<new version>` in `.env`.
4. Run `docker compose pull && docker compose up -d`.
5. Check `docker compose ps` and `docker compose logs -f bot`.

Migrations run automatically. To roll back, set the old `BOT_VERSION` and restore the backup taken
in step 2, because older versions don't understand newer database schemas.

To get updates automatically instead, set `BOT_VERSION=latest` and run
[Watchtower](https://containrrr.dev/watchtower/). This is convenient, but upgrades then happen
without you reading the changelog.

## 4. Monitoring

- `docker compose ps` shows `healthy` or `unhealthy`. The status turns unhealthy when the database
  is unreachable or Monobank hasn't been polled successfully for a while (e.g. an expired token).
- For alerts, point an uptime tool like [Uptime Kuma](https://github.com/louislam/uptime-kuma) at the
  container's health status (Docker monitor type).
- Set `LOG_FORMAT=json` if you ship logs to Loki or another collector.

## Troubleshooting

| Symptom | Fix |
|---|---|
| `open database: ... permission denied` | `sudo chown -R 65532:65532 data` |
| `config: ... is required` | Fill in the listed variables in `.env`, then `docker compose up -d` |
| `telegram: Unauthorized (401)` | Wrong `TELEGRAM_TOKEN` |
| `monobank poll failed ... HTTP 403` in logs | Monobank token revoked or wrong; get a new one |
| `monobank poll failed ... rate limited` | Harmless; the bot waits and retries |
