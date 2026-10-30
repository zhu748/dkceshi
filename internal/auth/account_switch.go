package auth

import "time"

// Bound failover waits even when the incoming request has no deadline. Queue
// capacity, account filters and exhausted-account exclusions still apply.
const accountSwitchWaitTimeout = 30 * time.Second
