// Package netz is who stands close to whom, and through what (owner
// 2026-11-09): the people seen together in the Immich key owner's photos,
// and what the phone books say (the same address, the same company, a
// relation a phone wrote). Only the key's owner sees it (pages); it holds
// the ties among all the people in those photos, not only the owner's.
//
// Once a day (Rechnen, from gesichter's background run) the photos are
// read again whole from Immich and kept as a small table in HOMES_DATA;
// the ties are rebuilt from that table each time, never changed in
// place, so a merge, a rename or a deletion in Immich cannot drift.
//
// A photo day at one place is an event; a pair counts once per event,
// less in a big group (1/(n-1)), less the longer ago (half-life
// halbwert), a little more for each year they meet in. A tie from photos
// alone shows from two days on. The order divides by how often both
// appear at all (hub), so a child on every photo is not close to all.
package netz

import (
	"cmp"
	"context"
	"encoding/json"
	"maps"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mdopp/homes/internal/gesichter"
	"github.com/mdopp/homes/internal/jsonfile"
)

const (
	tabelle      = "immich-netz.json"           // the photos and phone-book ties, rewritten daily
	verborgenNam = "immich-netz-verborgen.json" // who is hidden, per key owner
	takt         = 24 * time.Hour
	halbwert     = 4 * 365.25 * 24 * time.Hour
	jahrBonus    = 0.15  // per distinct year a pair meets in
	zelle        = 0.009 // a place cell, in degrees of latitude: about 1 km
	minTage      = 2     // a tie from photos shows from this many days on
)

// festGewicht is what a phone-book tie adds to a pair's strength.
var festGewicht = map[string]float64{Adresse: 1.5, Firma: 0.4, Beziehung: 3}

// The kinds of phone-book tie.
const (
	Adresse   = "adresse"
	Firma     = "firma"
	Beziehung = "beziehung"
)

// Hiding (owner 2026-11-09): Aus is not shown and restorable; Nicht is
// taken out of the photos before any tie is computed.
const (
	Aus   = "aus"
	Nicht = "nicht"
)

// foto is one photo of the table: its id, when (an instant), where, who
// (person ids).
type foto struct {
	ID    string    `json:"i"`
	T     time.Time `json:"t"`
	Lat   *float64  `json:"la,omitempty"`
	Lng   *float64  `json:"lo,omitempty"`
	Stadt string    `json:"s,omitempty"`
	Wer   []string  `json:"p"`
}

// kontakt is a contact of the phone books: its key (haushalt/<uid>,
// privat/<uid>), name and Immich person ("" none); for the suggestions
// (fragen.go) its family name, birthday and streets (strassen).
type kontakt struct {
	Key      string   `json:"k"`
	Name     string   `json:"n"`
	Person   string   `json:"p,omitempty"`
	Family   string   `json:"f,omitempty"`
	Geb      string   `json:"g,omitempty"`
	Strassen []string `json:"s,omitempty"`
}

// Fest is a phone-book tie between two contacts' keys; for a relation,
// A's card says B is A's Wie.
type Fest struct {
	A   string `json:"a"`
	B   string `json:"b"`
	Art string `json:"art"`
	Wie string `json:"wie,omitempty"`
	// Gesagt: the key owner's answer (fragen.go), A and B nodes, Wie a kind
	Gesagt bool `json:"-"`
}

// stand is what the daily run read.
type stand struct {
	Geholt   time.Time         `json:"geholt"`
	Fotos    []foto            `json:"fotos"`
	Namen    map[string]string `json:"namen"` // the named persons, by id
	Kontakte []kontakt         `json:"kontakte"`
	Fest     []Fest            `json:"fest"`
	Ich      string            `json:"ich,omitempty"` // the key owner's own contact
	Geb      map[string]string `json:"geb,omitempty"` // Immich's birthDate, by named person
}

// Knoten is a person: an Immich person (ID its id) or a contact without
// one (ID its key); Kontakt its contact ("" none).
type Knoten struct {
	ID, Name, Kontakt string
}

// Kante is a tie: Staerke for the rings, Rang for the order; Tage the
// distinct photo days and Seit the first one's year (0 none); Fest what
// the phone books say.
type Kante struct {
	A, B          string
	Staerke, Rang float64
	Tage, Seit    int
	Fest          []Fest
}

