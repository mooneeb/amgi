package marvin

import (
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
