package device

import (
	"strings"
	"testing"
)

func TestRedactIdentifierMasksRawValue(t *testing.T) {
	raw := "aa:bb:cc:dd:ee:ff"
	redacted := redactIdentifier(raw)
	if redacted == "" {
		t.Fatal("redacted identifier is empty")
	}
	if strings.Contains(redacted, raw) {
		t.Fatalf("redacted identifier leaked raw value: %q", redacted)
	}
}

func TestRedactIdentifierIsStable(t *testing.T) {
	raw := "192.168.1.42"
	first := redactIdentifier(raw)
	second := redactIdentifier(raw)
	if first != second {
		t.Fatalf("redaction is not stable: %q != %q", first, second)
	}
}

func TestRedactIdentifierHandlesEmptyValue(t *testing.T) {
	if got := redactIdentifier(""); got != "unknown" {
		t.Fatalf("redactIdentifier(empty) = %q, want unknown", got)
	}
}
