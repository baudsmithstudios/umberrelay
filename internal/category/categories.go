package category

import "strings"

const (
	Tracking      = "tracking"
	Advertising   = "advertising"
	Analytics     = "analytics"
	Telemetry     = "telemetry"
	Malware       = "malware"
	Uncategorized = "uncategorized"
)

type Option struct {
	Value string
	Label string
}

var options = []Option{
	{Value: Tracking, Label: Tracking},
	{Value: Advertising, Label: Advertising},
	{Value: Analytics, Label: Analytics},
	{Value: Telemetry, Label: Telemetry},
	{Value: Malware, Label: Malware},
	{Value: Uncategorized, Label: "unclassified"},
}

func Options() []Option {
	out := make([]Option, len(options))
	copy(out, options)
	return out
}

func Normalize(value string) (string, bool) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "unclassified" {
		return Uncategorized, true
	}
	for _, option := range options {
		if normalized == option.Value {
			return normalized, true
		}
	}
	return "", false
}
