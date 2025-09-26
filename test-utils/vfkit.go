package e2eutils

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/containers/gvisor-tap-vsock/pkg/types"
	vfkit "github.com/crc-org/vfkit/pkg/config"
	g "github.com/onsi/ginkgo/v2"
	"golang.org/x/mod/semver"
)

type VirtualMachineConfig struct {
	DiskImage    string
	IgnitionFile string
	// IgnitionSocket string // vfkit-specific
	networkSocket string
	// EFIStore       string // vfkit-specific
	servicesSocket string
	Logfile        string // for now only used with qemu
	SSHConfig      *SSHConfig
}

func VfkitCmd(vmConfig *VirtualMachineConfig) (*vfkit.VirtualMachine, error) {
	tmpDir := g.GinkgoT().TempDir()
	bootloader := vfkit.NewEFIBootloader(filepath.Join(tmpDir, "efistore"), true)
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
	net.SetUnixSocketPath(vmConfig.networkSocket)
	err = vm.AddDevice(net)
	if err != nil {
		return nil, err
	}
	ignition, err := vfkit.IgnitionNew(vmConfig.IgnitionFile, filepath.Join(tmpDir, "ign.sock"))
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
	cmd.AddEndpoint(fmt.Sprintf("unix://%s", vmConfig.servicesSocket))
	cmd.AddVfkitSocket("unixgram://" + vmConfig.networkSocket)
	cmd.SSHPort = vmConfig.SSHConfig.Port

	return &cmd
}

func NewVfkitVirtualMachine(vmConfig *VirtualMachineConfig) (*VirtualMachine, error) {
	// cannot be initialized early as `GinkgoT().TempDir()` cannot be called outside of specific locations
	GvproxyAPISocket = filepath.Join(g.GinkgoT().TempDir(), "api.sock")
	vmConfig.networkSocket = filepath.Join(g.GinkgoT().TempDir(), "net.sock")
	vmConfig.servicesSocket = GvproxyAPISocket
	vfkitCmd, err := VfkitCmd(vmConfig)
	if err != nil {
		return nil, err
	}
	gvCmd := VfkitGvproxyCmd(vmConfig)

	vm, err := newVirtualMachine(&VfkitCmdBuilder{vfkitCmd}, &GvproxyCmdBuilder{gvCmd})
	if err != nil {
		return nil, err
	}
	vm.SetGvproxySockets(vmConfig.servicesSocket, vmConfig.networkSocket)
	vm.SetSSHConfig(vmConfig.SSHConfig)

	return vm, nil
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
