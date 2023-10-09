package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/containers/gvisor-tap-vsock/pkg/net/stdio"
	"github.com/containers/gvisor-tap-vsock/pkg/sshclient"
	"github.com/containers/gvisor-tap-vsock/pkg/transport"
	"github.com/containers/gvisor-tap-vsock/pkg/types"
	"github.com/containers/gvisor-tap-vsock/pkg/virtualnetwork"
	"github.com/containers/winquit/pkg/winquit"
	"github.com/dustin/go-humanize"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

var (
	debug           bool
	mtu             int
	endpoints       arrayFlags
	vpnkitSocket    string
	qemuSocket      string
	bessSocket      string
	stdioSocket     string
	vfkitSocket     string
	forwardSocket   arrayFlags
	forwardDest     arrayFlags
	forwardUser     arrayFlags
	forwardIdentify arrayFlags
	sshPort         int
	pidFile         string
	exitCode        int
)

var (
	sshHostAndPort = net.JoinHostPort("192.168.127.2", "22")
)

func main() {
	flag.Var(&endpoints, "listen", "control endpoint")
	flag.BoolVar(&debug, "debug", false, "Print debug info")
	flag.IntVar(&mtu, "mtu", 1500, "Set the MTU")
	flag.IntVar(&sshPort, "ssh-port", 2222, "Port to access the guest virtual machine. Must be between 1024 and 65535")
	flag.StringVar(&vpnkitSocket, "listen-vpnkit", "", "VPNKit socket to be used by Hyperkit")
	flag.StringVar(&qemuSocket, "listen-qemu", "", "Socket to be used by Qemu")
	flag.StringVar(&bessSocket, "listen-bess", "", "unixpacket socket to be used by Bess-compatible applications")
	flag.StringVar(&stdioSocket, "listen-stdio", "", "accept stdio pipe")
	flag.StringVar(&vfkitSocket, "listen-vfkit", "", "unixgram socket to be used by vfkit-compatible applications")
	flag.Var(&forwardSocket, "forward-sock", "Forwards a unix socket to the guest virtual machine over SSH")
	flag.Var(&forwardDest, "forward-dest", "Forwards a unix socket to the guest virtual machine over SSH")
	flag.Var(&forwardUser, "forward-user", "SSH user to use for unix socket forward")
	flag.Var(&forwardIdentify, "forward-identity", "Path to SSH identity key for forwarding")
	flag.StringVar(&pidFile, "pid-file", "", "Generate a file with the PID in it")
	flag.Parse()

	ctx, cancel := context.WithCancel(context.Background())
	// Make this the last defer statement in the stack
	defer os.Exit(exitCode)

	groupErrs, ctx := errgroup.WithContext(ctx)
	// Setup signal channel for catching user signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)

	if debug {
		log.SetLevel(log.DebugLevel)
	}

	// Intercept WM_QUIT/WM_CLOSE events if on Windows as SIGTERM (noop on other OSs)
	winquit.SimulateSigTermOnQuit(sigChan)

	var gvproxy GvProxy

	// Make sure the qemu socket provided is valid syntax
	if len(qemuSocket) > 0 {
		if err := gvproxy.SetQemuSocket(qemuSocket); err != nil {
			exitWithError(err)
		}
	}
	if len(bessSocket) > 0 {
		if err := gvproxy.SetBessSocket(bessSocket); err != nil {
			exitWithError(err)
		}
	}
	if len(vfkitSocket) > 0 {
		if err := gvproxy.SetVfkitSocket(vfkitSocket); err != nil {
			exitWithError(err)
		}
	}

	if len(vpnkitSocket) > 0 {
		if err := gvproxy.SetVfkitSocket(vfkitSocket); err != nil {
			exitWithError(err)
		}
	}

	// If the given port is not between the privileged ports
	// and the oft considered maximum port, return an error.
	if err := gvproxy.SetSSHPort(sshPort); err != nil {
		exitWithError(err)
	}

	if err := gvproxy.SetForwards(forwardSocket, forwardDest, forwardUser, forwardIdentify, sshHostAndPort); err != nil {
		exitWithError(err)
	}

	// Create a PID file if requested
	if len(pidFile) > 0 {
		if err := gvproxy.SetPidFile(pidFile); err != nil {
			exitWithError(err)
		}

		if err := gvproxy.CreatePidFile(); err != nil {
			exitWithError(err)
		}
		// Remove the pid-file when exiting
		defer func() {
			if err := gvproxy.RemovePidFile(); err != nil {
				log.Error(err)
			}
		}()
	}

	config := defaultConfig(&gvproxy)
	if err := config.SetDebug(debug); err != nil {
		exitWithError(err)
	}
	if err := config.SetCaptureFile(captureFile()); err != nil {
		exitWithError(err)
	}
	if err := config.SetMTU(mtu); err != nil {
		exitWithError(err)
	}
	if err := config.SetSearchDomains(searchDomains()); err != nil {
		exitWithError(err)
	}

	if err := gvproxy.SetConfig(&config); err != nil {
		exitWithError(err)
	}

	groupErrs.Go(func() error {
		return run(ctx, groupErrs, &gvproxy, endpoints)
	})

	// Wait for something to happen
	groupErrs.Go(func() error {
		select {
		// Catch signals so exits are graceful and defers can run
		case <-sigChan:
			cancel()
			return errors.New("signal caught")
		case <-ctx.Done():
			return nil
		}
	})
	// Wait for all of the go funcs to finish up
	if err := groupErrs.Wait(); err != nil {
		log.Error(err)
		exitCode = 1
	}
}

