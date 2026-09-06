// Idle-workspace detection is a guest-side question: the host can only ask the
// container what is still holding its workspace. The probe below answers that
// and nothing else, so the LXD client in runtime.go stays a command client.
package lifecycle

import (
	"context"
	"strconv"
	"strings"

	"github.com/futrx-com/remote.futrx.com/internal/integration/containers/command"
	serviceproject "github.com/futrx-com/remote.futrx.com/internal/service/project"
)

func (c *Client) SetAutoStart(ctx context.Context, name string, enabled bool) error {
	value := "false"
	if enabled {
		value = "true"
	}
	_, err := command.RunWithTimeout(ctx, c.runner, queryTimeout, "config", "set", name, "boot.autostart", value)
	return err
}

// WorkspaceBusy treats shells with a workspace cwd, open workspace files,
// agents and listening applications as pins, even when CPU usage is zero.
func (c *Client) WorkspaceBusy(ctx context.Context, name string) (bool, error) {
	state, err := c.State(ctx, name)
	if err != nil {
		return true, err
	}
	if state == serviceproject.ContainerStateStopped {
		return false, nil
	}
	if state != serviceproject.ContainerStateRunning {
		return true, nil
	}
	busy, err := c.Busy(ctx, name)
	if err != nil || busy {
		return true, err
	}
	out, err := command.RunWithTimeout(ctx, c.runner, queryTimeout, "exec", name, "--", "python3", "-c", workspaceBusyScript)
	if err != nil {
		return true, err
	}
	return strings.TrimSpace(out) != "idle", nil
}

// idleProcessDir is where an installed application declares the daemon it
// leaves running, one process name per line, in a file of its own. It is part
// of the install-script contract, not a Remote-managed list.
const idleProcessDir = "/etc/remote/workspace-idle.d"

var workspaceBusyScript = `
import os, socket
IDLE_DIR = ` + strconv.Quote(idleProcessDir) + `
me = os.getpid()
def workspace(p):
    return p == '/workspace' or p.startswith('/workspace/')
# Fail closed on mount-backed projects, where /workspace itself is a FUSE
# mount: switching existing FUSE data must be an explicit, verified migration,
# never a backup of an empty mountpoint. A FUSE mount beneath the workspace is
# not a pin: an application that mounts remote storage there keeps its data at
# the far end, and the mount detaches with the container.
with open('/proc/self/mountinfo') as f:
    for line in f:
        parts=line.split()
        if len(parts)>4 and parts[4]=='/workspace' and ' - fuse' in line:
            print('busy'); raise SystemExit
# Daemons the stock Ubuntu image and Remote's own provisioning always run.
# Everything else is a workload pin; an idle custom executable can access
# workspace files later without holding an fd right now.
system={'systemd','systemd-journal','systemd-network','systemd-resolve',
        'systemd-timesyn','systemd-udevd','systemd-logind','systemd-oomd',
        'systemd-hostnam','systemd-userdbd','systemd-userwor',
        'dbus-daemon','dbus-broker','dbus-broker-lau','rsyslogd','cron','crond',
        'sshd','agetty','polkitd','(sd-pam)','udisksd','unattended-upgr',
        'snapd','snapfuse'}
# An installed application whose daemon runs for the life of the container
# would otherwise pin every workspace it is installed in forever. Rather than
# naming those daemons here — Remote does not know which applications exist —
# an install script declares its own by dropping a file of process names in
# this directory. Only names are read, and only for this container's own
# idleness: nothing here grants a process any access it did not already have.
try:
    for entry in sorted(os.listdir(IDLE_DIR)):
        try:
            with open(os.path.join(IDLE_DIR, entry)) as f:
                system.update(n.strip() for n in f if n.strip())
        except OSError: pass
except OSError: pass
for pid in os.listdir('/proc'):
    if not pid.isdigit() or int(pid) in (1,me,os.getppid()): continue
    base='/proc/'+pid
    try:
        for link in [base+'/cwd',base+'/exe']:
            try:
                if workspace(os.readlink(link)): print('busy'); raise SystemExit
            except FileNotFoundError: pass
        for fd in os.listdir(base+'/fd'):
            try:
                if workspace(os.readlink(base+'/fd/'+fd)): print('busy'); raise SystemExit
            except FileNotFoundError: pass
        with open(base+'/comm') as f: comm=f.read().strip()
        if comm not in system:
            print('busy'); raise SystemExit
    except (FileNotFoundError,ProcessLookupError): pass
    except PermissionError:
        print('busy'); raise SystemExit
# Keep non-system listeners, including applications launched from outside
# /workspace. A listening socket held only by systemd (PID 1) is an armed
# socket-activation unit such as code-server on :8842 with no service behind
# it; once a client connects, the activated service is a process pin above.
held={}
for pid in os.listdir('/proc'):
    if not pid.isdigit(): continue
    try:
        for fd in os.listdir('/proc/'+pid+'/fd'):
            try:
                target=os.readlink('/proc/'+pid+'/fd/'+fd)
            except OSError: continue
            if target.startswith('socket:['):
                held.setdefault(target[8:-1],set()).add(int(pid))
    except OSError: pass
for table in ('tcp','tcp6'):
    with open('/proc/net/'+table) as f:
        for line in list(f)[1:]:
            cols=line.split()
            if cols[3]!='0A' or int(cols[1].split(':')[1],16) in (22,53): continue
            if held.get(cols[9],set())-{1}:
                print('busy'); raise SystemExit
print('idle')
`
