package web

import (
	"strconv"

	"umberrelay/internal/store"
)

const (
	defaultRetentionDays    = 30
	defaultListRefreshHours = 24
)

type runtimeSettings struct {
	RetentionDays    int
	ListRefreshHours int
}

func loadRuntimeSettings(db *store.DB) (runtimeSettings, error) {
	retentionDays, err := readConfigInt(db, "retention_days", defaultRetentionDays, 1, 365)
	if err != nil {
		return runtimeSettings{}, err
	}
	listRefreshHours, err := readConfigInt(db, "list_refresh_hours", defaultListRefreshHours, 1, 168)
	if err != nil {
		return runtimeSettings{}, err
	}
	return runtimeSettings{
		RetentionDays:    retentionDays,
		ListRefreshHours: listRefreshHours,
	}, nil
}

func readConfigInt(db *store.DB, key string, fallback, min, max int) (int, error) {
	value, err := db.GetConfig(key)
	if err != nil {
		return 0, err
	}
	if value == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(value)
	if err != nil || n < min || n > max {
		return fallback, nil
	}
	return n, nil
}
