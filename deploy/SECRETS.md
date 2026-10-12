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
| `ttacommunity-tunnel` | `credentials.json` | `cloudflared` | The `ttacommunity` tunnel's credentials, from `cloudflared tunnel create ttacommunity` (`~/.cloudflared/<tunnel id>.json`) |
| `ratings-dev-engine` | `database-url` | `engine-dev`, `api-dev`'s `migrate` init container | The dev engine's connection URL, as `ratings_dev`, owner of `ratings_dev` only; the dev api's init container applies the migrations with it |
| `ratings-dev-api` | `password`, `database-url` | `api-dev`, `scripts/seed-dev-db.sh` | The dev api's read-only `ratings_dev_api` login |
| `ratings-dev-admin` | `password`, `database-url` | `api-dev` | The dev api's admin login `ratings_dev_admin_api`, a member of `ratings_admin` (writes only what admin edits) |
| `ttacommunity-dev-tunnel` | `credentials.json` | `cloudflared-dev` | The `ttacommunity-dev` tunnel's credentials, from `cloudflared tunnel create ttacommunity-dev` |
| `ttac-api-tunnel` | `credentials.json` | `ttac-api-tunnel` | The new Cloudflare account's `ttac-api` tunnel's credentials (below, "The new account's tunnels") |
| `ttac-api-dev-tunnel` | `credentials.json` | `ttac-api-dev-tunnel` | The new Cloudflare account's `ttac-api-dev` tunnel's credentials (below) |
| `ttacommunity-git-auth`, **in `flux-system`** | `username`, `password` | Flux | Reads this repo. Deploy token `flux-git-read`, `read_repository` only |

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

### The new account's tunnels

`ttac-api-tunnel` and `ttac-api-dev-tunnel` hold the credentials of the new
Cloudflare account's tunnels. `cloudflared tunnel create` needs a zone in the
account, which it does not have yet, so they were made through Cloudflare's
API instead: locally managed (`config_src: local`, so their routes come from
`cloudflare-tunnel.yaml`), each with a random 32-byte secret. The Secret holds
`{"AccountTag", "TunnelSecret", "TunnelID"}`, the same shape as the older ones.
To recreate one:

1. Create a new tunnel the same way, with an account API token that has
   **Cloudflare One Connector: cloudflared** (Edit)
   ([Cloudflare's guide](https://developers.cloudflare.com/cloudflare-one/networks/connectors/cloudflare-tunnel/get-started/create-remote-tunnel-api/))
2. Create the Secret from that JSON on stdin, as above
3. Put the new tunnel id in `cloudflare-tunnel.yaml`, and point the DNS
   records at it once its hostnames exist
