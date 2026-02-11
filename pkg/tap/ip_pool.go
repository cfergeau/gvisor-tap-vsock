package tap

import (
	"errors"
	"maps"
	"net"
	"net/netip"
	"sync"
)

type IPPool struct {
	base     *net.IPNet
	leases   map[string]string
	lock     sync.Mutex
	next     netip.Addr
	released []net.IP
}

func NewIPPool(base *net.IPNet) *IPPool {
	start, ok := netip.AddrFromSlice(base.IP)
	if !ok {
		// never
		panic("input is incorrect")
	}

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
		ip := p.released[len(p.released)-1]
		p.released = p.released[:len(p.released)-1]
		p.leases[ip.String()] = mac
		return ip, nil
	}
	for {
		ip := p.next.Next()
		p.next = ip
		var candidate net.IP = ip.AsSlice()

		if !p.base.Contains(candidate) {
			return nil, errors.New("cannot find available IP")
		}

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
