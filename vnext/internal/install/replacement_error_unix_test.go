//go:build !windows

package install

func nativeOpenReplacementDenied(error) bool { return false }
