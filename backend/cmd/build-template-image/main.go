// build-template-image publishes a dedicated LXD image for one project
// template — the optional FAST PATH described in
// docs/02-workspaces/08-project-templates.md.
//
// Without a template image, a project created from e.g. the WordPress template
// launches from futrx-remote-dev-base and installs PHP, MariaDB, and WP-CLI
// inside its own container on first start (minutes on a small host). With one,
// the lifecycle launches straight from futrx-remote-<template>-base and the
// provisioning is already baked in.
//
// Usage:
//
//	go run ./cmd/build-template-image -template wordpress
//	go run ./cmd/build-template-image -template wordpress -overwrite
//	go run ./cmd/build-template-image -list
//
// Publishing compresses the whole rootfs and regularly exceeds the default
// 5-minute budget on a 1 vCPU box, so the publish (and build) budgets are
// overridable:
//
//	go run ./cmd/build-template-image -template wordpress -publish-timeout 30m
//	FUTRX_IMAGE_PUBLISH_TIMEOUT=30m go run ./cmd/build-template-image -template wordpress
//
// The base image must already be published; run cmd/build-base-image first.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/futrx-com/remote.futrx.com/internal/config"
	"github.com/futrx-com/remote.futrx.com/internal/integration/lxc"
	"github.com/futrx-com/remote.futrx.com/internal/service"
	serviceimage "github.com/futrx-com/remote.futrx.com/internal/service/container/image"
	servicetemplates "github.com/futrx-com/remote.futrx.com/internal/service/container/templates"
)

const (
	publishTimeoutEnv = "FUTRX_IMAGE_PUBLISH_TIMEOUT"
	buildTimeoutEnv   = "FUTRX_IMAGE_BUILD_TIMEOUT"
)

func main() {
	templateName := flag.String("template", "", "template to build an image for (see -list)")
	alias := flag.String("alias", "", "image alias to publish under (default futrx-remote-<template>-base)")
	base := flag.String("base", serviceimage.Alias, "image the builder container starts from")
	overwrite := flag.Bool("overwrite", false, "delete any existing image at the alias before publishing")
	list := flag.Bool("list", false, "list the templates that can be built into an image and exit")
	publishTimeout := flag.Duration(
		"publish-timeout", 0,
		"override the publish budget (env "+publishTimeoutEnv+"); publishing is slow on small hosts",
	)
	buildTimeout := flag.Duration(
		"build-timeout", 0,
		"override the provisioning budget (env "+buildTimeoutEnv+")",
	)
	flag.Parse()

	log.SetFlags(log.Ltime)
	catalog := servicetemplates.MustLoad()

	if *list {
		printTemplates(catalog)
		return
	}
	if *templateName == "" {
		fmt.Fprintln(os.Stderr, "-template is required")
		printTemplates(catalog)
		os.Exit(2)
	}
	template, ok := catalog.Get(*templateName)
	if !ok {
		log.Fatalf("unknown template %q; run with -list to see the available templates", *templateName)
	}
	program := servicetemplates.ProvisionProgram(template)
	if program == "" {
		log.Fatalf("template %q installs nothing, so it has no image to build", template.Name)
	}
	target := *alias
	if target == "" {
		target = servicetemplates.ImageAlias(template.Name)
	}
	if !template.PrebuiltImage {
		log.Printf(
			"note: template %q does not declare prebuiltImage, so the runtime will not "+
				"look for %q; set prebuiltImage in its template.json to use this image",
			template.Name, target,
		)
	}

	lxcClient := lxc.New()
	if !lxcClient.Available() {
		log.Fatalf("lxc CLI not found on PATH - install LXD on the host first")
	}
	containerStack := config.NewContainerStack(
		lxcClient,
		service.AgentProfiles(),
		config.ContainerStackOptions{
			ImageBuildProgress: newLogBuildProgressReporter(log.Default()),
		},
	)
	containerStack.Images.SetBuildTimeout(resolveTimeout(*buildTimeout, buildTimeoutEnv))
	containerStack.Images.SetPublishTimeout(resolveTimeout(*publishTimeout, publishTimeoutEnv))

	// The overall budget must comfortably exceed build + publish, both of
	// which are individually overridable above.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Hour)
	defer cancel()

	if *overwrite {
		log.Printf("removing existing image %q (if any)...", target)
		if out, err := lxcClient.Run(ctx, "image", "delete", target); err != nil {
			log.Printf("note: image delete returned: %v; output: %s", err, out)
		}
	}

	log.Printf("building %q from %q (template %q)...", target, *base, template.Name)
	log.Printf("(progress is reported every 30 seconds)")

	if err := containerStack.Images.BuildTemplate(ctx, serviceimage.TemplateSpec{
		Name:      template.Name,
		Alias:     target,
		BaseAlias: *base,
		Program:   program,
		Description: "futrx remote " + template.Name + " template: " +
			*base + " + " + template.Title,
	}); err != nil {
		log.Fatalf("build failed: %v", err)
	}

	log.Printf(
		"done. published %q. new %q projects will launch from it instead of provisioning in-container.",
		target, template.Name,
	)
}

func printTemplates(catalog *servicetemplates.Catalog) {
	fmt.Fprintln(os.Stderr, "templates:")
	for _, template := range catalog.List() {
		marker := " "
		if template.PrebuiltImage {
			marker = "*"
		}
		if !template.Provisions() {
			fmt.Fprintf(os.Stderr, "  %s %-12s %s (installs nothing - no image to build)\n",
				marker, template.Name, template.Title)
			continue
		}
		fmt.Fprintf(os.Stderr, "  %s %-12s %s -> %s\n",
			marker, template.Name, template.Title, servicetemplates.ImageAlias(template.Name))
	}
	fmt.Fprintln(os.Stderr, "  (* declares a pre-built image the runtime will look for)")
}

// resolveTimeout prefers the flag, falls back to the environment variable, and
// returns zero to keep the builder's default.
func resolveTimeout(flagValue time.Duration, envName string) time.Duration {
	if flagValue > 0 {
		return flagValue
	}
	raw := os.Getenv(envName)
	if raw == "" {
		return 0
	}
	parsed, err := time.ParseDuration(raw)
	if err != nil {
		log.Fatalf("invalid %s=%q: %v", envName, raw, err)
	}
	return parsed
}
