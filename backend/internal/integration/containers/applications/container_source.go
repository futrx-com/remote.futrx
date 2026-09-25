package applications

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"path"
	"sort"
	"strings"

	configconstants "github.com/futrx-com/remote.futrx.com/internal/config/constants"
)

// containerSource is the packed result of an application's backend/container/
// directory: the archive the installer stages inside the target container, and a
// digest of the source it was built from.
//
// The digest is what makes a rebuild inside the container skippable. It covers
// the source rather than the archive so that it does not move when only the
// packing changes.
type containerSource struct {
	payload []byte
	digest  string
	// commands are the ./cmd/<name> directories to build, in sorted order. An
	// application whose container source is itself package main has none, and
	// is built as a single binary named after the application.
	commands []string
}

// packContainerSource packs an application's backend/container/ directory into
// the archive shape the installer already stages: members under infra/, so they
// land at $APP_PACKAGE_DIR/infra inside the container.
//
// This is what every application used to carry as its own infra/package.sh. It
// runs here instead so a application ships Go source and no shell, and so the archive
// cannot go stale against the source it was built from — there is no committed
// artifact to forget to regenerate.
//
// The archive is deterministic: sorted paths, zeroed timestamps and ownership,
// fixed modes. The same source packs to the same bytes, so an unchanged
// application does not look changed to anything downstream.
func packContainerSource(fsys fs.FS, root, applicationID string) (*containerSource, error) {
	dir := path.Join(root, backendDir, backendContainerDir)
	info, err := fs.Stat(fsys, dir)
	if err != nil || !info.IsDir() {
		return nil, nil
	}

	files, err := collectContainerFiles(fsys, dir)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("%s exists but carries no source", dir)
	}

	// A application that needs dependencies ships its own module file. One that does
	// not gets a synthesized one, so the common case is pure Go source with no
	// module bookkeeping to maintain.
	if _, ok := files["go.mod"]; !ok {
		files["go.mod"] = []byte(fmt.Sprintf(
			"module futrx.local/catalog/applications/%s/backend/container\n\ngo %s\n",
			applicationID, configconstants.ApplicationContainerGoVersion))
	}

	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)

	digest := sha256.New()
	var archive bytes.Buffer
	compressed, err := gzip.NewWriterLevel(&archive, gzip.BestCompression)
	if err != nil {
		return nil, err
	}
	writer := tar.NewWriter(compressed)
	for _, name := range names {
		contents := files[name]
		// Length-prefixed framing, so that no combination of names and contents
		// can be rearranged into the same digest.
		writeDigestChunk(digest, name)
		writeDigestChunk(digest, string(contents))
		header := &tar.Header{
			Typeflag: tar.TypeReg,
			Name:     path.Join(infraPayloadRoot, name),
			Size:     int64(len(contents)),
			Mode:     0o644,
			Format:   tar.FormatUSTAR,
		}
		if err := writer.WriteHeader(header); err != nil {
			return nil, err
		}
		if _, err := writer.Write(contents); err != nil {
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	if err := compressed.Close(); err != nil {
		return nil, err
	}

	payload := archive.Bytes()
	if len(payload) > maxInfraPayload {
		return nil, fmt.Errorf("%s exceeds %d MiB packed", dir, maxInfraPayload>>20)
	}
	if err := validateInfraPayload(payload); err != nil {
		return nil, err
	}
	return &containerSource{
		payload:  payload,
		digest:   hex.EncodeToString(digest.Sum(nil)),
		commands: containerCommands(names),
	}, nil
}

// collectContainerFiles reads every regular file under dir, keyed by its path
// relative to dir. Directories are not carried: tar extraction creates them, and
// leaving them out keeps the archive a pure function of the file contents.
func collectContainerFiles(fsys fs.FS, dir string) (map[string][]byte, error) {
	files := map[string][]byte{}
	total := 0
	err := fs.WalkDir(fsys, dir, func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		base := path.Base(name)
		// Skip what the Go tool itself ignores, so the archive holds what a
		// build would actually read.
		if entry.IsDir() {
			if name != dir && (strings.HasPrefix(base, ".") || strings.HasPrefix(base, "_")) {
				return fs.SkipDir
			}
			return nil
		}
		if !entry.Type().IsRegular() || strings.HasPrefix(base, ".") {
			return nil
		}
		contents, err := fs.ReadFile(fsys, name)
		if err != nil {
			return err
		}
		total += len(contents)
		if total > maxInfraPayloadExpanded {
			return fmt.Errorf("%s exceeds %d MiB of source", dir, maxInfraPayloadExpanded>>20)
		}
		files[strings.TrimPrefix(name, dir+"/")] = contents
		return nil
	})
	if err != nil {
		return nil, err
	}
	return files, nil
}

// containerCommands reports the cmd/<name> directories to build. Naming a binary
// after its directory is the ordinary Go convention, so an application states
// what it installs by where it puts main() and declares nothing.
func containerCommands(names []string) []string {
	seen := map[string]bool{}
	var commands []string
	for _, name := range names {
		rest, ok := strings.CutPrefix(name, "cmd/")
		if !ok {
			continue
		}
		command, _, ok := strings.Cut(rest, "/")
		if !ok || command == "" || seen[command] {
			continue
		}
		seen[command] = true
		commands = append(commands, command)
	}
	return commands
}

func writeDigestChunk(into io.Writer, value string) {
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(len(value)))
	into.Write(length[:])
	io.WriteString(into, value)
}
