# Vendored: Erisa/discord-oidc-worker

Copied from <https://github.com/Erisa/discord-oidc-worker> at commit
`ba7b6843b0420e61034861e4e5c02dc9d05c88d9` (MIT, see `LICENSE`). It lets
Cloudflare Access sign people in with Discord.

It mints the identity Access trusts, so it is reviewed as code we own: every
change here, including an upstream update, is read as a diff before it is
deployed. Our changes since the copy are the commits after the one that added
this file (`git log -- infra/access-oidc`).

Where this differs from upstream's `README.md`: there is no `config.json`.
`wrangler.jsonc` holds the settings (`CLIENT_ID`, `REDIRECT_URL`, the KV
namespace, the hostname), `DISCORD_CLIENT_SECRET` is a Worker secret, and
Workers Builds deploys it from `main`. The token endpoint also requires the
client's id and secret, and `npm test` runs the tests in `test/`.
