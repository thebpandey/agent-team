package migrate

import "testing"

func TestTrustedWindowsACL(t *testing.T) {
	read := uint32(0x80000000)
	write := windowsTrustWriteMask
	tests := []struct {
		name                  string
		owner, dacl, wantSafe bool
		aces                  []windowsTrustACE
	}{
		{name: "system and administrators only", owner: true, dacl: true, wantSafe: true, aces: []windowsTrustACE{{allow: true, known: true, trusted: true, mask: write}, {allow: true, known: true, mask: read}}},
		{name: "empty protected dacl", owner: true, dacl: true, wantSafe: true},
		{name: "untrusted writer", owner: true, dacl: true, aces: []windowsTrustACE{{allow: true, known: true, mask: write}}},
		{name: "generic writer", owner: true, dacl: true, aces: []windowsTrustACE{{allow: true, known: true, mask: 0x40000000}}},
		{name: "untrusted deny", owner: true, dacl: true, wantSafe: true, aces: []windowsTrustACE{{known: true, mask: write}}},
		{name: "unknown ace", owner: true, dacl: true, aces: []windowsTrustACE{{allow: true}}},
		{name: "untrusted owner", dacl: true},
		{name: "null dacl", owner: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := trustedWindowsACL(test.owner, test.dacl, test.aces); got != test.wantSafe {
				t.Fatalf("trustedWindowsACL() = %v, want %v", got, test.wantSafe)
			}
		})
	}
}
