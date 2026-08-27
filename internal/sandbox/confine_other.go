//go:build !linux

package sandbox

import (
	"fmt"
	"runtime"
)

func Confine(root string) error {
	return fmt.Errorf("sandbox: kernel confinement is not available on %s", runtime.GOOS)
}

func Support() (string, bool) {
	return "none (no kernel sandbox on " + runtime.GOOS + ")", false
}
