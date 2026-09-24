# Vigia protection list

Domains that receive stolen data, malicious skills and dangerous action patterns, reported by people who run Vigia and reviewed before they get in. `list.json` is signed by the maintainers. Every Vigia downloads it daily and refuses anything unsigned, tampered with, or older than what it already has. The version shipped inside the binary works offline.

When something on the list is hit, Vigia blocks it, and the receipt says "Proteção da comunidade" with the reason and how many people reported it. The Guard plugins apply the same list to OpenClaw and Hermes.

## Proposing an entry

```bash
vigia protect suggest --domain collect-data.example --reason "exfiltration endpoint in a fake invoice email"
```

This prints an entry to add to `entries.json` by pull request. It contains only the indicator and the reason, never the email, page or message where you saw it.

## False positives

Open an issue and say why. Entries confirmed as false positives are removed in the next version. Until then, the owner of a Vigia can ignore one entry locally (`POST /api/protection/<id>/ignore`, recorded in the event log).

## Maintainers

```bash
vigia protect sign --key <maintainer key> protection
vigia protect verify protection/list.json
```

The public keys are listed in `protection/embed.go`.
