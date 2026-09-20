//go:build windows

package migrate

import "os"

func systemTrustStorePath() string { return `C:\ProgramData\Agent-Team\cutover-trust.json` }

// Windows cutover remains fail-closed until the operator trust file's owner
// and DACL can be proven with a native verifier.
func systemTrustStoreOwner(os.FileInfo) bool { return false }
