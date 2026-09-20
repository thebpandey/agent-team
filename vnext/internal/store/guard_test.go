package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestProjectMutationGuardSerializesCanonicalAliases(t *testing.T) {
	root := t.TempDir()
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(root, alias); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	first, err := AcquireProjectMutation(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	acquired := make(chan func() error, 1)
	go func() {
		release, err := AcquireProjectMutation(context.Background(), alias)
		if err != nil {
			acquired <- func() error { return err }
			return
		}
		acquired <- release
	}()
	select {
	case <-acquired:
		t.Fatal("alias acquired project mutation guard concurrently")
	case <-time.After(20 * time.Millisecond):
	}
	if err := first(); err != nil {
		t.Fatal(err)
	}
	select {
	case release := <-acquired:
		if err := release(); err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("waiting acquirer did not proceed")
	}
}
