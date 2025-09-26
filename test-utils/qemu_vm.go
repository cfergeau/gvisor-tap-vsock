package e2eutils

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/containers/gvisor-tap-vsock/pkg/types"
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
	qemuCmd := defaultQemuConfig(vmConfig)
	gvCmd := defaultGvproxyConfig(vmConfig)

	vm, err := NewVirtualMachine(&QemuCmdBuilder{qemuCmd}, &GvproxyCmdBuilder{gvCmd})
	if err != nil {
		return nil, err
	}
	vm.SetGvproxySockets(vmConfig.ServicesSocket)

	return vm, nil
}

func defaultQemuConfig(vmConfig *VirtualMachineConfig) *qemuCmd {
	qemuCmd := newQemuCmd()
	qemuCmd.SetIgnition(vmConfig.IgnitionFile)
	qemuCmd.SetDrive(vmConfig.DiskImage, true)
	qemuCmd.SetNetdevSocket(vmConfig.NetworkSocket, "5a:94:ef:e4:0c:ee")
	qemuCmd.SetSerial(vmConfig.Logfile)

	return qemuCmd
}

func defaultGvproxyConfig(vmConfig *VirtualMachineConfig) *types.GvproxyCommand {
	cmd := types.NewGvproxyCommand()
	cmd.AddServiceEndpoint(fmt.Sprintf("unix://%s", vmConfig.ServicesSocket))
	cmd.AddQemuSocket("tcp://" + vmConfig.NetworkSocket)

	return &cmd
}
