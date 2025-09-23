package e2eutils

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

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
	return &VirtualMachine{}, nil
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
