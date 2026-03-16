package e2eqemu

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	e2e_utils "github.com/containers/gvisor-tap-vsock/test-utils"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
)

func TestSuite(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "gvisor-tap-vsock suite")
}

const (
	sshPort      = 2222
	ignitionUser = "test"

	// #nosec "test" (for manual usage)
	ignitionPasswordHash = "$y$j9T$TqJWt3/mKJbH0sYi6B/LD1$QjVRuUgntjTHjAdAkqhkr4F73m.Be4jBXdAaKw98sPC"
)

var (
	tmpDir     string
	vm         *e2e_utils.VirtualMachine
	hypervisor string
	vmKind     e2e_utils.VMKind
)

func init() {
	flag.StringVar(&hypervisor, "hypervisor", "qemu", "hypervisor to use (qemu or vfkit)")
}

var _ = ginkgo.BeforeSuite(func() {
	// Parse hypervisor from environment variable or flag
	var err error
	vmKind, err = e2e_utils.GetVMKind(hypervisor)
	gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

	tmpDir = ginkgo.GinkgoT().TempDir()
	privateKeyFile := filepath.Join(tmpDir, "id_test_qemu")
	publicKeyFile := privateKeyFile + ".pub"

	gomega.Expect(os.MkdirAll(filepath.Join("cache", "disks"), os.ModePerm)).Should(gomega.Succeed())
	fcosImage, err := e2e_utils.FetchDiskImage(vmKind)
	gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

	publicKey, err := e2e_utils.CreateSSHKeys(publicKeyFile, privateKeyFile)
	gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

	ignFile := filepath.Join(tmpDir, "test.ign")
	err = e2e_utils.CreateIgnition(ignFile, publicKey, ignitionUser, ignitionPasswordHash)
	gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

	vmConfig := e2e_utils.VirtualMachineConfig{
		DiskImage:    fcosImage,
		IgnitionFile: ignFile,
		SSHConfig: &e2e_utils.SSHConfig{
			IdentityPath:   privateKeyFile,
			Port:           sshPort,
			RemoteUsername: ignitionUser,
		},
	}

	vm, err = e2e_utils.NewVirtualMachine(vmKind, &vmConfig)
	gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

	err = vm.Start()
	gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
})

var _ = ginkgo.AfterSuite(func() {
	err := vm.Kill()
	gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
})
