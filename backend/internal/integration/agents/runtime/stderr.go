package runtime

import (
	"bufio"
	"bytes"
	"io"
	"log"

	configconstants "github.com/futrx-com/remote.futrx.com/internal/config/constants"
)

// captureStderr logs progress as it arrives while retaining the final diagnostic.
// RunProcess owns the goroutine and drains this reader before waiting on exit.
func captureStderr(stderr io.Reader, name, logID string, maxLineBytes int) string {
	sc := bufio.NewScanner(stderr)
	sc.Buffer(make([]byte, 0, 8192), maxBytes(maxLineBytes, 1<<20))
	var captured bytes.Buffer
	for sc.Scan() {
		line := sc.Text()
		log.Printf("%s[%s] stderr: %s", name, logID, line)
		captured.WriteString(line)
		captured.WriteByte('\n')
		if excess := captured.Len() - configconstants.AgentProcessStderrTailBytes; excess > 0 {
			captured.Next(excess)
		}
	}
	return captured.String()
}
