package e2eqemu

import (
	"flag"
	"os"
	"path/filepath"
	"testing"
	"time"

	e2e_utils "github.com/containers/gvisor-tap-vsock/test-utils"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
)

func TestSuite(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "gvisor-tap-vsock suite")
}

const (
	sshPort        = 2222
	ignitionUser   = "test"
	podmanSock     = "/run/user/1001/podman/podman.sock"
	podmanRootSock = "/run/podman/podman.sock"

	// #nosec "test" (for manual usage)
	ignitionPasswordHash = "$y$j9T$TqJWt3/mKJbH0sYi6B/LD1$QjVRuUgntjTHjAdAkqhkr4F73m.Be4jBXdAaKw98sPC"
)

var (
	tmpDir          string
	binDir          string
	privateKeyFile  string
	publicKeyFile   string
	forwardSock     string
	forwardRootSock string
	vm              *e2e_utils.VirtualMachine
)

func init() {
	flag.StringVar(&binDir, "bin", "../bin", "directory with compiled binaries")
	tmpDir = ginkgo.GinkgoT().TempDir()
	privateKeyFile = filepath.Join(tmpDir, "id_test_qemu")
	publicKeyFile = privateKeyFile + ".pub"
	forwardSock = filepath.Join(tmpDir, "podman-remote.sock")
	forwardRootSock = filepath.Join(tmpDir, "podman-root-remote.sock")
}

func addSSHForwards(vm *e2e_utils.VirtualMachine) {
	gvConfig := vm.GvproxyCmdBuilder()
	gvConfig.AddForwardSock(forwardSock)
	gvConfig.AddForwardDest(podmanSock)
	gvConfig.AddForwardUser(ignitionUser)
	gvConfig.AddForwardIdentity(privateKeyFile)

	gvConfig.AddForwardSock(forwardRootSock)
	gvConfig.AddForwardDest(podmanRootSock)
	gvConfig.AddForwardUser("root")
	gvConfig.AddForwardIdentity(privateKeyFile)
}

var _ = ginkgo.BeforeSuite(func() {
	gomega.Expect(os.MkdirAll(filepath.Join("cache", "disks"), os.ModePerm)).Should(gomega.Succeed())

	downloader, err := e2e_utils.NewFcosDownloader(filepath.Join("cache", "disks"))
	gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
	qemuImage, err := downloader.DownloadImage("qemu", "qcow2.xz")
	gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

	publicKey, err := e2e_utils.CreateSSHKeys(publicKeyFile, privateKeyFile)
	gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

	ignFile := ginkgo.GinkgoT().TempDir()
	err = e2e_utils.CreateIgnition(ignFile, publicKey, ignitionUser, ignitionPasswordHash)
	gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

	vmConfig := e2e_utils.VirtualMachineConfig{
		DiskImage:    qemuImage,
		IgnitionFile: ignFile,
		SSHConfig: &e2e_utils.SSHConfig{
			IdentityPath:   privateKeyFile,
			Port:           sshPort,
			RemoteUsername: ignitionUser,
		},
	}

	vm, err = e2e_utils.NewVirtualMachine(e2e_utils.QEMU, &vmConfig)
	gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
	addSSHForwards(vm)

	err = vm.Start()
	gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

	err = vm.CopyToVM(filepath.Join(binDir, "test-companion"), "/tmp/test-companion")
	gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

	// start an embedded DNS and http server in the VM. Wait a bit for the server to start.
	cmd := vm.SshCommand("sudo /tmp/test-companion")
	gomega.Expect(cmd.Start()).ShouldNot(gomega.HaveOccurred())
	time.Sleep(5 * time.Second)
})

var _ = ginkgo.AfterSuite(func() {
	err := vm.Kill()
	gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
})
