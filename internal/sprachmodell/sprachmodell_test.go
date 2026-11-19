package sprachmodell

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mdopp/homes/internal/modellease"
	"github.com/mdopp/homes/internal/modellease/leasefake"
)

// uhr is a clock the test moves; the fake lease and the switch share it.
type uhr struct {
	mu sync.Mutex
	t  time.Time
}

func (u *uhr) now() time.Time { u.mu.Lock(); defer u.mu.Unlock(); return u.t }
func (u *uhr) vor(d time.Duration) {
	u.mu.Lock()
	u.t = u.t.Add(d)
	u.mu.Unlock()
}

const key = "modell-key-geheim"

func aufbau(t *testing.T) (*Schalter, *leasefake.Server, *uhr) {
	t.Helper()
	u := &uhr{t: time.Date(2026, 11, 17, 20, 0, 0, 0, time.UTC)}
	f := leasefake.New(key)
	f.Now = u.now
	t.Cleanup(f.Close)
	return &Schalter{Lease: &modellease.Client{Base: f.Base(), Key: key}, Now: u.now}, f, u
}

// calls are the lease calls of one method, their holders.
func calls(f *leasefake.Server, method string) []string {
	var out []string
	for _, r := range f.Seen() {
		if r.Path == "/api/model-lease" && r.Method == method {
			var b struct {
				Holder string `json:"holder"`
			}
			json.Unmarshal(r.Body, &b)
			out = append(out, b.Holder)
		}
	}
	return out
}

// The probe the owner asked for: Homes renews the window only until the
// end the person chose, then gives it back (DELETE) and renews no more.
func TestRenewsUntilTheEndThenGivesBack(t *testing.T) {
	s, f, u := aufbau(t)
	ctx := context.Background()
	if err := s.Umschalten(ctx, "anna", "gemma-4-12b", time.Hour); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2*60*2; i++ { // two hours, a look every 30 s, as Laufen does
		u.vor(30 * time.Second)
		s.Pflegen(ctx)
		if l := f.Lease(); l == nil && u.now().Before(time.Date(2026, 11, 17, 21, 0, 0, 0, time.UTC)) {
			t.Fatalf("the window ended at %s, before its end: a renewal was missed", u.now().Format("15:04:05"))
		}
	}
	posts, deletes := calls(f, http.MethodPost), calls(f, http.MethodDelete)
	if len(posts) < 10 { // the take and a renewal every renew_after (5 min) for an hour
		t.Errorf("%d posts: renewed too seldom", len(posts))
	}
	if len(deletes) != 1 || deletes[0] != "homes-anna" {
		t.Errorf("deletes %v, want one for homes-anna", deletes)
	}
	after := false
	for _, r := range f.Seen() {
		if r.Method == http.MethodDelete {
			after = true
		} else if after && r.Method == http.MethodPost {
			t.Errorf("renewed after giving back: %s", r.Body)
		}
	}
	if f.Lease() != nil {
		t.Errorf("still held: %+v", f.Lease())
	}
}

// Without Homes the window lapses: Laib gives it back after two missed
// renewals, so the box is the household's again within its grace.
func TestAStoppedHomesLetsTheWindowLapse(t *testing.T) {
	s, f, u := aufbau(t)
	if err := s.Umschalten(context.Background(), "anna", "gemma-4-12b", 4*time.Hour); err != nil {
		t.Fatal(err)
	}
	u.vor(11 * time.Minute) // nobody looks: Homes is gone
	if l := f.Lease(); l != nil {
		t.Errorf("held after 11 min without a renewal: %+v", l)
	}
}

// A window of homes-… this Homes did not take (it restarted) is given back
// at the first look; someone else's is left alone.
func TestARestartedHomesGivesBackWhatItDidNotTake(t *testing.T) {
	s, f, u := aufbau(t)
	f.Hold("homes-ben", "gemma-4-e4b", u.now().Add(3*time.Hour))
	s.Pflegen(context.Background())
	if f.Lease() != nil || len(calls(f, http.MethodDelete)) != 1 {
		t.Errorf("homes-ben's window not given back: %+v, deletes %v", f.Lease(), calls(f, http.MethodDelete))
	}
	f.Hold("bote", "qwen3.8-27b", u.now().Add(time.Hour))
	s.Pflegen(context.Background())
	if l := f.Lease(); l == nil || l.Holder != "bote" || len(calls(f, http.MethodDelete)) != 1 || len(calls(f, http.MethodPost)) != 0 {
		t.Errorf("bote's window touched: %+v, %v %v", l, calls(f, http.MethodDelete), calls(f, http.MethodPost))
	}
}

