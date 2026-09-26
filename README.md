# ttacommunity

A community platform for **Through the Ages**, with three sites on one backend:

- **Ratings**: community Glicko ratings, with a public API. The rating math is
  DANeo's rating engine
- **Tournaments**: players in official tournament matches agree a start time,
  get reminders, and have the game started for them
- **TTA Pulse**: a blitz queue that pairs players near their level

**Early days.** The foundation is in place and the features are being built.
How the pieces fit: [`docs/architecture.md`](docs/architecture.md).

## Layout

| Directory | What | Language |
|---|---|---|
| `backend/` | The API and the worker (`cmd/api`, `cmd/worker`) | Go |
| `engine/` | The rating engine, run as a scheduled job | Python (uv) |
| `web/` | The public site, the admin site, and shared theme and types | Astro, React, TypeScript (pnpm) |
| `checks/` | Repo checks: secrets, digests, comment density, no raw HTML | Bash |
| `scripts/` | Run, test and CI entry points | Bash |

## Run it

**In containers**, with only Docker (or Podman with a compose provider):

```bash
scripts/dev.sh           # both databases, api, worker, the engine once, and the site
scripts/dev.sh down      # stop it; the database volumes are kept
```

Then open the site at http://127.0.0.1:4321, and the API at
http://127.0.0.1:8080/healthz. The first run creates `.env` from
`.env.example`, whose values are for local use only.

With Podman, the compose plugin talks to Podman's API socket. Turn it on once:
`systemctl --user enable --now podman.socket`.

**Natively**, for day-to-day work (Postgres still runs in a container):

```bash
scripts/dev.sh --native
```

This needs **Go 1.26**, **uv** (which fetches Python 3.12 itself), **Node 22**
with corepack (`corepack pnpm`), and `curl`. On Windows, use WSL2 or the
containers.

## Test it

```bash
scripts/test.sh    # every test: Go (with a throwaway Postgres), the engine, and web
scripts/ci.sh      # everything CI runs, in the same order
```

CI runs `scripts/ci.sh` unchanged, so a green run here means a green pipeline.
The script ends by listing what it did not cover.

Turn on the git hooks, which refuse a commit or push that looks like it holds a
credential:

```bash
git config core.hooksPath .githooks
```

## Data

No match or rating data is kept in this repo. How to load a ratings database
from a data directory will be documented here.
