package store

import (
	"errors"
	"strings"
	"testing"

	"github.com/thebpandey/agent-team/vnext/internal/core"
)

func TestInstallJournalStoreHasSeparateBound(t *testing.T) {
	journal := NewInstallJournal(t.TempDir())
	// Exercise the same copy/hash/atomic replacement paths used for progress
	// checkpoints with payloads larger than the ordinary canonical store allows.
	body := strings.Repeat("x", (32<<20)+1)
	for _, value := range []string{body, body + "updated"} {
		if _, err := journal.WriteJSON("install-attempt.json", value, 64<<20); err != nil {
			t.Fatalf("bounded installer journal write: %v", err)
		}
		var restored string
		if err := journal.ReadJSON("install-attempt.json", 64<<20, &restored); err != nil || restored != value {
			t.Fatalf("bounded installer journal read: %v", err)
		}
	}
	if _, err := journal.WriteJSON("too-large.json", "small", (64<<20)+1); !errors.Is(err, core.ErrLimit) {
		t.Fatalf("installer ceiling bypassed: %v", err)
	}
	ordinary := New(t.TempDir(), core.StorageLimits{CanonicalBytes: 64 << 20})
	if _, err := ordinary.WriteJSON("ordinary.json", "small", 64<<20); !errors.Is(err, core.ErrLimit) {
		t.Fatalf("ordinary store ceiling changed: %v", err)
	}
}
