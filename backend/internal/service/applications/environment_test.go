package applications

import (
	"errors"
	"testing"
)

func TestResolveEnvValidatesCompleteJSONSettingsBeforeInstall(t *testing.T) {
	application := Application{Env: []EnvVar{{
		Key: "SETTINGS_JSON", Format: "json", Required: true,
		Default: `{"editor.fontSize": 14}`,
	}}}
	for _, provided := range []string{`{"editor.fontSize":`, `[]`, `null`, `"text"`} {
		if _, err := resolveEnv(application, map[string]string{"SETTINGS_JSON": provided}); !errors.Is(err, ErrInvalidEnv) {
			t.Errorf("value %q: error = %v, want ErrInvalidEnv", provided, err)
		}
	}
	chosen := `{"editor.fontSize": 18, "files.autoSave": "off"}`
	env, err := resolveEnv(application, map[string]string{"SETTINGS_JSON": chosen})
	if err != nil || env["SETTINGS_JSON"] != chosen {
		t.Fatalf("selected settings were changed: %+v, %v", env, err)
	}
	env, err = resolveEnv(application, nil)
	if err != nil || env["SETTINGS_JSON"] != application.Env[0].Default {
		t.Fatalf("default settings were not selected: %+v, %v", env, err)
	}
}
