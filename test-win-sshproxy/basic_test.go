//go:build windows

package e2e

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/url"
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

const (
	// npipeSource is the win-sshproxy source for the pipe at npipePath
	npipeSource = "npipe:////./pipe/fake_docker_engine"
	npipePath   = `\\.\pipe\fake_docker_engine`
)

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
		checkProxying(npipeSource, func(context.Context) (net.Conn, error) {
			return winio.DialPipe(npipePath, &timeout)
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

	It("proxies over a unix socket", func() {
		// AF_UNIX socket paths must be absolute on Windows
		socketPath, err := filepath.Abs(filepath.Join(tmpDir, "fake_docker_engine.sock"))
		Expect(err).ShouldNot(HaveOccurred())

		var dialer net.Dialer
		checkProxying(unixSocketURL(socketPath), func(ctx context.Context) (net.Conn, error) {
			return dialer.DialContext(ctx, "unix", socketPath)
		})
	})
})

// checkProxying starts a proxy forwarding the socket at source to the
// mock SSH server, and checks that the connections dial returns are
// proxied: a "ping" HTTP request must get a "pong" reply. The proxy is
// stopped when the check is done.
func checkProxying(source string, dial func(ctx context.Context) (net.Conn, error)) {
	err := startProxy(source)
	Expect(err).ShouldNot(HaveOccurred())
	defer func() {
		Expect(stopProxy(false)).ShouldNot(HaveOccurred())
	}()

	httpClient := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return dial(ctx)
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

// unixSocketURL returns the unix:// URL for the unix socket at path, to
// be used as a proxy source. Windows paths ("c:/...") get a leading
// "/", which the proxy strips when parsing the URL.
func unixSocketURL(path string) string {
	path = filepath.ToSlash(path)
	if path[0] != '/' {
		path = "/" + path
	}
	return (&url.URL{Scheme: "unix", Path: path}).String()
}
