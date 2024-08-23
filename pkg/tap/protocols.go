package tap

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
)

type streamProtocol interface {
	ReadSize(io.Reader) (int, error)
	WriteSize(size int) ([]byte, error)
}

type hyperkitProtocol struct {
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
	if size < 0 || size > math.MaxUint16 {
		return nil, fmt.Errorf("size out of uint16 range (%d)", size)
	}
	var sizeBuf [2]byte
	binary.LittleEndian.PutUint16(sizeBuf[:], uint16(size))
	return sizeBuf[:], nil
}

type qemuProtocol struct {
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
	if size < 0 || size > math.MaxInt32 {
		return nil, fmt.Errorf("size out of int32 range (%d)", size)
	}
	var sizeBuf [4]byte
	binary.BigEndian.PutUint32(sizeBuf[:], uint32(size))
	return sizeBuf[:], nil
}
