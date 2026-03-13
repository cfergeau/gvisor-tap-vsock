package e2eqemu

import (
	"flag"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	e2e_utils "github.com/containers/gvisor-tap-vsock/test-utils"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	log "github.com/sirupsen/logrus"
)

func TestSuite(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "gvisor-tap-vsock suite")
}

const (
	qemuPort       = 5555
	sshPort        = 2222
	ignitionUser   = "test"
	qconLog        = "qcon.log"
	podmanSock     = "/run/user/1001/podman/podman.sock"
	podmanRootSock = "/run/podman/podman.sock"

	// #nosec "test" (for manual usage)
	ignitionPasswordHash = "$y$j9T$TqJWt3/mKJbH0sYi6B/LD1$QjVRuUgntjTHjAdAkqhkr4F73m.Be4jBXdAaKw98sPC"

	vmKind = e2e_utils.QEMU
)

var (
	tmpDir          string
	binDir          string
	client          *exec.Cmd
	privateKeyFile  string
	forwardSock     string
	forwardRootSock string
	vm              *e2e_utils.VirtualMachine
)

func init() {
	flag.StringVar(&binDir, "bin", "../bin", "directory with compiled binaries")

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
	tmpDir = ginkgo.GinkgoT().TempDir()
	privateKeyFile = filepath.Join(tmpDir, "id_test_qemu")
	publicKeyFile := privateKeyFile + ".pub"
	forwardSock = filepath.Join(tmpDir, "podman-remote.sock")
	forwardRootSock = filepath.Join(tmpDir, "podman-root-remote.sock")

	gomega.Expect(os.MkdirAll(filepath.Join("cache", "disks"), os.ModePerm)).Should(gomega.Succeed())
	fcosImage, err := e2e_utils.FetchDiskImage(vmKind)
	gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

	publicKey, err := e2e_utils.CreateSSHKeys(publicKeyFile, privateKeyFile)
	gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

	ignFile := filepath.Join(tmpDir, "test.ign")
	err = e2e_utils.CreateIgnition(ignFile, publicKey, ignitionUser, ignitionPasswordHash)
	gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

	vmConfig := e2e_utils.VirtualMachineConfig{
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

	qemuCmd := newQemuCmd()
	qemuCmd.SetIgnition(ignFile)
	qemuCmd.SetDrive(fcosImage, true)
	qemuCmd.SetNetdevSocket(net.JoinHostPort("127.0.0.1", strconv.Itoa(qemuPort)), "5a:94:ef:e4:0c:ee")
	qemuCmd.SetSerial(qconLog)
	client, err = qemuCmd.Cmd(qemuExecutable())
	gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
	client.Stderr = os.Stderr
	client.Stdout = os.Stdout
	gomega.Expect(client.Start()).Should(gomega.Succeed())
	err = e2e_utils.WaitSSH(client, sshExec)
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

	if client != nil {
		if err := client.Process.Kill(); err != nil {
			log.Error(err)
		}
	}
})
