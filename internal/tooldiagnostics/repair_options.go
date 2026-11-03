package tooldiagnostics

import (
	"context"
	"errors"
	"strings"
)

func RepairRetryCount(settings RepairSettings) int {
	if settings.RetryCount == nil {
		return 3
	}
	return *settings.RetryCount
}

func copyRetryCount(value *int) *int {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func normalizeRepairOptions(value *RepairSettings) error {
	retries := RepairRetryCount(*value)
	if retries < 0 || retries > 10 {
		return errors.New("repair retry count must be between 0 and 10")
	}
	value.RetryCount = &retries
	value.ReasoningEffort = strings.ToLower(strings.TrimSpace(value.ReasoningEffort))
	value.ThinkingMode = strings.ToLower(strings.TrimSpace(value.ThinkingMode))
	switch value.ReasoningEffort {
	case "", "low", "medium", "high":
	default:
		return errors.New("repair reasoning effort must be low, medium, high or empty")
	}
	switch value.ThinkingMode {
	case "", "auto":
		value.ThinkingMode = ""
	case "adaptive", "budget":
		if value.Protocol == "openai" || value.Protocol == "gemini" && value.ThinkingMode == "adaptive" {
			return errors.New("unsupported repair thinking mode for this protocol")
		}
	case "level":
		if value.Protocol != "gemini" {
			return errors.New("thinking level is only supported for Gemini")
		}
	default:
		return errors.New("invalid repair thinking mode")
	}
	return nil
}

type repairAttemptKey struct{}

func WithRepairAttempt(ctx context.Context, number int) context.Context {
	return context.WithValue(ctx, repairAttemptKey{}, number)
}

func RepairAttempt(ctx context.Context) int {
	number, _ := ctx.Value(repairAttemptKey{}).(int)
	if number < 1 {
		return 1
	}
	return number
}
