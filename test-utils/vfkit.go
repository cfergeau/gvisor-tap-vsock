package e2eutils

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/mod/semver"
	"github.com/containers/gvisor-tap-vsock/pkg/types"
	g "github.com/onsi/ginkgo/v2"
)

func VfkitGvproxyCmd(vmConfig *VirtualMachineConfig) *types.GvproxyCommand {
	cmd := gvproxyCmd(vmConfig)
	vmConfig.NetworkSocket = filepath.Join(g.GinkgoT().TempDir(), "net.sock")
	cmd.AddVfkitSocket("unixgram://" + vmConfig.NetworkSocket)

	return cmd
}

func NewVfkitVirtualMachine(vmConfig *VirtualMachineConfig) (*VirtualMachine, error) {
	gvCmd := VfkitGvproxyCmd(vmConfig)

	vm, err := newVirtualMachine(&GvproxyCmdBuilder{gvCmd})
	if err != nil {
		return nil, err
	}
	vm.SetGvproxySockets(vmConfig.servicesSocket, vmConfig.NetworkSocket)
	vm.SetSSHConfig(vmConfig.SSHConfig)

	return vm, nil
}

func VfkitVersion() (float64, error) {
	executable := VfkitExecutable()
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

func VfkitExecutable() string {
	vfkitBinaries := []string{"vfkit"}
	for _, binary := range vfkitBinaries {
		path, err := exec.LookPath(binary)
		if err == nil && path != "" {
			return path
		}
	}

	return ""
}
