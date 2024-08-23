package tap

import (
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"syscall"

	"github.com/containers/gvisor-tap-vsock/pkg/notification"
	"github.com/containers/gvisor-tap-vsock/pkg/types"
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

const maxStreamPacketSize = 128 * 1024

type Switch struct {
	Sent     uint64
	Received uint64

	debug bool

	nextConnID int
	conns      map[int]hypervisorConn
	connLock   sync.Mutex

	cam     map[tcpip.LinkAddress]int
	camLock sync.RWMutex

	writeLock sync.Mutex

	gateway VirtualDevice

	notificationSender *notification.NotificationSender
}

func NewSwitch(debug bool) *Switch {
	return &Switch{
		debug: debug,
		conns: make(map[int]hypervisorConn),
		cam:   make(map[tcpip.LinkAddress]int),
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

func (e *Switch) Accept(ctx context.Context, rawConn net.Conn, protocol types.Protocol) error {
	conn := HypervisorConnNew(rawConn, protocol)
	log.Debugf("new connection from %s to %s", conn.RemoteAddr().String(), conn.LocalAddr().String())
	id, failed := e.connect(conn)
	if failed {
		log.Error("connection failed")
		return conn.Close()

	}

	defer e.disconnect(id, conn)
	if err := e.rx(ctx, id, conn); err != nil {
		err := fmt.Errorf("cannot receive packets from %s, disconnecting: %w", conn.RemoteAddr().String(), err)
		log.Error(err)

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

type connTarget struct {
	id   int
	conn hypervisorConn
}

func (e *Switch) tx(pkt *stack.PacketBuffer) error {
	buf := pkt.ToView().AsSlice()
	eth := header.Ethernet(buf)
	dst := eth.DestinationAddress()
	src := eth.SourceAddress()

	size := pkt.Size()
	if size < 0 {
		return fmt.Errorf("packet size out of range")
	}
	if dst == header.EthernetBroadcastAddress {
		e.camLock.RLock()
		srcID, ok := e.cam[src]
		if !ok {
			srcID = -1
		}
		e.camLock.RUnlock()

		e.connLock.Lock()
		targets := make([]connTarget, 0, len(e.conns))
		for id, conn := range e.conns {
			if id != srcID {
				targets = append(targets, connTarget{id, conn})
			}
		}
		e.connLock.Unlock()

		for _, t := range targets {
			err := e.txBuf(t.conn, buf)
			if err != nil {
				e.disconnect(t.id, t.conn)
				return err
			}
			atomic.AddUint64(&e.Sent, uint64(size))
		}
	} else {
		e.camLock.RLock()
		id, ok := e.cam[dst]
		if !ok {
			e.camLock.RUnlock()
			return nil
		}
		e.camLock.RUnlock()

		e.connLock.Lock()
		conn, ok := e.conns[id]
		e.connLock.Unlock()
		if !ok {
			return nil
		}

		err := e.txBuf(conn, buf)
		if err != nil {
			e.disconnect(id, conn)
			return err
		}
		atomic.AddUint64(&e.Sent, uint64(size))
	}
	return nil
}

func (e *Switch) txBuf(conn hypervisorConn, buf []byte) error {
	e.writeLock.Lock()
	defer e.writeLock.Unlock()

	for {
		if err := conn.WriteBuf(buf); err != nil {
			if errors.Is(err, syscall.ENOBUFS) {
				// socket buffer can be full keep retrying sending the same data
				// again until it works or we get a different error
				// https://github.com/containers/gvisor-tap-vsock/issues/367
				continue
			}
			return err
		}
		return nil
	}
}

func (e *Switch) disconnect(id int, conn net.Conn) {
	e.connLock.Lock()
	defer e.connLock.Unlock()

	e.camLock.Lock()
	defer e.camLock.Unlock()

	for address, targetConn := range e.cam {
		if targetConn == id {
			if e.notificationSender != nil {
				e.notificationSender.Send(types.NotificationMessage{
					NotificationType: types.ConnectionClosed,
					MacAddress:       address.String(),
				})
			}
			delete(e.cam, address)
		}
	}
	_ = conn.Close()
	delete(e.conns, id)
}

func validateStreamPacketSize(size int) (err error) {
	if size < 0 || size > maxStreamPacketSize {
		err = fmt.Errorf("invalid packet size: %d is negative or exceeds maximum %d", size, maxStreamPacketSize)
	}
	return err
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
			return fmt.Errorf("cannot read packet from socket: %w", err)
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
	_, exists := e.cam[eth.SourceAddress()]
	e.cam[eth.SourceAddress()] = id
	e.camLock.Unlock()

	if !exists && e.notificationSender != nil {
		e.notificationSender.Send(types.NotificationMessage{
			NotificationType: types.ConnectionEstablished,
			MacAddress:       eth.SourceAddress().String(),
		})
	}

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

func (e *Switch) SetNotificationSender(notificationSender *notification.NotificationSender) {
	e.notificationSender = notificationSender
}
