# Secrets in the `ttacommunity` namespace

Every Secret here is created by hand and is never in git. This file lists them
and says how to recreate each one; it never holds a value. Update it in the
same change that adds or removes a Secret.

Pipe values in; never use `--from-literal`, which leaves them in shell history.

| Secret | Keys | Used by | What it is |
|---|---|---|---|
| `ratings-db` | `password` | `ratings-db` | Password of `ratings`, the ratings database's owner |
| `ratings-engine` | `database-url` | `engine`, `api`'s `migrate` init container | The engine's connection URL, as `ratings`; the api's init container applies the migrations with it |
| `ratings-api` | `password`, `database-url` | `api`, `scripts/seed-cluster-db.sh` | The read-only `ratings_api` login |
| `ratings-admin` | `password`, `database-url` | `api` | The api's admin login `ratings_admin_api`, a member of `ratings_admin` (writes only what admin edits) |
| `ghcr` | `.dockerconfigjson` | `api`, `engine` | Pulls the private images from GHCR. A machine account's classic token with `read:packages` only |
| `ratings-dev-engine` | `database-url` | `engine-dev`, `api-dev`'s `migrate` init container | The dev engine's connection URL, as `ratings_dev`, owner of `ratings_dev` only; the dev api's init container applies the migrations with it |
| `ratings-dev-api` | `password`, `database-url` | `api-dev`, `scripts/seed-dev-db.sh` | The dev api's read-only `ratings_dev_api` login |
| `ratings-dev-admin` | `password`, `database-url` | `api-dev` | The dev api's admin login `ratings_dev_admin_api`, a member of `ratings_admin` (writes only what admin edits) |
| `ratings-player` | `password`, `database-url` | `api` | The api's player login `ratings_player_api`, a member of `ratings_player` (sign-ins, sessions, requests to be linked, a member's own country and zone) |
| `ratings-dev-player` | `password`, `database-url` | `api-dev` | The dev api's player login `ratings_dev_player_api`, a member of `ratings_player` |
| `discord-signin` | `client-secret` | `api`, `api-dev` | The client secret of the players' Discord application, whose id is `DISCORD_CLIENT_ID` in `api.yaml` (not the Access sign-in Worker's app) |
| `email-signin` | `resend-api-key` | `api`, `api-dev` | Resend's API key for the sign-in emails: sending access, restricted to ttacommunity.org |
| `turnstile-signin` | `secret` | `api`, `api-dev` | The secret of the Turnstile widget "ttacommunity email sign-in", whose sitekey is the site's `PUBLIC_TURNSTILE_SITEKEY` (README) |
| `ratings-backup` | `password`, `database-url` | `backup` | The backup job's login `ratings_backup_job`, a member of `ratings_backup` (reads the backed-up tables, writes nothing) |
| `backup-r2` | `endpoint`, `access-key-id`, `secret-access-key` | `backup` | The R2 bucket `ttac-backups`: the account's S3 endpoint, and an R2 token that can write objects in that bucket only |
| `ttac-api-tunnel` | `credentials.json` | `ttac-api-tunnel` | The `ttac-api` tunnel's credentials (below, "The tunnels") |
| `ttac-api-dev-tunnel` | `credentials.json` | `ttac-api-dev-tunnel` | The `ttac-api-dev` tunnel's credentials (below) |
| `ttacommunity-github-app`, **in `flux-system`** | `githubAppID`, `githubAppInstallationID`, `githubAppPrivateKey` | Flux | Reads this repo as the GitHub App `ttacommunity-flux` (contents and metadata, read only). The cluster's maintainers hold it |

## If one is lost

None needs keeping in a password manager: each is either made new on a
rebuild or reissued by the service it comes from.

| Secret | How it comes back |
|---|---|
| `ratings-db`, `ratings-engine` | **New on a rebuild**, before the database's first start (below). On the running database, Postgres keeps the old password whatever the Secret says: set a new one with `ALTER ROLE ratings PASSWORD …` inside the pod, then replace both Secrets |
| `ratings-api`, `ratings-admin`, `ratings-player`, and the `ratings-dev-*` logins | **New on a rebuild**, or at any time: a new password with `ALTER ROLE`, then the Secret (below) |
| `discord-signin` | **Reissued** from Discord's developer portal (Reset Secret) |
| `email-signin` | **Reissued**: a new key in Resend's dashboard (API Keys: sending access, domain ttacommunity.org), then delete the old one |
| `turnstile-signin` | **Reissued**: `cf turnstile widgets rotate-secret <sitekey>`, piped straight into the Secret as below |
| `ttac-api-tunnel`, `ttac-api-dev-tunnel` | **Reissued**: a new tunnel on the Cloudflare account (below, "The tunnels") |
| `ratings-backup` | **New on a rebuild**, or at any time: a new password with `ALTER ROLE`, then the Secret (below, "Backups") |
| `backup-r2` | **Reissued**: a new R2 token for the bucket, then the Secret (below, "Backups"); delete the old token |
| `ghcr`, `ttacommunity-github-app` | **Reissued** by the cluster's maintainers, who issued them |

