# Zodim Guard for OpenClaw and Hermes

Keep your agent and put Zodim in front of it. Before OpenClaw or Hermes runs any tool, the Guard asks Zodim, which answers with the same rules engine and the same shared protection list it uses for itself:

- **allow**: the tool runs.
- **ask**: your agent shows its own approval prompt (OpenClaw `requireApproval`, Hermes `approve`).
- **block**: the tool does not run, and the agent sees why.

Every decision lands in Zodim's Receipts. If Zodim does not answer, the Guard blocks, unless you set `failOpen`.

Tools are sorted into capabilities you can write rules for:

| Capability | Examples |
|---|---|
| `guard.exec` | exec, bash, terminal, code |
| `guard.delete` | delete tools, `rm -rf`, `DROP TABLE`, force push |
| `guard.send` | messages, email, posts |
| `guard.write` | writing and editing files |
| `guard.web` | fetch, browser, anything with a URL (the host is the scope) |
| `guard.read` | reading, listing, searching |
| `guard.other` | the rest |

For example, in Zodim's Rules you can write "never let other agents delete anything", or "OpenClaw can run commands without asking in the project folder".

## Token

In Zodim, go to **Ajustes › Abrir no celular** and pair a device named "Guard" with address `http://127.0.0.1:7788`. Use the token from the link (the part after `token=`). You can revoke it there at any time.

## OpenClaw

```bash
openclaw plugins install --link ./guard/openclaw --force
openclaw plugins enable zodim-guard
```

In `openclaw.json`:

```json5
plugins: { entries: { "zodim-guard": { enabled: true, config: { url: "http://127.0.0.1:7788", token: "<token>" } } } }
```

## Hermes

```bash
cp -r guard/hermes ~/.hermes/plugins/zodim-guard
hermes plugins enable zodim-guard
```

Set `url` and `token` under `plugins.entries.zodim-guard.settings` in `~/.hermes/config.yaml`, or export `ZODIM_URL` and `ZODIM_TOKEN`. Restart Hermes.

## Tests

`node --test guard/openclaw/guard.test.ts` and `python3 -m unittest test_guard` (inside `guard/hermes`). Zodim's Go suite also runs both against a real Zodim (`TestGuardPluginsAgainstZodim`).
