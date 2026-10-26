# Contributing to ttacommunity

Thank you for helping. **Everything about working here is in
[`AGENTS.md`](AGENTS.md)**: getting started on Windows, Linux or macOS, how
we test, the safety rules, and how pull requests work. It is written for
people and coding agents alike. Please read it before changing anything.

The short version:

1. Start from an issue: open one through a form, or pick one. Ask when
   something is unclear, before building
2. Branch from `main` and write the failing test first
3. Run `scripts/ci.sh` before you push (`scripts\ci.ps1` on Windows)
4. Open a pull request with two sections, **What it does** and **How to
   test it**. A maintainer reviews and merges it

## Sign off your commits

If you are not one of the project's maintainers, sign off each commit with
`git commit -s`. It adds a last line in your git name and email:

```
Signed-off-by: Your Name <you@example.com>
```

With it you certify the [Developer Certificate of Origin](https://developercertificate.org/):
that you wrote the change, or have the right to submit it, under this
project's licenses (README, "License"). A check on each pull request fails
when a commit lacks it. To sign off commits you have already made, run
`git rebase --signoff main` (with `main` up to date), then
`git push --force-with-lease`.

The sign-off's email must be the commit's author email. To keep your address
private, use your GitHub noreply address for both (GitHub's email settings
show it).

## Conduct and security

Everyone taking part follows the [code of conduct](CODE_OF_CONDUCT.md).
Report a security problem privately, as [`SECURITY.md`](SECURITY.md) says,
never in a public issue.
