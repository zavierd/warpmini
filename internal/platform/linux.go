//go:build linux
// +build linux

package platform

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"warpmini/internal/cleanup"
)

// Linux doesn't use keychain, return unsupported
func StoreToMacKeychain(email string, jsonData []byte) error {
	return errors.New("macOS Keychain not available on Linux")
}

func RefreshMacMachineID() error {
	return errors.New("not supported on Linux")
}

func EnsureWarpClosedMac() error {
	// Kill any Warp-related processes on Linux
	for _, name := range []string{"warp", "warp-terminal", "warp-cli", "cloudflare-warp"} {
		_ = exec.Command("pkill", "-f", name).Run()
	}
	return nil
}

func StartWarpClientMac() error {
	// Try to start Warp on Linux if available
	cmd := exec.Command("warp-cli", "connect")
	return cmd.Run()
}

func CleanupMac() error {
	_ = EnsureWarpClosedMac()
	errs := &cleanup.Errors{}
	
	home, err := os.UserHomeDir()
	if err != nil {
		errs.Merge(fmt.Errorf("failed to get home directory: %w", err))
		return errs.Err()
	}
	
	// Linux Warp paths
	paths := []string{
		filepath.Join(home, ".local/share/warp"),
		filepath.Join(home, ".config/warp"),
		filepath.Join(home, ".cache/warp"),
		"/var/lib/cloudflare-warp",
		"/etc/cloudflare-warp",
	}
	
	cleanup.RemovePaths(paths, errs)
	return errs.Err()
}