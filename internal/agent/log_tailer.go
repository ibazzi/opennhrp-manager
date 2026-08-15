package agent

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"opennhrp-manager/internal/protocol"
)

var systemLogPrefix = regexp.MustCompile(`(?i)^.*?\bopennhrp(?:(?:-|_)ha)?(?:\[\d+\])?:\s*`)

func stripSystemLogPrefix(line string) string {
	if prefix := systemLogPrefix.FindString(line); prefix != "" {
		return strings.TrimSpace(line[len(prefix):])
	}
	return line
}

type LogTailer struct {
	nodeID  string
	logChan chan protocol.LogEntry
}

func NewLogTailer(nodeID string, logChan chan protocol.LogEntry) *LogTailer {
	return &LogTailer{
		nodeID:  nodeID,
		logChan: logChan,
	}
}

func (t *LogTailer) Start(ctx context.Context) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			cmd, stdout, readerName, err := t.startLogReader(ctx)
			if err != nil {
				log.Printf("[LogTailer %s] Start log reader error: %v, retrying in 5s...", t.nodeID, err)
				time.Sleep(5 * time.Second)
				continue
			}

			log.Printf("[LogTailer %s] Log tailing active via: %s", t.nodeID, readerName)
			scanner := bufio.NewScanner(stdout)
			for scanner.Scan() {
				line := strings.TrimSpace(scanner.Text())
				if line == "" {
					continue
				}

				lower := strings.ToLower(line)
				source := ""
				switch {
				case strings.Contains(lower, "opennhrp-ha") || strings.Contains(lower, "opennhrp_ha") || strings.Contains(lower, "opennhrp ha"):
					source = "ha"
				case strings.Contains(lower, "opennhrp") || strings.Contains(lower, "nhrp"):
					source = "core"
				default:
					continue
				}

				level := "INFO"
				if strings.Contains(lower, "err") || strings.Contains(lower, "fatal") || strings.Contains(lower, "failed") || strings.Contains(lower, "panic") {
					level = "ERROR"
				} else if strings.Contains(lower, "warn") {
					level = "WARN"
				} else if strings.Contains(lower, "debug") || strings.Contains(lower, "trace") {
					level = "DEBUG"
				}

				entry := protocol.LogEntry{
					NodeID:    t.nodeID,
					Source:    source,
					Level:     level,
					Message:   stripSystemLogPrefix(line),
					Timestamp: time.Now(),
				}

				select {
				case t.logChan <- entry:
				default:
				}
			}
			if err := scanner.Err(); err != nil {
				log.Printf("[LogTailer %s] Scanner error: %v", t.nodeID, err)
			}

			_ = cmd.Wait()
			time.Sleep(2 * time.Second)
		}
	}()
}

func (t *LogTailer) startLogReader(ctx context.Context) (*exec.Cmd, io.ReadCloser, string, error) {
	// 1. Try systemd journalctl (Standard on Ubuntu / Debian / CentOS / Oracle Linux)
	if _, err := exec.LookPath("journalctl"); err == nil {
		cmd := exec.CommandContext(ctx, "journalctl", "-f", "-n", "30", "--no-pager")
		stdout, err := cmd.StdoutPipe()
		if err == nil && cmd.Start() == nil {
			return cmd, stdout, "journalctl -f", nil
		}
	}

	// 2. Try OpenWrt logread
	if _, err := exec.LookPath("logread"); err == nil {
		cmd := exec.CommandContext(ctx, "logread", "-f")
		stdout, err := cmd.StdoutPipe()
		if err == nil && cmd.Start() == nil {
			return cmd, stdout, "logread -f", nil
		}
	}

	// 3. Try standard syslog files
	for _, logFile := range []string{"/var/log/syslog", "/var/log/messages", "/var/log/opennhrp.log", "/var/log/opennhrp-ha.log"} {
		if _, err := os.Stat(logFile); err == nil {
			cmd := exec.CommandContext(ctx, "tail", "-n", "30", "-F", logFile)
			stdout, err := cmd.StdoutPipe()
			if err == nil && cmd.Start() == nil {
				return cmd, stdout, "tail -F " + logFile, nil
			}
		}
	}

	return nil, nil, "", fmt.Errorf("no available system log reader found (journalctl, logread, /var/log/syslog)")
}
