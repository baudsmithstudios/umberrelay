package main

import "testing"

func TestRuntimeDefaults(t *testing.T) {
	if defaultConfigPath != "/etc/umberrelay/config.toml" {
		t.Fatalf("defaultConfigPath = %q, want %q", defaultConfigPath, "/etc/umberrelay/config.toml")
	}
	if defaultDBName != "umberrelay.db" {
		t.Fatalf("defaultDBName = %q, want %q", defaultDBName, "umberrelay.db")
	}
	if runtimeName != "umberrelay" {
		t.Fatalf("runtimeName = %q, want %q", runtimeName, "umberrelay")
	}
}
