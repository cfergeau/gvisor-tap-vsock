package virtualnetwork

import (
	"context"
	"net"

	gvnet "github.com/containers/gvisor-tap-vsock/pkg/net"

	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/adapters/gonet"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv4"
)

func (n *VirtualNetwork) Dial(network, addr string) (net.Conn, error) {
	ip, port, err := gvnet.SplitIPPort(network, addr)
	if err != nil {
		return nil, err
	}
	return gonet.DialTCP(n.stack, tcpip.FullAddress{
		NIC:  1,
		Addr: tcpip.AddrFrom4Slice(ip.To4()),
		Port: uint16(port),
	}, ipv4.ProtocolNumber)
}

func (n *VirtualNetwork) DialContextTCP(ctx context.Context, addr string) (net.Conn, error) {
	ip, port, err := gvnet.SplitIPPort("tcp", addr)
	if err != nil {
		return nil, err
	}

	return gonet.DialContextTCP(ctx, n.stack,
		tcpip.FullAddress{
			NIC:  1,
			Addr: tcpip.AddrFrom4Slice(ip.To4()),
			Port: uint16(port),
		}, ipv4.ProtocolNumber)
}

func (n *VirtualNetwork) Listen(network, addr string) (net.Listener, error) {
	ip, port, err := gvnet.SplitIPPort(network, addr)
	if err != nil {
		return nil, err
	}
	return gonet.ListenTCP(n.stack, tcpip.FullAddress{
		NIC:  1,
		Addr: tcpip.AddrFrom4Slice(ip.To4()),
		Port: uint16(port),
	}, ipv4.ProtocolNumber)
}
