#!/usr/bin/env bash
# Copyright (c) Abstract Machines
# SPDX-License-Identifier: Apache-2.0
#
# Propeller end-to-end integration test.
#
# Brings the stack up the same way the getting-started guide does — base
# services, provisioning, Propeller — then deploys the addition example and
# asserts the task ran to completion with the right result and that the
# service logs show the expected flow. Exits non-zero on the first failed
# assertion, after dumping container logs.
#
# Everything runs against images built from the working tree
# (`make ci-images` + `make start-propeller-ci`), not the published ghcr.io
# images, so a pull request is tested against its own code.
#
# Usage:
#   tools/ci/integration-test.sh
#
# Environment:
#   PROPELLER_CI_SKIP_BUILD=1  reuse the existing propeller-ci/* images
#   CI_ATOM_IDENTIFIER         Atom login    (default: admin)
#   CI_ATOM_SECRET             Atom password (default: $ATOM_ADMIN_SECRET in docker/.env)
#   CI_KEEP_STACK=1            leave the containers running on exit

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$REPO_ROOT"

MANAGER_URL="${CI_MANAGER_URL:-http://localhost:7070}"
ATOM_URL="${CI_ATOM_URL:-http://localhost:8080}"
TASK_NAME="add"
TASK_INPUTS='[10, 20]'
EXPECTED_RESULT="${CI_EXPECTED_RESULT:-30}"

# Task states (pkg/task.State): 0 Pending, 1 Scheduled, 2 Running, 3 Completed.
STATE_COMPLETED=3

log()   { printf '\n\033[1;34m==> %s\033[0m\n' "$*"; }
info()  { printf '    %s\n' "$*"; }
pass()  { printf '\033[1;32m    PASS\033[0m %s\n' "$*"; }

fail() {
    printf '\n\033[1;31m==> FAIL: %s\033[0m\n' "$*" >&2
    exit 1
}

# dump_diagnostics prints enough state to debug a failure from the CI log
# alone: container status, then the tail of every service's logs.
dump_diagnostics() {
    {
        echo "----- docker ps -a -----"
        docker ps -a --format '{{.Names}}\t{{.Image}}\t{{.Status}}' || true
        for container in propeller-atom propeller-fluxmq-node1 propeller-nginx \
            propeller-manager propeller-proplet propeller-proxy; do
            echo "----- $container -----"
            docker logs --tail 500 "$container" 2>&1 || true
        done
    } >&2
}

cleanup() {
    local status=$?
    if [ "$status" -ne 0 ]; then
        dump_diagnostics
    fi
    if [ "${CI_KEEP_STACK:-0}" = "1" ]; then
        info "CI_KEEP_STACK=1, leaving containers running"
    else
        log "Tearing down"
        make stop-propeller-ci >/dev/null 2>&1 || true
        make stop-base >/dev/null 2>&1 || true
    fi
    exit "$status"
}
trap cleanup EXIT

# wait_for_url polls a URL until it answers 2xx, up to a timeout in seconds.
# It must issue a real request: `test "$url"` only checks the string is
# non-empty, so it returns success immediately and lets the script race the
# service's startup.
wait_for_url() {
    local what="$1" url="$2" timeout="$3"
    local deadline=$((SECONDS + timeout))

    until curl -sSf -o /dev/null --max-time 2 "$url"; do
        if (( SECONDS >= deadline )); then
            echo "----- docker ps -a -----" >&2
            docker ps -a --format '{{.Names}}\t{{.Image}}\t{{.Status}}' >&2 || true
            fail "$what ($url) did not become ready within ${timeout}s"
        fi
        sleep 2
    done
    pass "$what ready"
}

# assert_eq compares an actual value against an expected one.
assert_eq() {
    local what="$1" expected="$2" actual="$3"
    if [ "$actual" != "$expected" ]; then
        fail "$what: expected '$expected', got '$actual'"
    fi
    pass "$what = $actual"
}

# assert_log_contains fails unless the container's logs contain the pattern.
# The logs are captured into a variable first: piping `docker logs` straight
# into `grep -q` makes grep exit on the first match, docker logs die of
# SIGPIPE, and `set -o pipefail` then reports the whole pipeline as a failure
# even though the pattern was found.
assert_log_contains() {
    local container="$1" pattern="$2" what="$3"
    local logs
    logs="$(docker logs "$container" 2>&1 || true)"
    if ! grep -qF -- "$pattern" <<<"$logs"; then
        fail "$what (no '$pattern' in $container logs)"
    fi
    pass "$what"
}

