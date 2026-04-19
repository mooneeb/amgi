package constants

import "time"

var (
	// Default polling interval is 30 minutes
	DefaultPollingInterval time.Duration = 30 * 60 * time.Second
)
