package config

// Standalone binaries and slim images must resolve the same IANA zones as the
// Node SDK even when the host does not provide a zoneinfo database.
import _ "time/tzdata"
