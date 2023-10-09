package main

import (
	"net"
	"strconv"

	"github.com/containers/gvisor-tap-vsock/pkg/types"
)

type Config types.Configuration

func (cfg *Config) SetDebug(debug bool) error {
	cfg.Debug = debug

	return nil
}

func (cfg *Config) SetCaptureFile(captureFile string) error {
	cfg.CaptureFile = captureFile

	return nil
}

func (cfg *Config) SetMTU(mtu int) error {
	cfg.MTU = mtu

	return nil
}

func (cfg *Config) SetSearchDomains(searchDomains []string) error {
	cfg.DNSSearchDomains = searchDomains

	return nil
}

func defaultConfig(gvproxy *GvProxy) Config {
	const (
		hostIP            = "192.168.127.254"
		vmIP              = "192.168.127.2"
		vmMacAddress      = "5a:94:ef:e4:0c:ee"
		gatewayIP         = "192.168.127.1"
		gatewayMacAddress = "5a:94:ef:e4:0c:dd"
		host              = "host"
		gateway           = "gateway"
	)

	config := Config{
		MTU:               1500,
		Subnet:            "192.168.127.0/24",
		GatewayIP:         gatewayIP,
		GatewayMacAddress: gatewayMacAddress,
		DHCPStaticLeases: map[string]string{
			vmIP: vmMacAddress,
		},
		DNS: []types.Zone{
			{
				Name: "containers.internal.",
				Records: []types.Record{
					{
						Name: gateway,
						IP:   net.ParseIP(gatewayIP),
					},
					{
						Name: host,
						IP:   net.ParseIP(hostIP),
					},
				},
			},
			{
				Name: "docker.internal.",
				Records: []types.Record{
					{
						Name: gateway,
						IP:   net.ParseIP(gatewayIP),
					},
					{
						Name: host,
						IP:   net.ParseIP(hostIP),
					},
				},
			},
		},
		Forwards: map[string]string{
			net.JoinHostPort("127.0.0.1", strconv.Itoa(sshPort)): sshHostAndPort,
		},
		NAT: map[string]string{
			hostIP: "127.0.0.1",
		},
		GatewayVirtualIPs: []string{hostIP},
		Protocol:          gvproxy.Protocol(),
	}

	if config.Protocol == types.HyperKitProtocol {
		config.VpnKitUUIDMacAddresses["c3d68012-0208-11ea-9fd7-f2189899ab08"] = vmMacAddress
	}
	return config
}
