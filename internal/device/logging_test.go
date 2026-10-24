package device

import (
	"strings"
	"testing"
)

func TestRedactIdentifier(t *testing.T) {
	raw := "aa:bb:cc:dd:ee:ff"
	redacted := redactIdentifier(raw)
	if redacted == "" || strings.Contains(redacted, raw) {
		t.Fatalf("redactIdentifier(%q) = %q, want a non-empty mask", raw, redacted)
	}
	if again := redactIdentifier(raw); again != redacted {
		t.Fatalf("redaction is not stable: %q != %q", redacted, again)
	}
	if got := redactIdentifier(""); got != "unknown" {
		t.Fatalf("redactIdentifier(empty) = %q, want unknown", got)
	}
}

