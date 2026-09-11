// Package version holds build metadata for patcode.
//
// Version is a var so release builds can stamp it at link time:
//
//	go build -ldflags "-X patcode/version.Version=v1.2.3"
//
// Development builds keep the default value.
package version

// Name is the user-facing binary/product name.
const Name = "patcode"

// Version is the current release version (with the "v" prefix). It is
// overridden at build time by the release pipeline (see .goreleaser.yaml
// ldflags), which stamps the leading-tag form such as "v1.2.3".
var Version = "v0.1.0"
