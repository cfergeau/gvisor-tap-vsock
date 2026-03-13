package e2eutils

import (
	"net"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/containers/gvisor-tap-vsock/pkg/types"
)

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
	gvCmd := defaultGvproxyConfig(vmConfig)

	vm, err := newVirtualMachine(&GvproxyCmdBuilder{gvCmd})
	if err != nil {
		return nil, err
	}
	vm.SetGvproxySockets(vmConfig.servicesSocket)
	vm.SetSSHConfig(vmConfig.SSHConfig)

	return vm, nil
}

func defaultGvproxyConfig(vmConfig *VirtualMachineConfig) *types.GvproxyCommand {
	cmd := gvproxyCmd(vmConfig)
	vmConfig.NetworkSocket = net.JoinHostPort("127.0.0.1", "5555")
	cmd.AddQemuSocket("tcp://" + vmConfig.NetworkSocket)

	return cmd
}
