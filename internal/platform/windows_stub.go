//go:build !windows
// +build !windows

package platform

import "errors"

func StoreToWindowsUserFile(email string, jsonData []byte) error {
	return errors.New("windows storage not supported on this OS build")
}

func CleanupWindows() error {
	return errors.New("windows cleanup not supported on this OS build")
}

func RefreshWindowsMachineID() error { return nil }
func EnsureWarpClosedWindows() error { return nil }
func StartWarpClientWindows() error { return nil }
