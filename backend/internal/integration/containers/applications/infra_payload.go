package applications

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"strings"
)

// Limits on an application's infra payload. They bound what the archive can make
// the server read before any of it reaches a container, and the error text is
// derived from them so a raised limit cannot leave a stale number behind.
const (
	// maxInfraPayload caps the compressed archive as it sits in the catalog.
	maxInfraPayload = 8 << 20
	// maxInfraPayloadExpanded caps everything it unpacks to, which is what
	// a compression bomb would otherwise blow past.
	maxInfraPayloadExpanded = 32 << 20
)

// infraPayloadRoot is the single directory a payload may carry. Confining
// it to one known name is what keeps an archive from writing anywhere the
// install script did not expect.
const infraPayloadRoot = "infra"

// infra/payload.tar.gz carries an application's infra-side files. In particular,
// Go embed does not traverse nested Go modules, so their source is packed as
// a reproducible asset. This accepts an fs.FS so uploaded catalogs can use the
// same staging contract later. Applications without a payload keep their raw script.
func withInfraPayload(fsys fs.FS, root string, script []byte) ([]byte, error) {
	name := path.Join(root, "infra", "payload.tar.gz")
	info, err := fs.Stat(fsys, name)
	if errors.Is(err, fs.ErrNotExist) {
		return script, nil
	}
	if err != nil {
		return nil, err
	}
	if info.Size() > maxInfraPayload {
		return nil, fmt.Errorf("infra payload exceeds %d MiB", maxInfraPayload>>20)
	}
	payload, err := fs.ReadFile(fsys, name)
	if err != nil {
		return nil, err
	}
	if err := validateInfraPayload(payload); err != nil {
		return nil, err
	}
	return stageInfraPayload(payload, script), nil
}

// stageInfraPayload prepends archive extraction to an install script. Payload
// discovery and validation stay with the caller so generated container source
// and legacy infra/payload.tar.gz use the same transport.
func stageInfraPayload(payload, script []byte) []byte {
	payloadEnd := fmt.Sprintf("REMOTE_PAYLOAD_%x", sha256.Sum256(payload))
	scriptEnd := fmt.Sprintf("REMOTE_SCRIPT_%x", sha256.Sum256(script))
	var out strings.Builder
	out.WriteString("set -euo pipefail\nAPP_PACKAGE_DIR=$(mktemp -d)\nexport APP_PACKAGE_DIR\ntrap 'rm -rf -- \"$APP_PACKAGE_DIR\"' EXIT\n")
	fmt.Fprintf(&out, "base64 --decode <<'%s' | tar -xz --no-same-owner --no-same-permissions -C \"$APP_PACKAGE_DIR\"\n", payloadEnd)
	encoded := base64.StdEncoding.EncodeToString(payload)
	for len(encoded) > 0 {
		n := min(76, len(encoded))
		out.WriteString(encoded[:n])
		out.WriteByte('\n')
		encoded = encoded[n:]
	}
	fmt.Fprintf(&out, "%s\nbash -s <<'%s'\n%s\n%s\n", payloadEnd, scriptEnd, script, scriptEnd)
	return []byte(out.String())
}

func validateInfraPayload(payload []byte) error {
	compressed, err := gzip.NewReader(bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("infra payload: %w", err)
	}
	defer compressed.Close()
	// Limit both file contents and tar metadata to avoid oversized expansion.
	limited := &io.LimitedReader{R: compressed, N: maxInfraPayloadExpanded + 1}
	archive := tar.NewReader(limited)
	seen := map[string]bool{}
	for {
		header, err := archive.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("infra payload: %w", err)
		}
		name := strings.TrimSuffix(header.Name, "/")
		if !isInfraPayloadPath(name) {
			return fmt.Errorf("invalid infra payload path %q", header.Name)
		}
		if seen[name] {
			return fmt.Errorf("duplicate infra payload path %q", name)
		}
		seen[name] = true
		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeDir {
			return fmt.Errorf("infra payload links and special files are not supported")
		}
		if _, err := io.Copy(io.Discard, archive); err != nil {
			return fmt.Errorf("infra payload: %w", err)
		}
	}
	// Drain the gzip trailer too: a corrupt checksum must fail before extraction.
	if _, err := io.Copy(io.Discard, limited); err != nil {
		return fmt.Errorf("infra payload: %w", err)
	}
	if limited.N <= 0 {
		return fmt.Errorf(
			"infra payload exceeds %d MiB expanded", maxInfraPayloadExpanded>>20)
	}
	return nil
}

// isInfraPayloadPath reports whether a tar member names a file the payload
// is allowed to carry: a clean, relative path inside infraPayloadRoot and
// nothing else. A backslash is refused outright rather than normalized, since
// a member that needs one is not describing a path this ever extracts.
func isInfraPayloadPath(name string) bool {
	if !fs.ValidPath(name) || path.Clean(name) != name || strings.Contains(name, "\\") {
		return false
	}
	return name == infraPayloadRoot || strings.HasPrefix(name, infraPayloadRoot+"/")
}
