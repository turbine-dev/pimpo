# Vigia

A personal AI agent that turns what you ask into software.

The first time you ask for something ("every morning, send me my agenda and the emails that matter"), Vigia does it with a language model and asks before anything irreversible. When it works, Vigia compiles the task into a **routine**: readable JavaScript with tests and a list of the only things it may touch. The routine then runs on its own, on schedule, without a language model. A small judgment model answers the few subjective questions ("is this email important?").

- **Reliable:** a routine is code with tests, not a prompt re-interpreted every time.
- **Cheap:** routines cost cents, not dollars a night.
- **Safe by construction:** routines reach the world only through declared capabilities; every action passes a policy engine and lands in a tamper-evident log.

Status: early development. See [docs/PLANNING.md](docs/PLANNING.md) for the plan and [docs/proof](docs/proof) for the compiler proof.

## Build

```bash
make build
./bin/vigia serve
```

Requires Go 1.24+ and Node 22+. Exploration and compilation use Claude Code (`claude`) with your own subscription.
