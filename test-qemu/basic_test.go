package e2eqemu

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"math"
	"os"
	"path"
	"path/filepath"
	"strings"

	gvproxyclient "github.com/containers/gvisor-tap-vsock/pkg/client"
	"github.com/containers/gvisor-tap-vsock/pkg/types"
	e2e "github.com/containers/gvisor-tap-vsock/test"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
)

func sshExec(cmd ...string) ([]byte, error) {
	return vm.Run(cmd...)
}

func gvproxyAPIClient() *gvproxyclient.Client {
	return vm.GvproxyAPIClient()
}

var _ = ginkgo.Describe("connectivity", func() {
	e2e.BasicConnectivityTests(e2e.BasicTestProps{
		SSHExec: sshExec,
	})
})

var _ = ginkgo.Describe("dns", func() {
	e2e.BasicDNSTests(e2e.BasicTestProps{
		SSHExec:          sshExec,
		GvproxyAPIClient: gvproxyAPIClient,
	})
})

var _ = ginkgo.Describe("dhcp", func() {
	e2e.BasicDHCPTests(e2e.BasicTestProps{
		SSHExec:          sshExec,
		GvproxyAPIClient: gvproxyAPIClient,
	})
})

var _ = ginkgo.Describe("upload and download", func() {
	tmpDir, err := os.MkdirTemp("", "vfkit")
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	sumMap := make(map[string]string)
	dstDir := "/tmp"
	ginkgo.AfterEach(func() {
		err := os.RemoveAll(tmpDir)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
	})
	ginkgo.It("should upload 1MB, 10MB, and 100MB files to hypervisor", func() {
		for _, size := range []int{6, 7, 8} {
			file, err := os.CreateTemp(tmpDir, "testfile")
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			err = file.Truncate(int64(math.Pow10(size)))
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			hasher := sha256.New()
			_, err = io.Copy(hasher, file)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			srcPath := file.Name()
			dstPath := filepath.Join(dstDir, path.Base(srcPath))

			err = vm.CopyToVM(srcPath, dstDir)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			out, err := vm.Run(fmt.Sprintf("sha256sum %s | awk '{print $1}'", dstPath))
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			localSum := hex.EncodeToString(hasher.Sum(nil))
			vmSum := strings.TrimSpace(string(out))
			gomega.Expect(vmSum).To(gomega.Equal(localSum))

			sumMap[dstPath] = vmSum
		}
	})
	ginkgo.It("should download the uploaded files from hypervisor", func() {
		// Download the uploaded files
		dlTmpDir, err := os.MkdirTemp("", "hv-dl")
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		for filename := range sumMap {
			err = vm.CopyFromVM(filename, dlTmpDir)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
		}

		dir, err := os.ReadDir(dlTmpDir)
		gomega.Expect(err).NotTo(gomega.HaveOccurred())
		for _, entry := range dir {
			hasher := sha256.New()
			file, err := os.Open(filepath.Join(dlTmpDir, entry.Name()))
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			_, err = io.Copy(hasher, file)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())

			gomega.Expect(hasher.Sum(nil)).NotTo(gomega.Equal(sumMap[entry.Name()]))

		}
		// Set tmpDir to dlTmpDir for cleanup in AfterEach
		tmpDir = dlTmpDir
	})
})

var _ = ginkgo.Describe("command-line format", func() {
	ginkgo.It("should convert Command to command line format", func() {
		command := types.NewGvproxyCommand()
		command.AddEndpoint("unix:///tmp/network.sock")
		command.AddServiceEndpoint("unix:///tmp/services.sock")
		command.Debug = true
		command.AddQemuSocket("tcp://0.0.0.0:1234")
		command.PidFile = "~/gv-pidfile.txt"
		command.LogFile = "~/gv.log"
		command.AddForwardUser("demouser")

		cmd := command.ToCmdline()
		gomega.Expect(cmd).To(gomega.Equal([]string{
			"-listen", "unix:///tmp/network.sock",
			"-services", "unix:///tmp/services.sock",
			"-debug",
			"-mtu", "1500",
			"-ssh-port", "2222",
			"-listen-qemu", "tcp://0.0.0.0:1234",
			"-forward-user", "demouser",
			"-pid-file", "~/gv-pidfile.txt",
			"-log-file", "~/gv.log",
		}))
	})
})
