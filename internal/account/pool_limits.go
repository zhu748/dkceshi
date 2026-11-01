package account

import (
	"os"
	"strconv"
	"strings"
	"time"
)

func (p *Pool) ApplyRuntimeLimits(maxInflightPerAccount, maxQueueSize, globalMaxInflight int) {
	if maxInflightPerAccount <= 0 {
		maxInflightPerAccount = 1
	}
	if maxQueueSize < 0 {
		maxQueueSize = 0
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if globalMaxInflight <= 0 {
		globalMaxInflight = maxInflightPerAccount * len(p.queue)
		if globalMaxInflight <= 0 {
			globalMaxInflight = maxInflightPerAccount
		}
	}
	p.maxInflightPerAccount = maxInflightPerAccount
	p.maxQueueSize = maxQueueSize
	p.globalMaxInflight = globalMaxInflight
	p.recommendedConcurrency = defaultRecommendedConcurrency(len(p.queue), p.maxInflightPerAccount)
	p.notifyWaiterLocked()
}

func maxInflightFromEnv() int {
	if raw := strings.TrimSpace(os.Getenv("DS2API_ACCOUNT_MAX_INFLIGHT")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			return n
		}
	}
	return 2
}

func defaultRecommendedConcurrency(accountCount, maxInflightPerAccount int) int {
	if accountCount <= 0 {
		return 0
	}
	if maxInflightPerAccount <= 0 {
		maxInflightPerAccount = 2
	}
	return accountCount * maxInflightPerAccount
}

func maxQueueFromEnv(defaultSize int) int {
	if raw := strings.TrimSpace(os.Getenv("DS2API_ACCOUNT_MAX_QUEUE")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n >= 0 {
			return n
		}
	}
	if defaultSize < 0 {
		return 0
	}
	return defaultSize
}

// acquireChecks carries the per-attempt loop-invariant facts of the acquire
// path. Computing them once per attempt (instead of per candidate) removes
// 4-5 store RLocks, one time.Now and one date Format per candidate from the
// schedulable-account scan, which previously made a 5000-account pool pay
// milliseconds of serialized work under the pool lock per acquire.
type acquireChecks struct {
	rateEnabled bool
	dailyActive bool
	dailyMax    int
	globalFull  bool
	// now is read once per attempt and shared by every per-candidate clock
	// consult (rate-window expiry); the scan used to call time.Now() once
	// per skipped candidate.
	now time.Time
}

func (p *Pool) acquireChecksLocked() acquireChecks {
	checks := acquireChecks{
		rateEnabled: p.rateLimitEnabledLocked(),
		dailyActive: p.dailyLimitActiveLocked(),
		globalFull:  p.globalMaxInflight > 0 && p.currentInUseLocked() >= p.globalMaxInflight,
		now:         time.Now(),
	}
	if checks.dailyActive {
		checks.dailyMax = p.dailyLimitMaxPerDayLocked()
		// Roll the day once per attempt instead of once per candidate; the date
		// cannot meaningfully change between two candidates of one scan.
		p.rollDailyLimitDateLocked()
	}
	return checks
}

func (p *Pool) canAcquireIDCheckedLocked(accountID string, checks acquireChecks) bool {
	if accountID == "" {
		return false
	}
	if checks.rateEnabled && p.isRateLimitedAtLocked(accountID, checks.now) {
		return false
	}
	if checks.dailyActive && p.dailyCounts[accountID] >= checks.dailyMax {
		return false
	}
	if p.inUse[accountID] >= p.maxInflightPerAccount {
		return false
	}
	if checks.globalFull {
		return false
	}
	return true
}

// currentInUseLocked returns the total number of in-flight acquisitions. It is
// maintained incrementally (see inUseTotal) so the global-limit check on the
// hot path stays O(1) instead of walking the whole inUse map per candidate.
func (p *Pool) currentInUseLocked() int {
	return p.inUseTotal
}
