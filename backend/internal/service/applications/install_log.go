package applications

import (
	"encoding/base64"
	"encoding/json"
	"regexp"
	"sort"
	"strings"
)

const installLogLimit = 16 * 1024

var installANSI = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]`)

// installLog redacts before retaining output. A possible secret prefix is held
// between writes so polling cannot expose a secret split across pipe chunks.
// Its owner holds the run mutex for every call.
type installLog struct {
	secrets []string
	pending string
	text    string
}

func (l *installLog) protect(application Application, env map[string]string) {
	for _, variable := range application.Env {
		if !variable.Secret || env[variable.Key] == "" {
			continue
		}
		value := env[variable.Key]
		encoded, _ := json.Marshal(value)
		l.secrets = append(l.secrets, value, base64.StdEncoding.EncodeToString([]byte(value)), string(encoded[1:len(encoded)-1]))
	}
	sort.Slice(l.secrets, func(i, j int) bool { return len(l.secrets[i]) > len(l.secrets[j]) })
}

func (l *installLog) write(text string) {
	l.append(l.redactChunk(text))
}

func (l *installLog) redactChunk(text string) string {
	l.pending += text
	var clean strings.Builder
	i := 0
	for i < len(l.pending) {
		matched := false
		for _, secret := range l.secrets {
			if strings.HasPrefix(l.pending[i:], secret) {
				clean.WriteString("[redacted]")
				i += len(secret)
				matched = true
				break
			}
			if strings.HasPrefix(secret, l.pending[i:]) {
				l.pending = l.pending[i:]
				return clean.String()
			}
		}
		if !matched {
			clean.WriteByte(l.pending[i])
			i++
		}
	}
	l.pending = ""
	return clean.String()
}

func (l *installLog) finish() {
	if l.pending != "" {
		l.append("[redacted]")
		l.pending = ""
	}
}

func (l *installLog) append(text string) {
	l.text += text
	if len(l.text) > installLogLimit {
		l.text = l.text[len(l.text)-installLogLimit:]
	}
}

func (l *installLog) snapshot() string {
	return strings.ToValidUTF8(installANSI.ReplaceAllString(strings.ReplaceAll(l.text, "\r", "\n"), ""), "")
}

func (l *installLog) redact(text string) string {
	copy := installLog{secrets: l.secrets}
	copy.write(text)
	copy.finish()
	return copy.snapshot()
}

// Instance errors retain a prefix and tail, unlike the rolling live log.
// Mask the full message before either bound can split a secret.
func (l *installLog) redactInstanceError(text string) string {
	copy := installLog{secrets: l.secrets}
	copy.text = copy.redactChunk(text)
	if copy.pending != "" {
		copy.text += "[redacted]"
	}
	return boundedInstanceError(copy.snapshot())
}
