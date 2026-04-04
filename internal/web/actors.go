package web

import "strings"

const (
	actorTypeDevice = "device"
	actorTypeSource = "source"
)

func actorKeyForDevice(mac string) string {
	return actorTypeDevice + ":" + mac
}

func actorKeyForSource(sourceIP string) string {
	return actorTypeSource + ":" + sourceIP
}

func parseActorKey(key string) (string, string, bool) {
	if strings.HasPrefix(key, actorTypeDevice+":") {
		mac := strings.TrimPrefix(key, actorTypeDevice+":")
		if mac == "" {
			return "", "", false
		}
		return actorTypeDevice, mac, true
	}
	if strings.HasPrefix(key, actorTypeSource+":") {
		sourceIP := strings.TrimPrefix(key, actorTypeSource+":")
		if sourceIP == "" {
			return "", "", false
		}
		return actorTypeSource, sourceIP, true
	}
	return "", "", false
}

func normalizeActorSelection(raw string) (string, string, string, bool) {
	if raw == "" {
		return "", "", "", false
	}
	if actorType, value, ok := parseActorKey(raw); ok {
		return raw, actorType, value, true
	}
	return actorKeyForDevice(raw), actorTypeDevice, raw, true
}

func sourceActorDisplayName(sourceIP, label string) string {
	if sourceIP == "" {
		return "Unattributed"
	}
	if label != "" {
		return label + " · " + sourceIP
	}
	return "Unattributed · " + sourceIP
}
