//go:build windows

package operatortrust

import (
	"os"
	"runtime"
	"syscall"
	"unsafe"
)

func Path() string { return `C:\ProgramData\Agent-Team\cutover-trust.json` }

const (
	readControl                 = 0x00020000
	openExisting                = 3
	shareAll                    = 0x00000001 | 0x00000002 | 0x00000004
	openReparsePoint            = 0x00200000
	backupSemantics             = 0x02000000
	reparseAttribute            = 0x00000400
	directoryAttribute          = 0x00000010
	seFileObject                = 1
	ownerSecurity               = 0x00000001
	daclSecurity                = 0x00000004
	accessAllowedACE            = 0
	accessDeniedACE             = 1
	localSystemSID              = 22
	builtinAdminsSID            = 26
	minimumAccessACESize        = 16
	trustWriteMask       uint32 = 0x00000002 | 0x00000004 | 0x00000010 | 0x00000100 | 0x00010000 | 0x00040000 | 0x00080000 | 0x10000000 | 0x40000000
)

var (
	advapi          = syscall.NewLazyDLL("advapi32.dll")
	kernel          = syscall.NewLazyDLL("kernel32.dll")
	getSecurityInfo = advapi.NewProc("GetSecurityInfo")
	getACE          = advapi.NewProc("GetAce")
	isValidSID      = advapi.NewProc("IsValidSid")
	isWellKnownSID  = advapi.NewProc("IsWellKnownSid")
	localFree       = kernel.NewProc("LocalFree")
)

type acl struct {
	revision, reserved        byte
	size, aceCount, reserved2 uint16
}
type aceHeader struct {
	typeID, flags byte
	size          uint16
}

func Owner(path string, expected os.FileInfo) bool {
	path16, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return false
	}
	handle, err := syscall.CreateFile(path16, readControl, shareAll, nil, openExisting, openReparsePoint|backupSemantics, 0)
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
	if syscall.GetFileInformationByHandle(handle, &information) != nil || information.FileAttributes&(reparseAttribute|directoryAttribute) != 0 {
		return false
	}
	var owner, dacl, descriptor unsafe.Pointer
	status, _, _ := getSecurityInfo.Call(uintptr(handle), seFileObject, ownerSecurity|daclSecurity, uintptr(unsafe.Pointer(&owner)), 0, uintptr(unsafe.Pointer(&dacl)), 0, uintptr(unsafe.Pointer(&descriptor)))
	if status != 0 || descriptor == nil || !trustedSID(owner) || dacl == nil {
		return false
	}
	defer func() {
		localFree.Call(uintptr(descriptor))
		runtime.KeepAlive(descriptor)
	}()
	list := (*acl)(dacl)
	for index := uint16(0); index < list.aceCount; index++ {
		var address unsafe.Pointer
		ok, _, _ := getACE.Call(uintptr(dacl), uintptr(index), uintptr(unsafe.Pointer(&address)))
		if ok == 0 || address == nil {
			return false
		}
		header := (*aceHeader)(address)
		if (header.typeID != accessAllowedACE && header.typeID != accessDeniedACE) || header.size < minimumAccessACESize {
			return false
		}
		mask := *(*uint32)(unsafe.Add(address, 4))
		sid := unsafe.Add(address, 8)
		valid, _, _ := isValidSID.Call(uintptr(sid))
		if valid == 0 || header.typeID == accessAllowedACE && mask&trustWriteMask != 0 && !trustedSID(sid) {
			return false
		}
	}
	runtime.KeepAlive(descriptor)
	return true
}

func trustedSID(sid unsafe.Pointer) bool {
	if sid == nil {
		return false
	}
	for _, kind := range []uintptr{localSystemSID, builtinAdminsSID} {
		if ok, _, _ := isWellKnownSID.Call(uintptr(sid), kind); ok != 0 {
			return true
		}
	}
	return false
}
