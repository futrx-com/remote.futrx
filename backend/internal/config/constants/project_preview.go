package constants

const (
	// ProjectPreviewMinPort and ProjectPreviewMaxPort mirror the preview-host
	// port range accepted by Caddy.
	ProjectPreviewMinPort = 1024
	ProjectPreviewMaxPort = 65535

	// ProjectPreviewAgentBrowserPort is the in-container noVNC listener.
	ProjectPreviewAgentBrowserPort = 6080
	// ProjectPreviewIDEProxyPort is the socket-activated code-server proxy.
	ProjectPreviewIDEProxyPort = 8842
	// ProjectPreviewIDEDirectPort is code-server's loopback listener.
	ProjectPreviewIDEDirectPort = 8081
	// ProjectPreviewBrowserDevToolsPort is Chromium's loopback CDP listener.
	ProjectPreviewBrowserDevToolsPort = 9222
)
