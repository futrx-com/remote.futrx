package applications

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/futrx-com/remote.futrx.com/internal/integration/containers/applications/hosttools"
	svc "github.com/futrx-com/remote.futrx.com/internal/service/applications"
	applicationapi "github.com/futrx-com/remote.futrx.com/pkg/applications"
)

var (
	serviceNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.@-]*$`)
	environmentPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	identityPattern    = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]*$`)
	eventNamePattern   = regexp.MustCompile(`^[a-z0-9]+(?:[.-][a-z0-9]+)*$`)
)

const (
	coreApplicationPublisher = applicationapi.RemoteApplicationsPublisher

	// Event declarations are control-plane metadata loaded into every catalog
	// reader and compared while routing. These deliberately generous limits
	// bound that work without constraining a normal application contract.
	maxApplicationPublishers       = 64
	maxPublisherEvents             = 128
	maxApplicationSubscriptions    = 128
	maxSubscriptionEvents          = 128
	maxLocalPublisherNameBytes     = 128
	maxCanonicalPublisherNameBytes = 256
	maxEventNameBytes              = 128
	maxEventDescriptionBytes       = 2 << 10
)

func validateApplication(application svc.Application) error {
	if len(application.HostTools) > 0 && !application.NeedsContainer() {
		return fmt.Errorf("host tools require a provisioned application")
	}
	for _, tool := range application.HostTools {
		if err := hosttools.Validate(tool); err != nil {
			return fmt.Errorf("host tool %q: %w", tool.Name, err)
		}
	}

	if application.Name == "" {
		return fmt.Errorf("missing name")
	}
	// Version is what an installed instance is compared against to decide
	// whether its container side must be converged again, so an application
	// without one could never be upgraded in place. It is free text — "8.0", "16",
	// "1.2.3-rc1" — because the only question ever asked of it is whether it
	// differs from what an instance recorded, never which of two is newer.
	if strings.TrimSpace(application.Version) == "" {
		return fmt.Errorf("missing version")
	}
	if len(application.Scopes) == 0 {
		return fmt.Errorf("missing scopes")
	}
	for _, scope := range application.Scopes {
		if !scope.Valid() {
			return fmt.Errorf("invalid scope %q", scope)
		}
	}
	if err := validateService(application); err != nil {
		return err
	}
	if err := validateApplicationEvents(application); err != nil {
		return err
	}
	if !application.NeedsContainer() {
		if application.Port.Internal != 0 || application.Port.DefaultExternal != 0 || application.Healthcheck.Command != "" {
			return fmt.Errorf("port and healthcheck require a container capability")
		}
	} else if application.Port.Internal == 0 && (application.Port.DefaultExternal != 0 || application.Healthcheck.Command != "") {
		return fmt.Errorf("port.defaultExternal and healthcheck require port.internal")
	}
	if application.UI == nil && application.Backend == nil && !application.NeedsContainer() && len(application.Skills) == 0 {
		return fmt.Errorf("application has no infra, backend, ui, or skills")
	}
	return nil
}

func validateApplicationEvents(application svc.Application) error {
	if (len(application.Publishers) > 0 || len(application.Subscriptions) > 0) && application.Backend == nil {
		return fmt.Errorf("publishers and subscriptions require an application backend")
	}
	if len(application.Publishers) > maxApplicationPublishers {
		return fmt.Errorf("publishers must not contain more than %d entries", maxApplicationPublishers)
	}
	if len(application.Subscriptions) > maxApplicationSubscriptions {
		return fmt.Errorf("subscriptions must not contain more than %d entries", maxApplicationSubscriptions)
	}

	publishers := make(map[string]bool, len(application.Publishers))
	for _, publisher := range application.Publishers {
		if len(publisher.Name) > maxLocalPublisherNameBytes {
			return fmt.Errorf("publisher name must not exceed %d bytes", maxLocalPublisherNameBytes)
		}
		if !validLocalPublisherName(publisher.Name) {
			return fmt.Errorf("publisher name %q must be a local lowercase dotted or hyphenated name", publisher.Name)
		}
		if publishers[publisher.Name] {
			return fmt.Errorf("publisher %q is duplicated", publisher.Name)
		}
		publishers[publisher.Name] = true
		if len(publisher.Events) == 0 {
			return fmt.Errorf("publisher %q must declare at least one event", publisher.Name)
		}
		if len(publisher.Events) > maxPublisherEvents {
			return fmt.Errorf("publisher %q must not declare more than %d events", publisher.Name, maxPublisherEvents)
		}
		events := make(map[string]bool, len(publisher.Events))
		for _, event := range publisher.Events {
			if len(event.Name) > maxEventNameBytes {
				return fmt.Errorf("publisher %q event name must not exceed %d bytes", publisher.Name, maxEventNameBytes)
			}
			if !eventNamePattern.MatchString(event.Name) {
				return fmt.Errorf("publisher %q event name %q must be lowercase, dotted, or hyphenated", publisher.Name, event.Name)
			}
			if events[event.Name] {
				return fmt.Errorf("publisher %q event %q is duplicated", publisher.Name, event.Name)
			}
			events[event.Name] = true
			if event.Version < 1 {
				return fmt.Errorf("publisher %q event %q version must be at least 1", publisher.Name, event.Name)
			}
			if len(event.Description) > maxEventDescriptionBytes {
				return fmt.Errorf(
					"publisher %q event %q description must not exceed %d bytes",
					publisher.Name, event.Name, maxEventDescriptionBytes)
			}
			if strings.ContainsAny(event.Description, "\r\n") {
				return fmt.Errorf("publisher %q event %q description must be one line", publisher.Name, event.Name)
			}
		}
	}

	subscriptions := make(map[string]bool, len(application.Subscriptions))
	for _, subscription := range application.Subscriptions {
		if len(subscription.Publisher) > maxCanonicalPublisherNameBytes {
			return fmt.Errorf("subscription publisher must not exceed %d bytes", maxCanonicalPublisherNameBytes)
		}
		if !validSubscriptionPublisher(subscription.Publisher) {
			return fmt.Errorf("subscription publisher %q is not canonical", subscription.Publisher)
		}
		if subscriptions[subscription.Publisher] {
			return fmt.Errorf("subscription publisher %q is duplicated", subscription.Publisher)
		}
		subscriptions[subscription.Publisher] = true
		if len(subscription.Events) == 0 {
			return fmt.Errorf("subscription %q must declare at least one event", subscription.Publisher)
		}
		if len(subscription.Events) > maxSubscriptionEvents {
			return fmt.Errorf(
				"subscription %q must not declare more than %d events",
				subscription.Publisher, maxSubscriptionEvents)
		}
		events := make(map[string]bool, len(subscription.Events))
		for _, event := range subscription.Events {
			if len(event) > maxEventNameBytes {
				return fmt.Errorf(
					"subscription %q event name must not exceed %d bytes",
					subscription.Publisher, maxEventNameBytes)
			}
			if !eventNamePattern.MatchString(event) {
				return fmt.Errorf("subscription %q event name %q must be lowercase, dotted, or hyphenated", subscription.Publisher, event)
			}
			if events[event] {
				return fmt.Errorf("subscription %q event %q is duplicated", subscription.Publisher, event)
			}
			events[event] = true
			if subscription.Publisher == coreApplicationPublisher &&
				!applicationapi.IsRemoteApplicationEvent(event) {
				return fmt.Errorf("subscription %q declares unknown event %q", subscription.Publisher, event)
			}
		}
	}
	return nil
}

