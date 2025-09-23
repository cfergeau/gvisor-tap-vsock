package e2eutils

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"
)

// SSHConfig contains remote access information for SSH
type SSHConfig struct {
	// IdentityPath is the fq path to the ssh priv key
	IdentityPath  string
	PublicKeyPath string
	// SSH port for user networking
	Port int
	// RemoteUsername of the vm user
	RemoteUsername string
}
type VirtualMachine struct {
	gvproxyCmd *exec.Cmd
	gvErrChan  chan error
	gvSockets  []string

	hypervisorCmd *exec.Cmd
	hvErrChan     chan error

	sshConfig SSHConfig
}

func NewVirtualMachine(hvCmd, gvCmd *exec.Cmd) (*VirtualMachine, error) {
	if hvCmd == nil || gvCmd == nil {
		return nil, fmt.Errorf("both hypervisor and gvproxy commands are required")
	}
	return &VirtualMachine{
		gvproxyCmd:    gvCmd,
		hypervisorCmd: hvCmd,
	}, nil
}

func (vm *VirtualMachine) SetGvproxySockets(sockets ...string) {
	vm.gvSockets = sockets
}

func (vm *VirtualMachine) SetSSHConfig(config *SSHConfig) {
	vm.sshConfig = *config
}

func (vm *VirtualMachine) Start() error {
	log.Debugf("starting gvproxy")
	if err := vm.gvproxyCmd.Start(); err != nil {
		return err
	}
	if err := WaitGvproxy(vm.gvproxyCmd, vm.gvSockets...); err != nil {
		return err
	}
	log.Infof("gvproxy running")

	log.Infof("starting hypervisor")
	if err := vm.hypervisorCmd.Start(); err != nil {
		return err
	}
	sshExec := func(cmd ...string) ([]byte, error) {
		return vm.Run(cmd...)
	}
	if err := WaitSSH(vm.hypervisorCmd, sshExec); err != nil {
		return err
	}
	log.Infof("hypervisor running")

	return nil
}

func (vm *VirtualMachine) Kill() error {
	if vm.gvproxyCmd != nil {
		log.Infof("killing gvproxy")
		if err := vm.gvproxyCmd.Process.Kill(); err != nil {
			log.Infof("error killing gvproxy: %v", err)
		} else {
			log.Infof("no error")
		}
	}

	if vm.hypervisorCmd != nil {
		log.Infof("killing hypervisor")
		if err := vm.hypervisorCmd.Process.Kill(); err != nil {
			log.Infof("error killing hypervisor: %v", err)
		} else {
			log.Infof("no error")
		}
	}

	return nil
}

func (vm *VirtualMachine) Run(cmd ...string) ([]byte, error) {
	return vm.sshCommand(cmd...).Output()
}

func (vm *VirtualMachine) sshCommand(cmd ...string) *exec.Cmd {
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
