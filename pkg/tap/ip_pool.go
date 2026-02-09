package tap

import (
	"errors"
	"maps"
	"math/big"
	"net"
	"sync"
)

type IPPool struct {
	base     *net.IPNet
	leases   map[string]string
	lock     sync.Mutex
	next     *big.Int
	released []net.IP
}

func NewIPPool(base *net.IPNet) *IPPool {
	start := big.NewInt(0)
	start.SetBytes(base.IP.To16())
	start.Add(start, big.NewInt(1))

	return &IPPool{
		base:   base,
		leases: make(map[string]string),
		next:   start,
	}
}

func (p *IPPool) Leases() map[string]string {
	p.lock.Lock()
	defer p.lock.Unlock()
	leases := map[string]string{}
	maps.Copy(leases, p.leases)
	return leases
}

func (p *IPPool) Mask() int {
	ones, _ := p.base.Mask.Size()
	return ones
}

func (p *IPPool) GetOrAssign(mac string) (net.IP, error) {
	p.lock.Lock()
	defer p.lock.Unlock()

	for ip, candidate := range p.leases {
		if candidate == mac {
			return net.ParseIP(ip), nil
		}
	}
	if len(p.released) > 0 {
		ip := p.released[0]
		p.released = p.released[1:]
		p.leases[ip.String()] = mac
		return ip, nil
	}
	for {
		ipBytes := p.next.Bytes()
		if len(ipBytes) < len(p.base.IP) {
			padded := make([]byte, len(p.base.IP))
			copy(padded[len(p.base.IP)-len(ipBytes):], ipBytes)
			ipBytes = padded
		}
		ipBytes = ipBytes[len(ipBytes)-len(p.base.IP):]

		candidate := net.IP(ipBytes)
		if !p.base.Contains(candidate) {
			return nil, errors.New("cannot find available IP")
		}

		p.next.Add(p.next, big.NewInt(1))

		if _, ok := p.leases[candidate.String()]; !ok {
			p.leases[candidate.String()] = mac
			return candidate, nil
		}
	}
}

func (p *IPPool) Reserve(ip net.IP, mac string) {
	p.lock.Lock()
	defer p.lock.Unlock()

	p.leases[ip.String()] = mac
}

func (p *IPPool) Release(given string) {
	p.lock.Lock()
	defer p.lock.Unlock()

	var found string
	for ip, mac := range p.leases {
		if mac == given {
			found = ip
			break
		}
	}
	if found != "" {
		delete(p.leases, found)
		p.released = append(p.released, net.ParseIP(found))
	}
}
