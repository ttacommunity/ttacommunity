# Security policy

Please don't report a vulnerability in a public issue, pull request or
comment. Report it privately, one of two ways:

- **On GitHub:** [report a vulnerability](https://github.com/ttacommunity/ttacommunity/security/advisories/new)
  (the repository's Security tab)
- **By email:** support@ttacommunity.org, with "Security" at the start of the
  subject. Only the maintainers read it

Say what you found, where (a URL, or a file and line), and how to reproduce
it. We are volunteers: we reply as soon as we can, tell you when it is fixed,
and credit you in the fix if you would like.

## What this covers

The code in this repository, and the sites it runs: ttacommunity.org, its
API and its admin site. Fixes go to `main`, which is what runs; there are no
older versions to patch.

Problems with Through the Ages' own site or game servers go to its
publisher, Czech Games Edition.

## Testing

Test against your own local copy ([`AGENTS.md`](AGENTS.md), "Getting
started"), never the live sites: no scanners or load tests against them, and
no reading other people's data.
