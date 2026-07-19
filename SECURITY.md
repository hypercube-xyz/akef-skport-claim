# Security policy

Report suspected vulnerabilities privately through the repository host's private security advisory mechanism. Do not open a public issue containing exploit details or secret material.

SKPORT `cred` and `game_role` values are session secrets equivalent to credentials. The project never asks users to submit them. Never attach a real config, cookies, request headers, screenshots containing tokens, full webhook URLs, bot tokens, chat IDs, or unreviewed logs.

If a secret is exposed, log out or otherwise rotate the affected session immediately. Remove the material from working copies and rewrite published Git history when applicable; deleting only the latest file is not sufficient.