## Creating them

The ratings database's two Secrets share one password, and must exist before
its first start: Postgres sets the owner's password only when it creates the
database. Changing the Secret later does not change the password.

```sh
ns=(-n ttacommunity)
pw=$(openssl rand -hex 24)
printf %s "$pw" | kubectl "${ns[@]}" create secret generic ratings-db --from-file=password=/dev/stdin
printf 'postgres://ratings:%s@ratings-db:5432/ratings?sslmode=disable' "$pw" |
  kubectl "${ns[@]}" create secret generic ratings-engine --from-file=database-url=/dev/stdin

pw=$(openssl rand -hex 24)
printf 'postgres://ratings_api:%s@ratings-db:5432/ratings?sslmode=disable' "$pw" > /dev/shm/url
printf %s "$pw" | kubectl "${ns[@]}" create secret generic ratings-api \
  --from-file=password=/dev/stdin --from-file=database-url=/dev/shm/url
rm /dev/shm/url
unset pw
```

`scripts/seed-cluster-db.sh` gives `ratings_api` the password in `ratings-api`.

The dev site's (`deploy/dev/`). `scripts/seed-dev-db.sh` then creates both
logins with these passwords, and `ratings_dev` as a copy of `ratings`:

```sh
ns=(-n ttacommunity)
pw=$(openssl rand -hex 24)
printf 'postgres://ratings_dev:%s@ratings-db:5432/ratings_dev?sslmode=disable' "$pw" |
  kubectl "${ns[@]}" create secret generic ratings-dev-engine --from-file=database-url=/dev/stdin

pw=$(openssl rand -hex 24)
printf 'postgres://ratings_dev_api:%s@ratings-db:5432/ratings_dev?sslmode=disable' "$pw" > /dev/shm/url
printf %s "$pw" | kubectl "${ns[@]}" create secret generic ratings-dev-api \
  --from-file=password=/dev/stdin --from-file=database-url=/dev/shm/url
rm /dev/shm/url
unset pw
```

The dev api's admin login. The role `ratings_admin` must exist first (as the
database's superuser: `CREATE ROLE ratings_admin NOLOGIN`); the password goes
in on stdin:

```sh
ns=(-n ttacommunity)
umask 077
pw=$(openssl rand -hex 24); printf %s "$pw" > /dev/shm/pw
printf 'postgres://ratings_dev_admin_api:%s@ratings-db:5432/ratings_dev?sslmode=disable' "$pw" > /dev/shm/url
unset pw
{ cat /dev/shm/pw; echo; cat <<'SQL'; } | kubectl "${ns[@]}" exec -i deploy/ratings-db -- \
  sh -c 'read -r PW; psql -q -v ON_ERROR_STOP=1 -v pw="$PW" -U ratings -d postgres'
DO $$ BEGIN CREATE ROLE ratings_dev_admin_api LOGIN; EXCEPTION WHEN duplicate_object THEN NULL; END $$;
SELECT format('ALTER ROLE ratings_dev_admin_api PASSWORD %L', :'pw') \gexec
GRANT ratings_admin TO ratings_dev_admin_api;
SQL
kubectl "${ns[@]}" create secret generic ratings-dev-admin \
  --from-file=password=/dev/shm/pw --from-file=database-url=/dev/shm/url
rm /dev/shm/pw /dev/shm/url
```

The api's admin login is made the same way, with `ratings_admin_api` for
`ratings_dev_admin_api`, `ratings` for `ratings_dev` in the URL, and Secret
`ratings-admin`. Only listed logins may connect to `ratings`, so its SQL also
has `GRANT CONNECT ON DATABASE ratings TO ratings_admin_api;`.

### Player sign-in

The player logins are made as the admin ones are. The role `ratings_player`
is per server, and the migration that grants to it can create it only when
the migrating login may create roles, so make it here first. For dev:

```sh
ns=(-n ttacommunity)
umask 077
pw=$(openssl rand -hex 24); printf %s "$pw" > /dev/shm/pw
printf 'postgres://ratings_dev_player_api:%s@ratings-db:5432/ratings_dev?sslmode=disable' "$pw" > /dev/shm/url
unset pw
{ cat /dev/shm/pw; echo; cat <<'SQL'; } | kubectl "${ns[@]}" exec -i deploy/ratings-db -- \
  sh -c 'read -r PW; psql -q -v ON_ERROR_STOP=1 -v pw="$PW" -U ratings -d postgres'
DO $$ BEGIN CREATE ROLE ratings_player NOLOGIN; EXCEPTION WHEN duplicate_object THEN NULL; END $$;
DO $$ BEGIN CREATE ROLE ratings_dev_player_api LOGIN; EXCEPTION WHEN duplicate_object THEN NULL; END $$;
SELECT format('ALTER ROLE ratings_dev_player_api PASSWORD %L', :'pw') \gexec
GRANT ratings_player TO ratings_dev_player_api;
SQL
kubectl "${ns[@]}" create secret generic ratings-dev-player \
  --from-file=password=/dev/shm/pw --from-file=database-url=/dev/shm/url
rm /dev/shm/pw /dev/shm/url
```