# assert_log_clean fails if a container logged an ERROR or panicked. WARN is
# allowed: a stock stack logs some (unconfigured FL coordinator, no local data
# store) and those are not failures.
assert_log_clean() {
    local container="$1"
    local logs offending
    logs="$(docker logs "$container" 2>&1 || true)"
    offending="$(grep -E '"level":"ERROR"|panic:|fatal runtime error' <<<"$logs" || true)"
    if [ -n "$offending" ]; then
        printf '%s\n' "$offending" | head -20 >&2
        fail "$container logged an error or panicked"
    fi
    pass "$container logged no errors"
}

json() { curl -sS --max-time 10 "$@"; }

# ---------------------------------------------------------------------------
# 1. Build the images under test
# ---------------------------------------------------------------------------

if [ "${PROPELLER_CI_SKIP_BUILD:-0}" != "1" ]; then
    log "Building images under test"
    make ci-images
fi

[ -x ./build/cli ] || fail "build/cli is missing — run without PROPELLER_CI_SKIP_BUILD=1"
[ -f build/addition.wasm ] || fail "build/addition.wasm is missing — run without PROPELLER_CI_SKIP_BUILD=1"

# ---------------------------------------------------------------------------
# 2. Start the base services (Atom, FluxMQ, Nginx) and wait for Atom
# ---------------------------------------------------------------------------

log "Starting base services"
make start-base
wait_for_url "atom" "${ATOM_URL}/health" 240

# ---------------------------------------------------------------------------
# 3. Provision Atom resources (tenant, entities, channel) -> config.toml
# ---------------------------------------------------------------------------

log "Provisioning"
# The Atom admin credentials come from docker/.env so there is a single
# source of truth for them.
ATOM_SECRET="$(sed -n 's/^ATOM_ADMIN_SECRET=//p' docker/.env | tail -1)"
[ -n "$ATOM_SECRET" ] || fail "could not read ATOM_ADMIN_SECRET from docker/.env"

# Start from a clean slate: both paths are gitignored, so a CI checkout has
# neither, but a local run may have leftovers from a previous run and a stale
# file would make a failed provisioning look like a success.
CONFIG=config.toml
rm -f "$CONFIG" docker/config.toml

PROPELLER_ATOM_IDENTIFIER="${CI_ATOM_IDENTIFIER:-admin}" \
PROPELLER_ATOM_SECRET="${CI_ATOM_SECRET:-$ATOM_SECRET}" \
PROPELLER_TENANT_NAME="${CI_TENANT_NAME:-propeller-ci}" \
PROPELLER_PROPLET_COUNT=1 \
    ./build/cli provision

# `provision` writes config.toml into the working directory, which is the repo
# root. The compose file bind-mounts ./docker/config.toml, so it is copied
# there below.
[ -f "$CONFIG" ] || fail "provision did not produce $CONFIG"

for section in manager proplet proxy; do
    grep -q "^\[${section}\]$" "$CONFIG" || fail "config.toml is missing the [$section] section"
done
# One non-empty api_key per service identity.
api_keys="$(grep -c '^api_key = "[^"]\+"' "$CONFIG" || true)"
assert_eq "config.toml api_key count" "3" "$api_keys"

cp "$CONFIG" docker/config.toml

# ---------------------------------------------------------------------------
# 4. Start Propeller and wait for the proplet to register
# ---------------------------------------------------------------------------

log "Starting Propeller"
make start-propeller-ci
wait_for_url "manager" "${MANAGER_URL}/health" 180

log "Waiting for the proplet to register"
# The proplet registers over MQTT after connecting to Atom/FluxMQ, so give the
# registration a generous window. The count defaults to 0 so a transient
# empty or non-JSON response from the manager is treated as "not yet" rather
# than blowing up the integer comparison.
alive_proplets() {
    local count
    count="$(json "${MANAGER_URL}/proplets" 2>/dev/null |
        jq -r '[.proplets[]? | select(.alive == true)] | length' 2>/dev/null)" || count=""

    printf '%s' "${count:-0}"
}