func TestSwitchingOverAndBack(t *testing.T) {
	s, f, u := aufbau(t)
	ctx := context.Background()
	st := s.Lesen(ctx)
	if !st.Erreichbar || st.Halter != "" || st.Haushalt != "qwen3.6-35b-a3b" || strings.Join(st.Presets, ",") != "gemma-4-e4b,gemma-4-12b,qwen3.8-27b" {
		t.Fatalf("household: %+v", st)
	}
	for _, bad := range []struct {
		preset string
		d      time.Duration
	}{{"qwen3.6-35b-a3b", time.Hour}, {"gemma-4-12b", 3 * time.Hour}, {"nichts", time.Hour}} {
		if err := s.Umschalten(ctx, "anna", bad.preset, bad.d); !errors.Is(err, ErrNicht) {
			t.Errorf("%s for %s: %v, want ErrNicht", bad.preset, bad.d, err)
		}
	}
	if len(calls(f, http.MethodPost)) != 0 {
		t.Fatal("a refused switch reached the lease")
	}
	if err := s.Umschalten(ctx, "Anna", "gemma-4-12b", time.Hour); err != nil {
		t.Fatal(err)
	}
	st = s.Lesen(ctx)
	if p, ok := st.Homes(); !ok || p != "anna" || st.Preset != "gemma-4-12b" || !st.Bis.Equal(u.now().Add(time.Hour)) {
		t.Errorf("after the switch: %+v", st)
	}
	// ben switches over anna's, as one lamp switches over another's
	if err := s.Umschalten(ctx, "ben", "qwen3.8-27b", 4*time.Hour); err != nil {
		t.Fatal(err)
	}
	if l := f.Lease(); l == nil || l.Holder != "homes-ben" || l.Alias != "qwen3.8-27b" {
		t.Errorf("ben's: %+v", l)
	}
	if err := s.Zurueck(ctx, "anna"); err != nil || f.Lease() != nil {
		t.Errorf("Zurück: %v, %+v", err, f.Lease())
	}
	if err := s.Zurueck(ctx, "anna"); err != nil { // the household already: nothing to do
		t.Errorf("Zurück in the household: %v", err)
	}
	// someone who is no person of Homes: neither switched over nor given back
	f.Hold("bote", "qwen3.8-27b", u.now().Add(time.Hour))
	var held *modellease.Held
	if err := s.Umschalten(ctx, "anna", "gemma-4-12b", time.Hour); !errors.As(err, &held) || held.Holder != "bote" {
		t.Errorf("over bote's: %v", err)
	}
	if err := s.Zurueck(ctx, "anna"); !errors.As(err, &held) {
		t.Errorf("Zurück from bote's: %v", err)
	}
	if l := f.Lease(); l == nil || l.Holder != "bote" {
		t.Errorf("bote's window touched: %+v", l)
	}
}

func TestTheCardInWords(t *testing.T) {
	s, f, u := aufbau(t)
	ctx := context.Background()
	loc := time.UTC
	a := s.Lesen(ctx).Sehen("de", u.now(), loc)
	if a.Zustand != "haushalt" || a.Zeile != "Haushalt · Qwen3.6 35B A3B" || a.Detail != "Sprachsteuerung läuft" || a.Kosten != "" || a.Zurueck != "" || len(a.Wahl) != 3 {
		t.Errorf("household: %+v", a)
	}
	if d := a.Wahl[1].Dauern; a.Wahl[1].Name != "Gemma 4 12B" || len(d) != 2 || d[0].Label != "1 Std." || d[1].Bis != "00:00" ||
		d[0].Frage != "Auf Gemma 4 12B umschalten? Die Sprachsteuerung pausiert bis 21:00." {
		t.Errorf("the choice: %+v", a.Wahl[1])
	}
	f.Hold("homes-anna", "gemma-4-12b", u.now().Add(90*time.Minute))
	a = s.Lesen(ctx).Sehen("de", u.now(), loc)
	if a.Zustand != "anderes" || a.Zeile != "Gemma 4 12B · für anna" || a.Kosten != "Sprachsteuerung pausiert bis 21:30" || a.Zurueck != "Zurück zum Haushalt" || len(a.Wahl) != 2 {
		t.Errorf("anna's: %+v", a)
	}
	if en := s.Lesen(ctx).Sehen("en", u.now(), loc); en.Kosten != "Voice control paused until 21:30" || en.Zurueck != "Back to the household" {
		t.Errorf("in English: %+v", en)
	}
	f.Hold("bote", "qwen3.8-27b", u.now().Add(time.Hour))
	a = s.Lesen(ctx).Sehen("de", u.now(), loc)
	if a.Zustand != "belegt" || a.Zurueck != "" || len(a.Wahl) != 0 || !strings.Contains(a.Hinweis, "bote") {
		t.Errorf("bote's: %+v", a)
	}
	f.Down = true
	a = s.Lesen(ctx).Sehen("de", u.now(), loc)
	if a.Zustand != "weg" || a.Zeile != "Das Sprachmodell antwortet gerade nicht." || len(a.Wahl) != 0 || a.Zurueck != "" {
		t.Errorf("down: %+v", a)
	}
	if err := s.Umschalten(ctx, "anna", "gemma-4-12b", time.Hour); !errors.Is(err, ErrWeg) {
		t.Errorf("switching while down: %v", err)
	}
}

func TestHalter(t *testing.T) {
	for in, want := range map[string]string{"anna": "homes-anna", "Anna.Müller": "homes-anna-m-ller", strings.Repeat("x", 80): "homes-" + strings.Repeat("x", 58)} {
		if got := Halter(in); got != want {
			t.Errorf("Halter(%q) = %q, want %q", in, got, want)
		}
	}
}

// A person who switches again gets the new end: in the same window a take
// would be a renewal and keep the old one, so Homes gives back first.
func TestSwitchingAgainTakesTheNewEnd(t *testing.T) {
	s, f, u := aufbau(t)
	ctx := context.Background()
	if err := s.Umschalten(ctx, "anna", "gemma-4-12b", time.Hour); err != nil {
		t.Fatal(err)
	}
	if err := s.Umschalten(ctx, "anna", "gemma-4-e4b", 4*time.Hour); err != nil {
		t.Fatal(err)
	}
	if l := f.Lease(); l == nil || l.Alias != "gemma-4-e4b" || !l.Deadline.Equal(u.now().Add(4*time.Hour)) {
		t.Errorf("the second switch: %+v", l)
	}
}
