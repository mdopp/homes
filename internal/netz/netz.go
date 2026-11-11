// Package netz is what is left in Homes of the contact net (owner
// 2026-11-09; ADR 0002): the relations of a contact, as the phone books
// say them (a relation a phone wrote) and as the Immich key's owner
// answered them, changed on the contact's page. Who stands close to whom
// is computed in the person's archive now, not here (owner 2026-11-11).
//
// Once a day (Rechnen, from gesichter's background run) the phone books
// are read again and kept as a small table in HOMES_DATA; the nodes are
// rebuilt from it and from gesichter's Immich persons each time, never
// changed in place, so a merge, a rename or a deletion cannot drift.
package netz

import (
	"cmp"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/mdopp/homes/internal/gesichter"
	"github.com/mdopp/homes/internal/jsonfile"
)

const (
	tabelle = "immich-netz.json" // the phone books' contacts and relations, rewritten daily
	takt    = 24 * time.Hour
)

// Beziehung is the kind of a phone-book tie: a relation a phone wrote.
const Beziehung = "beziehung"

// kontakt is a contact of the phone books: its key (haushalt/<uid>,
// privat/<uid>), name and Immich person ("" none).
type kontakt struct {
	Key    string `json:"k"`
	Name   string `json:"n"`
	Person string `json:"p,omitempty"`
}

// Fest is a phone-book relation between two contacts' keys: A's card
// says B is A's Wie.
type Fest struct {
	A   string `json:"a"`
	B   string `json:"b"`
	Art string `json:"art"`
	Wie string `json:"wie,omitempty"`
}

// stand is what the daily run read.
type stand struct {
	Geholt   time.Time `json:"geholt"`
	Kontakte []kontakt `json:"kontakte"`
	Fest     []Fest    `json:"fest"`
}

// Knoten is a person: an Immich person (ID its id) or a contact without
// one (ID its key); Kontakt its contact ("" none).
type Knoten struct {
	ID, Name, Kontakt string
}

// Netz is the relations the key's owner sees on a contact's page.
type Netz struct {
	G   *gesichter.Gesichter // its Immich persons, its contacts, whose key, HOMES_DATA
	Now func() time.Time

	mu        sync.Mutex
	geladen   bool
	st        stand
	antworten map[string]map[string]antwort // by login, by pair (paarKey): fragen.go
	knoten    map[string]Knoten
	von       map[string]string // node by contact key
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
	if b, err := os.ReadFile(filepath.Join(n.G.Dir, antwortNam)); err == nil {
		json.Unmarshal(b, &n.antworten)
	}
	if n.antworten == nil {
		n.antworten = map[string]map[string]antwort{}
	}
	n.bauen()
}

// Rechnen reads the phone books again when the last read is takt old (or
// there is none), keeps them and rebuilds; else it does nothing. Without
// the key nothing is read.
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
	st := stand{Geholt: n.now()}
	var err error
	st.Kontakte, st.Fest, err = n.telefonbuch(ctx)
	n.mu.Lock()
	defer n.mu.Unlock()
	if err != nil { // the phone books not read: theirs as before
		st.Kontakte, st.Fest = n.st.Kontakte, n.st.Fest
	}
	n.st = st
	n.bauen()
	return cmp.Or(jsonfile.Save(filepath.Join(n.G.Dir, tabelle), n.st), err)
}

// bauen rebuilds the nodes from gesichter's named Immich persons and st;
// n.mu is held.
func (n *Netz) bauen() {
	n.knoten, n.von = map[string]Knoten{}, map[string]string{}
	for id, name := range n.G.Namen() {
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
