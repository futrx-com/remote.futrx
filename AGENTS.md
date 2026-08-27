# QA workflows

Run commands from the repository root. QA settings come from `.qa.env`; never commit credentials or disable strict SSH host checks.

Before deploying a ref, ensure it is pushed, checked out locally, and the
tracked working tree is clean. Scripts resolve branches, tags, and commits to an
immutable SHA.

## Test a fresh installation

Use a rebuilt Ubuntu QA VM. Deleting `/opt/remote.futrx` is not a clean reset
because packages, services, Caddy, and LXD may remain.

```bash
# Public installer from main
bash infra/qa/install.sh

# Exact pushed branch, tag, or commit
bash infra/qa/install.sh <ref>
```

The script refuses an existing installation and verifies the deployed SHA,
service, and public URL. After rebuilding the VM, verify its provider identity
before replacing its stale `known_hosts` entry.

## Test a new update

Run against an existing installation:

```bash
bash infra/qa/update.sh <ref>
```

This tests the production updater: host dependencies, agent CLIs, application,
base image, and idle workspace recycling. It takes several minutes. Use it
before releases and for changes to `infra/`, versions, Caddy, systemd, LXD, or
workspace images. Do not clear the server first.

## Test a deployment

Use for normal frontend/backend development:

```bash
bash infra/qa/deploy-app.sh <ref>
```

This builds frontend and backend on QA, restarts `remote.futrx.service`, checks
health, and restores the previous binary on failure. It does not update host
dependencies, infrastructure, the base image, or project containers, so it is
not a substitute for testing the complete updater.

## Suggested cycle

1. Commit and push the candidate.
2. Iterate with `deploy-app.sh`.
3. Test `install.sh` on a rebuilt VM if installer behavior changed.
4. For a patch release, finish with `deploy-app.sh` against an existing installation.
5. For a major/minor release, run `update.sh` against an existing installation.
6. Release only the verified commit. Tags use `MAJOR.MINOR.PATCH`; infrastructure changes require a major/minor bump.
