package remote

import "embed"

// PublicFS contains the Vite-built SPA bundle. The frontend build writes into
// backend/public before the Go binary is built. The all: prefix keeps files
// whose names start with "_" (Rollup names lodash chunks like _baseUniq-*.js
// that way), which a plain embed silently drops.
//
//go:embed all:public
var PublicFS embed.FS
