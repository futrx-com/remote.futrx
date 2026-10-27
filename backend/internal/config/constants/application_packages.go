package constants

// Bounds on an uploaded application package. The transport cap and the archive
// cap are one rule, not two: an upload is buffered by the HTTP layer and then
// handed to the package store, so the request must be allowed to carry the
// largest archive the store will accept plus the multipart envelope around it.
// Deriving the transport cap from the archive cap is what keeps a change to
// one from quietly invalidating the other.
const (
	// MaxApplicationPackageArchiveBytes caps the ZIP a package is uploaded as.
	MaxApplicationPackageArchiveBytes = 64 << 20
	// ApplicationPackageUploadEnvelopeBytes is the headroom the multipart
	// framing around the archive is allowed to add.
	ApplicationPackageUploadEnvelopeBytes = 16 << 20
	// MaxApplicationPackageUploadBytes caps the whole upload request, so an
	// oversized one is refused by the transport before it is buffered.
	MaxApplicationPackageUploadBytes = MaxApplicationPackageArchiveBytes + ApplicationPackageUploadEnvelopeBytes
)
