package virtualnetwork

import (
	"context"
	"net"

	"github.com/containers/gvisor-tap-vsock/pkg/tap"
)

func (n *VirtualNetwork) AcceptVfkit(ctx context.Context, conn net.Conn) error {
	return n.networkSwitch.Accept(ctx, tap.NewPacketConn(conn))
}