func validLocalPublisherName(name string) bool {
	if !eventNamePattern.MatchString(name) {
		return false
	}
	first := name
	if separator := strings.IndexAny(name, ".-"); separator >= 0 {
		first = name[:separator]
	}
	return first != "remote" && first != "applications"
}

func validSubscriptionPublisher(name string) bool {
	if name == coreApplicationPublisher {
		return true
	}
	rest, ok := strings.CutPrefix(name, "applications.")
	if !ok {
		return false
	}
	applicationID, publisher, ok := strings.Cut(rest, ".")
	return ok && packageIDPattern.MatchString(applicationID) && validLocalPublisherName(publisher)
}

func validateService(application svc.Application) error {
	service := application.Service
	if service == nil {
		return nil
	}
	if !serviceNamePattern.MatchString(service.Name) || strings.HasSuffix(service.Name, ".service") {
		return fmt.Errorf("service.name must be a valid unit name without the .service suffix")
	}
	if len(service.Command) == 0 || !strings.HasPrefix(service.Command[0], "/") {
		return fmt.Errorf("service.command must start with an absolute executable path")
	}
	for _, argument := range service.Command {
		if strings.ContainsAny(argument, "\x00\r\n") {
			return fmt.Errorf("service.command arguments cannot contain control characters")
		}
	}
	if strings.ContainsAny(service.Description, "\r\n") {
		return fmt.Errorf("service.description must be one line")
	}
	if service.User != "" && !identityPattern.MatchString(service.User) {
		return fmt.Errorf("service.user is invalid")
	}
	if service.Group != "" && !identityPattern.MatchString(service.Group) {
		return fmt.Errorf("service.group is invalid")
	}
	validRestart := map[string]bool{
		"": true, "no": true, "on-success": true, "on-failure": true,
		"on-abnormal": true, "on-watchdog": true, "on-abort": true, "always": true,
	}
	if !validRestart[service.Restart] {
		return fmt.Errorf("service.restart %q is invalid", service.Restart)
	}
	if service.RestartSec < 0 {
		return fmt.Errorf("service.restartSec cannot be negative")
	}
	if protect := service.Hardening.ProtectSystem; protect != "" && protect != "true" && protect != "full" && protect != "strict" {
		return fmt.Errorf("service.hardening.protectSystem %q is invalid", protect)
	}

	declared := make(map[string]bool, len(application.Env))
	for _, variable := range application.Env {
		declared[variable.Key] = true
	}
	targets := make(map[string]bool, len(service.Environment))
	for _, variable := range service.Environment {
		if !environmentPattern.MatchString(variable.Key) {
			return fmt.Errorf("service.environment key %q is invalid", variable.Key)
		}
		if targets[variable.Key] {
			return fmt.Errorf("service.environment key %q is duplicated", variable.Key)
		}
		targets[variable.Key] = true
		if !declared[variable.FromEnv] {
			return fmt.Errorf("service.environment %q references undeclared env %q", variable.Key, variable.FromEnv)
		}
		if variable.Encoding != "base64" {
			return fmt.Errorf("service.environment %q must use base64 encoding", variable.Key)
		}
	}
	return nil
}
