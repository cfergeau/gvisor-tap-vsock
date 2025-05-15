package tap

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
	"syscall"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"gvisor.dev/gvisor/pkg/buffer"
	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/header"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
)

type VirtualDevice interface {
	DeliverNetworkPacket(protocol tcpip.NetworkProtocolNumber, pkt *stack.PacketBuffer)
	LinkAddress() tcpip.LinkAddress
	IP() string
}

type NetworkSwitch interface {
	DeliverNetworkPacket(protocol tcpip.NetworkProtocolNumber, pkt *stack.PacketBuffer)
}

type Switch struct {
	Sent     uint64
	Received uint64

	debug               bool
	maxTransmissionUnit int

	nextConnID int
	conns      map[int]hypervisorConn
	connLock   sync.Mutex

	cam     map[tcpip.LinkAddress]int
	camLock sync.RWMutex

	writeLock sync.Mutex

	gateway VirtualDevice
}

func NewSwitch(debug bool, mtu int) *Switch {
	return &Switch{
		debug:               debug,
		maxTransmissionUnit: mtu,
		conns:               make(map[int]hypervisorConn),
		cam:                 make(map[tcpip.LinkAddress]int),
	}
}

func (e *Switch) CAM() map[string]int {
	e.camLock.RLock()
	defer e.camLock.RUnlock()
	ret := make(map[string]int)
	for address, port := range e.cam {
		ret[address.String()] = port
	}
	return ret
}

func (e *Switch) Connect(ep VirtualDevice) {
	e.gateway = ep
}

func (e *Switch) DeliverNetworkPacket(_ tcpip.NetworkProtocolNumber, pkt *stack.PacketBuffer) {
	if err := e.tx(pkt); err != nil {
		log.Error(err)
	}
}

func (e *Switch) Accept(ctx context.Context, conn hypervisorConn) error {
	log.Debugf("new connection from %s to %s", conn.RemoteAddr().String(), conn.LocalAddr().String())
	id, failed := e.connect(conn)
	if failed {
		log.Error("connection failed")
		return conn.Close()

	}

	defer func() {
		e.connLock.Lock()
		defer e.connLock.Unlock()
		e.disconnect(id, conn)
	}()
	if err := e.rx(ctx, id, conn); err != nil {
		log.Error(errors.Wrapf(err, "cannot receive packets from %s, disconnecting", conn.RemoteAddr().String()))
		return err
	}
	return nil
}

func (e *Switch) connect(conn hypervisorConn) (int, bool) {
	e.connLock.Lock()
	defer e.connLock.Unlock()

	id := e.nextConnID
	e.nextConnID++

	e.conns[id] = conn
	return id, false
}

func (e *Switch) tx(pkt *stack.PacketBuffer) error {
	e.writeLock.Lock()
	defer e.writeLock.Unlock()

	e.connLock.Lock()
	defer e.connLock.Unlock()

	buf := pkt.ToView().AsSlice()
	eth := header.Ethernet(buf)
	dst := eth.DestinationAddress()
	src := eth.SourceAddress()

	if dst == header.EthernetBroadcastAddress {
		e.camLock.RLock()
		srcID, ok := e.cam[src]
		if !ok {
			srcID = -1
		}
		e.camLock.RUnlock()
		for id, conn := range e.conns {
			if id == srcID {
				continue
			}

			err := e.txBuf(id, conn, buf)
			if err != nil {
				return err
			}

			atomic.AddUint64(&e.Sent, uint64(pkt.Size()))
		}
	} else {
		e.camLock.RLock()
		id, ok := e.cam[dst]
		if !ok {
			e.camLock.RUnlock()
			return nil
		}
		e.camLock.RUnlock()
		conn := e.conns[id]
		err := e.txBuf(id, conn, buf)
		if err != nil {
			return err
		}
		atomic.AddUint64(&e.Sent, uint64(pkt.Size()))
	}
	return nil
}

func (e *Switch) txBuf(id int, conn hypervisorConn, buf []byte) error {
	for {
		if err := conn.WriteBuf(buf); err != nil {
			if errors.Is(err, syscall.ENOBUFS) {
				// socket buffer can be full keep retrying sending the same data
				// again until it works or we get a different error
				// https://github.com/containers/gvisor-tap-vsock/issues/367
				continue
			}
			e.disconnect(id, conn)
			return err
		}
		return nil
	}
}

func (e *Switch) disconnect(id int, conn net.Conn) {
	e.camLock.Lock()
	defer e.camLock.Unlock()

	for address, targetConn := range e.cam {
		if targetConn == id {
			delete(e.cam, address)
		}
	}
	_ = conn.Close()
	delete(e.conns, id)
}

func (e *Switch) rx(ctx context.Context, id int, conn hypervisorConn) error {
loop:
	for {
		select {
		case <-ctx.Done():
			break loop
		default:
			// passthrough
		}
		buf, err := conn.ReadBuf()
		if err != nil {
			return errors.Wrap(err, "cannot read data from socket")
		}
		e.rxBuf(ctx, id, buf)
	}
	return nil
}

func (e *Switch) rxBuf(_ context.Context, id int, buf []byte) {
	if e.debug {
		packet := gopacket.NewPacket(buf, layers.LayerTypeEthernet, gopacket.Default)
		log.Info(packet.String())
	}

	eth := header.Ethernet(buf)

	e.camLock.Lock()
	e.cam[eth.SourceAddress()] = id
	e.camLock.Unlock()

	if eth.DestinationAddress() != e.gateway.LinkAddress() {
		pkt := stack.NewPacketBuffer(stack.PacketBufferOptions{
			Payload: buffer.MakeWithData(buf),
		})
		if err := e.tx(pkt); err != nil {
			log.Error(err)
		}
		pkt.DecRef()
	}
	if eth.DestinationAddress() == e.gateway.LinkAddress() || eth.DestinationAddress() == header.EthernetBroadcastAddress {
		data := buffer.MakeWithData(buf)
		data.TrimFront(header.EthernetMinimumSize)
		pkt := stack.NewPacketBuffer(stack.PacketBufferOptions{
			Payload: data,
		})
		e.gateway.DeliverNetworkPacket(eth.Type(), pkt)
		pkt.DecRef()
	}

	atomic.AddUint64(&e.Received, uint64(len(buf)))
}
