package e2eutils

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/containers/gvisor-tap-vsock/pkg/types"
	vfkit "github.com/crc-org/vfkit/pkg/config"
	"golang.org/x/mod/semver"
)

type VirtualMachineConfig struct {
	DiskImage      string
	IgnitionFile   string
	IgnitionSocket string // vfkit-specific
	NetworkSocket  string
	EFIStore       string // vfkit-specific
	ServicesSocket string
	Logfile        string // for now only used with qemu
	SSHConfig      *SSHConfig
}

func VfkitCmd(vmConfig *VirtualMachineConfig) (*vfkit.VirtualMachine, error) {
	bootloader := vfkit.NewEFIBootloader(vmConfig.EFIStore, true)
	vm := vfkit.NewVirtualMachine(2, 2048, bootloader)
	disk, err := vfkit.VirtioBlkNew(vmConfig.DiskImage)
	if err != nil {
		return nil, err
	}
	err = vm.AddDevice(disk)
	if err != nil {
		return nil, err
	}
	net, err := vfkit.VirtioNetNew("5a:94:ef:e4:0c:ee")
	if err != nil {
		return nil, err
	}
	net.SetUnixSocketPath(vmConfig.NetworkSocket)
	err = vm.AddDevice(net)
	if err != nil {
		return nil, err
	}
	ignition, err := vfkit.IgnitionNew(vmConfig.IgnitionFile, vmConfig.IgnitionSocket)
	if err != nil {
		return nil, err
	}
	vm.Ignition = ignition

	return vm, nil
}

type VfkitCmdBuilder struct {
	*vfkit.VirtualMachine
}

func (cmd *VfkitCmdBuilder) Cmd() (*exec.Cmd, error) {
	goCmd, err := cmd.VirtualMachine.Cmd(vfkitExecutable())
	if err != nil {
		return nil, err
	}
	goCmd.Stdout = os.Stdout
	goCmd.Stderr = os.Stderr

	return goCmd, nil
}

func VfkitGvproxyCmd(vmConfig *VirtualMachineConfig) *types.GvproxyCommand {
	cmd := types.NewGvproxyCommand()
	cmd.AddEndpoint(fmt.Sprintf("unix://%s", vmConfig.ServicesSocket))
	cmd.AddVfkitSocket("unixgram://" + vmConfig.NetworkSocket)
	cmd.SSHPort = vmConfig.SSHConfig.Port

	return &cmd
}

func NewVfkitVirtualMachine(vmConfig *VirtualMachineConfig) (*VirtualMachine, error) {
	vfkitCmd, err := VfkitCmd(vmConfig)
	if err != nil {
		return nil, err
	}
	gvCmd := VfkitGvproxyCmd(vmConfig)

	vm, err := newVirtualMachine(&VfkitCmdBuilder{vfkitCmd}, &GvproxyCmdBuilder{gvCmd})
	if err != nil {
		return nil, err
	}
	vm.SetGvproxySockets(vmConfig.ServicesSocket, vmConfig.NetworkSocket)
	vm.SetSSHConfig(vmConfig.SSHConfig)

	return vm, nil
}

func VfkitCleanup(vmConfig *VirtualMachineConfig) {
	if vmConfig.EFIStore != "" {
		_ = os.Remove(vmConfig.EFIStore)
	}
	// this is handled by vfkit since vfkit v0.6.1 released in March 2025
	if vmConfig.IgnitionSocket != "" {
		_ = os.Remove(vmConfig.IgnitionSocket)
	}
	if vmConfig.NetworkSocket != "" {
		_ = os.Remove(vmConfig.NetworkSocket)
	}
	if vmConfig.ServicesSocket != "" {
		_ = os.Remove(vmConfig.ServicesSocket)
	}
}

func VfkitVersion() (float64, error) {
	executable := vfkitExecutable()
	if executable == "" {
		return 0, fmt.Errorf("vfkit executable not found")
	}
	out, err := exec.Command(executable, "-v").Output()
	if err != nil {
		return 0, err
	}
	version := strings.TrimPrefix(string(out), "vfkit version:")
	majorMinor := strings.TrimPrefix(semver.MajorMinor(strings.TrimSpace(version)), "v")
	versionF, err := strconv.ParseFloat(majorMinor, 64)
	if err != nil {
		return 0, err
	}
	return versionF, nil
}

func vfkitExecutable() string {
	vfkitBinaries := []string{"vfkit"}
	for _, binary := range vfkitBinaries {
		path, err := exec.LookPath(binary)
		if err == nil && path != "" {
			return path
		}
	}

	return ""
}
