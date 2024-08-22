package tap

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
)

type protocol interface {
	Stream() bool
}

type streamProtocol interface {
	protocol
	ReadSize(io.Reader) (int, error)
	WriteSize(size int) ([]byte, error)
}

type hyperkitProtocol struct {
}

func (s *hyperkitProtocol) Stream() bool {
	return true
}

func (s *hyperkitProtocol) ReadSize(reader io.Reader) (int, error) {
	var sizeBuf [2]byte
	_, err := io.ReadFull(reader, sizeBuf[:])
	if err != nil {
		return 0, fmt.Errorf("cannot read size from socket: %w", err)
	}
	size := binary.LittleEndian.Uint16(sizeBuf[:])
	return int(size), nil
}

func (s *hyperkitProtocol) WriteSize(size int) ([]byte, error) {
	var sizeBuf [2]byte
	if size < 0 || size > math.MaxUint16 {
		return nil, fmt.Errorf("%d is larger than 16 bits", size)
	}
	binary.LittleEndian.PutUint16(sizeBuf[:], uint16(size)) //#nosec G115 - 'size' was compared against MaxUint16
	return sizeBuf[:], nil
}

type qemuProtocol struct {
}

func (s *qemuProtocol) Stream() bool {
	return true
}

func (s *qemuProtocol) ReadSize(reader io.Reader) (int, error) {
	var sizeBuf [4]byte
	_, err := io.ReadFull(reader, sizeBuf[:])
	if err != nil {
		return 0, fmt.Errorf("cannot read size from socket: %w", err)
	}
	size := binary.BigEndian.Uint32(sizeBuf[:])
	return int(size), nil
}

func (s *qemuProtocol) WriteSize(size int) ([]byte, error) {
	var sizeBuf [4]byte
	if size < 0 || size > math.MaxUint32 {
		return nil, fmt.Errorf("%d is larger than 32 bits", size)
	}
	binary.LittleEndian.PutUint32(sizeBuf[:], uint32(size)) //#nosec G115 - 'size' was compared against MaxUint32
	return sizeBuf[:], nil
}

type bessProtocol struct {
}

func (s *bessProtocol) Stream() bool {
	return false
}

type vfkitProtocol struct {
}

func (s *vfkitProtocol) Stream() bool {
	return false
}
