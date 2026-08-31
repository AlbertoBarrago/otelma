package cli

import (
	"fmt"
	"runtime"
)

// Version is set at build time via -ldflags "-X
// github.com/albz/otelma/internal/cli.Version=vX.Y.Z" (see the Homebrew
// formula in homebrew-otelma). A source build that doesn't pass it keeps
// "dev" so the fallback is obviously not a real release.
var Version = "dev"

func runVersion(args []string) error {
	fmt.Printf("otelma %s (%s, %s/%s)\n", Version, runtime.Version(), runtime.GOOS, runtime.GOARCH)
	return nil
}
