//go:build darwin

package e2e_performance

import (
	"os"
	"path/filepath"
	"testing"

	e2e "github.com/containers/gvisor-tap-vsock/test"
	e2e_utils "github.com/containers/gvisor-tap-vsock/test-utils"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	log "github.com/sirupsen/logrus"

	"golang.org/x/sys/unix"
)

var _ = ginkgo.Describe("test performance with vfkit", func() {
	PerfTest(e2e.BasicTestProps{
		SSHExec: func(args ...string) ([]byte, error) { return vm.Run(args...) },
		Sock:    vmConfig.ServicesSocket,
	})
})

func TestSuite(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "gvisor-tap-vsock suite")
}

const (
	// #nosec "test" (for manual usage)
	ignitionPasswordHash = "$y$j9T$TqJWt3/mKJbH0sYi6B/LD1$QjVRuUgntjTHjAdAkqhkr4F73m.Be4jBXdAaKw98sPC" // notsecret
	vfkitVersionNeeded   = 0.6
	tmpDir               = "../tmp"
)

var (
	vm       *e2e_utils.VirtualMachine
	vmConfig = e2e_utils.VirtualMachineConfig{
		IgnitionFile:   filepath.Join(tmpDir, "test.ign"),
		IgnitionSocket: "/tmp/ignition.sock",
		NetworkSocket:  "/tmp/vfkit.sock",
		ServicesSocket: "/tmp/gvproxy-api-vfkit.sock",
		EFIStore:       "efi-variable-store",
	}

	sshConfig = e2e_utils.SSHConfig{
		IdentityPath:   filepath.Join(tmpDir, "id_test"),
		PublicKeyPath:  filepath.Join(tmpDir, "id_test.pub"),
		Port:           2223,
		RemoteUsername: "test",
	}
)

var _ = ginkgo.BeforeSuite(func() {
	// clear the environment before running the tests. It may happen the tests were abruptly stopped earlier leaving a dirty env
	e2e_utils.VfkitCleanup(&vmConfig)

	// check if vfkit version is greater than v0.5 (ignition support is available starting from v0.6)
	version, err := e2e_utils.VfkitVersion()
	gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
	gomega.Expect(version >= vfkitVersionNeeded).Should(gomega.BeTrue())

	// check if ssh port is free
	gomega.Expect(e2e_utils.IsPortAvailable(sshConfig.Port)).Should(gomega.BeTrue())

	// download disk image
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
	vmConfig.DiskImage = fcosImage

	// create ssh keys
	publicKey, err := e2e_utils.CreateSSHKeys(sshConfig.PublicKeyPath, sshConfig.IdentityPath)
	gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

	// create ignition file
	err = e2e_utils.CreateIgnition(vmConfig.IgnitionFile, publicKey, sshConfig.RemoteUsername, ignitionPasswordHash)
	gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

	// VM/gvproxy configuration / start
	host := e2e_utils.VfkitGvproxyCmd(&vmConfig, &sshConfig)

	client, err := e2e_utils.VfkitCmd(&vmConfig)
	gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

	vm, err = e2e_utils.NewVirtualMachine(client, host)
	gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
	vm.SetSSHConfig(&sshConfig)
	vm.SetGvproxySockets(vmConfig.ServicesSocket, vmConfig.NetworkSocket)

	err = vm.Start()
	gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
	log.Infof("ssh config: %+v", sshConfig)
})

var _ = ginkgo.AfterSuite(func() {
	log.Infof("after suite")
	err := vm.Kill()
	gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
	log.Infof("after kills")
	e2e_utils.VfkitCleanup(&vmConfig)
	log.Infof("after cleanup")
})
