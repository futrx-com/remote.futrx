package httptransport

import (
	"github.com/gorilla/websocket"
)

func NewUpgrader() websocket.Upgrader {
	return websocket.Upgrader{
		ReadBufferSize:  4096,
		WriteBufferSize: 4096,
		// Leave CheckOrigin unset: gorilla rejects browser origins whose Host
		// differs from the request Host. Sibling app origins are untrusted.
	}
}
