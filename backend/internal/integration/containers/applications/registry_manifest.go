package applications

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"reflect"
	"strings"

	svc "github.com/futrx-com/remote.futrx.com/internal/service/applications"
	applicationapi "github.com/futrx-com/remote.futrx.com/pkg/applications"
)

// application.json describes metadata and small declarative contracts, not
// application payloads. Keeping it bounded prevents an uploaded package from
// turning catalog reload into an unexpectedly large allocation even though the
// package archive itself has a much larger, payload-oriented limit.
const maxApplicationManifestBytes = 256 << 10

// applicationManifestSchema is the declarative application.json surface. It
// intentionally excludes Source, Container, and Skills: those values are
// derived from the catalog/package contents and a manifest cannot set them.
// Keeping this schema separate also makes adding a new manifest field an
// explicit compatibility decision instead of exposing every field later added
// to the service model automatically.
type applicationManifestSchema struct {
	HostTools     []svc.HostTool                        `json:"hostTools,omitempty"`
	ID            string                                `json:"id"`
	Name          string                                `json:"name"`
	Description   string                                `json:"description,omitempty"`
	Category      string                                `json:"category,omitempty"`
	Version       string                                `json:"version,omitempty"`
	Icon          string                                `json:"icon,omitempty"`
	Scopes        []svc.Scope                           `json:"scopes"`
	Port          svc.Port                              `json:"port"`
	Env           []svc.EnvVar                          `json:"env,omitempty"`
	Service       *svc.ApplicationService               `json:"service,omitempty"`
	Install       string                                `json:"install"`
	Healthcheck   svc.Healthcheck                       `json:"healthcheck,omitempty"`
	Connection    svc.Connection                        `json:"connection,omitempty"`
	Base          string                                `json:"base,omitempty"`
	UI            *svc.ApplicationUI                    `json:"ui,omitempty"`
	Backend       *svc.ApplicationBackend               `json:"backend,omitempty"`
	Publishers    []applicationapi.PublisherDeclaration `json:"publishers,omitempty"`
	Subscriptions []applicationapi.Subscription         `json:"subscriptions,omitempty"`
}

// legacyApplicationManifestSchema freezes the manifest-visible shape from
// before publishers and subscriptions existed. A stored upload accepted by an
// older release must keep that old meaning: a previously unknown field named
// "publishers" cannot suddenly fail decoding or activate a new capability just
// because Remote learned that name later.
type legacyApplicationManifestSchema struct {
	HostTools   []svc.HostTool            `json:"hostTools,omitempty"`
	ID          string                    `json:"id"`
	Name        string                    `json:"name"`
	Description string                    `json:"description,omitempty"`
	Category    string                    `json:"category,omitempty"`
	Version     string                    `json:"version,omitempty"`
	Icon        string                    `json:"icon,omitempty"`
	Source      svc.ApplicationSource     `json:"source,omitempty"`
	Scopes      []svc.Scope               `json:"scopes"`
	Port        svc.Port                  `json:"port"`
	Env         []svc.EnvVar              `json:"env,omitempty"`
	Service     *svc.ApplicationService   `json:"service,omitempty"`
	Install     string                    `json:"install"`
	Healthcheck svc.Healthcheck           `json:"healthcheck,omitempty"`
	Connection  svc.Connection            `json:"connection,omitempty"`
	Base        string                    `json:"base,omitempty"`
	UI          *svc.ApplicationUI        `json:"ui,omitempty"`
	Backend     *svc.ApplicationBackend   `json:"backend,omitempty"`
	Container   *svc.ApplicationContainer `json:"container,omitempty"`
	Skills      []string                  `json:"skills,omitempty"`
}

func readApplicationManifest(fsys fs.FS, name string) ([]byte, error) {
	return readBoundedApplicationManifest(fsys, name, maxApplicationManifestBytes)
}

// A package already accepted by an older release may legitimately have a
// manifest above the new metadata-oriented limit. The historical upload path
// still bounded each archive member at maxPackageFile, so using that ceiling
// preserves every previously valid package without returning to an unbounded
// read of mutable state-directory files.
func readPersistedApplicationManifest(fsys fs.FS, name string) ([]byte, error) {
	return readBoundedApplicationManifest(fsys, name, maxPackageFile)
}

