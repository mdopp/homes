package usage

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/mdopp/homes/internal/usage/usagefake"
)

// owner 2026-11-28: the counts under the old name are copied to App day by
// day, the day each was counted; a run cut off goes on where it stopped and
// a second run copies nothing.
func TestUmziehenCopiesEachDayOnce(t *testing.T) {
	f := usagefake.New("tok")
	t.Cleanup(f.Close)
	f.Fenster = fenster("2026-11-28")
	c := New(f.URL, "tok")
	ctx := context.Background()
	for _, x := range []struct {
		event, day string
		n          int64
	}{{"page.start", "2026-11-28", 3}, {"page.start", "2026-11-27", 2}, {"page.box", "2026-11-25", 5}, {"page.start", "2026-10-01", 9}} {
		if err := c.post(ctx, "/ingest", map[string]any{"app": "haus", "event": x.event, "day": x.day, "count": x.n}); err != nil {
			t.Fatal(err)
		}
	}
	fertig := map[string]bool{"2026-11-27": true} // copied by a run cut off
	var days []string
	n, err := c.Umziehen(ctx, "haus", 30, fertig, func(d string) error { days = append(days, d); return nil })
	if err != nil || n != 8 || len(days) != 29 {
		t.Fatalf("%d %v, %d days", n, err, len(days))
	}
	got := f.Counts()
	for k, want := range map[usagefake.Count]int64{
		{App: App, Event: "page.start", Day: "2026-11-28"}: 3, {App: App, Event: "page.box", Day: "2026-11-25"}: 5,
		{App: App, Event: "page.start", Day: "2026-11-27"}: 0, {App: App, Event: "page.start", Day: "2026-10-01"}: 0, // done before; outside the 30 days
		{App: "haus", Event: "page.start", Day: "2026-11-28"}: 3,
	} {
		if got[k] != want {
			t.Errorf("%v = %d, want %d", k, got[k], want)
		}
	}
	if n, err := c.Umziehen(ctx, "haus", 30, fertig, func(string) error { return nil }); err != nil || n != 0 {
		t.Fatalf("a second run: %d %v", n, err)
	}
	f.Down(true)
	if _, err := c.Umziehen(ctx, "haus", 30, map[string]bool{}, func(string) error { return nil }); err == nil {
		t.Fatal("usage-metrics down is no error")
	}
}

// fenster is usage-metrics' summary of the days up to heute.
func fenster(heute string) func(string, map[usagefake.Count]int64) any {
	return func(days string, counts map[usagefake.Count]int64) any {
		n, _ := strconv.Atoi(days)
		h, _ := time.Parse(time.DateOnly, heute)
		from := h.AddDate(0, 0, -(n - 1)).Format(time.DateOnly)
		byApp := map[string][]map[string]any{}
		for k, c := range counts {
			if k.Day >= from && k.Day <= heute {
				byApp[k.App] = append(byApp[k.App], map[string]any{"event": k.Event, "count": c})
			}
		}
		apps := []map[string]any{}
		for app, es := range byApp {
			apps = append(apps, map[string]any{"app": app, "events": es})
		}
		return map[string]any{"days": n, "from": from, "apps": apps}
	}
}
