package sandbox

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/landlock-lsm/go-landlock/landlock"
	lsyscall "github.com/landlock-lsm/go-landlock/landlock/syscall"
)

func Confine(root string) error {
	if _, ok := Support(); !ok {
		return fmt.Errorf("sandbox: this kernel has no Landlock support")
	}

	abs, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("sandbox: %w", err)
	}

	readWrite := append([]string{abs, os.TempDir(), "/tmp", "/dev/shm"}, buildCaches()...)
	readOnly := []string{
		"/usr", "/bin", "/sbin", "/lib", "/lib32", "/lib64", "/libx32",
		"/opt", "/etc", "/proc", "/sys/devices/system/cpu",
	}
	devices := []string{
		"/dev/null", "/dev/zero", "/dev/full",
		"/dev/urandom", "/dev/random", "/dev/tty",
	}

	err = landlock.V5.BestEffort().RestrictPaths(
		landlock.RWDirs(readWrite...).IgnoreIfMissing(),
		landlock.RODirs(readOnly...).IgnoreIfMissing(),
		landlock.RWFiles(devices...).IgnoreIfMissing(),
	)
	if err != nil {
		return fmt.Errorf("sandbox: could not confine this process: %w", err)
	}
	return nil
}

func buildCaches() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}

	names := []string{
		".cinch",
		".cache",
		".local/share",
		"go/pkg",
		"go/bin",
		".npm",
		".cargo",
		".rustup",
		".gradle",
		".m2",
	}

	out := make([]string, 0, len(names))
	for _, name := range names {
		out = append(out, filepath.Join(home, name))
	}
	return out
}

func Support() (string, bool) {
	version, err := lsyscall.LandlockGetABIVersion()
	if err != nil || version < 1 {
		return "none (this kernel has no Landlock support)", false
	}
	return fmt.Sprintf("landlock (kernel ABI v%d)", version), true
}
