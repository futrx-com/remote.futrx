package constants

const (
	// ApplicationContainerGoVersion keeps the generated module directive and the
	// downloaded toolchain in sync so backend/container/ builds are reproducible.
	ApplicationContainerGoVersion = "1.24.7"

	// ApplicationContainerBuildMarkerDir holds the source-version markers that
	// make backend/container/ installation idempotent without requiring each
	// application binary to implement its own version command.
	ApplicationContainerBuildMarkerDir = "/usr/local/lib/remote"
)
