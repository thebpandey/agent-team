//go:build windows

package migrate

import (
	"os"
	"syscall"
	"unsafe"
)

func systemTrustStorePath() string { return `C:\ProgramData\Agent-Team\cutover-trust.json` }

const (
	windowsReadControl          = 0x00020000
	windowsOpenExisting         = 3
	windowsShareAll             = 0x00000001 | 0x00000002 | 0x00000004
	windowsOpenReparsePoint     = 0x00200000
	windowsBackupSemantics      = 0x02000000
	windowsReparseAttribute     = 0x00000400
	windowsDirectoryAttribute   = 0x00000010
	windowsSEFileObject         = 1
	windowsOwnerSecurity        = 0x00000001
	windowsDACLSecurity         = 0x00000004
	windowsAccessAllowedACE     = 0
	windowsAccessDeniedACE      = 1
	windowsLocalSystemSID       = 22
	windowsBuiltinAdminsSID     = 26
	windowsMinimumAccessACESize = 16
)

var (
	windowsAdvapi          = syscall.NewLazyDLL("advapi32.dll")
	windowsKernel          = syscall.NewLazyDLL("kernel32.dll")
	windowsGetSecurityInfo = windowsAdvapi.NewProc("GetSecurityInfo")
	windowsGetACE          = windowsAdvapi.NewProc("GetAce")
	windowsIsValidSID      = windowsAdvapi.NewProc("IsValidSid")
	windowsIsWellKnownSID  = windowsAdvapi.NewProc("IsWellKnownSid")
	windowsLocalFree       = windowsKernel.NewProc("LocalFree")
)

type windowsACL struct {
	revision, reserved byte
	size               uint16
	aceCount           uint16
	reserved2          uint16
}

type windowsACEHeader struct {
	typeID, flags byte
	size          uint16
}

func systemTrustStoreOwner(path string, expected os.FileInfo) bool {
	path16, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return false
	}
	handle, err := syscall.CreateFile(path16, windowsReadControl, windowsShareAll, nil, windowsOpenExisting, windowsOpenReparsePoint|windowsBackupSemantics, 0)
	if err != nil {
		return false
	}
	file := os.NewFile(uintptr(handle), path)
	if file == nil {
		syscall.CloseHandle(handle)
		return false
	}
	defer file.Close()
	actual, err := file.Stat()
	if err != nil || !actual.Mode().IsRegular() || !os.SameFile(expected, actual) {
		return false
	}
	var information syscall.ByHandleFileInformation
	if syscall.GetFileInformationByHandle(handle, &information) != nil || information.FileAttributes&(windowsReparseAttribute|windowsDirectoryAttribute) != 0 {
		return false
	}
	var owner, dacl, descriptor uintptr
	status, _, _ := windowsGetSecurityInfo.Call(uintptr(handle), windowsSEFileObject, windowsOwnerSecurity|windowsDACLSecurity, uintptr(unsafe.Pointer(&owner)), 0, uintptr(unsafe.Pointer(&dacl)), 0, uintptr(unsafe.Pointer(&descriptor)))
	if status != 0 || descriptor == 0 {
		return false
	}
	defer windowsLocalFree.Call(descriptor)
	ownerTrusted := windowsTrustedSID(owner)
	if dacl == 0 {
		return false
	}
	acl := (*windowsACL)(unsafe.Pointer(dacl))
	aces := make([]windowsTrustACE, 0, acl.aceCount)
	for index := uint16(0); index < acl.aceCount; index++ {
		var address uintptr
		ok, _, _ := windowsGetACE.Call(dacl, uintptr(index), uintptr(unsafe.Pointer(&address)))
		if ok == 0 || address == 0 {
			return false
		}
		header := (*windowsACEHeader)(unsafe.Pointer(address))
		ace := windowsTrustACE{known: header.typeID == windowsAccessAllowedACE || header.typeID == windowsAccessDeniedACE}
		if !ace.known || header.size < windowsMinimumAccessACESize {
			return false
		}
		ace.allow = header.typeID == windowsAccessAllowedACE
		ace.mask = *(*uint32)(unsafe.Pointer(address + 4))
		sid := address + 8
		valid, _, _ := windowsIsValidSID.Call(sid)
		if valid == 0 {
			return false
		}
		ace.trusted = windowsTrustedSID(sid)
		aces = append(aces, ace)
	}
	return trustedWindowsACL(ownerTrusted, true, aces)
}

func windowsTrustedSID(sid uintptr) bool {
	if sid == 0 {
		return false
	}
	for _, kind := range []uintptr{windowsLocalSystemSID, windowsBuiltinAdminsSID} {
		if ok, _, _ := windowsIsWellKnownSID.Call(sid, kind); ok != 0 {
			return true
		}
	}
	return false
}
