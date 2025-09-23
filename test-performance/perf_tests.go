package e2e_performance

import (
	"context"
	"net"
	"net/http"
	"os/exec"

	gvclient "github.com/containers/gvisor-tap-vsock/pkg/client"
	"github.com/containers/gvisor-tap-vsock/pkg/types"
	e2e "github.com/containers/gvisor-tap-vsock/test"
	g "github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
)

func iperf3Executable() string {
	iperf3Binaries := []string{"iperf3", "iperf3-darwin"}
	for _, binary := range iperf3Binaries {
		path, err := exec.LookPath(binary)
		if err == nil && path != "" {
			return path
		}
	}

	return ""
}

func PerfTest(props e2e.BasicTestProps) {
	g.It("runs iperf3 in the VM", func() {
		g.By("running iperf3 server on the host")
		iperf3Path := iperf3Executable()
		gomega.Expect(iperf3Path).NotTo(gomega.Equal(""))
		iperf3Cmd := exec.Command(iperf3Path, "-s", "--logfile", "iperf3.log")
		err := iperf3Cmd.Start()
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		g.By("installing iperf3 in the VM")
		_, err = props.SSHExec("sudo rpm-ostree install --apply-live --assumeyes iperf3")
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		g.By("sending TCP data from VM to host")
		_, err = props.SSHExec("/usr/bin/iperf3 -c host.containers.internal")
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		g.By("receiving TCP data from host to VM")
		_, err = props.SSHExec("/usr/bin/iperf3 -c host.containers.internal -R")
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		g.By("sending UDP data from VM to host")
		_, err = props.SSHExec("/usr/bin/iperf3 -c host.containers.internal -u")
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		g.By("receiving UDP data from host to VM")
		_, err = props.SSHExec("/usr/bin/iperf3 -c host.containers.internal -R -u")
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		err = iperf3Cmd.Process.Kill()
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		// server
		g.By("exposing VM’s iperf3 5201 port on the host")
		client := gvclient.New(&http.Client{
			Transport: &http.Transport{
				DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
					return net.Dial("unix", props.Sock)
				},
			},
		}, "http://base")
		err = client.Expose(&types.ExposeRequest{
			Local:    "127.0.0.1:5201",
			Remote:   "192.168.127.2:5201",
			Protocol: types.TCP,
		})
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
		err = client.Expose(&types.ExposeRequest{
			Local:    "127.0.0.1:5201",
			Remote:   "192.168.127.2:5201",
			Protocol: types.UDP,
		})
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

		g.By("Starting an iperf3 server in the VM")
		_, err = props.SSHExec("/usr/bin/iperf3 --server --daemon --pidfile ~/iperf3.pid")
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		g.By("sending TCP data from host to VM")
		iperf3Cmd = exec.Command(iperf3Path, "-c", "127.0.0.1")
		err = iperf3Cmd.Run()
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		g.By("sending UDP data from host to VM")
		iperf3Cmd = exec.Command(iperf3Path, "-c", "127.0.0.1", "-R")
		err = iperf3Cmd.Run()
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		g.By("receiving TCP data from host to VM")
		iperf3Cmd = exec.Command(iperf3Path, "-c", "127.0.0.1", "-u", "--length", "9216")
		err = iperf3Cmd.Run()
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		g.By("receiving UDP data from host to VM")
		iperf3Cmd = exec.Command(iperf3Path, "-c", "127.0.0.1", "-u", "-R", "--length", "9216")
		err = iperf3Cmd.Run()
		gomega.Expect(err).NotTo(gomega.HaveOccurred())

		g.By("unexposing VM’s iperf3 5201 port")
		err = client.Unexpose(&types.UnexposeRequest{
			Local:    "127.0.0.1:5201",
			Protocol: types.TCP,
		})
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
		err = client.Unexpose(&types.UnexposeRequest{
			Local:    "127.0.0.1:5201",
			Protocol: types.UDP,
		})
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

		g.By("stopping the VM iperf3 server")
		_, err = props.SSHExec("kill $(cat ~/iperf3.pid)")
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

	})
}
