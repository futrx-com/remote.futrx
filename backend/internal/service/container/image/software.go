package image

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
)

// Software is an operator-owned additive recipe. Core runtime dependencies
// stay managed by Remote; optional services should use installable applications.
type Software struct {
	Apt            []string `json:"apt,omitempty"`
	NPM            []string `json:"npm,omitempty"`
	PrebakeBrowser *bool    `json:"prebakeBrowser,omitempty"`
	PrebakeIDE     *bool    `json:"prebakeIDE,omitempty"`
	Digest         string   `json:"-"`
}

var aptPackage = regexp.MustCompile(`^[a-z0-9][a-z0-9+.-]*(?::[a-z0-9]+)?(?:=[A-Za-z0-9.+:~_-]+)?$`)
var npmPackage = regexp.MustCompile(`^(?:@[a-z0-9][a-z0-9._-]*/)?[a-z0-9][a-z0-9._-]*@[0-9]+\.[0-9]+\.[0-9]+(?:-[A-Za-z0-9.-]+)?$`)

func LoadSoftware(filename string) (Software, error) {
	if filename == "" {
		return Software{}, nil
	}
	file, err := os.Open(filename)
	if err != nil {
		return Software{}, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, 64*1024+1))
	if err != nil {
		return Software{}, err
	}
	if len(data) > 64*1024 || !bytes.HasPrefix(bytes.TrimSpace(data), []byte("{")) {
		return Software{}, fmt.Errorf("software manifest must be a JSON object below 64 KiB")
	}
	var software Software
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&software); err != nil {
		return Software{}, err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return Software{}, fmt.Errorf("software manifest must contain one object")
	}
	if err := software.Validate(); err != nil {
		return Software{}, err
	}
	digest := sha256.Sum256(data)
	software.Digest = hex.EncodeToString(digest[:])
	return software, nil
}
func (s Software) Validate() error {
	if len(s.Apt)+len(s.NPM) > 256 {
		return fmt.Errorf("too many software packages")
	}
	for _, name := range s.Apt {
		if !aptPackage.MatchString(name) {
			return fmt.Errorf("invalid apt package %q", name)
		}
	}
	for _, name := range s.NPM {
		if !npmPackage.MatchString(name) {
			return fmt.Errorf("npm tool %q requires an exact semantic version", name)
		}
	}
	return nil
}
func (s Software) Script() string {
	var script strings.Builder
	if len(s.Apt) > 0 {
		script.WriteString("\napt-get update -qq\napt-get install -y --no-install-recommends -- ")
		for _, name := range s.Apt {
			script.WriteString("'" + name + "' ")
		}
		script.WriteString("\n")
	}
	if len(s.NPM) > 0 {
		script.WriteString("\nnpm install -g -- ")
		for _, name := range s.NPM {
			script.WriteString("'" + name + "' ")
		}
		script.WriteString("\n")
	}
	return script.String()
}
func (b *Builder) WithSoftware(software Software) *Builder { b.software = software; return b }
