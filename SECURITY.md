# Security policy

The bot handles a Monobank API token, which grants read access to the owner's bank statements,
plus students' names and payment history. Please report security problems privately.

## Reporting a vulnerability

Use GitHub's [private vulnerability reporting](https://github.com/Goodmoder-ux/collect-bot/security/advisories/new).
Don't open a public issue. You should get a reply within a week.

## Supported versions

Only the latest release gets security fixes.

## Operator checklist

- Keep `.env` private (`chmod 600 .env`) and never commit it.
- If a token leaks, revoke it: Monobank at <https://api.monobank.ua/>, Telegram with `/revoke` in @BotFather.
- Store backups as carefully as the live database: they contain personal data.
