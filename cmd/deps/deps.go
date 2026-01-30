//go:build deps
// +build deps

package deps

// We depend on these in our `Makefile`, but not in our code.
// To keep `go mod tidy` from removing these dependencies, we create this fake package.
//
// The `go:build` directive with the made up `deps` tag at the top of this file will preclude this
// file from accidentally being built. See: https://pkg.go.dev/cmd/go#hdr-Build_constraints
import (
	_ "github.com/deepmap/oapi-codegen/pkg/codegen"
)
