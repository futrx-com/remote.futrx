#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" >/dev/null 2>&1 && pwd)"
CLASSIFIER="$SCRIPT_DIR/classify-release.sh"
TEST_REPO="$(mktemp -d)"
trap 'rm -rf "$TEST_REPO"' EXIT

fail() {
    echo "FAIL: $*" >&2
    exit 1
}

commit_file() {
    local path="$1" contents="$2" message="$3"
    mkdir -p "$TEST_REPO/$(dirname "$path")"
    printf '%s\n' "$contents" > "$TEST_REPO/$path"
    git -C "$TEST_REPO" add "$path"
    git -C "$TEST_REPO" commit -q -m "$message"
}

assert_output() {
    local output="$1" expected="$2"
    grep -Fxq "$expected" <<<"$output" || fail "missing output: $expected"
}

git -C "$TEST_REPO" init -q
git -C "$TEST_REPO" config user.name "Release Test"
git -C "$TEST_REPO" config user.email "release-test@example.invalid"

commit_file README.md initial initial
git -C "$TEST_REPO" tag 0.3.1
output="$(cd "$TEST_REPO" && "$CLASSIFIER" 0.3.1)"
assert_output "$output" "kind=infrastructure"
assert_output "$output" "label=Infrastructure"
assert_output "$output" "previous="

commit_file frontend/app.ts patch application
git -C "$TEST_REPO" tag 0.3.2
output="$(cd "$TEST_REPO" && "$CLASSIFIER" 0.3.2)"
assert_output "$output" "kind=application"
assert_output "$output" "label=Application"
assert_output "$output" "previous=0.3.1"

commit_file infra/versions.env protected infrastructure
git -C "$TEST_REPO" tag 0.3.3
if error="$(cd "$TEST_REPO" && "$CLASSIFIER" 0.3.3 2>&1)"; then
    fail "protected infrastructure change was accepted as an application release"
fi
grep -Fq "patch release 0.3.3 changes infrastructure-managed paths:" <<<"$error" \
    || fail "protected-path rejection did not explain the failure"

commit_file backend/cmd/install-host-agents/main.go host-installer protected-host-installer
git -C "$TEST_REPO" tag 0.3.4
if error="$(cd "$TEST_REPO" && "$CLASSIFIER" 0.3.4 2>&1)"; then
    fail "host installer change was accepted as an application release"
fi
grep -Fq "backend/cmd/install-host-agents/main.go" <<<"$error" || \
    fail "host installer rejection did not identify the protected path"

commit_file backend/internal/integration/agents/future/profile.go future-profile protected-future-profile
git -C "$TEST_REPO" tag 0.3.5
if error="$(cd "$TEST_REPO" && "$CLASSIFIER" 0.3.5 2>&1)"; then
    fail "future agent profile was accepted as an application release"
fi
grep -Fq "backend/internal/integration/agents/future/profile.go" <<<"$error" || \
    fail "future agent profile rejection did not identify the protected path"

commit_file backend/internal/integration/agents/future/install_linux.go future-installer protected-future-installer
git -C "$TEST_REPO" tag 0.3.6
if error="$(cd "$TEST_REPO" && "$CLASSIFIER" 0.3.6 2>&1)"; then
    fail "future agent install helper was accepted as an application release"
fi
grep -Fq "backend/internal/integration/agents/future/install_linux.go" <<<"$error" || \
    fail "future agent install-helper rejection did not identify the protected path"

commit_file backend/internal/integration/agents/future/factory.go future-factory protected-future-factory
git -C "$TEST_REPO" tag 0.3.7
if error="$(cd "$TEST_REPO" && "$CLASSIFIER" 0.3.7 2>&1)"; then
    fail "future agent factory was accepted as an application release"
fi
grep -Fq "backend/internal/integration/agents/future/factory.go" <<<"$error" || \
    fail "future agent factory rejection did not identify the protected path"

commit_file backend/internal/service/agent/module/catalog.go module-contract protected-module-contract
git -C "$TEST_REPO" tag 0.3.8
if error="$(cd "$TEST_REPO" && "$CLASSIFIER" 0.3.8 2>&1)"; then
    fail "agent module contract was accepted as an application release"
fi
grep -Fq "backend/internal/service/agent/module/catalog.go" <<<"$error" || \
    fail "agent module rejection did not identify the protected path"

commit_file backend/internal/config/agents.go agent-composition protected-agent-composition
git -C "$TEST_REPO" tag 0.3.9
if error="$(cd "$TEST_REPO" && "$CLASSIFIER" 0.3.9 2>&1)"; then
    fail "agent composition was accepted as an application release"
fi
grep -Fq "backend/internal/config/agents.go" <<<"$error" || \
    fail "agent composition rejection did not identify the protected path"

commit_file README.md next-minor minor
git -C "$TEST_REPO" tag 0.4.0
output="$(cd "$TEST_REPO" && "$CLASSIFIER" 0.4.0)"
assert_output "$output" "kind=infrastructure"
assert_output "$output" "label=Infrastructure"
assert_output "$output" "previous=0.3.9"

if error="$(cd "$TEST_REPO" && "$CLASSIFIER" 0.4 2>&1)"; then
    fail "malformed release tag was accepted"
fi
grep -Fq "release tags must use MAJOR.MINOR.PATCH (got: 0.4)" <<<"$error" \
    || fail "malformed-tag rejection did not explain the failure"

echo "Release classification tests passed"
