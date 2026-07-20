# Troubleshooting

Run `akef-claim doctor` for local checks. Add `--network` only when you want refresh/status network traffic; doctor never claims or sends notifications.

- Config error: run `akef-claim config validate`; replace placeholders and remove unknown keys.
- Authentication expired: log in again and update both session secrets.
- Exit 30: a refresh/status request, process-lock wait, or scheduled deadline failed before a definite claim result. The claim POST is not retried automatically.
- Exit 41: the claim result is ambiguous. Do not manually or automatically retry until a later status check establishes what happened.
- Overlapping run: the later process waits up to 10 minutes for the claim lock, then rechecks attendance. `status` remains available while a run holds the lock.
- Windows task has no visible window: inspect the daily `scheduled-YYYY-MM-DD.log` under the user cache directory and query it with `schtasks.exe /Query /TN "Arknights Endfield SKPORT Daily Claim" /V /FO LIST`.
- Notification failure: run `akef-claim notify test TARGET`; it does not contact SKPORT.

Scheduled log files older than 45 days are removed automatically at the start of silent runs.

Issue reports must contain placeholders only. Do not attach config files, cookies, headers, screenshots containing secrets, webhook URLs, chat IDs, or bot tokens.
