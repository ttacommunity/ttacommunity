# Architecture

**Draft.** How the pieces fit together, and where the trust boundaries are.

## Pieces

```mermaid
flowchart LR
    subgraph cf["Cloudflare"]
        pages["Cloudflare-built Astro sites<br/>ttacommunity.*: /ratings · /tournaments · /pulse<br/>ttac-admin.* (+ Worker proxy)"]
        access["Access + Discord OIDC worker<br/>(ttac-admin always; the site while experimenting)"]
    end
    subgraph cluster["k3s — namespace ttacommunity (default-deny)"]
        api["api (Go)<br/>public ratings API · auth · tournaments/pulse"]
        worker["worker (Go)<br/>reminders · kick-offs · rounds · outbox · ingestion"]
        engine["engine (Python)<br/>CronJob: recompute → new run"]
        rdb[("ratings DB")]
        adb[("app DB")]
    end
    browser["Browser"] --> pages
    browser -->|"ttac-api.* (CORS, Origin-checked)"| api
    pages -->|"admin /api/* + Access JWT"| api
    api --> rdb
    api --> adb
    worker --> adb
    worker --> rdb
    engine --> rdb
    api -->|OAuth2| discord["Discord"]
    worker -->|bot DMs| discord
    worker -->|games, results| cge["CGE API"]
    worker -->|SA token| relay["mail-relay"]
```

| Piece | Role | Writes to |
|---|---|---|
| **api** | Public ratings reads, Discord sign-in, Tournaments/Pulse actions, and **admin routes that accept only a valid admin-app Access JWT** (audience-pinned, fail closed), reached through the admin site's Worker | app DB. Ratings DB **import batches and recompute requests** (admin routes only) |
| **worker** | Everything that happens *at a time*. **Exactly one acts at a time** (a Postgres advisory lock), so a rollout cannot start a game twice. | app DB. Ratings DB import batches (CGE ingestion, later) |
| **engine** | Reads matches, computes all models, and writes a **new run**. It never overwrites a published run | Ratings DB result tables only |
| **site, admin** | Astro, built and hosted by Cloudflare from git. One public site with sections, and admin on its own host behind Access. Types generated from the OpenAPI spec | nothing |

**Two databases with separate roles.** The public read path uses a read-only
role on the ratings DB, so a bug in a public endpoint cannot write ratings.

## Data flow for ratings

`upload (now) / CGE API (later)` → **import batch** (validated, duplicates
removed, aliases resolved, junk *refused* rather than warned) →
**recompute request** → engine job → **new run** → **published** once its
checks pass (a one-row switch) → API serves it, and Cloudflare caches until the next publish.

### Rating runs

The engine never changes the ratings people are reading. Each recompute
writes a whole **run**, and every result table (ratings, history,
calibration) carries its `run_id`. A single row, `current_run`, names the run
the API serves. Publishing is changing that row in one transaction, and
rolling back is pointing it at the previous run. Only a run whose checks
passed (`status = 'complete'`) can be current: the schema refuses anything
else.

A run records its **cause**: the engine version and a fingerprint of the
input matches. A few recent runs are kept, so two can be compared before
one is published.

**Tournament results** (standings, podiums, Hall of Fame) are derived by the
engine from matches and the tournament rules. Organisers record what matches
cannot show, such as seeding and official rulings, in the admin site, scoped
to the series they run.

## Site content

Text pages and cards (the FAQ, the methodology, the analysis commentary)
are MDX files in `web/site/src/content/`, changed by merge request. MDX is
used across the whole site so there is one way to write content: Markdown
for prose, maths in `$…$` rendered to MathML by KaTeX at build (no maths
script in the browser), and HTML with classes or small components where a
page needs styled boxes or tables. A new page is a new file.
Interactive parts write maths the same way, as TeX, through a small
`<Tex>` component that runs the same KaTeX in the browser.

## The community's data outlives the site

The worker exports the community's data (matches, tournaments, results,
published ratings) as CSV on a schedule to a public data repository, whose
releases are archived independently. **The export format is the import
format**, so anyone can rebuild an instance from the public backup alone.
**It contains no personal data.** Operational backups, which do, are
separate and private.

## Time

Instants are `timestamptz` in UTC, and each player has an **IANA timezone**
name, never an offset. The browser's timezone only pre-fills the setup form.
Times are rendered in the viewer's zone.

## Notifications

An **outbox** row is written in the same transaction as its cause. The worker
sends it and records the result. A crash between those steps causes at worst a
duplicate, never a lost message. Channels are a Discord DM from the bot on the
TTA community server and email via `mail-relay`.

## Trust boundaries

- **Sign-in proves a Discord identity.** A TTA username is a *claim* until it
  is confirmed by email to the address CGE holds. Unverified players
  cannot act on CGE matches.
- **CSRF:** the sites and the API are same-site, so `SameSite` does not
  protect. Every state-changing request checks `Origin`.
- **Egress:** only `worker` and `api` (for the OAuth token exchange) reach the
  internet, and RFC1918 ranges are excluded.
- **Personal data** (Discord id and handle, optional email, TTA username,
  timezone) is **never logged at any level**.
- **Analytics:** the public site allows the beacon Cloudflare injects for
  its Web Analytics (`https://static.cloudflareinsights.com/beacon.min.js/`
  in the CSP's `script-src`, trailing slash included, since the beacon's
  path continues past it); it reports to the site's own origin. Cloudflare
  states that Web Analytics "does not collect or use your visitors'
  personal data" ([Cloudflare Web
  Analytics](https://developers.cloudflare.com/web-analytics/about/))

## What it sits on

The home k3s cluster managed by `picluster`: four Pi 4s (arm64) and
`unstableserver`/`wsl-laptop` (amd64), with Longhorn, Flux and the GitLab
runners. Both databases and the engine are pinned to **`unstableserver`**,
because the upstream DB is ~1.4 GB (unmeasured) and the engine is numpy-heavy.
TLS terminates at Cloudflare, and traffic arrives through a cloudflared tunnel.
