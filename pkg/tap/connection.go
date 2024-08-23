package tap

import (
	"bufio"
	"io"
	"net"

	"github.com/containers/gvisor-tap-vsock/pkg/types"
)

func HypervisorConnNew(rawConn net.Conn, protocol types.Protocol) hypervisorConn {
	switch protocol {
	case types.QemuProtocol:
		return NewStreamConn(rawConn, &qemuProtocol{})
	case types.BessProtocol:
		return NewPacketConn(rawConn)
	case types.VfkitProtocol:
		return NewPacketConn(rawConn)
	default:
		return NewStreamConn(rawConn, &hyperkitProtocol{})
	}
}

type hypervisorConn interface {
	net.Conn
	WriteBuf(buf []byte) error
	ReadBuf() ([]byte, error)
}

type streamConn struct {
	net.Conn
	streamProtocolImpl streamProtocol
	reader             io.Reader
}

func NewStreamConn(rawConn net.Conn, streamProtocolImpl streamProtocol) *streamConn {
	reader := bufio.NewReader(rawConn)
	return &streamConn{Conn: rawConn, streamProtocolImpl: streamProtocolImpl, reader: reader}
}

func (conn *streamConn) WriteBuf(buf []byte) error {
	sizeBuf, err := conn.streamProtocolImpl.WriteSize(len(buf))
	if err != nil {
		return err
	}
	buf = append(sizeBuf, buf...)
	_, err = conn.Write(buf)
	return err
}

func (conn *streamConn) ReadBuf() ([]byte, error) {
	// FIXME: need to init conn.reader
	size, err := conn.streamProtocolImpl.ReadSize(conn.reader)
	if err != nil {
		return nil, err
	}

	buf := make([]byte, size)
	_, err = io.ReadFull(conn.reader, buf)
	if err != nil {
		return nil, err
	}
	return buf, nil
}

type packetConn struct {
	net.Conn
	buf [1024 * 128]byte
}

func NewPacketConn(rawConn net.Conn) *packetConn {
	return &packetConn{Conn: rawConn}
}
func (conn *packetConn) WriteBuf(buf []byte) error {
	_, err := conn.Write(buf)
	return err
}

func (conn *packetConn) ReadBuf() ([]byte, error) {
	n, err := conn.Read(conn.buf[:])
	if err != nil {
		return nil, err
	}
	return conn.buf[:n], nil
}
