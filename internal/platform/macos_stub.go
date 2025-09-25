//go:build !darwin && !linux
// +build !darwin,!linux

package platform

import "errors"

func RefreshMacMachineID() error { return errors.New("not supported on this platform") }
func EnsureWarpClosedMac() error { return nil }
func StartWarpClientMac() error { return nil }
