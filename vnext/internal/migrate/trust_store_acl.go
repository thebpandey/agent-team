package migrate

const windowsTrustWriteMask uint32 = 0x00000002 | 0x00000004 | 0x00000010 | 0x00000100 | 0x00010000 | 0x00040000 | 0x00080000 | 0x10000000 | 0x40000000

type windowsTrustACE struct {
	allow, known, trusted bool
	mask                  uint32
}

func trustedWindowsACL(ownerTrusted, daclPresent bool, aces []windowsTrustACE) bool {
	if !ownerTrusted || !daclPresent {
		return false
	}
	for _, ace := range aces {
		if !ace.known || ace.allow && ace.mask&windowsTrustWriteMask != 0 && !ace.trusted {
			return false
		}
	}
	return true
}
