package templates

import "strings"

// Decision is the outcome of the provisioning rule for one container. It is a
// pure function of the template and two observations, which keeps the policy
// unit-testable without a container runtime.
type Decision struct {
	// Run reports whether the provision program must be executed now.
	Run bool
	// Status is the state to report while/after acting on this decision.
	Status Status
	// Reason explains the decision in logs and tests.
	Reason string
}

// Observation is what the caller knows about a container before deciding.
type Observation struct {
	// MarkerPresent is true when MarkerPath exists in the container, meaning a
	// previous run (or a pre-built template image) already provisioned it.
	MarkerPresent bool
	// FailurePresent is true when FailurePath exists, meaning the last attempt
	// started and did not reach the end of the program.
	FailurePresent bool
	// InFlight is true when this process already has a provisioning goroutine
	// running for the container.
	InFlight bool
}

// Decide applies the provisioning rule:
//
//   - a template with nothing to install never touches the container;
//   - the success marker wins over everything else, so a container launched
//     from a pre-built template image is never re-provisioned;
//   - one run at a time per container;
//   - otherwise provision, including after a previous failure — the rootfs is
//     disposable, and every script is written to be idempotent.
func Decide(template Template, observation Observation) Decision {
	if !template.Provisions() {
		return Decision{Status: StatusNone, Reason: "template installs nothing"}
	}
	if observation.MarkerPresent {
		return Decision{Status: StatusDone, Reason: "marker present"}
	}
	if observation.InFlight {
		return Decision{Status: StatusRunning, Reason: "provisioning already in flight"}
	}
	if observation.FailurePresent {
		return Decision{Run: true, Status: StatusRunning, Reason: "retrying after a failed run"}
	}
	return Decision{Run: true, Status: StatusRunning, Reason: "marker absent"}
}

// ObservedStatus reports the durable status of a container whose provisioning
// this process is not tracking in memory — after a backend restart, say.
func ObservedStatus(template Template, observation Observation) Status {
	if !template.Provisions() {
		return StatusNone
	}
	switch {
	case observation.MarkerPresent:
		return StatusDone
	case observation.InFlight:
		return StatusRunning
	case observation.FailurePresent:
		return StatusFailed
	default:
		return StatusPending
	}
}

// provisionPreamble is the harness every template script runs inside. It
// supplies the non-interactive apt environment, the shared log, the marker
// protocol, and a single apt retry (package mirrors on a fresh container
// occasionally answer before their network is fully up).
const provisionPreamble = `set -eu
export DEBIAN_FRONTEND=noninteractive
export NEEDRESTART_MODE=a
export APT_LISTCHANGES_FRONTEND=none
REMOTE_TEMPLATE_NAME='__TEMPLATE__'
REMOTE_TEMPLATE_LOG='__LOG__'
REMOTE_TEMPLATE_MARKER='__MARKER__'
REMOTE_TEMPLATE_FAILED='__FAILED__'
mkdir -p "$(dirname "$REMOTE_TEMPLATE_LOG")" "$(dirname "$REMOTE_TEMPLATE_MARKER")"
# Everything below is both streamed back to the caller and appended to the
# in-container log, so a failure is diagnosable from the host and from inside.
exec > >(tee -a "$REMOTE_TEMPLATE_LOG") 2>&1
echo "=== remote template '$REMOTE_TEMPLATE_NAME' provisioning $(date -u '+%Y-%m-%dT%H:%M:%SZ') ==="
if [ -f "$REMOTE_TEMPLATE_MARKER" ]; then
  echo "marker $REMOTE_TEMPLATE_MARKER present - nothing to do"
  exit 0
fi
# The failure marker exists for the duration of the run and is removed on
# success, so an interrupted run is reported as failed rather than pending.
printf 'started %s\n' "$(date -u '+%Y-%m-%dT%H:%M:%SZ')" > "$REMOTE_TEMPLATE_FAILED"

# apt_retry runs apt-get once, then refreshes the index and retries exactly
# once. Templates use it for every network-touching apt call.
apt_retry() {
  if apt-get "$@"; then
    return 0
  fi
  echo "apt-get $* failed; refreshing package index and retrying once"
  sleep 5
  apt-get update -qq || true
  apt-get "$@"
}

`

// provisionEpilogue records success. Reaching it means every command in the
// payload succeeded, because the harness runs under 'set -e'.
const provisionEpilogue = `

rm -f "$REMOTE_TEMPLATE_FAILED"
printf '%s\n' "$REMOTE_TEMPLATE_NAME" > "$REMOTE_TEMPLATE_MARKER"
echo "=== remote template '$REMOTE_TEMPLATE_NAME' provisioning complete ==="
`

// ProvisionProgram wraps a template's payload in the shared harness. The
// result is a bash program: it is executed with 'bash -c' inside the
// container and is safe to run repeatedly. A template with seeds but no
// script still gets the harness, so that its marker is written once and the
// seeding is not retried on every start.
func ProvisionProgram(template Template) string {
	if !template.Provisions() {
		return ""
	}
	replacer := strings.NewReplacer(
		"__TEMPLATE__", template.Name,
		"__LOG__", LogPath,
		"__MARKER__", MarkerPath,
		"__FAILED__", FailurePath,
	)
	var program strings.Builder
	program.WriteString(replacer.Replace(provisionPreamble))
	program.WriteString(strings.TrimRight(string(template.Script), "\n"))
	program.WriteString(provisionEpilogue)
	return program.String()
}
