package applications

import (
	"encoding/json"
	"fmt"
)

// ValidateEnvValue checks structured inputs before they can be persisted or
// passed to an application's install script. The browser performs the same
// check for quick feedback, but direct API requests must meet it too.
func ValidateEnvValue(variable EnvVar, value string) error {
	if variable.Format != "json" || value == "" {
		return nil
	}
	if len(value) > 128<<10 {
		return fmt.Errorf("%w: %s exceeds 128 KiB", ErrInvalidEnv, variable.Key)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal([]byte(value), &object); err != nil {
		return fmt.Errorf("%w: %s must be a JSON object: %v", ErrInvalidEnv, variable.Key, err)
	}
	if object == nil {
		return fmt.Errorf("%w: %s must be a JSON object", ErrInvalidEnv, variable.Key)
	}
	return nil
}
