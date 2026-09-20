package main

import "testing"

func TestVerifyGatesCLI(t *testing.T) {
	if _, _, err := parseVerifyGates(nil); err == nil {
		t.Fatal("missing evidence accepted")
	}
	evidence, revision, err := parseVerifyGates([]string{"--evidence", "readiness.json", "--revision", "abc123"})
	if err != nil || evidence != "readiness.json" || revision != "abc123" {
		t.Fatal(evidence, revision, err)
	}
	if _, _, err := parseVerifyGates([]string{"--evidence", "readiness.json", "--revision", "abc123", "extra"}); err == nil {
		t.Fatal("positional extra accepted")
	}
}