// Netz is the contact net of the key's owner.
type Netz struct {
	G   *gesichter.Gesichter // its Immich, its contacts, whose key, HOMES_DATA
	Loc *time.Location       // the house's: a photo's day; nil time.Local
	Now func() time.Time

	mu        sync.Mutex
	geladen   bool
	st        stand
	verborgen map[string]map[string]string  // by login, by node: Aus or Nicht
	antworten map[string]map[string]antwort // by login, by pair (paarKey): fragen.go
	merkmale  map[string]merkmale           // by node
	knoten    map[string]Knoten
	von       map[string]string // node by contact key
	kanten    map[string][]*Kante
	ich       string
}

func (n *Netz) now() time.Time {
	if n.Now != nil {
		return n.Now()
	}
	return time.Now()
}

// laden reads the kept files once and builds; n.mu is held.
func (n *Netz) laden() {
	if n.geladen {
		return
	}
	n.geladen = true
	if b, err := os.ReadFile(filepath.Join(n.G.Dir, tabelle)); err == nil {
		json.Unmarshal(b, &n.st)
	}
	if b, err := os.ReadFile(filepath.Join(n.G.Dir, verborgenNam)); err == nil {
		json.Unmarshal(b, &n.verborgen)
	}
	if b, err := os.ReadFile(filepath.Join(n.G.Dir, antwortNam)); err == nil {
		json.Unmarshal(b, &n.antworten)
	}
	if n.verborgen == nil {
		n.verborgen = map[string]map[string]string{}
	}
	if n.antworten == nil {
		n.antworten = map[string]map[string]antwort{}
	}
	n.bauen()
}

// Rechnen reads the photos and the phone books again when the last read
// is takt old (or there is none), keeps them and rebuilds the ties; else
// it does nothing. Without the key nothing is read.
func (n *Netz) Rechnen(ctx context.Context) error {
	if n == nil || n.G == nil || n.G.Immich == nil {
		return nil
	}
	n.mu.Lock()
	n.laden()
	faellig := n.now().Sub(n.st.Geholt) >= takt
	n.mu.Unlock()
	if !faellig {
		return nil
	}
	as, err := n.G.Immich.Aufnahmen(ctx)
	if err != nil {
		return err
	}
	st := stand{Geholt: n.now(), Namen: map[string]string{}, Geb: map[string]string{}}
	for _, a := range as {
		f := foto{ID: a.ID, Lat: a.Exif.Lat, Lng: a.Exif.Lng, Stadt: strings.TrimSpace(a.Exif.Stadt)}
		f.T, _ = time.Parse(time.RFC3339Nano, a.Exif.Zeit)
		for _, p := range a.Personen {
			if p.ID != "" && !p.Versteckt {
				f.Wer = append(f.Wer, p.ID)
			}
			if name := strings.Join(strings.Fields(p.Name), " "); name != "" && !p.Versteckt {
				st.Namen[p.ID] = name
				st.Geb[p.ID] = cmp.Or(p.Geburtstag, st.Geb[p.ID])
			}
		}
		if len(f.Wer) > 0 && !f.T.IsZero() {
			st.Fotos = append(st.Fotos, f)
		}
	}
	st.Kontakte, st.Fest, st.Ich, err = n.telefonbuch(ctx)
	n.mu.Lock()
	defer n.mu.Unlock()
	if err != nil { // the phone books not read: theirs as before
		st.Kontakte, st.Fest, st.Ich = n.st.Kontakte, n.st.Fest, n.st.Ich
	}
	n.st = st
	n.bauen()
	return cmp.Or(jsonfile.Save(filepath.Join(n.G.Dir, tabelle), n.st), err)
}

// ereignis is one day at one place: its day, year, age and who.
type ereignis struct {
	tag  string
	jahr int
	t    time.Time
	wer  map[string]bool
}

// paar is a pair as it is summed up.
type paar struct {
	s, ueber    float64 // ueber: shared events over both's, geometric
	tage, jahre map[string]bool
	geteilt     int
	seit        int
}

