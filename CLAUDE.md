# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

gvisor-tap-vsock is a userspace network stack implementation that replaces libslirp and VPNKit. It's written in pure Go and built on gVisor's network stack (`gvisor.dev/gvisor/pkg/tcpip`). It enables VMs to communicate with the host and external networks without requiring root privileges or kernel modules.

## Build Commands

- `make build` - Build all executables (gvproxy, qemu-wrapper, gvforwarder)
- `make gvproxy` - Build only the host-side daemon
- `make vm` - Build only the guest-side forwarder (Linux-only, CGO disabled)
- `make qemu-wrapper` - Build the QEMU socket wrapper
- `make win-sshproxy` - Build Windows SSH proxy (amd64 and arm64)
- `make win-gvproxy` - Build Windows gvproxy (amd64 and arm64)
- `make cross` - Cross-compile for multiple platforms
- `make vendor` - Tidy and vendor Go modules
- `make image` - Build container image (uses CONTAINER_RUNTIME, defaults to podman)
- `make clean` - Remove all built binaries

## Testing Commands

- `go test -v ./...` - Run all tests
- `make test` - Run all tests (builds gvproxy and test-companion first)
- `make test-qemu` - Run QEMU-specific integration tests
- `make test-mac` - Run vfkit (macOS) integration tests
- `make test-mac-debug` - Run vfkit tests with Delve debugger on port 2345

For running individual tests:
```bash
go test -v ./pkg/services/dns -run TestSpecificTest
```

### Debugging Tests

The `make test-mac-debug` command runs tests with Delve debugger listening on port 2345. You can connect with `dlv connect :2345` or use IDE integrations (GoLand, VS Code). See DEVELOPMENT.md for detailed setup instructions.

## Linting

- `make lint` - Run golangci-lint for current platform
- `make cross-lint` - Run linter for Linux, Darwin, and Windows

## Architecture

### Main Components

1. **gvproxy** (`cmd/gvproxy/`) - Host-side daemon that:
   - Runs the virtual network gateway
   - Provides HTTP API for stats and port forwarding
   - Listens for VM connections via various transports
   - Manages network services (DHCP, DNS, NAT)

2. **gvforwarder** (`cmd/vm/`) - Guest-side executable that:
   - Runs inside the VM (Linux only)
   - Creates a TAP interface
   - Connects to gvproxy via vsock, stdio, or other transports

3. **Transport Layer** (`pkg/transport/`) - Protocol-specific implementations:
   - **vsock**: Hyper-V/virtio-vsock for Windows/Linux/macOS
   - **QEMU**: TCP or Unix socket communication with QEMU
   - **Bess**: SOCK_SEQPACKET for User Mode Linux
   - **vfkit**: SOCK_DGRAM for vfkit VMs on macOS
   - **stdio**: Standard input/output for containerized scenarios
   - **VPNKit**: Hyperkit protocol compatibility

### Core Packages

- **pkg/virtualnetwork** - Virtual network implementation using gVisor stack:
  - Creates and manages the network stack
  - Integrates the network switch and services
  - Exposes HTTP mux for control API

- **pkg/tap** - Network switch and link layer:
  - `Switch`: L2 network switch with CAM table for packet routing
  - `LinkEndpoint`: Virtual TAP device implementation
  - `IPPool`: DHCP IP address management
  - Handles packet transmission/reception between VMs and gateway

- **pkg/services** - Network services:
  - `dhcp/`: DHCP server for automatic VM network configuration
  - `dns/`: DNS server with support for static zones and regex records
  - `forwarder/`: Dynamic port forwarding (TCP/UDP)

- **pkg/types** - Configuration structures:
  - `Configuration`: Main stack configuration (subnet, MTU, DNS zones, etc.)
  - `Protocol`: Transport protocol types (HyperKit, Qemu, Bess, Stdio, Vfkit)

- **pkg/notification** - Event notifications:
  - Sends JSON notifications over Unix socket
  - Events: ready, connection_established, connection_closed, hypervisor_error

### HTTP API Endpoints

