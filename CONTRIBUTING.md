# Contributing to remote.futrx

Everyone is welcome to contribute. The most important rule is simple:

> **Everything starts with an issue. No issue, no pull request.**

Every bug fix, feature, refactor, documentation change, and other improvement
must be connected to an open issue before work begins. The issue is where we
agree on the problem, keep decisions visible, and prevent duplicated work.

## How to open a new problem

First, [search the open issues](https://github.com/futrx-com/remote.futrx/issues)
to see whether the problem has already been reported.

- If an issue already exists, add any useful context there instead of opening
  a duplicate.
- If no issue exists, [open a new issue](https://github.com/futrx-com/remote.futrx/issues/new/choose)
  and choose the template that best matches the problem.
- Do not publish security vulnerabilities in a public issue. Follow the
  private reporting instructions in [SECURITY.md](SECURITY.md).

This rule also applies to every new feature and feature request. Before
starting a new feature, open a **Feature request** issue—even when the change
seems small. Explain the user problem, the result you want, and how you will
know the feature is complete. A pull request must not be the first place a new
feature is proposed.

Write the issue so that someone unfamiliar with the problem can understand it.
Include:

1. A clear title describing one problem.
2. What you were trying to do and why it matters.
3. What happened and what you expected instead.
4. Steps to reproduce the problem, if it is a bug.
5. What a successful result should look like.
6. Relevant screenshots, logs, versions, or links. Remove secrets and personal
   information first.

Keep one issue focused on one problem. If the work contains separate problems,
open separate issues so each can be discussed, assigned, and completed on its
own.

## Assign an issue to yourself

Anybody can claim an open issue, including first-time contributors. You do not
need repository access and you do not need to wait for a maintainer.

1. Open the issue you want to work on and check its current assignees.
2. Add a new comment containing exactly:

   ```text
   /assign
   ```

3. The bot will add you as an assignee and react to your comment with a
   thumbs-up.

You may self-assign any open issue. If somebody is already working on it,
coordinate with them in the issue before starting so effort is not duplicated.

## Connect every pull request to an issue

A pull request is the implementation of an issue, not the beginning of a new
discussion. Open and assign the issue first, then create your branch from
`qa` and make the change.

Every pull request must:

- target the `qa` branch;
- stay within the scope agreed in its issue; and
- include `Fixes #123` or `Closes #123` in the pull request description,
  replacing `123` with the real issue number.

This link is mandatory, including for small changes and draft pull requests.
**A pull request that is not connected to an issue will be rejected and closed
without review.**

## Architecture

remote.futrx has a Preact/Vite frontend, a Go backend, a catalog of installable
applications, and infrastructure scripts for installation and updates. Keep
changes inside the existing boundaries and follow the patterns in the code
around them. For deeper implementation details, read the
[development documentation](docs/dev/).

## Development setup

Use the commands below to build, test, and run the project locally. Before you
start, also read [AGENTS.md](AGENTS.md) for the repository-specific development
and QA workflow, including how to deploy and verify frontend, backend,
installation, and update changes.

### Prerequisites

- Go 1.25+
- Node.js 22.14+
- Linux with LXD only when running the complete application; builds and most
  tests also work without LXD

### Backend

Build and test the backend:

```bash
cd backend
go build ./...
go test ./...
go run ./cmd/remote
```

The backend listens on port `7682` by default. From the repository root, also
build and test the separate application-catalog module:

```bash
go build ./...
go test ./...
```

### Frontend

```bash
cd frontend
npm install
npm test
npm run dev
```

The frontend runs on port `5174` and proxies API and WebSocket requests to the
backend on port `7682`. Run `npm run build` before submitting frontend changes.