type arrayFlags []string

func (i *arrayFlags) String() string {
	return "my string representation"
}

func (i *arrayFlags) Set(value string) error {
	*i = append(*i, value)
	return nil
}

func captureFile() string {
	if !debug {
		return ""
	}
	return "capture.pcap"
}

func listenVpnkit(ctx context.Context, g *errgroup.Group, vpnkitSocket string, vn *virtualnetwork.VirtualNetwork) error {
	vpnkitListener, err := transport.Listen(vpnkitSocket)
	if err != nil {
		return err
	}
	g.Go(func() error {
	vpnloop:
		for {
			select {
			case <-ctx.Done():
				break vpnloop
			default:
				// pass through
			}
			conn, err := vpnkitListener.Accept()
			if err != nil {
				log.Errorf("vpnkit accept error: %s", err)
				continue
			}
			g.Go(func() error {
				return vn.AcceptVpnKit(conn)
			})
		}
		return nil
	})

	return nil
}

func listenQemu(ctx context.Context, g *errgroup.Group, qemuSocket string, vn *virtualnetwork.VirtualNetwork) error {
	qemuListener, err := transport.Listen(qemuSocket)
	if err != nil {
		return err
	}

	g.Go(func() error {
		<-ctx.Done()
		if err := qemuListener.Close(); err != nil {
			log.Errorf("error closing %s: %q", qemuSocket, err)
		}
		return os.Remove(qemuSocket)
	})

	g.Go(func() error {
		conn, err := qemuListener.Accept()
		if err != nil {
			return errors.Wrap(err, "qemu accept error")

		}
		return vn.AcceptQemu(ctx, conn)
	})

	return nil
}

func listenBess(ctx context.Context, g *errgroup.Group, bessSocket string, vn *virtualnetwork.VirtualNetwork) error {
	bessListener, err := transport.Listen(bessSocket)
	if err != nil {
		return err
	}

	g.Go(func() error {
		<-ctx.Done()
		if err := bessListener.Close(); err != nil {
			log.Errorf("error closing %s: %q", bessSocket, err)
		}
		return os.Remove(bessSocket)
	})

	g.Go(func() error {
		conn, err := bessListener.Accept()
		if err != nil {
			return errors.Wrap(err, "bess accept error")

		}
		return vn.AcceptBess(ctx, conn)
	})

	return nil
}

func listenVfkit(ctx context.Context, g *errgroup.Group, vfkitSocket string, vn *virtualnetwork.VirtualNetwork) error {
	conn, err := transport.ListenUnixgram(vfkitSocket)
	if err != nil {
		return err
	}

	g.Go(func() error {
		<-ctx.Done()
		if err := conn.Close(); err != nil {
			log.Errorf("error closing %s: %q", vfkitSocket, err)
		}
		return os.Remove(vfkitSocket)
	})

	g.Go(func() error {
		vfkitConn, err := transport.AcceptVfkit(conn)
		if err != nil {
			return err
		}
		return vn.AcceptVfkit(ctx, vfkitConn)
	})

	return nil
}

func listenStdio(ctx context.Context, g *errgroup.Group, stdioSocket string, vn *virtualnetwork.VirtualNetwork) error {
	g.Go(func() error {
		conn := stdio.GetStdioConn()
		return vn.AcceptStdio(ctx, conn)
	})

	return nil
}

