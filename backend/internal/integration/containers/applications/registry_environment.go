package applications

import (
	"fmt"
	"io/fs"
	"path"
	"strings"
	"unicode/utf8"

	svc "github.com/futrx-com/remote.futrx.com/internal/service/applications"
)

func resolveEnvironmentDefaults(catalog fs.FS, root string, variables []svc.EnvVar) error {
	for index := range variables {
		variable := &variables[index]
		if variable.DefaultFile == "" {
			continue
		}
		if variable.Default != "" || !fs.ValidPath(variable.DefaultFile) ||
			!strings.HasPrefix(variable.DefaultFile, "infra/") {
			return fmt.Errorf("env %q has an invalid defaultFile", variable.Key)
		}
		contents, err := fs.ReadFile(catalog, path.Join(root, variable.DefaultFile))
		if err != nil {
			return fmt.Errorf("env %q defaultFile: %w", variable.Key, err)
		}
		if len(contents) > 128<<10 {
			return fmt.Errorf("env %q defaultFile exceeds 128 KiB", variable.Key)
		}
		if !utf8.Valid(contents) {
			return fmt.Errorf("env %q defaultFile is not UTF-8", variable.Key)
		}
		variable.Default = string(contents)
		variable.DefaultFile = ""
	}
	return nil
}
