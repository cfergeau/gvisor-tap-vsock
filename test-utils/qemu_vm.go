package e2eutils

import (
	"net"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/containers/gvisor-tap-vsock/pkg/types"
	g "github.com/onsi/ginkgo/v2"
)

const (
	PodmanSock     = "/run/user/1001/podman/podman.sock"
	PodmanRootSock = "/run/podman/podman.sock"
)

type CmdBuilder interface {
	Cmd() (*exec.Cmd, error)
}

type QemuCmdBuilder struct {
	*qemuCmd
}

func (cmd *QemuCmdBuilder) Cmd() (*exec.Cmd, error) {
	goCmd, err := cmd.qemuCmd.Cmd(qemuExecutable())
	if err != nil {
		return nil, err
	}
	goCmd.Stdout = os.Stdout
	goCmd.Stderr = os.Stderr

	return goCmd, nil
}

type GvproxyCmdBuilder struct {
	*types.GvproxyCommand
}

func (cmd *GvproxyCmdBuilder) Cmd() (*exec.Cmd, error) {
	goCmd := cmd.GvproxyCommand.Cmd(filepath.Join("..", "bin", "gvproxy"))
	goCmd.Stdout = os.Stdout
	goCmd.Stderr = os.Stderr

	return goCmd, nil
}

func NewQemuVirtualMachine(vmConfig *VirtualMachineConfig) (*VirtualMachine, error) {
	// cannot be initialized early as `GinkgoT().TempDir()` cannot be called outside of specific locations
	GvproxyAPISocket = filepath.Join(g.GinkgoT().TempDir(), "api.sock")
	gvCmd := defaultGvproxyConfig(vmConfig)
	qemuCmd := defaultQemuConfig(vmConfig)

	vm, err := newVirtualMachine(vmConfig, &QemuCmdBuilder{qemuCmd}, &GvproxyCmdBuilder{gvCmd})
	if err != nil {
		return nil, err
	}
	vm.vmKind = QEMU
	vm.SetGvproxySockets(vmConfig.servicesSocket)
	vm.SetGvproxyUnixForwardSocks(vmConfig.gvForwardSocks)
	vm.SetSSHConfig(vmConfig.SSHConfig)

	return vm, nil
}

func defaultQemuConfig(vmConfig *VirtualMachineConfig) *qemuCmd {
	qemuCmd := newQemuCmd()
	qemuCmd.SetIgnition(vmConfig.IgnitionFile)
	qemuCmd.SetDrive(vmConfig.DiskImage, true)
	qemuCmd.SetNetdevSocket(vmConfig.networkSocket, "5a:94:ef:e4:0c:ee")
	tmpDir := g.GinkgoT().TempDir()
	qemuCmd.SetSerial(filepath.Join(tmpDir, "serial.log"))

	return qemuCmd
}

func addSSHForwards(cmd *types.GvproxyCommand, vmConfig *VirtualMachineConfig) []string {
	tmpDir := g.GinkgoT().TempDir()
	forwardSock := filepath.Join(tmpDir, "podman-remote.sock")
	cmd.AddForwardSock(forwardSock)
	cmd.AddForwardDest(PodmanSock)
	cmd.AddForwardUser(vmConfig.SSHConfig.RemoteUsername)
	cmd.AddForwardIdentity(vmConfig.SSHConfig.IdentityPath)

	forwardRootSock := filepath.Join(tmpDir, "podman-root-remote.sock")
	cmd.AddForwardSock(forwardRootSock)
	cmd.AddForwardDest(PodmanRootSock)
	cmd.AddForwardUser("root")
	cmd.AddForwardIdentity(vmConfig.SSHConfig.IdentityPath)

	return []string{forwardSock, forwardRootSock}
}

func defaultGvproxyConfig(vmConfig *VirtualMachineConfig) *types.GvproxyCommand {
	cmd := gvproxyCmd(vmConfig)
	vmConfig.networkSocket = net.JoinHostPort("127.0.0.1", "5555")
	cmd.AddQemuSocket("tcp://" + vmConfig.networkSocket)

	return cmd
}
