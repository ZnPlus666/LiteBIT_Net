//go:build !linux

// Non-Linux hosts may run offline development tests, not install services.
package main

import "os"

func ownerUID(os.FileInfo) int { return os.Geteuid() }
func openNoFollow(path string, flags int, perm os.FileMode) (*os.File, error) {
	return os.OpenFile(path, flags, perm)
}
func privateUmask() {}
func terminalState() (any, error) {
	return nil, fail("此版本仅适用于 Linux。 / This version requires Linux.")
}
func hideEcho() (func(), error) {
	return nil, fail("此版本仅适用于 Linux。 / This version requires Linux.")
}
func acquireOperationLock() (func(), error) {
	return nil, fail("此版本仅适用于 Linux。 / This version requires Linux.")
}
