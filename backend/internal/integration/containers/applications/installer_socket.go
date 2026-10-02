package applications

import (
	"context"
	"fmt"
	svc "github.com/futrx-com/remote.futrx.com/internal/service/applications"
)

// stopSocketProxy disarms activation before the application service is stopped.
// Like service shutdown, socket cleanup is best effort when a container is gone.
func (in *Installer) stopSocketProxy(ctx context.Context, spec svc.InstallSpec, serviceName string) {
	if spec.Application.Service.SocketProxy == nil {
		return
	}
	_, _ = in.exec(ctx, spec.Instance.ContainerName, nil, controlTimeout, "systemctl", "disable", "--now", serviceName+".socket")
	_, _ = in.exec(ctx, spec.Instance.ContainerName, nil, controlTimeout, "systemctl", "stop", serviceName+"-proxy.service")
}

func socketProxyUnit(name string, socket svc.SocketProxy) []byte {
	return []byte(fmt.Sprintf("[Unit]\nDescription=%s on-demand proxy\nRequires=%s.service\nAfter=%s.service\n\n[Service]\nExecStart=/usr/lib/systemd/systemd-socket-proxyd --exit-idle-time=%ds 127.0.0.1:%d\n",
		name, name, name, socket.IdleSeconds, socket.TargetPort))
}

func socketUnit(name string, socket svc.SocketProxy) []byte {
	return []byte(fmt.Sprintf("[Unit]\nDescription=%s on-demand socket\n\n[Socket]\nListenStream=0.0.0.0:%d\nService=%s-proxy.service\n\n[Install]\nWantedBy=sockets.target\n",
		name, socket.ListenPort, name))
}