proplet_deadline=$((SECONDS + 180))
until [ "$(alive_proplets)" -ge 1 ]; do
    if (( SECONDS >= proplet_deadline )); then
        fail "no proplet registered as alive within 180s"
    fi
    sleep 2
done
pass "proplet registered and alive"

# ---------------------------------------------------------------------------
# 5. Deploy the addition example
# ---------------------------------------------------------------------------

log "Deploying the addition example"
TASK_ID="$(json -X POST "${MANAGER_URL}/tasks" \
    -H 'Content-Type: application/json' \
    -d "{\"name\":\"${TASK_NAME}\",\"inputs\":${TASK_INPUTS}}" | jq -r '.id // empty')"
[ -n "$TASK_ID" ] || fail "could not create task"
info "task id: $TASK_ID"

# Assert the upload was accepted: the task must come back with a file attached
# to the same id. A bare `curl` would pass on an HTTP error body.
uploaded="$(json -X PUT "${MANAGER_URL}/tasks/${TASK_ID}/upload" \
    -F "file=@${REPO_ROOT}/build/addition.wasm")"
assert_eq "uploaded task id" "$TASK_ID" "$(jq -r '.id // empty' <<<"$uploaded")"
[ "$(jq -r '.file // empty' <<<"$uploaded")" != "" ] || fail "task has no file after upload"
pass "uploaded build/addition.wasm"

started="$(json -X POST "${MANAGER_URL}/tasks/${TASK_ID}/start" | jq -r '.started // false')"
assert_eq "task accepted for start" "true" "$started"

# ---------------------------------------------------------------------------
# 6. Wait for completion and check the result
# ---------------------------------------------------------------------------

log "Waiting for the task to complete"
task_deadline=$((SECONDS + 120))
while true; do
    task="$(json "${MANAGER_URL}/tasks/${TASK_ID}")"
    state="$(jq -r '.state' <<<"$task")"
    # 3 Completed, 4 Failed, 6 Interrupted — only the last two are terminal
    # failures we care about; 5 Skipped is not expected either.
    case "$state" in
        "$STATE_COMPLETED") break ;;
        4 | 6) fail "task ended in a failure state (state=${state})" ;;
    esac
    if (( SECONDS >= task_deadline )); then
        jq . <<<"$task" >&2
        fail "task did not complete within 120s (state=${state})"
    fi
    sleep 1
done
pass "task completed"

# The example prints its sum with a trailing newline, so compare trimmed.
result="$(jq -r '.results // ""' <<<"$task" | tr -d '\n' | tr -d '\r')"
assert_eq "task result" "$EXPECTED_RESULT" "$result"

proplet_id="$(jq -r '.proplet_id // empty' <<<"$task")"
[ -n "$proplet_id" ] || fail "completed task has no proplet_id"
pass "task executed on proplet $proplet_id"

# ---------------------------------------------------------------------------
# 7. Check the service logs tell the same story
# ---------------------------------------------------------------------------

log "Checking service logs"
# The manager's "successfully created proplet" line is deliberately not
# asserted: the proplet publishes its discovery message exactly once at
# startup, so whether the manager sees it is a race with the manager's own
# subscription. A missed message is not a failure — the proplet's periodic
# liveness heartbeat recreates the record (see updateLivenessHandler). The
# /proplets assertion above is the real registration check.
assert_log_contains propeller-manager "MQTT connection established" "manager connected to MQTT"
assert_log_contains propeller-manager "Subscribe to MQTT topic completed successfully" "manager subscribed to control topics"
assert_log_contains propeller-manager "\"msg\":\"Starting task completed successfully\"" "manager started the task"
assert_log_contains propeller-proplet "Successfully re-subscribed to topics after reconnection" "proplet resubscribed to its control topics"
assert_log_contains propeller-proplet "Decoded wasm binary" "proplet received the wasm module"
assert_log_contains propeller-proplet "Starting Host runtime app" "proplet executed the wasm module"
assert_log_contains propeller-proplet "Successfully published result for task ${TASK_ID}" "proplet published the result"

for container in propeller-manager propeller-proplet propeller-proxy; do
    assert_log_clean "$container"
done

log "Integration test passed"
