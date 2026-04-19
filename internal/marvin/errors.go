package marvin

import (
	"fmt"
	"time"
)

type APIError struct {
	StatusCode int
	Body       string
}

func (a *APIError) Error() string {
	return fmt.Sprintf("Status Code: %v - %s", a.StatusCode, a.Body)
}

// DailyBudgetExceededError is returned when Marvin's daily request cap (1440/day,
// UTC-aligned window) is hit. The ResetsAt field carries the next UTC midnight,
// so callers can log it or schedule around it.
type DailyBudgetExceededError struct {
	ResetsAt time.Time
}

func (e *DailyBudgetExceededError) Error() string {
	return fmt.Sprintf("marvin daily budget exhausted; resets at %s",
		e.ResetsAt.Format(time.RFC3339))
}
