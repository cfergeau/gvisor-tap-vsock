//go:build darwin

package e2evfkit

import (
	// "flag"
	"os"
	"path/filepath"
	"testing"

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
	sshPort      = 2223
	ignitionUser = "test"
	// #nosec "test" (for manual usage)
	ignitionPasswordHash = "$y$j9T$TqJWt3/mKJbH0sYi6B/LD1$QjVRuUgntjTHjAdAkqhkr4F73m.Be4jBXdAaKw98sPC" // notsecret
	vfkitVersionNeeded   = 0.6
)

var (
	tmpDir string
	vm     *e2e_utils.VirtualMachine
)

// var debugEnabled = flag.Bool("debug", false, "enable debugger")

var _ = ginkgo.BeforeSuite(func() {
	tmpDir = ginkgo.GinkgoT().TempDir()

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

	privateKeyFile := filepath.Join(tmpDir, "id_test_vfkit")
	publicKeyFile := privateKeyFile + ".pub"
	publicKey, err := e2e_utils.CreateSSHKeys(publicKeyFile, privateKeyFile)
	gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

	ignFile := filepath.Join(tmpDir, "test.ign")
	err = e2e_utils.CreateIgnition(ignFile, publicKey, ignitionUser, ignitionPasswordHash)
	gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

	/*
		if *debugEnabled {
			gvproxyArgs := host.Args[1:]
			dlvArgs := []string{"debug", "--headless", "--listen=:2345", "--api-version=2", "--accept-multiclient", filepath.Join(cmdDir, "gvproxy"), "--"}
			dlvArgs = append(dlvArgs, gvproxyArgs...)
			host = exec.Command("dlv", dlvArgs...)
		}
	*/

	vmConfig := &e2e_utils.VirtualMachineConfig{
		DiskImage:    fcosImage,
		IgnitionFile: ignFile,
		SSHConfig: &e2e_utils.SSHConfig{
			IdentityPath:   privateKeyFile,
			Port:           sshPort,
			RemoteUsername: ignitionUser,
		},
	}
	vm, err := e2e_utils.NewVirtualMachine(e2e_utils.VFKit, vmConfig)
	gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

	err = vm.Start()
	gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
})

var _ = ginkgo.AfterSuite(func() {
	log.Infof("after suite")
	err := vm.Kill()
	gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
	log.Infof("after kills")
	// e2e_utils.VfkitCleanup(vmConfig)
	log.Infof("after cleanup")
})
