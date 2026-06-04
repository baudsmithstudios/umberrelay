package store

import (
	"encoding/json"
	"strings"
	"testing"
)

// These structs are serialized directly as API responses, so their JSON keys
// are part of the public contract and must stay snake_case. Each case checks a
// representative compound key plus the PascalCase form it must not regress to.
func TestAPIResponseStructsUseSnakeCase(t *testing.T) {
	cases := []struct {
		name    string
		value   any
		want    string
		notWant string
	}{
		{"Query", Query{}, `"device_mac"`, `"DeviceMAC"`},
		{"Device", Device{}, `"first_seen"`, `"FirstSeen"`},
		{"DeviceWithStats", DeviceWithStats{}, `"query_count"`, `"QueryCount"`},
		{"DashboardStats", DashboardStats{}, `"total_queries"`, `"TotalQueries"`},
		{"DeviceSummary", DeviceSummary{}, `"query_count"`, `"QueryCount"`},
		{"ListEntry", ListEntry{}, `"last_fetch"`, `"LastFetch"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(tc.value)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			got := string(data)
			if !strings.Contains(got, tc.want) {
				t.Errorf("missing %s in %s", tc.want, got)
			}
			if strings.Contains(got, tc.notWant) {
				t.Errorf("found PascalCase %s in %s", tc.notWant, got)
			}
		})
	}
}
