package agent

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"os/exec"
	"strings"
	"time"
)

// SocketClient communicates directly with opennhrp / opennhrp-ha UNIX domain sockets on the Agent node
type SocketClient struct {
	opennhrpSocket   string
	opennhrpHASocket string
	opennhrpctlPath  string
}

func NewSocketClient(opennhrpSocket, opennhrpHASocket, opennhrpctlPath string) *SocketClient {
	return &SocketClient{
		opennhrpSocket:   opennhrpSocket,
		opennhrpHASocket: opennhrpHASocket,
		opennhrpctlPath:  opennhrpctlPath,
	}
}

// SendSocketCommand connects to a UNIX socket, writes command, and returns response string
func (s *SocketClient) SendSocketCommand(socketPath, command string, timeout time.Duration) (string, error) {
	if !strings.HasSuffix(command, "\n") {
		command += "\n"
	}

	conn, err := net.DialTimeout("unix", socketPath, timeout)
	if err != nil {
		return "", fmt.Errorf("dial socket %s failed: %w", socketPath, err)
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(timeout))

	if _, err := conn.Write([]byte(command)); err != nil {
		return "", fmt.Errorf("write to socket failed: %w", err)
	}

	// For stream sockets, shutdown write side so server knows command is complete
	if unixConn, ok := conn.(*net.UnixConn); ok {
		_ = unixConn.CloseWrite()
	}

	var buf bytes.Buffer
	_, err = io.Copy(&buf, conn)
	if err != nil && err != io.EOF {
		return "", fmt.Errorf("read from socket failed: %w", err)
	}

	return buf.String(), nil
}

// RunCLICommand executes opennhrpctl with arguments and returns stdout/err
func (s *SocketClient) RunCLICommand(ctx context.Context, args ...string) (string, error) {
	bin := s.opennhrpctlPath
	if bin == "" {
		bin = "opennhrpctl"
	}

	cmd := exec.CommandContext(ctx, bin, args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("run %s %v failed (%v): %s", bin, args, err, stderr.String())
	}

	return strings.TrimSpace(stdout.String()), nil
}
