//go:build !darwin && !linux
// +build !darwin,!linux

package platform

import "errors"

// Stubs for macOS-only functions when building on non-darwin platforms (e.g., Windows)
func StoreToMacKeychain(email string, jsonData []byte) error { return errors.New("macOS Keychain not available on this platform") }
func RefreshMacMachineID() error { return errors.New("not supported on this platform") }
func EnsureWarpClosedMac() error { return nil }
func StartWarpClientMac() error { return nil }
func CleanupMac() error { return nil }
