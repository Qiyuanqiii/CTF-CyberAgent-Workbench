//go:build desktop

// Package webassets exposes only the production Vite bundle compiled into the
// Desktop binary. TypeScript remains a renderer and cannot access Go secrets,
// process control, Docker, or local paths through this package.
package webassets

import "embed"

// Files is populated only after `npm run build` has produced dist/. A Desktop
// build intentionally fails when the audited production bundle is absent.
//
//go:embed all:dist
var Files embed.FS
