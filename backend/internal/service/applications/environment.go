package applications

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"strings"
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

// resolveEnv applies defaults, generates secrets, and enforces required inputs.
func resolveEnv(application Application, provided map[string]string) (map[string]string, error) {
	out := map[string]string{}
	for _, e := range application.Env {
		v := strings.TrimSpace(provided[e.Key])
		if v == "" {
			v = e.Default
		}
		if v == "" && e.Generate == "password" {
			v = generatePassword()
		}
		if v == "" && e.Required {
			return nil, fmt.Errorf("%w: %s", ErrRequiredEnv, e.Key)
		}
		if v != "" {
			if err := ValidateEnvValue(e, v); err != nil {
				return nil, err
			}
			out[e.Key] = v
		}
	}
	return out, nil
}

// reconcileInstanceEnv projects persisted inputs onto the current manifest.
// It preserves values that still exist, applies defaults/generators for new
// declarations, and drops removed keys so an upgrade cannot retain obsolete
// credentials or expose them after their secret declaration disappears.
func reconcileInstanceEnv(application Application, inst *Instance) error {
	env, err := resolveEnv(application, inst.Env)
	if err != nil {
		return err
	}
	inst.Env = env
	return nil
}

const passwordAlphabet = "abcdefghijkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"

func generatePassword() string {
	const n = 24
	b := make([]byte, n)
	_, _ = rand.Read(b)
	out := make([]byte, n)
	for i := range b {
		out[i] = passwordAlphabet[int(b[i])%len(passwordAlphabet)]
	}
	return string(out)
}
