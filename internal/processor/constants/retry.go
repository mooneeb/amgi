package constants

import "time"

var (
	// DefaultRetryInterval is the fallback cadence for the retry sweep when
	// retry_interval_seconds is not set in config.
	DefaultRetryInterval time.Duration = 5 * time.Minute
)
