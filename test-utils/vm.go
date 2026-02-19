package e2eutils

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	gvproxyclient "github.com/containers/gvisor-tap-vsock/pkg/client"
	g "github.com/onsi/ginkgo/v2"
)

var GvproxyAPISocket string

type VMKind int

const (
	QEMU VMKind = iota
	VFKit
)

func (k VMKind) String() string {
	switch k {
	case QEMU:
		return "qemu"
	case VFKit:
		return "applehv"
	default:
		return ""
	}
}

func (k VMKind) FcosFormatType() string {
	switch k {
	case QEMU:
		return "qcow2.xz"
	case VFKit:
		return "raw.gz"
	default:
		return ""
	}
}

// SSHConfig contains remote access information for SSH
type SSHConfig struct {
	// IdentityPath is the fq path to the ssh priv key
	IdentityPath string
	// SSH port for user networking
	Port int
	// RemoteUsername of the vm user
	RemoteUsername string
}

type VirtualMachine struct {
	sshConfig SSHConfig
}

func NewVirtualMachine() (*VirtualMachine, error) {
	// cannot be initialized early as `GinkgoT().TempDir()` cannot be called outside of specific locations
	GvproxyAPISocket = filepath.Join(g.GinkgoT().TempDir(), "api.sock")
	return &VirtualMachine{}, nil
}

func (vm *VirtualMachine) GvproxyAPISocket() string {
	return GvproxyAPISocket
}

func (vm *VirtualMachine) GvproxyAPIClient() *gvproxyclient.Client {
	return gvproxyclient.New(&http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", vm.GvproxyAPISocket())
			},
		},
	}, "http://base")
}

func FetchDiskImage(vmKind VMKind) (string, error) {
	cachePath := filepath.Join("cache", "disks")
	if err := os.MkdirAll(cachePath, os.ModePerm); err != nil {
		return "", err
	}

	downloader, err := NewFcosDownloader(cachePath)
	if err != nil {
		return "", err
	}
	image, err := downloader.DownloadImage(vmKind.String(), vmKind.FcosFormatType())
	if err != nil {
		return "", err
	}

	return image, nil
}

func (vm *VirtualMachine) SetSSHConfig(config *SSHConfig) {
	vm.sshConfig = *config
}

func (vm *VirtualMachine) Run(cmd ...string) ([]byte, error) {
	return vm.SshCommand(cmd...).Output()
}

func (vm *VirtualMachine) SshCommand(cmd ...string) *exec.Cmd {
	sshCmd := exec.Command("ssh",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "StrictHostKeyChecking=no",
		"-o", "IdentitiesOnly=yes",
		"-i", vm.sshConfig.IdentityPath,
		"-p", strconv.Itoa(vm.sshConfig.Port),
		fmt.Sprintf("%s@127.0.0.1", vm.sshConfig.RemoteUsername), "--", strings.Join(cmd, " ")) // #nosec G204
	return sshCmd
}

func (vm *VirtualMachine) scp(src, dst string) error {
	sshCmd := exec.Command("/usr/bin/scp",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "StrictHostKeyChecking=no",
		"-o", "IdentitiesOnly=yes",
		"-i", vm.sshConfig.IdentityPath,
		"-P", strconv.Itoa(vm.sshConfig.Port),
		src, dst) // #nosec G204
	sshCmd.Stderr = os.Stderr
	sshCmd.Stdout = os.Stdout
	return sshCmd.Run()
}
func (vm *VirtualMachine) CopyToVM(src, dst string) error {
	return vm.scp(src, fmt.Sprintf("%s@127.0.0.1:%s", vm.sshConfig.RemoteUsername, dst))
}

func (vm *VirtualMachine) CopyFromVM(src, dst string) error {
	return vm.scp(fmt.Sprintf("%s@127.0.0.1:%s", vm.sshConfig.RemoteUsername, src), dst)
}
