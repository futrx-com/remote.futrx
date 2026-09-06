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

// container.tar.gz carries an image's container-side files. In particular,
// Go embed does not traverse nested Go modules, so their source is packed as
// a reproducible asset. This accepts an fs.FS so uploaded catalogs can use the
// same staging contract later. Images without a payload keep their raw script.
func withContainerPayload(fsys fs.FS, root string, script []byte) ([]byte, error) {
	name := path.Join(root, "container.tar.gz")
	info, err := fs.Stat(fsys, name)
	if errors.Is(err, fs.ErrNotExist) {
		return script, nil
	}
	if err != nil {
		return nil, err
	}
	if info.Size() > 8<<20 {
		return nil, fmt.Errorf("container payload exceeds 8 MiB")
	}
	payload, err := fs.ReadFile(fsys, name)
	if err != nil {
		return nil, err
	}
	if err := validateContainerPayload(payload); err != nil {
		return nil, err
	}
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
	return []byte(out.String()), nil
}

func validateContainerPayload(payload []byte) error {
	compressed, err := gzip.NewReader(bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("container payload: %w", err)
	}
	defer compressed.Close()
	// Limit both file contents and tar metadata to avoid oversized expansion.
	limited := &io.LimitedReader{R: compressed, N: 32<<20 + 1}
	archive := tar.NewReader(limited)
	seen := map[string]bool{}
	for {
		header, err := archive.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("container payload: %w", err)
		}
		name := strings.TrimSuffix(header.Name, "/")
		if !fs.ValidPath(name) || path.Clean(name) != name || (name != "container" && !strings.HasPrefix(name, "container/")) || strings.Contains(name, "\\") {
			return fmt.Errorf("invalid container payload path %q", header.Name)
		}
		if seen[name] {
			return fmt.Errorf("duplicate container payload path %q", name)
		}
		seen[name] = true
		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeDir {
			return fmt.Errorf("container payload links and special files are not supported")
		}
		if _, err := io.Copy(io.Discard, archive); err != nil {
			return fmt.Errorf("container payload: %w", err)
		}
	}
	// Drain the gzip trailer too: a corrupt checksum must fail before extraction.
	if _, err := io.Copy(io.Discard, limited); err != nil {
		return fmt.Errorf("container payload: %w", err)
	}
	if limited.N <= 0 {
		return fmt.Errorf("container payload exceeds 32 MiB expanded")
	}
	return nil
}