// bauen rebuilds the nodes and the ties from st; n.mu is held.
func (n *Netz) bauen() {
	nicht := map[string]bool{}
	for id, art := range n.verborgen[n.G.Wessen()] {
		nicht[id] = art == Nicht
	}
	n.knoten, n.von, n.kanten = map[string]Knoten{}, map[string]string{}, map[string][]*Kante{}
	for id, name := range n.st.Namen {
		n.knoten[id] = Knoten{ID: id, Name: name}
	}
	for _, k := range n.st.Kontakte {
		id := cmp.Or(k.Person, k.Key)
		kn, da := n.knoten[id]
		if !da {
			kn = Knoten{ID: id, Name: k.Name}
		}
		if kn.Kontakt == "" || strings.HasPrefix(k.Key, "haushalt/") && !strings.HasPrefix(kn.Kontakt, "haushalt/") {
			kn.Kontakt = k.Key
		}
		n.knoten[id], n.von[k.Key] = kn, id
	}
	n.ich = n.von[n.st.Ich]
	paare := n.fotoPaare(nicht)
	alle := map[[2]string]*Kante{}
	for key, p := range paare {
		if len(p.tage) >= minTage {
			s := p.s + jahrBonus*float64(len(p.jahre))
			alle[key] = &Kante{A: key[0], B: key[1], Staerke: s, Rang: s * (0.25 + 0.75*p.ueber), Tage: len(p.tage), Seit: p.seit}
		}
	}
	n.festDazu(alle, nicht)
	n.gesagtDazu(alle, nicht)
	n.merkmaleBauen()
	for _, k := range alle {
		n.kanten[k.A], n.kanten[k.B] = append(n.kanten[k.A], k), append(n.kanten[k.B], k)
	}
}

// fotoPaare sums the photos up by pair, those in nicht left out.
func (n *Netz) fotoPaare(nicht map[string]bool) map[[2]string]*paar {
	loc := cmp.Or(n.Loc, time.Local)
	evs := map[string]*ereignis{}
	for _, f := range n.st.Fotos {
		t := f.T.In(loc)
		key := t.Format("2006-01-02") + "|" + ort(f)
		e := evs[key]
		if e == nil {
			e = &ereignis{tag: t.Format("2006-01-02"), jahr: t.Year(), t: f.T, wer: map[string]bool{}}
			evs[key] = e
		}
		for _, id := range f.Wer {
			if n.st.Namen[id] != "" && !nicht[id] {
				e.wer[id] = true
			}
		}
	}
	je, paare := map[string]int{}, map[[2]string]*paar{}
	for _, e := range evs {
		wer := slices.Sorted(maps.Keys(e.wer))
		alter := max(0, n.now().Sub(e.t).Hours()) / halbwert.Hours()
		w := math.Pow(0.5, alter) / float64(max(1, len(wer)-1))
		for i, a := range wer {
			je[a]++
			for _, b := range wer[i+1:] {
				p := paare[[2]string{a, b}]
				if p == nil {
					p = &paar{tage: map[string]bool{}, jahre: map[string]bool{}, seit: e.jahr}
					paare[[2]string{a, b}] = p
				}
				p.s, p.geteilt, p.seit = p.s+w, p.geteilt+1, min(p.seit, e.jahr)
				p.tage[e.tag], p.jahre[strconv.Itoa(e.jahr)] = true, true
			}
		}
	}
	for key, p := range paare { // the hub: a child on every photo shares many, out of many
		p.ueber = float64(p.geteilt) / math.Sqrt(float64(je[key[0]]*je[key[1]]))
	}
	return paare
}

// ort is a photo's place: its cell of about 1 km, else its city, else none.
func ort(f foto) string {
	if f.Lat != nil && f.Lng != nil && (*f.Lat != 0 || *f.Lng != 0) {
		breit := zelle / math.Max(0.1, math.Cos(*f.Lat*math.Pi/180))
		return strconv.Itoa(int(math.Floor(*f.Lat/zelle))) + "," + strconv.Itoa(int(math.Floor(*f.Lng/breit)))
	}
	return strings.ToLower(f.Stadt)
}

