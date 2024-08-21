package net

import (
	"errors"
	"net"
	"strconv"
)

func SplitIPPort(network string, addr string) (net.IP, uint16, error) {
	if network != "tcp" {
		return nil, 0, errors.New("only tcp is supported")
	}
	host, portString, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, 0, err
	}
	port, err := strconv.ParseUint(portString, 10, 16)
	if err != nil {
		return nil, 0, err
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return nil, 0, errors.New("invalid address, must be an IP")
	}
	return ip, uint16(port), nil //#nosec G115 -- strconv.ParseUint(.., .., 16) guarantees no overflow
}
