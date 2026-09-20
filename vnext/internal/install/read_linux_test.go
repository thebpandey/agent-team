//go:build linux

package install

import (
	"context"
	"os"
	"syscall"
	"testing"
	"time"
)

func TestInstallRejectsFIFOWithoutBlocking(t *testing.T) {
	layout, release := internalFixture(t, "fifo-source")
	if err := os.Remove(release.Binary.Path); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(release.Binary.Path, 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := Install(ctx, layout, release, []Host{Codex}, 0); err == nil {
		t.Fatal("FIFO source accepted")
	}
	if ctx.Err() != nil {
		t.Fatal("FIFO source blocked")
	}
	assertNoJournal(t, layout)
}

func TestInstallRejectsSwapToFIFOWithoutBlocking(t *testing.T) {
	layout, release := internalFixture(t, "fifo-swap")
	stableReadHook = func(path string) {
		if path != release.Binary.Path {
			return
		}
		stableReadHook = nil
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		if err := syscall.Mkfifo(path, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { stableReadHook = nil })
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := Install(ctx, layout, release, []Host{Codex}, 0); err == nil {
		t.Fatal("FIFO swap accepted")
	}
	if ctx.Err() != nil {
		t.Fatal("FIFO swap blocked")
	}
	assertNoJournal(t, layout)
}
