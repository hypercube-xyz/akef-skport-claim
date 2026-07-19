# Notifications

Destinations are independent. A failed target does not stop later targets and never triggers another SKPORT request. Each delivery attempt has a 15-second timeout and is retried at most once for connection failure, timeout, HTTP 429, or HTTP 5xx. Repeated authentication/error notifications are deduplicated per account and target for `notification_error_cooldown`; successful claim notifications are not deduplicated.

Notifications use the same base line format across destinations: `[account]: result`. Discord mentions are disabled. ntfy uses the neutral title `AKEF` and raises native priority for errors; each destination truncates only when required by its service limit.

Discord requires an HTTPS webhook on an official Discord host with a path beginning `/api/webhooks/`. Telegram requires a bot token and chat ID. ntfy requires an HTTPS server and a conservative topic name.

```bash
akef-claim notify test discord-home
akef-claim notify test telegram-admin
akef-claim notify test ntfy-phone
```

These commands send synthetic reports only and make no SKPORT request. Errors never contain a full webhook URL, token, or chat ID.
