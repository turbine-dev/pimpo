# Running Pimpo on a server

Routines run on schedule, so Pimpo does best on a machine that is always on: a home server, a Raspberry Pi, or a small VPS. The desktop and phone apps then open that Pimpo instead of running their own.

There are two ways to run it: the Docker image, or the single binary with systemd. Both keep everything in one data folder.

## With Docker

The image is `ghcr.io/turbine-dev/pimpo`, for amd64 and arm64. `latest` is the latest release, a version number (`0.6.0`) pins one, and `edge` follows the main branch.

```bash
docker run -d --name pimpo --restart unless-stopped \
  -p 127.0.0.1:7788:7788 -v pimpo:/data -e TZ=Europe/Lisbon \
  ghcr.io/turbine-dev/pimpo:latest
docker logs pimpo | grep auth
```

The last command prints the login link. Or, with the [`compose.yaml`](../compose.yaml) in the repository:

```bash
docker compose up -d
docker compose logs pimpo | grep auth
```

- **Data**: everything lives in the `/data` volume (the database, keys, memory, connectors, local models). Back it up like any volume, or use **Settings › Automatic cloud backup**.
- **Time zone**: set `TZ`, since routines run at local times.
- **User**: the container runs as an unprivileged user (uid 10001).
- **Health**: `GET /api/health` answers when Pimpo is up; the image's health check uses it.
- **Updating**: `docker compose pull && docker compose up -d`. The new version takes a snapshot of the data the first time it starts; to go back, run the previous tag and restore that snapshot (`docker exec pimpo pimpo snapshots`, then `pimpo restore NAME` with the container stopped).

## With the binary and systemd

```bash
curl -fsSL https://raw.githubusercontent.com/turbine-dev/pimpo/main/scripts/install.sh | sh -s -- --service
journalctl --user -u pimpo | grep auth
```

`--service` installs a user service that starts at boot. Update with `pimpo update` and restart the service (`systemctl --user restart pimpo`); `pimpo update --rollback` puts the previous binary back.

## Reaching it

Pimpo speaks plain http and authenticates with a token, so it must not be exposed to the internet directly. The port above is published on the server itself only. To reach it from elsewhere:

- **Tailscale, built in** (recommended): in **Settings › Phone**, turn on **From anywhere**. Pimpo joins your tailnet as `pimpo` and gets an `https://pimpo.<your-network>.ts.net` link, with no port opened on the server. Sign in to Tailscale once in the browser.
- **A reverse proxy with https** (Caddy, nginx, Traefik) in front of `127.0.0.1:7788`, if you already run one.
- **Home network only**: **At home** in the same screen serves Pimpo on the server's private address.

## Using it from the apps

- **Desktop**: on the server, create a link in **Settings › Phone** (**Open on your phone**). On your computer, choose **Connect to another Pimpo…** in the menu bar and paste it. The Pimpo on your computer stops, so the same routines and bots never run twice.
- **Phone**: pair it with the QR code on the same screen.

## Models on a server

Exploring and compiling routines need a model:

- **An API key** is the simplest: the welcome screen sets it up.
- **A local model**: run Ollama next to Pimpo (the commented `ollama` service in `compose.yaml`) and set its address, `http://ollama:11434`, in **Settings › Models**. A model of 8B or more learns tasks best and needs about 8 GB of memory.
- **Claude Code, Codex or opencode** work too if they are installed and signed in on the same machine. In Docker they are not in the image, so an API key or Ollama is the usual choice.

See [CONFIGURATION.md](CONFIGURATION.md) for every setting and [VALIDATION.md](VALIDATION.md) to check how routines do over time (`docker exec pimpo pimpo report`).
