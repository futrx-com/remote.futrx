package antigravity

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os/exec"
	"sync"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/agent"
)

// streamPrintRun executes one agy print-mode process, forwarding stdout to the
// chat as raw text deltas. Chunked reads (not line scanning) keep blank lines
// intact so markdown paragraphs survive, and text streams as it arrives. The
// combined output tail is returned for error reporting.
func streamPrintRun(
	ctx context.Context,
	cmd *exec.Cmd,
	req agent.RunRequest,
	emit func(agent.Event),
) (string, error) {
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", err
	}
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("spawn agy: %w", err)
	}

	var tail tailBuffer
	stderrDone := make(chan struct{})
	go func() {
		defer close(stderrDone)
		scanner := bufio.NewScanner(stderr)
		scanner.Buffer(make([]byte, 0, 8192), 1<<20)
		for scanner.Scan() {
			line := scanner.Text()
			log.Printf("agy[%s] stderr: %s", req.ConversationID, line)
			tail.append(line + "\n")
		}
	}()

	var decoder UTF8StreamDecoder
	reader := bufio.NewReader(stdout)
	chunk := make([]byte, 4096)
	for {
		n, readErr := reader.Read(chunk)
		if n > 0 {
			text := decoder.Decode(chunk[:n])
			if len(text) > 0 {
				tail.append(text)
				emit(agent.Event{
					T:              time.Now().UnixMilli(),
					Type:           agent.EventAssistantTextDelta,
					Provider:       agent.ProviderAntigravity,
					ConversationID: req.ConversationID,
					ItemKind:       agent.ItemMessage,
					Text:           text,
				})
			}
		}
		if readErr != nil {
			if !errors.Is(readErr, io.EOF) && ctx.Err() == nil {
				log.Printf("agy[%s] stdout: %v", req.ConversationID, readErr)
			}
			break
		}
	}
	if remaining := decoder.Flush(); len(remaining) > 0 {
		tail.append(remaining)
		emit(agent.Event{
			T:              time.Now().UnixMilli(),
			Type:           agent.EventAssistantTextDelta,
			Provider:       agent.ProviderAntigravity,
			ConversationID: req.ConversationID,
			ItemKind:       agent.ItemMessage,
			Text:           remaining,
		})
	}

	err = cmd.Wait()
	<-stderrDone
	return tail.String(), err
}

// tailBuffer keeps the last few KB of process output for error messages.
// Appended to from both the stdout loop and the stderr goroutine.
type tailBuffer struct {
	mu   sync.Mutex
	data []byte
}

const tailBufferLimit = 4096

func (b *tailBuffer) append(s string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.data = append(b.data, s...)
	if len(b.data) > tailBufferLimit {
		b.data = b.data[len(b.data)-tailBufferLimit:]
	}
}

func (b *tailBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.data)
}
