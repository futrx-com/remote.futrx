package devinharness

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os/exec"
	"strings"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
)

const (
	acpStdoutInitialBufferSize = 64 * 1024
	acpStdoutMaxBufferSize     = 16 * 1024 * 1024
	acpStderrInitialBufferSize = 4 * 1024
	acpStderrMaxBufferSize     = 1 * 1024 * 1024
	acpStderrCaptureLimit      = 8 * 1024
)

type acpScanResult struct {
	envelope acpEnvelope
	err      error
}

// acpProcess owns the pipes and lifecycle of one `devin acp` child. The run
// state machine consumes decoded envelopes without managing OS resources.
type acpProcess struct {
	cmd           *exec.Cmd
	provider      agent.ProviderID
	providerLabel string
	logID         string

	stdin      io.WriteCloser
	encoder    *json.Encoder
	scanner    *bufio.Scanner
	stderrDone chan string
}

func newACPProcess(
	cmd *exec.Cmd,
	provider agent.ProviderID,
	providerLabel string,
	logID string,
) *acpProcess {
	return &acpProcess{
		cmd:           cmd,
		provider:      provider,
		providerLabel: providerLabel,
		logID:         logID,
	}
}

func (process *acpProcess) start() error {
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
		return fmt.Errorf("spawn %s ACP server: %w", process.providerLabel, err)
	}

	process.stdin = stdin
	process.encoder = json.NewEncoder(stdin)
	process.scanner = bufio.NewScanner(stdout)
	process.scanner.Buffer(
		make([]byte, acpStdoutInitialBufferSize),
		acpStdoutMaxBufferSize,
	)
	process.stderrDone = make(chan string, 1)
	go captureACPStderr(stderr, process.provider, process.logID, process.stderrDone)
	return nil
}

func (process *acpProcess) write(message any) error {
	return process.encoder.Encode(message)
}

func (process *acpProcess) scan(results chan<- acpScanResult, stop <-chan struct{}) {
	defer close(results)
	for process.scanner.Scan() {
		line := append([]byte(nil), process.scanner.Bytes()...)
		if len(line) == 0 {
			continue
		}
		var envelope acpEnvelope
		if err := json.Unmarshal(line, &envelope); err != nil {
			log.Printf("%s[%s] acp parse: %v", process.provider, process.logID, err)
			continue
		}
		select {
		case results <- acpScanResult{envelope: envelope}:
		case <-stop:
			return
		}
	}
	if err := process.scanner.Err(); err != nil {
		select {
		case results <- acpScanResult{
			err: fmt.Errorf("%s ACP server stdout: %w", process.providerLabel, err),
		}:
		case <-stop:
		}
	}
}

func (process *acpProcess) closeInput() {
	_ = process.stdin.Close()
}

func (process *acpProcess) kill() {
	if process.cmd.Process != nil {
		_ = process.cmd.Process.Kill()
	}
}

func (process *acpProcess) wait() (error, string) {
	waitErr := process.cmd.Wait()
	return waitErr, <-process.stderrDone
}

func (process *acpProcess) abort() {
	process.kill()
	_, _ = process.wait()
}

func captureACPStderr(reader io.Reader, provider agent.ProviderID, logID string, done chan<- string) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(
		make([]byte, acpStderrInitialBufferSize),
		acpStderrMaxBufferSize,
	)
	var captured strings.Builder
	for scanner.Scan() {
		line := scanner.Text()
		log.Printf("%s[%s] stderr: %s", provider, logID, line)
		if captured.Len() < acpStderrCaptureLimit {
			captured.WriteString(line)
			captured.WriteByte('\n')
		}
	}
	done <- captured.String()
}