Production's is the same with `ratings_player_api`, `ratings` for
`ratings_dev` in the URL, Secret `ratings-player`, and
`GRANT CONNECT ON DATABASE ratings TO ratings_player_api;`.

`discord-signin` is the players' application's client secret, from
Discord's developer portal (the application's OAuth2 page, Reset Secret).
Its redirects must include each api's `/v1/auth/discord/callback`:

```sh
read -rsp 'Discord client secret: ' secret; echo
printf %s "$secret" | kubectl -n ttacommunity create secret generic discord-signin --from-file=client-secret=/dev/stdin
unset secret
```

`email-signin` is a Resend API key with sending access only, restricted to
the domain ttacommunity.org (Resend's dashboard, API Keys):

```sh
read -rsp 'Resend API key: ' key; echo
printf %s "$key" | kubectl -n ttacommunity create secret generic email-signin --from-file=resend-api-key=/dev/stdin
unset key
```

`turnstile-signin` is the Turnstile widget's secret. The widget is made once
with `cf turnstile widgets create`, hostnames ttacommunity.org and
dev.ttacommunity.org; a lost secret is rotated. Pipe the answer's `secret`
straight into the Secret, never to the screen:

```sh
cf turnstile widgets rotate-secret <sitekey> | python3 -c 'import json,sys; d=json.load(sys.stdin); print(d.get("result", d)["secret"], end="")' \
  | kubectl -n ttacommunity create secret generic turnstile-signin --from-file=secret=/dev/stdin --dry-run=client -o yaml | kubectl apply -f -
```

`ghcr` holds a **machine account's classic token with `read:packages` only**
(a personal token would tie the cluster to one person):

```sh
read -rp 'machine account username: ' user
read -rsp 'token: ' token; echo
printf '{"auths":{"ghcr.io":{"auth":"%s"}}}' "$(printf '%s:%s' "$user" "$token" | base64 -w0)" |
  kubectl -n ttacommunity create secret generic ghcr \
    --type=kubernetes.io/dockerconfigjson --from-file=.dockerconfigjson=/dev/stdin
unset token
```

### Backups

The backup job's login is made as the player logins are, with the role
`ratings_backup` (made by its migration when the migrating login may create
roles, so make it here first):

```sh
ns=(-n ttacommunity)
umask 077
pw=$(openssl rand -hex 24); printf %s "$pw" > /dev/shm/pw
printf 'postgres://ratings_backup_job:%s@ratings-db:5432/ratings?sslmode=disable' "$pw" > /dev/shm/url
unset pw
{ cat /dev/shm/pw; echo; cat <<'SQL'; } | kubectl "${ns[@]}" exec -i deploy/ratings-db -- \
  sh -c 'read -r PW; psql -q -v ON_ERROR_STOP=1 -v pw="$PW" -U ratings -d postgres'
DO $$ BEGIN CREATE ROLE ratings_backup NOLOGIN; EXCEPTION WHEN duplicate_object THEN NULL; END $$;
DO $$ BEGIN CREATE ROLE ratings_backup_job LOGIN; EXCEPTION WHEN duplicate_object THEN NULL; END $$;
SELECT format('ALTER ROLE ratings_backup_job PASSWORD %L', :'pw') \gexec
GRANT ratings_backup TO ratings_backup_job;
GRANT CONNECT ON DATABASE ratings TO ratings_backup_job;
SQL
kubectl "${ns[@]}" create secret generic ratings-backup \
  --from-file=password=/dev/shm/pw --from-file=database-url=/dev/shm/url
rm /dev/shm/pw /dev/shm/url
```

### The tunnels

`ttac-api-tunnel` and `ttac-api-dev-tunnel` hold the credentials of the
tunnels `ttac-api` and `ttac-api-dev`. They were made through Cloudflare's
API: locally managed (`config_src: local`, so their routes come from
`cloudflare-tunnel.yaml`), each with a random 32-byte secret. The Secret holds
`{"AccountTag", "TunnelSecret", "TunnelID"}`, the shape `cloudflared tunnel
create` writes. To recreate one:

1. Create a new tunnel the same way, with an account API token that has
   **Cloudflare One Connector: cloudflared** (Edit)
   ([Cloudflare's guide](https://developers.cloudflare.com/cloudflare-one/networks/connectors/cloudflare-tunnel/get-started/create-remote-tunnel-api/))
2. Create the Secret from that JSON on stdin, as above
3. Put the new tunnel id in `cloudflare-tunnel.yaml`, and point the DNS
   records at it once its hostnames exist