func createForwards(ctx context.Context, g *errgroup.Group, forwards []Forward, vn *virtualnetwork.VirtualNetwork) error {
	for i := 0; i < len(forwards); i++ {
		j := i
		g.Go(func() error {
			defer os.Remove(forwards[j].socketPath)
			forward, err := sshclient.CreateSSHForward(ctx, forwards[j].src, forwards[j].dest, forwards[j].identity, vn)
			if err != nil {
				return err
			}
			go func() {
				<-ctx.Done()
				// Abort pending accepts
				forward.Close()
			}()
		loop:
			for {
				select {
				case <-ctx.Done():
					break loop
				default:
					// proceed
				}
				err := forward.AcceptAndTunnel(ctx)
				if err != nil {
					log.Debugf("Error occurred handling ssh forwarded connection: %q", err)
				}
			}
			return nil
		})

	}
	return nil
}

func run(ctx context.Context, g *errgroup.Group, gvproxy *GvProxy, endpoints []string) error {
	vn, err := virtualnetwork.New((*types.Configuration)(gvproxy.config))
	if err != nil {
		return err
	}
	log.Info("waiting for clients...")

	for _, endpoint := range endpoints {
		log.Infof("listening %s", endpoint)
		ln, err := transport.Listen(endpoint)
		if err != nil {
			return errors.Wrap(err, "cannot listen")
		}
		httpServe(ctx, g, ln, withProfiler(vn))
	}

	ln, err := vn.Listen("tcp", fmt.Sprintf("%s:80", gvproxy.config.GatewayIP))
	if err != nil {
		return err
	}
	mux := http.NewServeMux()
	mux.Handle("/services/forwarder/all", vn.Mux())
	mux.Handle("/services/forwarder/expose", vn.Mux())
	mux.Handle("/services/forwarder/unexpose", vn.Mux())
	httpServe(ctx, g, ln, mux)

	if debug {
		g.Go(func() error {
		debugLog:
			for {
				select {
				case <-time.After(5 * time.Second):
					log.Debugf("%v sent to the VM, %v received from the VM\n", humanize.Bytes(vn.BytesSent()), humanize.Bytes(vn.BytesReceived()))
				case <-ctx.Done():
					break debugLog
				}
			}
			return nil
		})
	}

	if vpnkitSocket != "" {
		if err := listenVpnkit(ctx, g, vpnkitSocket, vn); err != nil {
			return err
		}
	}

	if qemuSocket != "" {
		if err := listenQemu(ctx, g, qemuSocket, vn); err != nil {
			return err
		}
	}

	if bessSocket != "" {
		if err := listenBess(ctx, g, bessSocket, vn); err != nil {
			return err
		}
	}

	if vfkitSocket != "" {
		if err := listenVfkit(ctx, g, vfkitSocket, vn); err != nil {
			return err
		}
	}

	if stdioSocket != "" {
		if err := listenStdio(ctx, g, stdioSocket, vn); err != nil {
			return err
		}
	}

	if err := createForwards(ctx, g, gvproxy.forwards, vn); err != nil {
		return err
	}

	return nil
}

func httpServe(ctx context.Context, g *errgroup.Group, ln net.Listener, mux http.Handler) {
	g.Go(func() error {
		<-ctx.Done()
		return ln.Close()
	})
	g.Go(func() error {
		s := &http.Server{
			Handler:      mux,
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 10 * time.Second,
		}
		err := s.Serve(ln)
		if err != nil {
			if err != http.ErrServerClosed {
				return err
			}
			return err
		}
		return nil
	})
}

func withProfiler(vn *virtualnetwork.VirtualNetwork) http.Handler {
	mux := vn.Mux()
	if debug {
		mux.HandleFunc("/debug/pprof/", pprof.Index)
		mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
		mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
		mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	}
	return mux
}

func exitWithError(err error) {
	log.Error(err)
	os.Exit(1)
}

func searchDomains() []string {
	if runtime.GOOS == "darwin" || runtime.GOOS == "linux" {
		f, err := os.Open("/etc/resolv.conf")
		if err != nil {
			log.Errorf("open file error: %v", err)
			return nil
		}
		defer f.Close()
		sc := bufio.NewScanner(f)
		searchPrefix := "search "
		for sc.Scan() {
			if strings.HasPrefix(sc.Text(), searchPrefix) {
				searchDomains := strings.Split(strings.TrimPrefix(sc.Text(), searchPrefix), " ")
				log.Debugf("Using search domains: %v", searchDomains)
				return searchDomains
			}
		}
		if err := sc.Err(); err != nil {
			log.Errorf("scan file error: %v", err)
			return nil
		}
	}
	return nil
}
