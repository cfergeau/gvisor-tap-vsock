//go:build darwin

package e2evfkit

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/containers/gvisor-tap-vsock/pkg/types"
	e2e_utils "github.com/containers/gvisor-tap-vsock/test-utils"
	vfkit "github.com/crc-org/vfkit/pkg/config"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	log "github.com/sirupsen/logrus"
)

func TestSuite(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "gvisor-tap-vsock suite")
}

const (
	vfkitSock    = "/tmp/vfkit.sock"
	ignitionSock = "/tmp/ignition.sock"
	sshPort      = 2223
	ignitionUser = "test"
	// #nosec "test" (for manual usage)
	ignitionPasswordHash = "$y$j9T$TqJWt3/mKJbH0sYi6B/LD1$QjVRuUgntjTHjAdAkqhkr4F73m.Be4jBXdAaKw98sPC" // notsecret
	efiStore             = "efi-variable-store"
	vfkitVersionNeeded   = 0.6
)

var (
	tmpDir string
	binDir string
	vm     *e2e_utils.VirtualMachine
	host   *exec.Cmd
	client *exec.Cmd
	cmdDir string
)

var debugEnabled = flag.Bool("debug", false, "enable debugger")

func init() {
	flag.StringVar(&binDir, "bin", "../bin", "directory with compiled binaries")
	cmdDir = "../cmd"
}

func gvproxyCmd(apiSocket string) *exec.Cmd {
	cmd := types.NewGvproxyCommand()
	cmd.AddEndpoint(fmt.Sprintf("unix://%s", apiSocket))
	cmd.AddVfkitSocket("unixgram://" + vfkitSock)
	cmd.SSHPort = sshPort

	return cmd.Cmd(filepath.Join(binDir, "gvproxy"))
}

func vfkitCmd(diskImage, ignFile string) (*exec.Cmd, error) {
	bootloader := vfkit.NewEFIBootloader(efiStore, true)
	vm := vfkit.NewVirtualMachine(2, 2048, bootloader)
	disk, err := vfkit.VirtioBlkNew(diskImage)
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
	net.SetUnixSocketPath(vfkitSock)
	err = vm.AddDevice(net)
	if err != nil {
		return nil, err
	}
	ignition, err := vfkit.IgnitionNew(ignFile, ignitionSock)
	if err != nil {
		return nil, err
	}
	vm.Ignition = ignition
	return vm.Cmd(e2e_utils.VfkitExecutable())
}

var _ = ginkgo.BeforeSuite(func() {
	tmpDir = ginkgo.GinkgoT().TempDir()
	// clear the environment before running the tests. It may happen the tests were abruptly stopped earlier leaving a dirty env
	cleanup()

	// check if vfkit version is greater than v0.5 (ignition support is available starting from v0.6)
	version, err := e2e_utils.VfkitVersion()
	gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
	gomega.Expect(version >= vfkitVersionNeeded).Should(gomega.BeTrue())

	// check if ssh port is free
	gomega.Expect(e2e_utils.IsPortAvailable(sshPort)).Should(gomega.BeTrue())

	gomega.Expect(os.MkdirAll(filepath.Join(tmpDir, "disks"), os.ModePerm)).Should(gomega.Succeed())

	fcosImage, err := e2e_utils.FetchDiskImage(e2e_utils.VFKit)
	gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

	privateKeyFile := filepath.Join(tmpDir, "id_test_vfkit")
	publicKeyFile := privateKeyFile + ".pub"
	publicKey, err := e2e_utils.CreateSSHKeys(publicKeyFile, privateKeyFile)
	gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

	ignFile := filepath.Join(tmpDir, "test.ign")
	err = e2e_utils.CreateIgnition(ignFile, publicKey, ignitionUser, ignitionPasswordHash)
	gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

	vm, err = e2e_utils.NewVirtualMachine()
	gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
	vm.SetSSHConfig(&e2e_utils.SSHConfig{
		IdentityPath:   privateKeyFile,
		Port:           sshPort,
		RemoteUsername: ignitionUser,
	})

	host = gvproxyCmd(vm.GvproxyAPISocket())
	if *debugEnabled {
		gvproxyArgs := host.Args[1:]
		dlvArgs := []string{"debug", "--headless", "--listen=:2345", "--api-version=2", "--accept-multiclient", filepath.Join(cmdDir, "gvproxy"), "--"}
		dlvArgs = append(dlvArgs, gvproxyArgs...)
		host = exec.Command("dlv", dlvArgs...)
	}

	host.Stderr = os.Stderr
	host.Stdout = os.Stdout

	gomega.Expect(host.Start()).Should(gomega.Succeed())
	err = e2e_utils.WaitGvproxy(host, vm.GvproxyAPISocket(), vfkitSock)
	gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

	client, err := vfkitCmd(fcosImage, ignFile)
	gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
	client.Stderr = os.Stderr
	client.Stdout = os.Stdout
	gomega.Expect(client.Start()).Should(gomega.Succeed())
	err = e2e_utils.WaitSSH(client, sshExec)
	gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
})

func cleanup() {
	_ = os.Remove(efiStore)
	_ = os.Remove(vm.GvproxyAPISocket())
	_ = os.Remove(vfkitSock)

	// this is handled by vfkit since vfkit v0.6.1 released in March 2025
	// it removes the ignition.sock file
	socketPath := filepath.Join(os.TempDir(), "ignition.sock")
	_ = os.Remove(socketPath)
}

var _ = ginkgo.AfterSuite(func() {
	if host != nil {
		if err := host.Process.Kill(); err != nil {
			log.Error(err)
		}
	}
	if client != nil {
		if err := client.Process.Kill(); err != nil {
			log.Error(err)
		}
	}
	cleanup()
})
