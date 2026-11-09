//go:build windows

package e2e

import (
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	winio "github.com/Microsoft/go-winio"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var timeout = 1 * time.Minute

const npipeSource = "npipe:////./pipe/fake_docker_engine"

// expectPingThroughProxy sends an HTTP request through the proxied
// connection returned by dial and checks the mock server's reply.
func expectPingThroughProxy(dial func() (net.Conn, error)) {
	httpClient := &http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return dial()
			},
		},
	}

	Eventually(func(g Gomega) {
		resp, err := httpClient.Get("http://host/ping")
		g.Expect(err).ShouldNot(HaveOccurred())
		defer resp.Body.Close()

		g.Expect(resp.StatusCode).To(Equal(http.StatusOK))
		g.Expect(resp.ContentLength).To(Equal(int64(4)))

		reply := make([]byte, resp.ContentLength)
		_, err = io.ReadAtLeast(resp.Body, reply, len(reply))

		g.Expect(err).ShouldNot(HaveOccurred())
		g.Expect(string(reply)).To(Equal("pong"))

	}).Should(Succeed())
}

var _ = Describe("connectivity", func() {
	It("proxy exits as requested, without a kill", func() {
		err := startProxy(npipeSource)
		Expect(err).ShouldNot(HaveOccurred())

		var pid, tid uint32
		for i := 0; i < 20; i++ {
			pid, tid, err = readTid()
			if err == nil && tid != 0 {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}

		Expect(tid).ShouldNot(Equal(0))
		Expect(err).ShouldNot(HaveOccurred())
		proc, err := os.FindProcess(int(pid))
		Expect(err).ShouldNot(HaveOccurred())
		Expect(proc).ShouldNot(BeNil())
		err = stopProxy(true)
		Expect(err).ShouldNot(HaveOccurred())
	})

	It("proxies over a windows pipe", func() {
		err := startProxy(npipeSource)
		Expect(err).ShouldNot(HaveOccurred())
		defer func() {
			Expect(stopProxy(false)).ShouldNot(HaveOccurred())
		}()

		expectPingThroughProxy(func() (net.Conn, error) {
			return winio.DialPipe(`\\.\pipe\fake_docker_engine`, &timeout)
		})
	})

	It("windows event logs were created", func() {
		cmd := exec.Command("powershell", "-Command", "&{Get-WinEvent -ProviderName \".NET Runtime\" -MaxEvents 10 | Where-Object -Property Message -Match \"test:\"}")
		reader, err := cmd.StdoutPipe()
		Expect(err).ShouldNot(HaveOccurred())
		err = cmd.Start()
		Expect(err).ShouldNot(HaveOccurred())
		output, err := io.ReadAll(reader)
		Expect(err).ShouldNot(HaveOccurred())
		err = cmd.Wait()
		Expect(err).ShouldNot(HaveOccurred())
		Expect(strings.Contains(string(output), `[info ] test: Listening on: \\.\pipe\fake_docker_engine`)).Should(BeTrue())
		Expect(strings.Contains(string(output), `[debug] test: Socket forward established`)).Should(BeTrue())
	})

	Context("unix socket", func() {
		var socketPath string

		BeforeEach(func() {
			socketPath = filepath.Join(tmpDir, "fake_docker_engine.sock")
			_ = os.Remove(socketPath)
			DeferCleanup(func() {
				_ = os.Remove(socketPath)
			})
		})

		dialSocket := func() (net.Conn, error) {
			return net.DialTimeout("unix", socketPath, timeout)
		}

		It("proxies over a unix socket", func() {
			err := startProxy(socketPath)
			Expect(err).ShouldNot(HaveOccurred())
			defer func() {
				Expect(stopProxy(false)).ShouldNot(HaveOccurred())
			}()

			expectPingThroughProxy(dialSocket)
		})

		It("replaces a stale unix socket", func() {
			listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: socketPath, Net: "unix"})
			Expect(err).ShouldNot(HaveOccurred())
			// Leave the socket file behind, as a crashed proxy would
			listener.SetUnlinkOnClose(false)
			Expect(listener.Close()).Should(Succeed())
			fi, err := os.Lstat(socketPath)
			Expect(err).ShouldNot(HaveOccurred())
			Expect(fi.Mode() & os.ModeSocket).ShouldNot(BeZero())

			err = startProxy(socketPath)
			Expect(err).ShouldNot(HaveOccurred())
			defer func() {
				Expect(stopProxy(false)).ShouldNot(HaveOccurred())
			}()

			expectPingThroughProxy(dialSocket)
		})

		It("does not remove a plain file at the unix socket path", func() {
			const content = "not a socket"
			Expect(os.WriteFile(socketPath, []byte(content), 0o600)).Should(Succeed())

			cmd := proxyCommand(socketPath)
			Expect(cmd.Start()).Should(Succeed())
			// The proxy cannot listen on the socket path and exits by itself
			state := waitTimeout(cmd.Process, 20*time.Second)
			if state == nil {
				_ = cmd.Process.Kill()
			}
			Expect(state).ShouldNot(BeNil())

			Expect(os.ReadFile(socketPath)).Should(Equal([]byte(content)))
		})
	})
})
