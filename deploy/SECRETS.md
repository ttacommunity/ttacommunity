# Secrets in the `ttacommunity` namespace

Every Secret here is created by hand and is never in git. This file lists them
and says how to recreate each one; it never holds a value. Update it in the
same change that adds or removes a Secret.

Pipe values in; never use `--from-literal`, which leaves them in shell history.

| Secret | Keys | Used by | What it is |
|---|---|---|---|
| `ratings-db` | `password` | `ratings-db` | Password of `ratings`, the ratings database's owner |
| `ratings-engine` | `database-url` | `engine` | The engine's connection URL, as `ratings` |
| `ratings-api` | `password`, `database-url` | `api`, `scripts/seed-cluster-db.sh` | The read-only `ratings_api` login |
| `gitlab-registry` | `.dockerconfigjson` | `api`, `engine` | Pulls images from the project's GitLab registry. Deploy token `cluster-image-pull` |
| `ttacommunity-tunnel` | `credentials.json` | `cloudflared` | The `ttacommunity` tunnel's credentials, from `cloudflared tunnel create ttacommunity` (`~/.cloudflared/<tunnel id>.json`) |
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

`gitlab-registry` holds a **project deploy token with `read_registry` only**
(GitLab → Settings → Repository → Deploy tokens):

```sh
read -rp 'deploy token username: ' user
read -rsp 'deploy token: ' token; echo
printf '{"auths":{"registry.gitlab.com":{"auth":"%s"}}}' "$(printf '%s:%s' "$user" "$token" | base64 -w0)" |
  kubectl -n ttacommunity create secret generic gitlab-registry \
    --type=kubernetes.io/dockerconfigjson --from-file=.dockerconfigjson=/dev/stdin
unset token
```
