# RFC 0001: A sandbox for running code

- Author: denerFernandes
- Status: accepted (behind **Laboratório**, off by default)
- Discussion: roadmap item 4.2 ([ROADMAP.md](../ROADMAP.md))

## Problem

Some tasks need a little code: convert a spreadsheet, analyze a CSV, resize photos, run the script a skill comes with. Pimpo's routines are JavaScript with no access to anything but capabilities, and the agent has no way to run code. OpenClaw and Hermes run shell commands on the owner's machine, which is exactly the power that makes them dangerous.

## Proposal

A capability, `code.run`, that runs a short program in an isolated container and returns what it printed and the files it wrote:

```
code.run({language, code, files?, stdin?}) → {stdout, stderr, exit_code, files, seconds}
```

- `language` is `python` (Python 3.12), `javascript` (Node 22) or `shell` (POSIX sh on Alpine).
- `files` are text files placed next to the program (`name → content`); files the program writes under `out/` come back, text as text and anything else in base64.
- Limits: 60 seconds, one CPU, 512 MB of memory, 128 processes, 64 KB of output per stream, 5 MB of files each way.

The container runs with **no network**, a **read-only** root filesystem (only its working folder and a small `/tmp` are writable), **all Linux capabilities dropped**, `no-new-privileges`, an unprivileged user, and **no environment** from Pimpo, so no secret can be in it. Each call gets a fresh container that is removed afterwards.

The backend is Docker (or anything with Docker's command line, such as Podman's). The images are pulled the first time they are needed. Without Docker the capability says so and does nothing.

It is a Labs feature, off by default: **Ajustes › Laboratório › Rodar código isolado**.

Because nothing inside the container reaches anything outside it, `code.run` is a `read` capability: it may run during explorations (which rehearse changes) and inside routines like any read, and its results are recorded in tests like any other capability's.

## Safety

- *Actions without approval*: code cannot act on the world; it has no network and no capability of its own. Anything it produces reaches the world only through other capabilities, which pass the rules as always.
- *Leaked secrets*: the container gets no environment, no mounted folder but a fresh working folder, and no network to send anything.
- *Instructions smuggled from content*: output is data returned to the agent, like any capability's result.
- *Resource abuse*: time, memory, CPU, processes and output are capped; a call that overruns is killed.

Tests: `internal/sandbox` checks the flags every run gets (network, read-only, capabilities, user, limits), and a live test runs real programs, including ones that try the network and write outside their folder.

## Alternatives

- **Running code on the host**, as other agents do: rejected, it is the main risk this project exists to remove.
- **WebAssembly runtimes**: safer still and no Docker needed, but few Python libraries work there today. A later backend.
- **gVisor or a microVM (Firecracker)**: stronger isolation than containers; a later backend where available.
- **A network allowlist per call**: useful (a script that downloads one file), but it needs a filtering proxy to be safe. Left for a follow-up RFC; until then, fetching stays with `http.getJSON` and `web.read`, which have scopes.

## Migration

Nothing changes for existing installs: the capability does not exist until the owner turns it on, and turning it off again removes it.
