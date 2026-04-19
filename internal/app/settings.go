package app

import (
	"fmt"
	"strconv"

	"umberrelay/internal/classify"
	"umberrelay/internal/store"
)

type SettingsInput struct {
	RetentionDays    *int
	ListRefreshHours *int
}

func UpdateSettings(db *store.DB, mgr *classify.Manager, input SettingsInput) error {
	if input.RetentionDays == nil && input.ListRefreshHours == nil {
		return newInvalidInputError("at least one setting is required")
	}

	if input.RetentionDays != nil {
		if err := validateSettingRange("retention_days", *input.RetentionDays, 1, 365); err != nil {
			return err
		}
		if err := db.SetConfig("retention_days", strconv.Itoa(*input.RetentionDays)); err != nil {
			return err
		}
	}

	if input.ListRefreshHours != nil {
		if err := validateSettingRange("list_refresh_hours", *input.ListRefreshHours, 1, 168); err != nil {
			return err
		}
		if err := db.SetConfig("list_refresh_hours", strconv.Itoa(*input.ListRefreshHours)); err != nil {
			return err
		}
	}

	if mgr != nil {
		mgr.NotifyConfigChanged()
	}

	return nil
}

func validateSettingRange(name string, value, min, max int) error {
	if value < min || value > max {
		return newInvalidInputError(fmt.Sprintf("%s must be between %d and %d", name, min, max))
	}
	return nil
}
