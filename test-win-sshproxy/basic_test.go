//go:build windows

package e2e

import (
	"context"
	"errors"
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
		_, err := startProxy(npipeSource)
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
		socketPath, err := unixSocketPath("fake_docker_engine.sock")
		Expect(err).ShouldNot(HaveOccurred())

		var dialer net.Dialer
		checkProxying(unixSocketURL(socketPath), func(ctx context.Context) (net.Conn, error) {
			return dialer.DialContext(ctx, "unix", socketPath)
		})
	})

	It("can start when a stale unix socket is left behind", func() {
		socketPath, err := unixSocketPath("stale_docker_engine.sock")
		Expect(err).ShouldNot(HaveOccurred())

		// Leave a socket behind, as a crashed proxy would
		createStaleSocket(socketPath)

		var dialer net.Dialer
		checkProxying(unixSocketURL(socketPath), func(ctx context.Context) (net.Conn, error) {
			return dialer.DialContext(ctx, "unix", socketPath)
		})

		// The proxy replaced the stale socket with its own, and removed
		// the latter when stopping: nothing is left behind.
		_, err = os.Stat(socketPath)
		Expect(errors.Is(err, os.ErrNotExist)).To(BeTrue())
	})

	It("does not remove a plain file sitting at the unix socket path", func() {
		socketPath, err := unixSocketPath("plain.file")
		Expect(err).ShouldNot(HaveOccurred())

		plainFile := []byte("not a unix socket")
		Expect(os.WriteFile(socketPath, plainFile, 0o600)).Should(Succeed())

		cmd, err := startProxy(unixSocketURL(socketPath))
		Expect(err).ShouldNot(HaveOccurred())

		// The proxy cannot listen while the plain file sits at the
		// socket path, so it must exit on its own instead of serving.
		state := waitTimeout(cmd.Process, 20*time.Second)
		if state == nil {
			// Don't leave a running proxy behind for the next tests
			_ = cmd.Process.Kill()
			Fail("proxy did not exit after failing to listen on its unix socket")
		}
		Expect(state.Exited()).To(BeTrue())

		// The proxy must not have removed a file it does not own
		remaining, err := os.ReadFile(socketPath)
		Expect(err).ShouldNot(HaveOccurred())
		Expect(remaining).To(Equal(plainFile))

		// The proxy is gone, clean up the state file it left behind
		Expect(os.Remove(tidFile)).Should(Succeed())
	})
})

// checkProxying starts a proxy forwarding the socket at source to the
// mock SSH server, and checks that the connections dial returns are
// proxied: a "ping" HTTP request must get a "pong" reply. The proxy is
// stopped when the check is done.
func checkProxying(source string, dial func(ctx context.Context) (net.Conn, error)) {
	_, err := startProxy(source)
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

// unixSocketPath returns the absolute path of the unix socket file
// named name in the suite's temporary directory. AF_UNIX socket paths
// must be absolute on Windows.
func unixSocketPath(name string) (string, error) {
	return filepath.Abs(filepath.Join(tmpDir, name))
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

// createStaleSocket leaves a unix socket file behind at path, as a
// crashed proxy would: nothing listens on it anymore, but the file is
// still there.
func createStaleSocket(path string) {
	listener, err := net.Listen("unix", path)
	Expect(err).ShouldNot(HaveOccurred())
	listener.(*net.UnixListener).SetUnlinkOnClose(false)
	Expect(listener.Close()).ShouldNot(HaveOccurred())

	// The socket file must have been left behind
	_, err = os.Stat(path)
	Expect(err).ShouldNot(HaveOccurred())
}
