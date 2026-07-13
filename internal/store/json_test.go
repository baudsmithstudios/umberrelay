package store

import (
	"encoding/json"
	"strings"
	"testing"
)

// These structs are serialized directly as API responses, so their JSON keys
// are part of the public contract and must stay snake_case.
func TestAPIResponseStructsUseSnakeCase(t *testing.T) {
	cases := []struct {
		name  string
		value any
		want  string
	}{
		{"Query", Query{}, `"device_mac"`},
		{"Device", Device{}, `"first_seen"`},
		{"DeviceWithStats", DeviceWithStats{}, `"query_count"`},
		{"DashboardStats", DashboardStats{}, `"total_queries"`},
		{"DeviceSummary", DeviceSummary{}, `"query_count"`},
		{"ListEntry", ListEntry{}, `"last_fetch"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(tc.value)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if got := string(data); !strings.Contains(got, tc.want) {
				t.Errorf("missing %s in %s", tc.want, got)
			}
		})
	}
}
