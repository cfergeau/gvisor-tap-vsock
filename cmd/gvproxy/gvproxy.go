package main

import (
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/containers/gvisor-tap-vsock/pkg/types"
	"github.com/pkg/errors"
)

type GvProxy struct {
	qemuSocket   string
	bessSocket   string
	vfkitSocket  string
	vpnkitSocket string

	protocol types.Protocol

	sshPort int
	pidFile string

	config   *types.Configuration
	forwards []Forward
}

func (gvproxy *GvProxy) SetVpnkitSocket(vpnkitSocket string) error {
	if gvproxy.vfkitSocket != "" || gvproxy.qemuSocket != "" || gvproxy.bessSocket != "" {
		return errors.New("only one of qemu, bess, vfkit or vpnkit can be used at a time")
	}
	gvproxy.vpnkitSocket = vpnkitSocket
	gvproxy.protocol = types.HyperKitProtocol

	return nil
}

func (gvproxy *GvProxy) SetQemuSocket(qemuSocket string) error {
	if gvproxy.vfkitSocket != "" || gvproxy.bessSocket != "" || gvproxy.vpnkitSocket != "" {
		return errors.New("only one of qemu, bess, vfkit or vpnkit can be used at a time")
	}
	uri, err := url.Parse(qemuSocket)
	if err != nil || uri == nil {
		return errors.Wrapf(err, "invalid value for listen-qemu")
	}
	if _, err := os.Stat(uri.Path); err == nil && uri.Scheme == "unix" {
		return errors.Errorf("%q already exists", uri.Path)
	}

	gvproxy.qemuSocket = qemuSocket
	gvproxy.protocol = types.QemuProtocol

	return nil
}

func (gvproxy *GvProxy) SetBessSocket(bessSocket string) error {
	if gvproxy.vfkitSocket != "" || gvproxy.qemuSocket != "" || gvproxy.vpnkitSocket != "" {
		return errors.New("only one of qemu, bess, vfkit or vpnkit can be used at a time")
	}
	uri, err := url.Parse(bessSocket)
	if err != nil || uri == nil {
		return errors.Wrapf(err, "invalid value for listen-bess")
	}
	if uri.Scheme != "unixpacket" {
		return errors.New("listen-bess must be unixpacket:// address")
	}
	if _, err := os.Stat(uri.Path); err == nil {
		return errors.Errorf("%q already exists", uri.Path)
	}
	gvproxy.bessSocket = bessSocket
	gvproxy.protocol = types.BessProtocol

	return nil
}

func (gvproxy *GvProxy) SetVfkitSocket(vfkitSocket string) error {
	if gvproxy.qemuSocket != "" || gvproxy.bessSocket != "" || gvproxy.vpnkitSocket != "" {
		return errors.New("only one of qemu, bess, vfkit or vpnkit can be used at a time")
	}
	uri, err := url.Parse(vfkitSocket)
	if err != nil || uri == nil {
		return errors.Wrapf(err, "invalid value for listen-vfkit")
	}
	if uri.Scheme != "unixgram" {
		return errors.New("listen-vfkit must be unixgram:// address")
	}
	if _, err := os.Stat(uri.Path); err == nil {
		return errors.Errorf("%q already exists", uri.Path)
	}
	gvproxy.vfkitSocket = vfkitSocket
	gvproxy.protocol = types.VfkitProtocol

	return nil
}

func (gvproxy *GvProxy) SetSSHPort(port int) error {
	if port < 1024 || port > 65535 {
		return errors.New("ssh-port value must be between 1024 and 65535")
	}

	gvproxy.sshPort = port

	return nil
}

func (gvproxy *GvProxy) SetPidFile(pidFile string) error {
	gvproxy.pidFile = pidFile

	return nil
}

func (gvproxy *GvProxy) CreatePidFile() error {
	f, err := os.Create(gvproxy.pidFile)
	if err != nil {
		return err
	}
	defer f.Close()
	pid := os.Getpid()
	if _, err := f.WriteString(strconv.Itoa(pid)); err != nil {
		return err
	}

	return f.Close()
}

func (gvproxy *GvProxy) RemovePidFile() error {
	if err := os.Remove(gvproxy.pidFile); err != nil {
		return err
	}
	return nil
}

func (gvproxy *GvProxy) SetConfig(config *types.Configuration) error {
	gvproxy.config = config

	return nil
}

type Forward struct {
	socketPath string
	src        *url.URL
	dest       *url.URL
	identity   string
}

func (gvproxy *GvProxy) SetForwards(forwardSocket, forwardDest, forwardUser, forwardIdentity []string, sshHostPort string) error {
	var forwards []Forward

	count := len(forwardSocket)
	if count != len(forwardDest) || count != len(forwardUser) || count != len(forwardIdentify) {
		return errors.New("-forward-sock, --forward-dest, --forward-user, and --forward-identity must all be specified together, " +
			"the same number of times, or not at all")
	}

	for i := 0; i < len(forwardIdentity); i++ {
		if _, err := os.Stat(forwardIdentity[i]); err != nil {
			return errors.Wrapf(err, "Identity file %s can't be loaded", forwardIdentity[i])
		}
	}

	for i := 0; i < len(forwardSocket); i++ {
		var (
			src *url.URL
			err error
		)
		if strings.Contains(forwardSocket[i], "://") {
			src, err = url.Parse(forwardSocket[i])
			if err != nil {
				return err
			}
		} else {
			src = &url.URL{
				Scheme: "unix",
				Path:   forwardSocket[i],
			}
		}

		dest := &url.URL{
			Scheme: "ssh",
			User:   url.User(forwardUser[i]),
			Host:   sshHostPort,
			Path:   forwardDest[i],
		}
		forward := Forward{
			socketPath: forwardSocket[i],
			src:        src,
			dest:       dest,
			identity:   forwardIdentity[i],
		}
		forwards = append(forwards, forward)
	}

	gvproxy.forwards = forwards

	return nil
}

func (gvproxy *GvProxy) Protocol() types.Protocol {
	return gvproxy.protocol
}
