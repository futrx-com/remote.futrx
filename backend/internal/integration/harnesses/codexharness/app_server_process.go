package codexharness

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os/exec"
	"strings"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
	configconstants "github.com/futrx-com/remote.futrx.com/internal/config/constants"
)

type appServerScanResult struct {
	envelope appServerEnvelope
	err      error
}

// appServerProcess owns the pipes and lifecycle of one app-server child. The
// run state machine consumes decoded envelopes without managing OS resources.
type appServerProcess struct {
	cmd           *exec.Cmd
	provider      agent.ProviderID
	providerLabel string
	logID         string

	stdin      io.WriteCloser
	encoder    *json.Encoder
	scanner    *bufio.Scanner
	stderrDone chan string
}

func newAppServerProcess(
	cmd *exec.Cmd,
	provider agent.ProviderID,
	providerLabel string,
	logID string,
) *appServerProcess {
	return &appServerProcess{
		cmd:           cmd,
		provider:      provider,
		providerLabel: providerLabel,
		logID:         logID,
	}
}

func (process *appServerProcess) start() error {
	stdin, err := process.cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := process.cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := process.cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := process.cmd.Start(); err != nil {
		return fmt.Errorf("spawn %s app-server: %w", process.providerLabel, err)
	}

	process.stdin = stdin
	process.encoder = json.NewEncoder(stdin)
	process.scanner = bufio.NewScanner(stdout)
	process.scanner.Buffer(
		make([]byte, configconstants.CodexHarnessStdoutInitialBufferSize),
		configconstants.CodexHarnessStdoutMaxBufferSize,
	)
	process.stderrDone = make(chan string, 1)
	go captureAppServerStderr(stderr, process.provider, process.logID, process.stderrDone)
	return nil
}

func (process *appServerProcess) write(message any) error {
	return process.encoder.Encode(message)
}

func (process *appServerProcess) scan(results chan<- appServerScanResult, stop <-chan struct{}) {
	defer close(results)
	for process.scanner.Scan() {
		line := append([]byte(nil), process.scanner.Bytes()...)
		if len(line) == 0 {
			continue
		}
		var envelope appServerEnvelope
		if err := json.Unmarshal(line, &envelope); err != nil {
			log.Printf("%s[%s] app-server parse: %v", process.provider, process.logID, err)
			continue
		}
		select {
		case results <- appServerScanResult{envelope: envelope}:
		case <-stop:
			return
		}
	}
	if err := process.scanner.Err(); err != nil {
		select {
		case results <- appServerScanResult{
			err: fmt.Errorf("%s app-server stdout: %w", process.providerLabel, err),
		}:
		case <-stop:
		}
	}
}

func (process *appServerProcess) closeInput() {
	_ = process.stdin.Close()
}

func (process *appServerProcess) kill() {
	if process.cmd.Process != nil {
		_ = process.cmd.Process.Kill()
	}
}

func (process *appServerProcess) wait() (error, string) {
	waitErr := process.cmd.Wait()
	return waitErr, <-process.stderrDone
}

func (process *appServerProcess) abort() {
	process.kill()
	_, _ = process.wait()
}

func captureAppServerStderr(reader io.Reader, provider agent.ProviderID, logID string, done chan<- string) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(
		make([]byte, configconstants.CodexHarnessStderrInitialBufferSize),
		configconstants.CodexHarnessStderrMaxBufferSize,
	)
	var captured strings.Builder
	for scanner.Scan() {
		line := scanner.Text()
		log.Printf("%s[%s] stderr: %s", provider, logID, line)
		if captured.Len() < configconstants.CodexHarnessStderrCaptureLimit {
			captured.WriteString(line)
			captured.WriteByte('\n')
		}
	}
	done <- captured.String()
}
