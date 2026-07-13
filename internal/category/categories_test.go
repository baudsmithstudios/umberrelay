package category

import "testing"

func TestNormalize(t *testing.T) {
	tests := []struct {
		input string
		want  string
		ok    bool
	}{
		{input: "tracking", want: Tracking, ok: true},
		{input: " Tracking ", want: Tracking, ok: true},
		{input: "unclassified", want: Uncategorized, ok: true},
		{input: "unknown", want: "", ok: false},
	}

	for _, tt := range tests {
		got, ok := Normalize(tt.input)
		if got != tt.want || ok != tt.ok {
			t.Fatalf("Normalize(%q) = (%q, %t), want (%q, %t)", tt.input, got, ok, tt.want, tt.ok)
		}
	}
}

func TestOptionsReturnsCopy(t *testing.T) {
	got := Options()
	got[0].Label = "changed"
	again := Options()
	if again[0].Label == "changed" {
		t.Fatal("Options() returned shared slice; expected copy")
	}
}
