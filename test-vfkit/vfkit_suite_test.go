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

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	log "github.com/sirupsen/logrus"

	"golang.org/x/sys/unix"
)

func TestSuite(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "gvisor-tap-vsock suite")
}

const (
	sock         = "/tmp/gvproxy-api-vfkit.sock"
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
	tmpDir         string
	binDir         string
	vm             *e2e_utils.VirtualMachine
	privateKeyFile string
	publicKeyFile  string
	ignFile        string
	cmdDir         string
)

var debugEnabled = flag.Bool("debug", false, "enable debugger")

func init() {
	flag.StringVar(&tmpDir, "tmpDir", "../tmp", "temporary working directory")
	flag.StringVar(&binDir, "bin", "../bin", "directory with compiled binaries")
	privateKeyFile = filepath.Join(tmpDir, "id_test_vfkit")
	publicKeyFile = privateKeyFile + ".pub"
	ignFile = filepath.Join(tmpDir, "test.ign")
	cmdDir = "../cmd"
}

func gvproxyCmd() *exec.Cmd {
	cmd := types.NewGvproxyCommand()
	cmd.AddEndpoint(fmt.Sprintf("unix://%s", sock))
	cmd.AddVfkitSocket("unixgram://" + vfkitSock)
	cmd.SSHPort = sshPort

	goCmd := cmd.Cmd(filepath.Join(binDir, "gvproxy"))
	goCmd.Stderr = os.Stderr
	goCmd.Stdout = os.Stdout

	return goCmd
}

var _ = ginkgo.BeforeSuite(func() {
	// clear the environment before running the tests. It may happen the tests were abruptly stopped earlier leaving a dirty env
	cleanup()

	// check if vfkit version is greater than v0.5 (ignition support is available starting from v0.6)
	version, err := e2e_utils.VfkitVersion()
	gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
	gomega.Expect(version >= vfkitVersionNeeded).Should(gomega.BeTrue())

	// check if ssh port is free
	gomega.Expect(e2e_utils.IsPortAvailable(sshPort)).Should(gomega.BeTrue())

	gomega.Expect(os.MkdirAll(filepath.Join(tmpDir, "disks"), os.ModePerm)).Should(gomega.Succeed())

	var fcosImage string
	const useCached = true
	if useCached {
		fcosImage = "../tmp/disks/fedora-coreos-43.20250917.1.0-applehv.aarch64.raw"
	} else {
		downloader, err := e2e_utils.NewFcosDownloader(filepath.Join(tmpDir, "disks"))
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
		fcosImage, err = downloader.DownloadImage("applehv", "raw.gz")
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
	}
	cloneFile := "../tmp/disks/fcos-clone.raw"
	os.Remove(cloneFile)
	err = unix.Clonefile(fcosImage, cloneFile, 0)
	gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
	fcosImage = cloneFile

	publicKey, err := e2e_utils.CreateSSHKeys(publicKeyFile, privateKeyFile)
	gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

	err = e2e_utils.CreateIgnition(ignFile, publicKey, ignitionUser, ignitionPasswordHash)
	gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

	host := gvproxyCmd()
	if *debugEnabled {
		gvproxyArgs := host.Args[1:]
		dlvArgs := []string{"debug", "--headless", "--listen=:2345", "--api-version=2", "--accept-multiclient", filepath.Join(cmdDir, "gvproxy"), "--"}
		dlvArgs = append(dlvArgs, gvproxyArgs...)
		host = exec.Command("dlv", dlvArgs...)
	}

	vmConfig := e2e_utils.VirtualMachineConfig{
		DiskImage:      fcosImage,
		IgnitionFile:   ignFile,
		IgnitionSocket: ignitionSock,
		NetworkSocket:  vfkitSock,
		ServicesSocket: sock,
		EFIStore:       efiStore,
	}
	client, err := e2e_utils.VfkitCmd(&vmConfig)
	gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

	vm, err = e2e_utils.NewVirtualMachine(client, host)
	gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
	vm.SetSSHConfig(&e2e_utils.SSHConfig{
		IdentityPath:   privateKeyFile,
		Port:           sshPort,
		RemoteUsername: ignitionUser,
	})
	vm.SetGvproxySockets(vmConfig.ServicesSocket, vmConfig.NetworkSocket)
	err = vm.Start()
	gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
})

func cleanup() {
	_ = os.Remove(efiStore)
	_ = os.Remove(sock)
	_ = os.Remove(vfkitSock)

	// this is handled by vfkit since vfkit v0.6.1 released in March 2025
	// it removes the ignition.sock file
	socketPath := filepath.Join(os.TempDir(), "ignition.sock")
	_ = os.Remove(socketPath)
}

var _ = ginkgo.AfterSuite(func() {
	log.Infof("after suite")
	err := vm.Kill()
	gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
	log.Infof("after kills")
	cleanup()
	log.Infof("after cleanup")
})
