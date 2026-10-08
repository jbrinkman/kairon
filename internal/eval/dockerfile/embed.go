// Package dockerfile embeds the Dockerfile for the sandbox base image so the
// file is the single definition of the image content.
package dockerfile

import _ "embed"

// Base is the content of base.Dockerfile, the tools-only sandbox base image.
//
//go:embed base.Dockerfile
var Base string