When gvproxy runs with `--listen` or `--services`, it exposes an HTTP API (accessible via Unix socket or TCP):
- `GET /stats` - Network statistics (bytes sent/received, packets, etc.)
- `POST /services/forwarder/expose` - Dynamically expose a port (JSON: `{"local":":6443","remote":"192.168.127.2:6443"}`)
- `POST /services/forwarder/unexpose` - Unexpose a port (JSON: `{"local":":6443"}`)
- `GET /services/forwarder/all` - List all exposed ports
- `/connect` - Internal endpoint for VM connections (only with `--listen`, not `--services`)

Example: `curl --unix-socket /tmp/network.sock http:/unix/stats`

### Network Flow

1. **VM → Internet**: Packets from VM TAP → Switch → gVisor stack → Host network syscalls
2. **Host → VM**: Port forwarding creates listening sockets → gVisor stack → Switch → VM
3. **Packet routing**: Switch uses CAM (Content Addressable Memory) table to map MAC addresses to connection IDs

### Configuration

Configuration can be provided via:
- Command-line flags (see `cmd/gvproxy/config.go`)
- YAML configuration file (`--config-file` flag)
- Environment variables or defaults

Key configuration options:
- Subnet: Default `192.168.127.0/24`
- Gateway IP: Default `192.168.127.1`
- MTU: Default 1500
- DNS zones for custom name resolution
- Static DHCP leases for predictable VM IPs
- Port forwards map (host:port → VM:port)

## Testing Infrastructure

- **test-qemu/** - Integration tests using QEMU VMs
- **test-vfkit/** - Integration tests using vfkit on macOS
- **test-win-sshproxy/** - Windows SSH proxy tests
- **cmd/test-companion/** - Helper binary that runs inside test VMs

Test suite uses Ginkgo/Gomega for BDD-style testing.

## Platform-Specific Code

The codebase has platform-specific implementations in several places:
- `pkg/transport/listen_*.go` - Transport listening logic per platform
- `pkg/transport/dial_*.go` - Connection dialing per platform
- `cmd/vm/main_linux.go` - VM-side code (Linux only)
- Build tags control Windows GUI mode for `gvproxy.exe` and `win-sshproxy.exe`

## Key Behaviors

- **Graceful shutdown**: gvproxy handles SIGTERM/SIGINT and cleans up PID files, sockets
- **Debug mode**: When `-debug` flag is set, packets are logged and pprof endpoints exposed
- **Notifications**: Optional Unix socket for receiving connection state changes as JSON
- **EC2 metadata**: Optional EC2 metadata service access (169.254.169.254) when enabled

## Common Usage Patterns

### Running gvproxy with QEMU
```bash
# Terminal 1: Start gvproxy
bin/gvproxy -listen unix:///tmp/network.sock -listen-qemu unix:///tmp/qemu.sock

# Terminal 2: Start QEMU (with QEMU 7.2+)
qemu-system-x86_64 ... -netdev stream,id=vlan,addr.type=unix,addr.path=/tmp/qemu.sock \
  -device virtio-net-pci,netdev=vlan,mac=5a:94:ef:e4:0c:ee
```

### Running gvproxy with vfkit (macOS)
```bash
# Terminal 1: Start gvproxy
bin/gvproxy -listen unix:///tmp/network.sock --listen-vfkit unixgram:///tmp/vfkit.sock

# Terminal 2: Start vfkit
vfkit ... --device virtio-net,unixSocketPath=/tmp/vfkit.sock,mac=5a:94:ef:e4:0c:ee
```

### Running gvforwarder in VM
```bash
# Inside the VM (Linux only)
./gvforwarder -debug
```

## Limitations

- ICMP packets are not forwarded outside the virtual network (ping to external hosts won't work)
- Performance: 1.6-2.3 Gbits/s with MTU 4000 (tested with QEMU on macOS)

## Development Notes

- The project uses Go 1.25+ (see `go.mod`)
- Windows builds use `-H=windowsgui` to support backgrounding
- VM binary (`gvforwarder`) is built with `CGO_ENABLED=0` for portability
- Version is injected at build time via `-ldflags` using git describe
