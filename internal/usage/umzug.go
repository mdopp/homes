package usage

import (
	"context"
	"fmt"
	"time"
)

// Umziehen copies app von's counts of the last tage days to App, a day at
// a time (the rename's migration, cmd/homes): usage-metrics answers totals
// per window only, so one day's counts are the window that reaches it
// minus the window a day shorter. fertig holds the days already copied;
// kopiert is told each day once it is copied, so a run cut off goes on
// where it stopped and copies no whole day twice. The counts it copied.
func (c *Counter) Umziehen(ctx context.Context, von string, tage int, fertig map[string]bool, kopiert func(day string) error) (int64, error) {
	if c == nil || von == App {
		return 0, nil
	}
	var n int64
	kuerzer := map[string]int64{}
	var vorher string // the shorter window's first day
	for k := 1; k <= tage; k++ {
		s, err := c.Summary(ctx, k)
		if err != nil {
			return n, err
		}
		day := s.From
		if vorher != "" {
			if d, err := time.Parse(time.DateOnly, vorher); err != nil || d.AddDate(0, 0, -1).Format(time.DateOnly) != day {
				return n, fmt.Errorf("usage-metrics: the window of %d days starts %s, a day before %s expected (midnight passed?)", k, day, vorher)
			}
		}
		laenger := s.Events(von)
		if !fertig[day] {
			for event, total := range laenger {
				if d := total - kuerzer[event]; d > 0 {
					if err := c.post(ctx, "/ingest", map[string]any{"app": App, "event": event, "day": day, "count": d}); err != nil {
						return n, err
					}
					n += d
				}
			}
			fertig[day] = true
			if err := kopiert(day); err != nil {
				return n, err
			}
		}
		kuerzer, vorher = laenger, day
	}
	return n, nil
}
