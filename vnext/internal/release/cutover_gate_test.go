package release_test

import (
	"context"
	"errors"
	"testing"

	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/install"
	"github.com/thebpandey/agent-team/vnext/internal/release"
)

func TestCutover(t *testing.T) {
	layout, rel, manifest := canaryFixture(t)
	canary, err := release.RunInstallCanary(context.Background(), layout, rel, manifest, []install.Host{install.Codex, install.Claude})
	if err != nil || len(canary.Hosts) != 2 || !canary.RollbackVerified || canary.RestoredManifestSHA256 == "" || len(canary.RestoredFiles) != len(manifest.Files) || len(canary.Actions) == 0 {
		t.Fatal(canary, err)
	}
	if err := release.VerifyRollback(context.Background(), canary); err != nil {
		t.Fatal(err)
	}
	cutover := release.Cutover{Canary: canary, ProviderVerified: true, InstalledVerified: true, RollbackRehearsed: true, Evidence: []string{"provider-test.json", "rollback-test.json", "installed-test.json"}}
	if err := release.VerifyCutover(cutover); err != nil {
		t.Fatal(err)
	}
	cutover.RollbackRehearsed = false
	if !errors.Is(release.VerifyCutover(cutover), core.ErrPhase) {
		t.Fatal("cutover accepted without rollback evidence")
	}
}
