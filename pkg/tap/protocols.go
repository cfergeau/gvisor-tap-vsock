package tap

import (
	"encoding/binary"
	"fmt"
	"math"
)

type protocol interface {
	Stream() bool
}

type streamProtocol interface {
	protocol
	Buf() []byte
	Write(buf []byte, size int) error
	Read(buf []byte) int
}

type hyperkitProtocol struct {
}

func (s *hyperkitProtocol) Stream() bool {
	return true
}

func (s *hyperkitProtocol) Buf() []byte {
	return make([]byte, 2)
}

func (s *hyperkitProtocol) Write(buf []byte, size int) error {
	if size < 0 || size > math.MaxUint16 {
		return fmt.Errorf("%d is larger than 16 bits", size)
	}

	binary.BigEndian.PutUint16(buf, uint16(size)) //#nosec G115 - 'size' was compared against MaxUint16
	return nil
}

func (s *hyperkitProtocol) Read(buf []byte) int {
	return int(binary.LittleEndian.Uint16(buf[0:2]))
}

type qemuProtocol struct {
}

func (s *qemuProtocol) Stream() bool {
	return true
}

func (s *qemuProtocol) Buf() []byte {
	return make([]byte, 4)
}

func (s *qemuProtocol) Write(buf []byte, size int) error {
	if size < 0 || size > math.MaxUint32 {
		return fmt.Errorf("%d is larger than 32 bits", size)
	}

	binary.BigEndian.PutUint32(buf, uint32(size)) //#nosec G115 - 'size' was compared against MaxUint32
	return nil
}

func (s *qemuProtocol) Read(buf []byte) int {
	return int(binary.BigEndian.Uint32(buf[0:4]))
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
