package client

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// FileRateLimitError preserves business-level throttling even with HTTP 200.
type FileRateLimitError struct {
	Op         string
	Code       int
	BizCode    int
	RetryAfter string
}

func (e *FileRateLimitError) Error() string {
	return fmt.Sprintf("%s: upstream requests are rate limited (code=%d, biz_code=%d, TOO_MANY_REQUESTS); retry later", e.Op, e.Code, e.BizCode)
}

func (*FileRateLimitError) HTTPStatus() int { return http.StatusTooManyRequests }

func fileRateLimit(op string, status, code, bizCode int, msg, bizMsg string) *FileRateLimitError {
	if status == http.StatusTooManyRequests || code == 40029 || bizCode == 40029 ||
		strings.Contains(strings.ToUpper(msg+" "+bizMsg), "TOO_MANY_REQUESTS") {
		return &FileRateLimitError{Op: op, Code: code, BizCode: bizCode}
	}
	return nil
}

func fileRateLimitDelay(attempt int, retryAfter string) time.Duration {
	delay := time.Second << min(max(attempt-1, 0), 4)
	if seconds, err := strconv.ParseInt(strings.TrimSpace(retryAfter), 10, 64); err == nil && seconds > 0 {
		// Bound integer conversion without shortening any practical server delay.
		return max(delay, time.Duration(min(seconds, 86400))*time.Second)
	}
	if until, err := http.ParseTime(retryAfter); err == nil {
		return max(delay, time.Until(until))
	}
	return delay
}
