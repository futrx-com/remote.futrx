package applications

// Upgrading an installed app to a new version of its image.
//
// An instance records the `version` from the image.json it was installed from.
// When the catalog's version for that image no longer matches, the container
// side is stale: the install script that provisioned it belonged to a
// different release. Re-running that script is what makes it current, and the
// recorded version is what tells us it has to happen.
//
// The comparison is equality, not ordering. Versions are free text — "8.0",
// "16", "1.2.3-rc1" — so there is no ordering to read, and none is invented:
// a version that *differs* re-installs, whether that is forward or back. An
// author who changes nothing keeps the version and nothing is re-run.

// needsUpgrade reports whether an instance's container side was provisioned by
// a different version of the image than the catalog now holds.
//
// Only kinds that reach a container can be stale. A backend image installs
// nothing to re-install, so bumping its version is a catalog change and
// nothing more: its new code is picked up by restarting the plugin.
func needsUpgrade(inst Instance, img Image) bool {
	return img.Type.NeedsContainer() && inst.ImageVersion != img.Version
}
