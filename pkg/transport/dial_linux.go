package transport

import (
	"fmt"
	"math"
	"net"
	"net/url"
	"strconv"

	"github.com/containers/gvisor-tap-vsock/pkg/net/stdio"
	mdlayhervsock "github.com/mdlayher/vsock"
	"github.com/pkg/errors"
)

func Dial(endpoint string) (net.Conn, string, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return nil, "", err
	}
	switch parsed.Scheme {
	case "vsock":
		contextID, err := strconv.ParseUint(parsed.Hostname(), 10, 32)
		if err != nil {
			return nil, "", err
		}
		if contextID > math.MaxUint32 {
			return nil, "", fmt.Errorf("%d is an invalid vsock context ID", contextID)
		}
		port, err := strconv.ParseUint(parsed.Port(), 10, 32)
		if err != nil {
			return nil, "", err
		}
		if port > math.MaxUint32 {
			return nil, "", fmt.Errorf("%d is an invalid vsock port number", port)
		}
		conn, err := mdlayhervsock.Dial(uint32(contextID), uint32(port), nil) //#nosec G115 -- strconv.ParseUint(.., .., 32) guarantees no overflow
		return conn, parsed.Path, err
	case "unix":
		conn, err := net.Dial("unix", parsed.Path)
		return conn, "/connect", err
	case "stdio":
		var values []string
		for k, vs := range parsed.Query() {
			for _, v := range vs {
				values = append(values, fmt.Sprintf("-%s=%s", k, v))
			}
		}
		conn, err := stdio.Dial(parsed.Path, values...)
		return conn, "", err
	default:
		return nil, "", errors.New("unexpected scheme")
	}
}
