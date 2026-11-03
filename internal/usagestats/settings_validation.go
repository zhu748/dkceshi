package usagestats

import (
	"fmt"
)

func validatePeakSettings(peak PeakSettings) error {
	ranges := []struct {
		start string
		end   string
		label string
	}{
		{peak.Start1, peak.End1, "peak window 1"},
		{peak.Start2, peak.End2, "peak window 2"},
	}
	for _, r := range ranges {
		if r.start == "" && r.end == "" {
			continue
		}
		if r.start == "" || r.end == "" {
			return fmt.Errorf("%w: %s must have both start and end times", ErrInvalidUsageSettings, r.label)
		}
		startMinutes, err := clockMinutes(r.start)
		if err != nil {
			return fmt.Errorf("%w: %s start %q is invalid", ErrInvalidUsageSettings, r.label, r.start)
		}
		endMinutes, err := clockMinutes(r.end)
		if err != nil {
			return fmt.Errorf("%w: %s end %q is invalid", ErrInvalidUsageSettings, r.label, r.end)
		}
		if startMinutes >= endMinutes {
			return fmt.Errorf("%w: %s start must be before end", ErrInvalidUsageSettings, r.label)
		}
	}
	if peak.Enabled && peak.Start1 != "" && peak.Start2 != "" &&
		peak.Start1 < peak.End2 && peak.Start2 < peak.End1 {
		return fmt.Errorf("%w: peak windows must not overlap", ErrInvalidUsageSettings)
	}
	if peak.Multiplier < 0 {
		return fmt.Errorf("%w: peak multiplier must be >= 0", ErrInvalidUsageSettings)
	}
	return nil
}

func clockMinutes(value string) (int, error) {
	if len(value) != 5 || value[2] != ':' {
		return 0, fmt.Errorf("clock value must use HH:MM")
	}
	for _, i := range [...]int{0, 1, 3, 4} {
		if value[i] < '0' || value[i] > '9' {
			return 0, fmt.Errorf("clock value must use HH:MM")
		}
	}
	hour := int(value[0]-'0')*10 + int(value[1]-'0')
	minute := int(value[3]-'0')*10 + int(value[4]-'0')
	if hour > 23 || minute > 59 {
		return 0, fmt.Errorf("invalid clock value")
	}
	return hour*60 + minute, nil
}

// validateModelPrices 拒绝负单价，避免手改请求写出负价条目。
func validateModelPrices(models map[string]ModelPrice) error {
	for key, price := range models {
		if price.InputPrice < 0 || price.OutputPrice < 0 {
			return fmt.Errorf("%w: model %q prices must be >= 0", ErrInvalidUsageSettings, key)
		}
		if price.InputMissPrice != nil && *price.InputMissPrice < 0 {
			return fmt.Errorf("%w: model %q cache-miss input price must be >= 0", ErrInvalidUsageSettings, key)
		}
	}
	return nil
}