// festDazu adds the phone books' ties to alle, between the contacts'
// nodes, those in nicht left out; the same tie twice (both books) once.
func (n *Netz) festDazu(alle map[[2]string]*Kante, nicht map[string]bool) {
	for _, f := range n.st.Fest {
		a, b := n.von[f.A], n.von[f.B]
		if a == "" || b == "" || a == b || nicht[a] || nicht[b] {
			continue
		}
		key := [2]string{min(a, b), max(a, b)}
		k := alle[key]
		if k == nil {
			k = &Kante{A: key[0], B: key[1]}
			alle[key] = k
		}
		if slices.ContainsFunc(k.Fest, func(g Fest) bool { return g.Art == f.Art && g.Wie == f.Wie && n.von[g.A] == a }) {
			continue
		}
		if !slices.ContainsFunc(k.Fest, func(g Fest) bool { return g.Art == f.Art }) { // a kind counts once
			k.Staerke, k.Rang = k.Staerke+festGewicht[f.Art], k.Rang+festGewicht[f.Art]
		}
		k.Fest = append(k.Fest, f)
	}
}

// Verbindung is a tie as seen from one end: the other, and the tie.
type Verbindung struct {
	Mit   Knoten
	Kante Kante
}

// Nahe are the k ties of id ranked first, those hidden left out.
func (n *Netz) Nahe(id string, k int) []Verbindung {
	if n == nil {
		return nil
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	n.laden()
	weg := n.verborgen[n.G.Wessen()]
	var out []Verbindung
	for _, e := range n.kanten[id] {
		other := cmp.Or(map[bool]string{true: e.B}[e.A == id], e.A)
		if weg[other] == "" && weg[id] == "" {
			out = append(out, Verbindung{n.knoten[other], *e})
		}
	}
	slices.SortFunc(out, func(a, b Verbindung) int {
		return cmp.Or(cmp.Compare(b.Kante.Rang, a.Kante.Rang), strings.Compare(a.Mit.Name, b.Mit.Name))
	})
	return out[:min(k, len(out))]
}

// Zwischen are the ties among ids, each as the two indexes into ids.
func (n *Netz) Zwischen(ids []string) (out [][2]int) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.laden()
	for i, a := range ids {
		for _, e := range n.kanten[a] {
			if j := slices.Index(ids, cmp.Or(map[bool]string{true: e.B}[e.A == a], e.A)); j > i {
				out = append(out, [2]int{i, j})
			}
		}
	}
	return out
}

// Person is node id; ok false for none.
func (n *Netz) Person(id string) (Knoten, bool) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.laden()
	k, ok := n.knoten[id]
	return k, ok
}

// Von is the node of the contact key ("haushalt/<uid>"); "" none.
func (n *Netz) Von(key string) string {
	if n == nil {
		return ""
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	n.laden()
	return n.von[key]
}

// Mitte is where the picture starts: the key owner's node, else the one
// with the most ties; "" without any.
func (n *Netz) Mitte() string {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.laden()
	if len(n.kanten[n.ich]) > 0 {
		return n.ich
	}
	best := ""
	for id, ks := range n.kanten {
		if len(ks) > len(n.kanten[best]) || len(ks) == len(n.kanten[best]) && id < best {
			best = id
		}
	}
	return best
}

// Verborgen is a hidden node and how (Aus, Nicht).
type Verborgen struct {
	Knoten
	Art string
}

// Verborgene are the key owner's hidden nodes, by name.
func (n *Netz) Verborgene() (out []Verborgen) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.laden()
	for id, art := range n.verborgen[n.G.Wessen()] {
		out = append(out, Verborgen{cmp.Or(n.knoten[id], Knoten{ID: id, Name: id}), art})
	}
	slices.SortFunc(out, func(a, b Verborgen) int { return strings.Compare(a.Name, b.Name) })
	return out
}

// Verbergen hides id for the key's owner (Aus, Nicht), or with "" shows
// it again; Nicht's change rebuilds the ties at once from the table.
func (n *Netz) Verbergen(id, art string) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.laden()
	wer := n.G.Wessen()
	if wer == "" || id == "" {
		return os.ErrPermission
	}
	m := n.verborgen[wer]
	if m == nil {
		m = map[string]string{}
		n.verborgen[wer] = m
	}
	vorher := m[id]
	if art == "" {
		delete(m, id)
	} else {
		m[id] = art
	}
	if vorher == Nicht || art == Nicht {
		n.bauen()
	}
	return jsonfile.Save(filepath.Join(n.G.Dir, verborgenNam), n.verborgen)
}
