package e2e_performance

import (
	"os"
	"path/filepath"
	"testing"

	gvproxyclient "github.com/containers/gvisor-tap-vsock/pkg/client"
	e2e "github.com/containers/gvisor-tap-vsock/test"
	e2e_utils "github.com/containers/gvisor-tap-vsock/test-utils"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
)

var _ = ginkgo.Describe("test performance with vfkit", func() {
	PerfTest(e2e.BasicTestProps{
		SSHExec:          func(args ...string) ([]byte, error) { return vm.Run(args...) },
		GvproxyAPIClient: func() *gvproxyclient.Client { return vm.GvproxyAPIClient() },
	})
})

func TestSuite(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "gvisor-tap-vsock suite")
}

const (
	// #nosec "test" (for manual usage)
	ignitionPasswordHash = "$y$j9T$TqJWt3/mKJbH0sYi6B/LD1$QjVRuUgntjTHjAdAkqhkr4F73m.Be4jBXdAaKw98sPC" // notsecret
)

var (
	vm         *e2e_utils.VirtualMachine
	hypervisor string
	vmKind     e2e_utils.VMKind
)

var _ = ginkgo.BeforeSuite(func() {
	// Parse hypervisor from environment variable or flag
	var err error
	vmKind, err = e2e_utils.GetVMKind(hypervisor)
	gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

	tmpDir := ginkgo.GinkgoT().TempDir()

	vmConfig := e2e_utils.VirtualMachineConfig{
		IgnitionFile: filepath.Join(tmpDir, "test.ign"),
		SSHConfig: &e2e_utils.SSHConfig{
			IdentityPath:   filepath.Join(tmpDir, "id_test"),
			Port:           2223,
			RemoteUsername: "test",
		},
	}

	// check if ssh port is free
	gomega.Expect(e2e_utils.IsPortAvailable(vmConfig.SSHConfig.Port)).Should(gomega.BeTrue())

	// download disk image
	gomega.Expect(os.MkdirAll(filepath.Join(tmpDir, "disks"), os.ModePerm)).Should(gomega.Succeed())

	fcosImage, err := e2e_utils.FetchDiskImage(vmKind)
	gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
	vmConfig.DiskImage = fcosImage

	// create ssh keys
	publicKeyFile := vmConfig.SSHConfig.IdentityPath + ".pub"
	publicKey, err := e2e_utils.CreateSSHKeys(publicKeyFile, vmConfig.SSHConfig.IdentityPath)
	gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

	// create ignition file
	err = e2e_utils.CreateIgnition(vmConfig.IgnitionFile, publicKey, vmConfig.SSHConfig.RemoteUsername, ignitionPasswordHash)
	gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

	// VM/gvproxy configuration / start
	vm, err = e2e_utils.NewVirtualMachine(vmKind, &vmConfig)
	gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

	err = vm.Start()
	gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
})

var _ = ginkgo.AfterSuite(func() {
	err := vm.Kill()
	gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
})
