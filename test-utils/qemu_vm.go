package e2eutils

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/containers/gvisor-tap-vsock/pkg/types"
	g "github.com/onsi/ginkgo/v2"
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
	vmConfig.networkSocket = net.JoinHostPort("127.0.0.1", "5555")
	vmConfig.servicesSocket = filepath.Join(g.GinkgoT().TempDir(), "api.sock")
	qemuCmd := defaultQemuConfig(vmConfig)
	gvCmd := defaultGvproxyConfig(vmConfig)

	vm, err := newVirtualMachine(&QemuCmdBuilder{qemuCmd}, &GvproxyCmdBuilder{gvCmd})
	if err != nil {
		return nil, err
	}
	vm.SetGvproxySockets(vmConfig.servicesSocket)
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

func defaultGvproxyConfig(vmConfig *VirtualMachineConfig) *types.GvproxyCommand {
	cmd := types.NewGvproxyCommand()
	cmd.AddServiceEndpoint(fmt.Sprintf("unix://%s", vmConfig.servicesSocket))
	cmd.AddQemuSocket("tcp://" + vmConfig.networkSocket)
	cmd.SSHPort = vmConfig.SSHConfig.Port

	return &cmd
}
