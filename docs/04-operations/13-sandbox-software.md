# Reproducible sandbox software

Remote's core tools and provider CLIs remain in the managed image. Additional
software can be declared in a host-owned JSON file outside the source checkout:

```json
{
  "apt": ["ripgrep", "sqlite3"],
  "npm": ["typescript@5.9.3"],
  "prebakeBrowser": false,
  "prebakeIDE": false
}
```

Set `SANDBOX_SOFTWARE_FILE=/etc/remote.futrx/software.json` in the installer/update
environment and keep the file backed up. The image step compares the manifest's
SHA-256 with the image description and rebuilds when it changes. Manual builds
use the same variable with `go run ./cmd/build-base-image -overwrite`. The file is
validated before the builder changes LXD state. npm tools require exact versions;
apt supports `package=version`. Reproducibility of unpinned apt packages depends
on the configured repositories. Core provider versions keep Remote's own pins.

The manifest is additive: arbitrary shell scripts, package removals and unknown
keys are rejected. `--no-install-recommends` limits optional apt dependencies.
Both prebake options default to true. Disabling them keeps browser/IDE packages
out of the published image; Remote's existing provisioners install them when
needed. IDE provisioning currently occurs during project launch, while browser
provisioning occurs on browser use. Disabling prebaking is not a permission rule.

Use installable project applications for optional databases, services and tools
when a catalog application is available; their declared installation/lifecycle
is restored by Remote. Use the image manifest for tools required in every new
project. Project-specific setup can remain in `/workspace/setup.sh`, but arbitrary
workspace scripts are not automatically executed by this change.

New and recreated project containers launch from the configured image alias, so
they receive the same declared base software. Existing containers retain their
current image until the normal recycle/update workflow replaces them. Changing
the file alone does not mutate running containers.

Agent execution defaults npm's cache and Go's module cache to
`/workspace/.remote-cache/npm` and `/workspace/.remote-cache/go-mod`. These paths
are per project, survive recreation and avoid growth in the container root.
Explicit project environment values override the defaults. Terminal users can
export the same `npm_config_cache` and `GOMODCACHE` variables in `setup.sh` or their
shell profile. These caches are disposable, but clean them only while relevant
builds are stopped. They are not shared across projects and are not covered by a
root-disk quota; include workspace storage in disk monitoring and capacity plans.

The general `~/.cache` tree is not blindly deleted or shared: it can contain
installed Playwright browsers and provider-specific assets. Image publication
still runs apt/npm cleanup and removes known temporary installer artifacts after
all additional software is installed. Real LXD rebuild/recreation testing is
required before release.
