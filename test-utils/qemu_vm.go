package e2eutils

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/containers/gvisor-tap-vsock/pkg/types"
)

type QemuConfig struct {
	gvConfig        *types.GvproxyCommand
	gvproxyEndpoint string
	qemuConfig      *qemuCmd
}

func (cfg *QemuConfig) GvproxyConfig() *types.GvproxyCommand {
	return cfg.gvConfig
}

func (cfg *QemuConfig) GvproxyEndpoint() string {
	return cfg.gvproxyEndpoint
}
func (cfg *QemuConfig) QemuConfig() *qemuCmd {
	return cfg.qemuConfig
}

func NewQemuConfig(vmConfig *VirtualMachineConfig) *QemuConfig {
	return &QemuConfig{
		gvproxyEndpoint: vmConfig.ServicesSocket,
		gvConfig:        defaultGvproxyConfig(vmConfig),
		qemuConfig:      defaultQemuConfig(vmConfig),
	}

}

func NewQemuVirtualMachine(qemuConfig *QemuConfig) (*VirtualMachine, error) {
	qemuCmd, err := qemuConfig.qemuConfig.Cmd(qemuExecutable())
	if err != nil {
		return nil, err
	}
	qemuCmd.Stdout = os.Stdout
	qemuCmd.Stderr = os.Stderr

	gvCmd := qemuConfig.gvConfig.Cmd(filepath.Join("..", "bin", "gvproxy"))
	gvCmd.Stdout = os.Stdout
	gvCmd.Stderr = os.Stderr

	vm, err := NewVirtualMachine(qemuCmd, gvCmd)
	if err != nil {
		return nil, err
	}
	vm.SetGvproxySockets(qemuConfig.gvproxyEndpoint)

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

/*
	cmd.AddForwardSock(forwardSock)
	cmd.AddForwardDest(podmanSock)
	cmd.AddForwardUser(ignitionUser)
	cmd.AddForwardIdentity(privateKeyFile)

	cmd.AddForwardSock(forwardRootSock)
	cmd.AddForwardDest(podmanRootSock)
	cmd.AddForwardUser("root")
	cmd.AddForwardIdentity(privateKeyFile)
*/
