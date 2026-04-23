package marvin

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func TestNextUTCMidnight(t *testing.T) {
	plus5 := time.FixedZone("+05:00", 5*60*60)

	tests := []struct {
		name string
		now  time.Time
		want time.Time
	}{
		{
			name: "2026-04-19 10:40 UTC",
			now:  time.Date(2026, 4, 19, 10, 40, 0, 0, time.UTC),
			want: time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "2026-04-19 02:00 +05:00 (= 21:00 UTC previous day)",
			now:  time.Date(2026, 4, 19, 2, 0, 0, 0, plus5),
			want: time.Date(2026, 4, 19, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "2026-04-19 23:59:59 UTC",
			now:  time.Date(2026, 4, 19, 23, 59, 59, 0, time.UTC),
			want: time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "2026-04-19 00:00:00 UTC exactly",
			now:  time.Date(2026, 4, 19, 0, 0, 0, 0, time.UTC),
			want: time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := nextUTCMidnight(tt.now)
			if !got.Equal(tt.want) {
				t.Errorf("nextUTCMidnight(%v) = %v, want %v", tt.now, got, tt.want)
			}
		})
	}
}

// TestConsumeDailyBudget covers the D-044 daily budget gate.
// consumeDailyBudget reads time.Now() internally, so tests set dailyResetAt
// relative to "far future" (forces no-rollover branch) or "recent past"
// (forces rollover branch) to exercise each code path deterministically.
func TestConsumeDailyBudget(t *testing.T) {
	const max = 1440

	tests := []struct {
		name              string
		initialCount      int
		resetAtOffset     time.Duration // from now; negative = past (rollover fires), positive = future (no rollover)
		wantErr           bool          // expect *DailyBudgetExceededError
		wantCountAfter    int
		wantResetAdvanced bool // expect dailyResetAt to have moved past its initial value
	}{
		{
			name:              "below cap, no rollover — increments",
			initialCount:      5,
			resetAtOffset:     24 * time.Hour,
			wantErr:           false,
			wantCountAfter:    6,
			wantResetAdvanced: false,
		},
		{
			name:              "at cap, no rollover — returns DailyBudgetExceededError, no increment",
			initialCount:      max,
			resetAtOffset:     24 * time.Hour,
			wantErr:           true,
			wantCountAfter:    max,
			wantResetAdvanced: false,
		},
		{
			name:              "rollover fires: count was mid-range, resets to 0 then increments to 1",
			initialCount:      500,
			resetAtOffset:     -time.Hour,
			wantErr:           false,
			wantCountAfter:    1,
			wantResetAdvanced: true,
		},
		{
			name:              "rollover fires: count was at cap, resets to 0 then increments to 1 (budget restored)",
			initialCount:      max,
			resetAtOffset:     -time.Hour,
			wantErr:           false,
			wantCountAfter:    1,
			wantResetAdvanced: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Now().UTC()
			initialResetAt := now.Add(tc.resetAtOffset)
			m := &marvin{
				dailyMax:     max,
				dailyCount:   tc.initialCount,
				dailyResetAt: initialResetAt,
			}

			err := m.consumeDailyBudget()

			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected *DailyBudgetExceededError, got nil")
				}
				var budgetErr *DailyBudgetExceededError
				if !errors.As(err, &budgetErr) {
					t.Fatalf("expected *DailyBudgetExceededError, got %T: %v", err, err)
				}
				if !budgetErr.ResetsAt.Equal(m.dailyResetAt) {
					t.Errorf("ResetsAt = %v, want %v (current dailyResetAt)",
						budgetErr.ResetsAt, m.dailyResetAt)
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if m.dailyCount != tc.wantCountAfter {
				t.Errorf("dailyCount = %d, want %d", m.dailyCount, tc.wantCountAfter)
			}

			rolloverHappened := !m.dailyResetAt.Equal(initialResetAt)
			if rolloverHappened != tc.wantResetAdvanced {
				t.Errorf("rollover-happened = %v, want %v (initial=%v, current=%v)",
					rolloverHappened, tc.wantResetAdvanced, initialResetAt, m.dailyResetAt)
			}
		})
	}
}

// TestConsumeDailyBudget_ConcurrentAccessRespectsCapExactly is the reason
// dailyMu exists. Without the mutex, concurrent callers race on dailyCount —
// some would go over the cap, some would see stale reads. With the mutex,
// check+increment is atomic and exactly dailyMax calls succeed regardless
// of goroutine interleaving.
//
// This test also catches data races when run under `go test -race`.
func TestConsumeDailyBudget_ConcurrentAccessRespectsCapExactly(t *testing.T) {
	const (
		max            = 500
		goroutines     = 100
		callsPerWorker = 10 // 1000 attempts against a cap of 500
	)

	m := &marvin{
		dailyMax:     max,
		dailyCount:   0,
		dailyResetAt: time.Now().UTC().Add(24 * time.Hour), // far future — no rollover during the test
	}

	var (
		wg       sync.WaitGroup
		okCount  int
		errCount int
		countMu  sync.Mutex // protects test counters, not the code under test
	)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < callsPerWorker; j++ {
				if err := m.consumeDailyBudget(); err != nil {
					countMu.Lock()
					errCount++
					countMu.Unlock()
				} else {
					countMu.Lock()
					okCount++
					countMu.Unlock()
				}
			}
		}()
	}
	wg.Wait()

	if okCount != max {
		t.Errorf("successful consumes = %d, want exactly %d", okCount, max)
	}
	wantErrs := goroutines*callsPerWorker - max
	if errCount != wantErrs {
		t.Errorf("budget-exceeded errors = %d, want %d", errCount, wantErrs)
	}
	if m.dailyCount != max {
		t.Errorf("dailyCount = %d, want %d", m.dailyCount, max)
	}
}