func readBoundedApplicationManifest(fsys fs.FS, name string, maximum int64) ([]byte, error) {
	file, err := fsys.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	raw, err := io.ReadAll(io.LimitReader(file, maximum+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > maximum {
		return nil, fmt.Errorf("application.json is larger than %d KiB", maximum>>10)
	}
	return raw, nil
}

func decodeApplicationManifest(raw []byte, application *svc.Application) error {
	if len(raw) > maxApplicationManifestBytes {
		return fmt.Errorf("application.json is larger than %d KiB", maxApplicationManifestBytes>>10)
	}
	if err := validateManifestJSONShape(raw); err != nil {
		return err
	}

	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(application); err != nil {
		return err
	}
	if err := requireJSONEOF(decoder); err != nil {
		return err
	}
	return nil
}

// decodePersistedApplicationManifest first honors the current strict contract.
// That is how a package uploaded by this release retains its event declarations
// after it moves from staging into the persistent catalog. If strict decoding
// fails, the frozen pre-events schema recreates the behavior under which an
// older package was accepted, including case-insensitive field matching and
// ignored unknown fields, while deliberately omitting new event capabilities.
func decodePersistedApplicationManifest(raw []byte, application *svc.Application) error {
	var current svc.Application
	if err := decodeApplicationManifest(raw, &current); err == nil {
		*application = current
		return nil
	}

	var legacy legacyApplicationManifestSchema
	if err := json.Unmarshal(raw, &legacy); err != nil {
		return err
	}
	*application = legacy.application()
	return nil
}

func (legacy legacyApplicationManifestSchema) application() svc.Application {
	return svc.Application{
		HostTools:   legacy.HostTools,
		ID:          legacy.ID,
		Name:        legacy.Name,
		Description: legacy.Description,
		Category:    legacy.Category,
		Version:     legacy.Version,
		Icon:        legacy.Icon,
		Source:      legacy.Source,
		Scopes:      legacy.Scopes,
		Port:        legacy.Port,
		Env:         legacy.Env,
		Service:     legacy.Service,
		Install:     legacy.Install,
		Healthcheck: legacy.Healthcheck,
		Connection:  legacy.Connection,
		Base:        legacy.Base,
		UI:          legacy.UI,
		Backend:     legacy.Backend,
		Container:   legacy.Container,
		Skills:      legacy.Skills,
	}
}

// validateManifestJSONShape walks the JSON token stream before decoding it.
// encoding/json otherwise accepts field names case-insensitively and silently
// keeps the last value for a repeated field, either of which could make a
// reviewed declaration and the declaration Remote uses disagree.
func validateManifestJSONShape(raw []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := inspectJSONValue(decoder, "$", reflect.TypeOf(applicationManifestSchema{})); err != nil {
		return err
	}
	return requireJSONEOF(decoder)
}

func inspectJSONValue(decoder *json.Decoder, location string, expected reflect.Type) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}

	switch delimiter {
	case '{':
		expected = indirectJSONType(expected)
		var schemaFields map[string]reflect.Type
		var mapElement reflect.Type
		if expected != nil {
			switch expected.Kind() {
			case reflect.Struct:
				schemaFields = jsonStructFields(expected)
			case reflect.Map:
				if expected.Key().Kind() == reflect.String {
					mapElement = expected.Elem()
				}
			}
		}
		seen := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return fmt.Errorf("object key at %s is not a string", location)
			}
			if _, exists := seen[key]; exists {
				return fmt.Errorf("duplicate JSON field %q at %s", key, location)
			}
			seen[key] = struct{}{}

			var childType reflect.Type
			if expected != nil && expected.Kind() == reflect.Struct {
				var known bool
				childType, known = schemaFields[key]
				if !known {
					return fmt.Errorf("unknown JSON field %q at %s", key, location)
				}
			} else {
				childType = mapElement
			}
			if err := inspectJSONValue(decoder, location+"."+key, childType); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil {
			return err
		}
		if closing != json.Delim('}') {
			return fmt.Errorf("object at %s has invalid closing delimiter", location)
		}
	case '[':
		expected = indirectJSONType(expected)
		var element reflect.Type
		if expected != nil && (expected.Kind() == reflect.Slice || expected.Kind() == reflect.Array) {
			element = expected.Elem()
		}
		for index := 0; decoder.More(); index++ {
			if err := inspectJSONValue(decoder, fmt.Sprintf("%s[%d]", location, index), element); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil {
			return err
		}
		if closing != json.Delim(']') {
			return fmt.Errorf("array at %s has invalid closing delimiter", location)
		}
	default:
		return fmt.Errorf("invalid JSON delimiter %q at %s", delimiter, location)
	}
	return nil
}

func indirectJSONType(value reflect.Type) reflect.Type {
	for value != nil && value.Kind() == reflect.Pointer {
		value = value.Elem()
	}
	return value
}

func jsonStructFields(value reflect.Type) map[string]reflect.Type {
	fields := make(map[string]reflect.Type, value.NumField())
	for i := 0; i < value.NumField(); i++ {
		field := value.Field(i)
		if !field.IsExported() {
			continue
		}
		name := field.Name
		if tag, ok := field.Tag.Lookup("json"); ok {
			name, _, _ = strings.Cut(tag, ",")
			if name == "-" {
				continue
			}
			if name == "" {
				name = field.Name
			}
		}
		fields[name] = field.Type
	}
	return fields
}

func requireJSONEOF(decoder *json.Decoder) error {
	var trailing any
	err := decoder.Decode(&trailing)
	if err == io.EOF {
		return nil
	}
	if err != nil {
		return err
	}
	return fmt.Errorf("application.json must contain exactly one JSON value")
}
