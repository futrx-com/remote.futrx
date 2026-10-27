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

# 0.16.1 is the narrowly scoped recovery patch for the release entrypoints
# broken in 0.16.0. It stays application-classified for healthy
# 0.16.0 hosts, while older hosts classify the cross-minor update themselves.
commit_file README.md 0.16.0 release-0.16.0
git -C "$TEST_REPO" tag 0.16.0
commit_file infra/install.sh fixed-installer hotfix-installer
commit_file infra/update.sh fixed-updater hotfix-updater
commit_file infra/steps/00-checkout.sh fixed-checkout-reexec hotfix-checkout-reexec
hotfix_commit="$(git -C "$TEST_REPO" rev-parse HEAD)"
git -C "$TEST_REPO" tag 0.16.1
output="$(cd "$TEST_REPO" && "$CLASSIFIER" 0.16.1)"
assert_output "$output" "kind=application"
assert_output "$output" "label=Application"
assert_output "$output" "previous=0.16.0"

# Any additional protected file still makes that exact release unsafe.
git -C "$TEST_REPO" tag -d 0.16.1 >/dev/null
commit_file infra/versions.env changed-pin unsafe-extra-change
git -C "$TEST_REPO" tag 0.16.1
if error="$(cd "$TEST_REPO" && "$CLASSIFIER" 0.16.1 2>&1)"; then
    fail "0.16.1 recovery exception accepted an unrelated protected change"
fi
grep -Fq "infra/versions.env" <<<"$error" || \
    fail "0.16.1 recovery rejection did not identify the extra protected path"

# The exception ends at 0.16.1; future updater changes require a minor bump.
git -C "$TEST_REPO" tag -d 0.16.1 >/dev/null
git -C "$TEST_REPO" reset --hard -q "$hotfix_commit"
git -C "$TEST_REPO" tag 0.16.1
commit_file infra/update.sh future-updater future-protected-change
git -C "$TEST_REPO" tag 0.16.2
if error="$(cd "$TEST_REPO" && "$CLASSIFIER" 0.16.2 2>&1)"; then
    fail "0.16.1 recovery exception leaked into a later patch release"
fi
grep -Fq "infra/update.sh" <<<"$error" || \
    fail "later patch rejection did not identify the updater change"

if error="$(cd "$TEST_REPO" && "$CLASSIFIER" 0.4 2>&1)"; then
    fail "malformed release tag was accepted"
fi
grep -Fq "release tags must use MAJOR.MINOR.PATCH (got: 0.4)" <<<"$error" \
    || fail "malformed-tag rejection did not explain the failure"

echo "Release classification tests passed"
