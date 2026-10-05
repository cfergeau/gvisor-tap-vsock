package stdio

import (
	"net"
	"os"
	"os/exec"
	"strconv"
)

func Dial(endpoint string, arg ...string) (net.Conn, error) {
	cmd := exec.Command(endpoint, arg...)
	cmd.Stderr = os.Stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	err = cmd.Start()
	if err != nil {
		return nil, err
	}

	local := IoAddr{path: strconv.Itoa(os.Getpid())}
	remote := IoAddr{path: strconv.Itoa(cmd.Process.Pid)}
	conn := IoConn{
		reader: stdout,
		writer: stdin,
		local:  local,
		remote: remote,
		// Kill alone leaves the exited child unreaped (a zombie per
		// redial, e.g. when the endpoint fails to start); Wait reaps it.
		close: func() error {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			return nil
		},
	}
	return conn, nil
}

func GetStdioConn() net.Conn {
	local := IoAddr{path: strconv.Itoa(os.Getpid())}
	remote := IoAddr{path: "remote"}
	conn := IoConn{
		writer: os.Stdout,
		reader: os.Stdin,
		local:  local,
		remote: remote,
	}
	return conn
}
