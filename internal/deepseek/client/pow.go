package client

import (
	"context"
	"encoding/base64"
	"errors"
	"time"

	dsprotocol "ds2api/internal/deepseek/protocol"
	"ds2api/pow"
)

type powSolution struct {
	header    string
	expiresAt time.Time
}

func powChallengeExpiry(challenge map[string]any, receivedAt time.Time) time.Time {
	// APK expire_after is a millisecond duration, independent of wall-clock skew.
	if millis := toInt64(challenge["expire_after"], 0); millis > 0 && millis <= 24*60*60*1000 {
		return receivedAt.Add(time.Duration(millis) * time.Millisecond)
	}
	if epoch := toInt64(challenge["expire_at"], 0); epoch > 0 {
		if epoch >= 1_000_000_000_000 {
			return time.UnixMilli(epoch)
		}
		return time.Unix(epoch, 0)
	}
	return time.Time{}
}

// ComputePow 使用纯 Go 实现求解 PoW challenge (DeepSeekHashV1)。
func ComputePow(ctx context.Context, challenge map[string]any) (int64, error) {
	algo, _ := challenge["algorithm"].(string)
	if algo != "DeepSeekHashV1" {
		return 0, errors.New("unsupported algorithm")
	}
	challengeStr, _ := challenge["challenge"].(string)
	salt, _ := challenge["salt"].(string)
	expireAt := toInt64(challenge["expire_at"], 1680000000)
	difficulty := toInt64FromFloat(challenge["difficulty"], 144000)

	return pow.SolvePow(ctx, challengeStr, salt, expireAt, difficulty)
}

// BuildPowHeader preserves APK AuthedPowSolution field order before Base64.
func BuildPowHeader(challenge map[string]any, answer int64) (string, error) {
	payload := struct {
		Algorithm  any   `json:"algorithm"`
		Challenge  any   `json:"challenge"`
		Salt       any   `json:"salt"`
		Signature  any   `json:"signature"`
		Answer     int64 `json:"answer"`
		TargetPath any   `json:"target_path"`
	}{
		Algorithm: challenge["algorithm"], Challenge: challenge["challenge"],
		Salt: challenge["salt"], Signature: challenge["signature"],
		Answer: answer, TargetPath: challenge["target_path"],
	}
	b, err := dsprotocol.MarshalAppJSON("", payload)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

func toFloat64(v any, d float64) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case int64:
		return float64(n)
	default:
		return d
	}
}

func toInt64(v any, d int64) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case int:
		return int64(n)
	case int64:
		return n
	default:
		return d
	}
}

// toInt64FromFloat 与 toInt64 等价，仅名称区分用途。
func toInt64FromFloat(v any, d int64) int64 {
	return toInt64(v, d)
}
