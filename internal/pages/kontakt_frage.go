package pages

import (
	"cmp"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"log"
	"maps"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mdopp/homes/internal/caldav"
	"github.com/mdopp/homes/internal/kontaktantwort"
	kontaktnetz "github.com/mdopp/homes/internal/netz"
	"github.com/mdopp/homes/internal/texte"
)

// KontaktFrage is the one page of Homes without a login (owner
// 2026-11-07): the link a person sent one of their contacts,
// https://kontakt.<domain>/<token>. homes.<domain> stays wholly behind
// Authelia; this page has a host of its own (Laib's homes route
// `kontakt`, auth none, Caddy limiting the rate and the body) and a
// listener of its own (HOMES_KONTAKT_LISTEN) on which Homes registers
// nothing but this page (KontaktServer): a routing mistake on the public
// host reaches no other Homes route. It shows that contact's own fields
// and nothing of the house, takes one answer and is dead then, or after
// 14 days. A token that is no link, one answered, one over and one never
// made all get the same page; requests are limited per address and in
// all here too; the answer goes into kontaktantwort's store, never into
// an address book. No secret and no Remote-User is read here.

// KontaktServer is everything the public listener serves: the page at
// /<token>, and a robots.txt and a favicon that say nothing. With a
// TeaserHost (owner 2026-11-08) the listener tells its two hosts apart:
// that Host gets the teaser, the kontakt host these pages, any other 404.
func KontaktServer(cfg StartConfig) http.Handler {
	page := KontaktFrage(cfg)
	teaser := Teaser(cfg)
	kontakt := ""
	if u, err := url.Parse(cfg.KontaktURL); err == nil {
		kontakt = u.Hostname()
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if cfg.TeaserHost != "" {
			switch host := hostOf(r); {
			case strings.EqualFold(host, cfg.TeaserHost):
				teaser(w, r)
				return
			case !strings.EqualFold(host, kontakt):
				http.Error(w, saysNoSuchPage, http.StatusNotFound)
				return
			}
		}
		switch r.URL.Path {
		case "/robots.txt":
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.Write([]byte("User-agent: *\nDisallow: /\n"))
		case "/favicon.ico":
			w.WriteHeader(http.StatusNoContent)
		default:
			page(w, r)
		}
	})
}

// The limits: per client address a burst of fragePro and one more every
// frageTakt; for everyone together fragAlle, one more a second.
const (
	fragePro   = 10
	frageTakt  = 30 * time.Second
	frageAlle  = 120
	frageBytes = 32 << 10 // a form of 12 numbers, mails and addresses is a few KB
)

// drossel is a token bucket per address, and one for all.
type drossel struct {
	mu   sync.Mutex
	je   map[string]*eimer
	alle eimer
	now  func() time.Time
}

type eimer struct {
	voll float64
	am   time.Time
}

func (e *eimer) nimm(now time.Time, cap float64, takt time.Duration) bool {
	if e.am.IsZero() {
		e.voll = cap
	}
	e.voll = min(cap, e.voll+now.Sub(e.am).Seconds()/takt.Seconds())
	e.am = now
	if e.voll < 1 {
		return false
	}
	e.voll--
	return true
}

func (d *drossel) darf(ip string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	now := d.now()
	if len(d.je) > 10000 { // an address that waited long enough is full again: forget it
		for k, e := range d.je {
			if now.Sub(e.am) > fragePro*frageTakt {
				delete(d.je, k)
			}
		}
	}
	e := d.je[ip]
	if e == nil {
		e = &eimer{}
		d.je[ip] = e
	}
	return e.nimm(now, fragePro, frageTakt) && d.alle.nimm(now, frageAlle, time.Second)
}

// hostOf is the request's Host without a port.
func hostOf(r *http.Request) string {
	if h, _, err := net.SplitHostPort(r.Host); err == nil {
		return h
	}
	return r.Host
}

// clientIP is the address Caddy saw (it sets X-Forwarded-For to it, the
// client's own value dropped), else the connection's.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[len(parts)-1])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// fremd is a POST another site sent: the browser says so, or the Origin
// is not this host. The token in the address is the capability; this
// keeps a page elsewhere from posting with it.
func fremd(r *http.Request) bool {
	if site := r.Header.Get("Sec-Fetch-Site"); site != "" {
		return site != "same-origin"
	}
	if o := r.Header.Get("Origin"); o != "" {
		u, err := url.Parse(o)
		return err != nil || u.Host != r.Host
	}
	return false
}

// KontaktFrage is /<token>: GET the form, POST the answer.
func KontaktFrage(cfg StartConfig) http.HandlerFunc {
	d := &drossel{je: map[string]*eimer{}, now: time.Now}
	if cfg.Now != nil {
		d.now = cfg.Now
	}
	return func(w http.ResponseWriter, r *http.Request) {
		nonce := frageNonce()
		h := w.Header()
		h.Set("Cache-Control", "no-store")
		h.Set("Referrer-Policy", "no-referrer") // the token is in the address
		h.Set("X-Robots-Tag", "noindex, nofollow")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; script-src 'nonce-"+nonce+"'; img-src blob:; form-action 'self'; frame-ancestors 'none'; base-uri 'none'")
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodPost {
			h.Set("Allow", "GET, HEAD, POST")
			frageSeite(w, http.StatusMethodNotAllowed, nonce, wort(texte.FrageUngueltig), "<p>"+wort(texte.FrageUngueltigText)+"</p>")
			return
		}
		if !d.darf(clientIP(r)) {
			h.Set("Retry-After", "60")
			frageSeite(w, http.StatusTooManyRequests, nonce, wort(texte.FrageZuViele), "<p>"+wort(texte.FrageZuVieleText)+"</p>")
			return
		}
		token := strings.TrimPrefix(r.URL.Path, "/")
		ungueltig := func() {
			frageSeite(w, http.StatusNotFound, nonce, wort(texte.FrageUngueltig), "<p>"+wort(texte.FrageUngueltigText)+"</p>")
		}
		if cfg.Antworten == nil || r.URL.RawQuery != "" {
			ungueltig()
			return
		}
		f, err := cfg.Antworten.Zum(token)
		if err != nil {
			ungueltig()
			return
		}
		form := func(st frageStand, problem string) string { return frageForm(token, f, st, problem) }
		if r.Method != http.MethodPost {
			frageSeite(w, http.StatusOK, nonce, wort(texte.FrageTitel), form(frageStand{f: kontaktantwort.Sauber(f.Vorher)}, ""))
			return
		}
		if fremd(r) {
			ungueltig()
			return
		}
		st, nachher, code, problem := frageGeprueft(w, r, f.Vorher, cfg.PhoneCountry, d.now())
		if code != 0 {
			frageSeite(w, code, nonce, wort(texte.FrageTitel), form(st, problem))
			return
		}
		as := kontaktantwort.Vergleich(f.Vorher, nachher)
		var hinweise []string
		fetch(r, func(ctx context.Context) {
			hinweise = kontaktantwort.Pruefung{Land: cfg.PhoneCountry, DNS: cfg.DNS, Now: time.Now()}.Pruefen(ctx, f.Vorher, nachher, as)
		})
		_, err = cfg.Antworten.Beantworten(token, kontaktantwort.Antwort{Bestaetigt: len(as) == 0, Aenderungen: as, Hinweise: hinweise})
		switch {
		case errors.Is(err, kontaktantwort.ErrWeg):
			ungueltig()
			return
		case err != nil:
			log.Printf("homes: kontakt-frage %s: not kept: %v", f.ID, err)
			frageSeite(w, http.StatusOK, nonce, wort(texte.FrageTitel), form(st, texte.T(texte.FrageNicht)))
			return
		}
		log.Printf("homes: kontakt-frage %s answered: %d changes", f.ID, len(as))
		frageSeite(w, http.StatusOK, nonce, wort(texte.FrageDanke), "<p>"+wort(texte.FrageDankeText)+"</p>")
	}
}

func frageNonce() string {
	b := make([]byte, 16)
	rand.Read(b)
	return base64.RawStdEncoding.EncodeToString(b)
}

// frageLesen reads the answer into r.PostForm: a plain form, or with the
// photo a multipart one, read in memory (nothing to disk), its text at
// most frageBytes and its file at most kontaktantwort.FotoBytes; the file,
// and what was too much ("" nothing).
func frageLesen(w http.ResponseWriter, r *http.Request) ([]byte, texte.ID) {
	mr, err := r.MultipartReader()
	if err != nil {
		r.Body = http.MaxBytesReader(w, r.Body, frageBytes)
		if r.ParseForm() != nil {
			return nil, texte.FrageZuGross
		}
		return nil, ""
	}
	r.Body = http.MaxBytesReader(w, r.Body, frageBytes+kontaktantwort.FotoBytes+16<<10)
	vals, rest := url.Values{}, int64(frageBytes)
	var foto []byte
	for {
		p, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, texte.FrageZuGross
		}
		if p.FormName() == "foto" {
			b, err := io.ReadAll(io.LimitReader(p, kontaktantwort.FotoBytes+1))
			if err != nil || len(b) > kontaktantwort.FotoBytes {
				return nil, texte.FrageFotoGross
			}
			if len(b) > 0 && foto == nil {
				foto = b
			}
			continue
		}
		b, err := io.ReadAll(io.LimitReader(p, rest+1))
		if rest -= int64(len(b)); err != nil || rest < 0 {
			return nil, texte.FrageZuGross
		}
		vals.Add(p.FormName(), string(b))
	}
	r.PostForm, r.Form = vals, vals
	return foto, ""
}

// frageGeprueft reads the answer and holds it to its form: the answer, or
// the form again with what is wrong where (its code and problem).
func frageGeprueft(w http.ResponseWriter, r *http.Request, vorher kontaktantwort.Felder, land string, now time.Time) (frageStand, kontaktantwort.Felder, int, string) {
	foto, zuviel := frageLesen(w, r)
	if zuviel != "" {
		st := frageStand{f: kontaktantwort.Sauber(vorher)}
		if zuviel == texte.FrageFotoGross {
			st.fehler = map[string]string{"foto": texte.T(zuviel)}
		}
		return st, kontaktantwort.Felder{}, http.StatusRequestEntityTooLarge, texte.T(zuviel)
	}
	if k := r.PostForm.Get("loeschen"); k != "" { // a trash can without the script: the form again, the row marked
		r.PostForm["weg"] = append(r.PostForm["weg"], k)
	}
	if k := r.PostForm.Get("zurueck"); k != "" {
		r.PostForm["weg"] = slices.DeleteFunc(r.PostForm["weg"], func(w string) bool { return w == k })
	}
	st, nachher := frageNachher(r, vorher, now)
	if r.PostForm.Get("loeschen")+r.PostForm.Get("zurueck") != "" {
		return st, nachher, http.StatusOK, ""
	}
	var err error
	if foto != nil {
		if nachher.Foto, err = kontaktantwort.Foto(foto); err != nil {
			st.fehler["foto"] = texte.T(texte.FrageFotoKeins)
		}
	}
	maps.Copy(st.fehler, kontaktantwort.Formfehler(vorher, nachher, land))
	if len(st.fehler) == 0 {
		return st, nachher, 0, ""
	}
	if nachher.Foto != nil {
		st.fehler["foto"] = texte.T(texte.FrageFotoNochmal) // a file field never comes back filled
	}
	return st, nachher, http.StatusOK, texte.T(texte.FrageFehler)
}

// frageStand is the form as it is shown: the fields, the card's entries
// ticked to go („tel.0“, „mail.1“, „adr.0“), and what is wrong where.
type frageStand struct {
	f      kontaktantwort.Felder
	weg    map[string]bool
	fehler map[string]string
}

// frageNachher is the form as it is shown again and as the answer: the
// fields in the places they were shown, a ticked „entfernen“ that place
// empty; the birthday's error when it is no day.
func frageNachher(r *http.Request, vorher kontaktantwort.Felder, now time.Time) (frageStand, kontaktantwort.Felder) {
	pf := r.PostForm
	f := kontaktantwort.Felder{Vorname: pf.Get("vorname"), Nachname: pf.Get("nachname"), Firma: pf.Get("firma"), Position: pf.Get("position"), Nachricht: pf.Get("nachricht")}
	at := func(vs []string, i int) string {
		if i < len(vs) {
			return vs[i]
		}
		return ""
	}
	tel, art, haupt := pf["tel"], pf["tel-art"], pf.Get("tel-haupt")
	for i := range min(len(tel), kontaktantwort.GrenzeJeArt) {
		f.Telefone = append(f.Telefone, kontaktantwort.Telefon{Nummer: tel[i], Art: at(art, i), Haupt: haupt == strconv.Itoa(i)})
	}
	mail, art, haupt := pf["mail"], pf["mail-art"], pf.Get("mail-haupt")
	for i := range min(len(mail), kontaktantwort.GrenzeJeArt) {
		f.Mails = append(f.Mails, kontaktantwort.Mail{Adresse: mail[i], Art: at(art, i), Haupt: haupt == strconv.Itoa(i)})
	}
	str := pf["adr-strasse"]
	for i := range min(len(str), kontaktantwort.GrenzeJeArt) {
		f.Adressen = append(f.Adressen, kontaktantwort.Adresse{Strasse: str[i], Hausnummer: at(pf["adr-nr"], i), PLZ: at(pf["adr-plz"], i), Ort: at(pf["adr-ort"], i), Land: at(pf["adr-land"], i)})
	}
	bez := pf["bez-art"]
	for i, name := range pf["bez-name"][:min(len(pf["bez-name"]), kontaktantwort.GrenzeJeArt)] { // owner 2026-11-09, a kind of the net's or none
		f.Beziehungen = append(f.Beziehungen, kontaktantwort.Beziehung{Name: name, Art: map[bool]string{true: at(bez, i)}[slices.Contains(kontaktnetz.Arten, at(bez, i))]})
	}
	f = kontaktantwort.Sauber(f)
	st := frageStand{f: f, weg: map[string]bool{}, fehler: map[string]string{}}
	for i := range f.Beziehungen { // one shown keeps its name; a new one needs its kind
		if i < len(vorher.Beziehungen) {
			f.Beziehungen[i].Name = vorher.Beziehungen[i].Name
		} else if f.Beziehungen[i].Name != "" && f.Beziehungen[i].Art == "" {
			st.fehler["bez."+strconv.Itoa(i)] = texte.T(texte.FrageBezOhneArt)
		}
	}
	geb, ok := tag(pf.Get("geb-tag"), pf.Get("geb-monat"), pf.Get("geb-jahr"))
	st.f.Geburtstag, f.Geburtstag = geb, geb
	if !ok {
		st.fehler["geb"] = texte.T(texte.FrageDatum)
	} else if h := kontaktantwort.Geburtstag(geb, now); geb != "" && geb != vorher.Geburtstag && h != "" {
		st.fehler["geb"] = h
	}
	f.Telefone, f.Mails, f.Adressen, f.Beziehungen = slices.Clone(f.Telefone), slices.Clone(f.Mails), slices.Clone(f.Adressen), slices.Clone(f.Beziehungen)
	for _, w := range pf["weg"] {
		was, nr, _ := strings.Cut(w, ".")
		i, err := strconv.Atoi(nr)
		switch {
		case err != nil:
		case was == "tel" && i < min(len(f.Telefone), len(vorher.Telefone)):
			f.Telefone[i] = kontaktantwort.Telefon{}
		case was == "mail" && i < min(len(f.Mails), len(vorher.Mails)):
			f.Mails[i] = kontaktantwort.Mail{}
		case was == "adr" && i < min(len(f.Adressen), len(vorher.Adressen)):
			f.Adressen[i] = kontaktantwort.Adresse{}
		case was == "bez" && i < min(len(f.Beziehungen), len(vorher.Beziehungen)):
			f.Beziehungen[i] = kontaktantwort.Beziehung{}
		default:
			continue
		}
		st.weg[w] = true
	}
	return st, f
}

// tag is day, month and the year if given as JJJJ-MM-TT or --MM-TT; all
// three empty is none; false when they are no day.
func tag(t, m, j string) (string, bool) {
	t, m, j = strings.TrimSpace(t), strings.TrimSpace(m), strings.TrimSpace(j)
	if t == "" && m == "" && j == "" {
		return "", true
	}
	ti, err1 := strconv.Atoi(t)
	mi, err2 := strconv.Atoi(m)
	s := fmt.Sprintf("--%02d-%02d", mi, ti)
	if j != "" {
		ji, err := strconv.Atoi(j)
		if err != nil || len(j) != 4 {
			return "", false
		}
		s = fmt.Sprintf("%04d-%02d-%02d", ji, mi, ti)
	}
	return s, err1 == nil && err2 == nil && caldav.ValidBirthday(s)
}

// tagTeile is a stored birthday as the form's day, month and year.
func tagTeile(s string) (t, m, j string) {
	y, md, ok := strings.Cut(strings.TrimPrefix(s, "--"), "-")
	if strings.HasPrefix(s, "--") {
		md, y, ok = s[2:], "", true
	}
	mo, d, ok2 := strings.Cut(md, "-")
	if !ok || !ok2 {
		return "", "", ""
	}
	return strings.TrimLeft(d, "0"), strings.TrimLeft(mo, "0"), y
}

// kindToken is a kind's autofill hint (owner 2026-11-08: the browser fills
// the contact's own number and address where it can).
var kindToken = map[string]string{"mobil": "mobile ", "privat": "home ", "geschaeftlich": "work "}

// frageForm is the form (owner 2026-11-07 and 2026-11-08): first and last
// name, numbers and e-mail addresses with their kind and the main one,
// addresses with street and number apart, birthday (the month a German
// word, the year optional), company and position, their photo and a
// message; each of the card's numbers, addresses and e-mail addresses may
// be ticked to go. Every field has its form as an HTML pattern and its
// error beside it; autofill names each field.
func frageForm(token string, fr kontaktantwort.Frage, st frageStand, problem string) string {
	var b strings.Builder
	f, alt := st.f, fr.Vorher
	ro := "" // a row ticked to go: its fields stay as they were, not checked
	in := func(name, key, label, value, extra string) {
		b.WriteString(`<label>` + html.EscapeString(label) + `<input name=` + name + ` value="` + html.EscapeString(value) + `" ` + ro + eigenesMuster(extra, value))
		if msg := st.fehler[key]; msg != "" {
			b.WriteString(` aria-invalid=true aria-describedby=f-` + key + `><span class=fehler id=f-` + key + `>` + html.EscapeString(msg) + `</span></label>`)
			return
		}
		b.WriteString(`></label>`)
	}
	muster := func(p string, id texte.ID) string {
		return `pattern="` + html.EscapeString(p) + `" data-fehler="` + wort(id) + `"`
	}
	wahl := func(name, value, label string, an bool) {
		b.WriteString(`<label class=wahl><input type=radio name=` + name + ` value=` + value + map[bool]string{true: " checked"}[an] + `>` + html.EscapeString(label) + `</label>`)
	}
	// weg is a card's row's trash can (owner 2026-11-08): the script greys the row and offers
	// Rückgängig; without it the button sends the form back with the row marked.
	weg := func(key string, label texte.ID) {
		aus := st.weg[key]
		if aus {
			b.WriteString(`<input type=hidden name=weg value=` + key + `>`)
		}
		b.WriteString(`<button type=submit name=loeschen value=` + key + ` class=papierkorb formnovalidate aria-label="` + wort(label) + `" title="` + wort(label) + `"` +
			map[bool]string{true: " hidden"}[aus] + `>` + papierkorbIcon + `</button><button type=submit name=zurueck value=` + key + ` class=zurueck formnovalidate` +
			map[bool]string{false: " hidden"}[aus] + `>` + wort(texte.FrageRueckgaengig) + `</button>`)
	}
	reihe := func(key string) {
		ro = map[bool]string{true: "readonly "}[st.weg[key]]
		b.WriteString(`<div class="eintrag` + map[bool]string{true: " geloescht"}[st.weg[key]] + `">`)
	}
	auswahl := func(name, cur string, arten []string) {
		b.WriteString(`<label>` + wort(texte.FrageNummerArt) + `<select name=` + name + `>`)
		for _, a := range arten {
			b.WriteString(`<option value=` + a + map[bool]string{true: " selected"}[a == cur] + `>` + wort(nummerArt[a]) + `</option>`)
		}
		b.WriteString(`</select></label>`)
	}
	auf := func(legend string) { b.WriteString(`<fieldset><legend>` + html.EscapeString(legend) + `</legend>`) }
	zu := func() { b.WriteString(`</fieldset>`) }
	b.WriteString(`<p>` + wort(texte.FrageHinweis) + `</p>` + gesagt("", problem))
	b.WriteString(`<form method=post enctype=multipart/form-data action="/` + html.EscapeString(token) + `" data-plz="` + html.EscapeString(plzMuster()) + `">`)
	auf(texte.T(texte.ArtName))
	in("vorname", "vorname", texte.T(texte.ArtVorname), f.Vorname, `maxlength=200 autocomplete=given-name `+muster(kontaktantwort.MusterName, texte.FrageNameForm))
	in("nachname", "nachname", texte.T(texte.ArtNachname), f.Nachname, `maxlength=200 autocomplete=family-name `+muster(kontaktantwort.MusterName, texte.FrageNameForm))
	zu()
	auf(texte.T(texte.FrageNummern))
	for i, p := range liste(f.Telefone, len(alt.Telefone), func(t kontaktantwort.Telefon) bool { return t.Nummer == "" }) {
		n, label := strconv.Itoa(i), texte.T(texte.ArtTelefon)
		if i >= len(alt.Telefone) {
			label = texte.T(texte.FrageWeitereNummer)
		}
		reihe("tel." + n)
		b.WriteString(`<div class=nummer>`)
		in("tel", "tel."+n, label, p.Nummer, `type=tel inputmode=tel maxlength=40 autocomplete="`+kindToken[cmp.Or(p.Art, "mobil")]+`tel" `+muster(kontaktantwort.MusterNummer, texte.FrageNummerForm))
		auswahl("tel-art", cmp.Or(p.Art, "mobil"), kontaktantwort.Arten)
		b.WriteString(`</div><div class=wahlen>`)
		wahl("tel-haupt", n, texte.T(texte.FrageHauptNummer), p.Haupt)
		if i < len(alt.Telefone) {
			weg("tel."+n, texte.FrageNummerLoeschen)
		}
		b.WriteString(`</div></div>`)
		ro = ""
	}
	zu()
	auf(texte.T(texte.ArtMail))
	for i, m := range liste(f.Mails, len(alt.Mails), func(m kontaktantwort.Mail) bool { return m.Adresse == "" }) {
		n, label := strconv.Itoa(i), texte.T(texte.ArtMail)
		if i >= len(alt.Mails) {
			label = texte.T(texte.FrageWeitereMail)
		}
		reihe("mail." + n)
		b.WriteString(`<div class=nummer>`)
		in("mail", "mail."+n, label, m.Adresse, `type=email inputmode=email maxlength=200 autocomplete="`+kindToken[cmp.Or(m.Art, "privat")]+`email" data-fehler="`+wort(texte.FrageMailForm)+`"`)
		auswahl("mail-art", cmp.Or(m.Art, "privat"), kontaktantwort.MailArten)
		b.WriteString(`</div><div class=wahlen>`)
		wahl("mail-haupt", n, texte.T(texte.FrageHauptMail), m.Haupt)
		if i < len(alt.Mails) {
			weg("mail."+n, texte.FrageMailLoeschen)
		}
		b.WriteString(`</div></div>`)
		ro = ""
	}
	zu()
	for i, a := range liste(f.Adressen, len(alt.Adressen), kontaktantwort.Adresse.Leer) {
		n, sec := strconv.Itoa(i), `autocomplete="section-adr`+strconv.Itoa(i)+` `
		legend := texte.T(texte.ArtAdresse)
		if i >= len(alt.Adressen) && i > 0 {
			legend = texte.T(texte.FrageWeitereAdresse)
		}
		auf(legend)
		reihe("adr." + n)
		b.WriteString(`<div class=strasse>`)
		in("adr-strasse", "adr-strasse."+n, texte.T(texte.FrageStrasse), a.Strasse, `maxlength=200 `+sec+`address-line1" `+muster(kontaktantwort.MusterStrasse, texte.FrageStrasseForm))
		in("adr-nr", "adr-nr."+n, texte.T(texte.FrageHausnummer), a.Hausnummer, `maxlength=10 `+muster(kontaktantwort.MusterHausnummer, texte.FrageHausnummerForm))
		b.WriteString(`</div><div class=zeile>`)
		plz, mode := kontaktantwort.PLZSonst, ""
		if l, ok := kontaktantwort.LandVon(a.Land); ok {
			plz = l.PLZ
		}
		if plz == `[0-9]{5}` || plz == `[0-9]{4}` {
			mode = "inputmode=numeric "
		}
		in("adr-plz", "adr-plz."+n, texte.T(texte.FragePLZ), a.PLZ, `maxlength=10 `+mode+sec+`postal-code" `+muster(plz, texte.FragePLZForm))
		in("adr-ort", "adr-ort."+n, texte.T(texte.FrageOrt), a.Ort, `maxlength=200 `+sec+`address-level2" `+muster(kontaktantwort.MusterOrt, texte.FrageOrtForm))
		b.WriteString(`</div>`)
		in("adr-land", "adr-land."+n, texte.T(texte.FrageLand), a.Land, `maxlength=60 placeholder=Deutschland `+sec+`country-name" `+muster(kontaktantwort.MusterOrt, texte.FrageLandForm))
		if i < len(alt.Adressen) {
			b.WriteString(`<div class=wahlen>`)
			weg("adr."+n, texte.FrageAdresseLoeschen)
			b.WriteString(`</div>`)
		}
		b.WriteString(`</div>`)
		ro = ""
		zu()
	}
	auf(texte.T(texte.ArtGeburtstag))
	t, m, j := tagTeile(f.Geburtstag)
	b.WriteString(`<div class=tag>`)
	in("geb-tag", "geb", texte.T(texte.FrageTag), t, `inputmode=numeric maxlength=2 autocomplete=bday-day `+muster(kontaktantwort.MusterTag, texte.FrageTagForm))
	b.WriteString(`<label>` + wort(texte.FrageMonat) + `<select name=geb-monat autocomplete=bday-month><option value="">–</option>`)
	for k, name := range months {
		v := strconv.Itoa(k + 1)
		b.WriteString(`<option value=` + v + map[bool]string{true: " selected"}[v == m] + `>` + name + `</option>`)
	}
	b.WriteString(`</select></label>`)
	in("geb-jahr", "geb-jahr", texte.T(texte.FrageJahr), j, `inputmode=numeric maxlength=4 autocomplete=bday-year `+muster(kontaktantwort.MusterJahr, texte.FrageJahrForm))
	b.WriteString(`</div>`)
	zu()
	auf(texte.T(texte.FrageBeruf))
	in("firma", "firma", texte.T(texte.ArtFirma), f.Firma, "maxlength=200 autocomplete=organization")
	in("position", "position", texte.T(texte.ArtPosition), f.Position, "maxlength=200 autocomplete=organization-title")
	zu()
	if alt.Netz { // the relations recorded for them (owner 2026-11-09), from their view: „Hilde ist für dich: Mutter/Vater“
		frageBeziehungen(&b, f, alt, st, in, reihe, weg)
		ro = ""
	}
	auf(texte.T(texte.FrageFoto))
	in("foto", "foto", texte.T(texte.FrageFotoWahl), "", `type=file accept="image/jpeg,image/png,image/heic,image/heif,image/webp" data-fehler="`+wort(texte.FrageFotoKeins)+`"`)
	b.WriteString(`<img id=foto-bild alt="" hidden>`)
	zu()
	auf(texte.T(texte.FrageNachricht))
	b.WriteString(`<label>` + html.EscapeString(fmt.Sprintf(texte.T(texte.FrageNachrichtAn), cmp.Or(fr.Von, texte.T(texte.FrageDir)))) +
		`<textarea name=nachricht rows=4 maxlength=` + strconv.Itoa(kontaktantwort.GrenzeNachricht) + `>` + html.EscapeString(f.Nachricht) + `</textarea></label>`)
	zu()
	b.WriteString(`<p class=knoepfe><button name=aktion value=senden data-laeuft="` + wort(texte.FrageSendet) + `">` + wort(texte.FrageSenden) + `</button>` +
		`<button name=aktion value=stimmt class=zweit data-laeuft="` + wort(texte.FrageSendet) + `">` + wort(texte.FrageStimmt) + `</button></p></form>`)
	b.WriteString(`<p class=k>` + html.EscapeString(fmt.Sprintf(texte.T(texte.FrageBis), fr.Bis.Local().Format("02.01.2006"))) + `</p>`)
	return b.String()
}

// frageBeziehungen is the form's „Beziehungen“: each relation recorded with
// its kind to correct and its trash can, and one more by name and kind.
func frageBeziehungen(b *strings.Builder, f, alt kontaktantwort.Felder, st frageStand, in func(name, key, label, value, extra string), reihe func(string), weg func(string, texte.ID)) {
	b.WriteString(`<fieldset><legend>` + wort(texte.FrageBeziehungen) + `</legend><p class=k>` + wort(texte.FrageBezHinweis) + `</p>`)
	for i, z := range liste(f.Beziehungen, len(alt.Beziehungen), func(z kontaktantwort.Beziehung) bool { return z.Name == "" }) {
		n, label := strconv.Itoa(i), texte.T(texte.FrageBezArt)
		reihe("bez." + n)
		if i < len(alt.Beziehungen) {
			label = fmt.Sprintf(texte.T(texte.FrageBezIst), alt.Beziehungen[i].Name)
			b.WriteString(`<input type=hidden name=bez-name value="` + html.EscapeString(alt.Beziehungen[i].Name) + `">`)
		} else {
			in("bez-name", "bez-name."+n, texte.T(texte.FrageBezName), z.Name, "maxlength=200")
		}
		b.WriteString(`<label>` + html.EscapeString(label) + `<select name=bez-art><option value="">` + wort(texte.NetzWaehlen) + `</option>`)
		for _, a := range kontaktnetz.Arten {
			b.WriteString(`<option value=` + a + map[bool]string{true: " selected"}[a == z.Art] + `>` + html.EscapeString(kontaktnetz.Wort(a)) + `</option>`)
		}
		b.WriteString(`</select>`)
		if msg := st.fehler["bez."+n]; msg != "" {
			b.WriteString(`<span class=fehler>` + html.EscapeString(msg) + `</span>`)
		}
		b.WriteString(`</label>`)
		if i < len(alt.Beziehungen) {
			b.WriteString(`<div class=wahlen>`)
			weg("bez."+n, texte.FrageBezLoeschen)
			b.WriteString(`</div>`)
		}
		b.WriteString(`</div>`)
	}
	b.WriteString(`</fieldset>`)
}

// liste is the rows of a list as the form shows them: the card's n, the
// new ones the contact filled, and one empty to add.
func liste[T any](vs []T, n int, leer func(T) bool) []T {
	var out []T
	for i, v := range vs {
		if i < n || !leer(v) {
			out = append(out, v)
		}
	}
	var zero T
	return append(out, zero)
}

// plzMuster is each country's postal code pattern by its names ("" Germany, "*" any
// other), for the script that sets the PLZ field's pattern when the country changes.
var plzMuster = sync.OnceValue(func() string {
	m := map[string]string{"*": kontaktantwort.PLZSonst}
	for _, l := range kontaktantwort.Laender {
		for _, n := range l.Namen {
			m[n] = l.PLZ
		}
	}
	b, _ := json.Marshal(m)
	return string(b)
})

// frageScript is the page's own: the pressed button says „Wird gesendet …“;
// a field's error is said beside it in German as soon as it is left (the
// pattern's data-fehler), and on sending the first wrong one is focused;
// the country sets the postal code's pattern; a street with its number
// (autofill) is split; the photo is made a JPEG of at most 1024 pixels in
// the browser (HEIC and WebP too, where the browser opens them) and shown.
const frageScript = `(function(){var f=document.querySelector('form');
document.addEventListener('submit',function(e){var b=e.submitter;if(!b||!b.hasAttribute('data-laeuft'))return;setTimeout(function(){b.textContent=b.getAttribute('data-laeuft');document.querySelectorAll('button').forEach(function(x){x.disabled=true})},0)});
if(!f)return;
function zeig(el){if(!el.willValidate)return;var l=el.closest('label'),s=l.querySelector('.fehler'),ok=el.validity.valid;if(ok&&!s)return;
if(!s){s=document.createElement('span');s.className='fehler';s.id='f-'+Math.random().toString(36).slice(2);l.appendChild(s);el.setAttribute('aria-describedby',s.id)}
s.textContent=ok?'':(el.getAttribute('data-fehler')||el.validationMessage);s.hidden=ok;el.setAttribute('aria-invalid',ok?'false':'true')}
var erst=0;f.addEventListener('invalid',function(e){e.preventDefault();zeig(e.target);if(!erst){var t=erst=e.target;setTimeout(function(){t.focus();erst=0},0)}},true);
f.addEventListener('input',function(e){var el=e.target;if(el.getAttribute('aria-invalid')==='true')zeig(el);
if(el.name==='adr-land'){var m=JSON.parse(f.getAttribute('data-plz'));el.closest('fieldset').querySelector('[name=adr-plz]').pattern=m[el.value.trim().toLowerCase().replace(/[.\s]+$/,'')]||m['*']}},true);
f.addEventListener('click',function(e){var k=e.target.closest('button[name=loeschen],button[name=zurueck]');if(!k)return;e.preventDefault();
var r=k.closest('.eintrag'),aus=k.name==='loeschen',h=r.querySelector('input[name=weg]');r.classList.toggle('geloescht',aus);
r.querySelectorAll('input:not([type=hidden]):not([type=radio])').forEach(function(x){x.readOnly=aus;zeig(x)});
if(aus&&!h){h=document.createElement('input');h.type='hidden';h.name='weg';h.value=k.value;r.appendChild(h)}if(!aus&&h)h.remove();
r.querySelector('button[name=loeschen]').hidden=aus;r.querySelector('button[name=zurueck]').hidden=!aus;r.querySelector(aus?'button[name=zurueck]':'button[name=loeschen]').focus()});
f.addEventListener('change',function(e){var el=e.target;if(el.name==='foto')return foto(el);
if(el.name==='adr-strasse'){var n=el.closest('fieldset').querySelector('[name=adr-nr]'),m=el.value.trim().match(/^(.*[^\d\s,])[\s,]+(\d+\s?[a-zA-Z]?(?:\s?[-\/]\s?\d+\s?[a-zA-Z]?)?)$/);if(m&&!n.value){el.value=m[1];n.value=m[2]}}zeig(el)},true);
function foto(el){var file=el.files&&el.files[0],img=document.getElementById('foto-bild'),bs=f.querySelectorAll('button');img.hidden=true;el.setCustomValidity('');zeig(el);
if(!file||!window.createImageBitmap||!window.DataTransfer)return;bs.forEach(function(b){b.disabled=true});
createImageBitmap(file).then(function(bm){var k=Math.min(1,1024/Math.max(bm.width,bm.height)),c=document.createElement('canvas'),x;c.width=Math.max(1,Math.round(bm.width*k));c.height=Math.max(1,Math.round(bm.height*k));
x=c.getContext('2d');x.fillStyle='#fff';x.fillRect(0,0,c.width,c.height);x.drawImage(bm,0,0,c.width,c.height);return new Promise(function(ok){c.toBlob(ok,'image/jpeg',0.9)})})
.then(function(blob){if(!blob)throw 0;var d=new DataTransfer();d.items.add(new File([blob],'foto.jpg',{type:'image/jpeg'}));el.files=d.files;img.src=URL.createObjectURL(blob);img.hidden=false})
.catch(function(){el.value='';el.setCustomValidity(el.getAttribute('data-fehler'));zeig(el);el.setCustomValidity('')})
.then(function(){bs.forEach(function(b){b.disabled=false})})}})()`

var musterAttr = regexp.MustCompile(`pattern="([^"]*)"`)

// eigenesMuster is extra without its pattern when value, the card's own, has not that form:
// Homes checks it only once changed, so the browser must not stop the contact over it.
func eigenesMuster(extra, value string) string {
	m := musterAttr.FindStringSubmatch(extra)
	if m == nil || value == "" {
		return extra
	}
	if ok, err := regexp.MatchString(`^(?:`+html.UnescapeString(m[1])+`)$`, value); ok || err != nil {
		return extra
	}
	return strings.Replace(extra, m[0], "", 1)
}

// papierkorbIcon is the trash can of a row the contact may take away.
const papierkorbIcon = `<svg viewBox="0 0 24 24" aria-hidden=true><path d="M4 7h16M9 7V4h6v3M6 7l1 13h10l1-13M10 11v6M14 11v6"/></svg>`

// frageSeite is the page: its own calm frame, no menu, no login, nothing
// fetched from elsewhere (the pages' font and icons are behind the login).
func frageSeite(w http.ResponseWriter, code int, nonce, title, body string) {
	var b strings.Builder
	b.WriteString(`<!doctype html><html lang=de><meta charset=utf-8><meta name=viewport content="width=device-width,initial-scale=1"><meta name=color-scheme content="light dark">` +
		`<meta name=robots content="noindex,nofollow"><title>` + title + `</title>` + frageStyle + `<main><h1>` + title + `</h1>` + body + `</main>` +
		`<script nonce="` + nonce + `">` + frageScript + `</script>`)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(code)
	w.Write([]byte(b.String()))
}

const frageStyle = `<style>:root{color-scheme:light;--bg:#f6f7f9;--card:#fff;--ink:#111827;--muted:#5b6474;--line:#d9dee6;--accent:#c2560c;--warn:#a15c00;--bad:#b42318}
@media (prefers-color-scheme:dark){:root{color-scheme:dark;--bg:#0c0c0e;--card:#16161c;--ink:#f2f2f5;--muted:#a0a6b4;--line:#2a2a33;--accent:#f97316;--warn:#f59e0b;--bad:#fda29b}}
*{box-sizing:border-box}body{margin:0;background:var(--bg);color:var(--ink);font:17px/1.5 system-ui,-apple-system,"Segoe UI",Roboto,sans-serif}
main{max-width:32rem;margin:0 auto;padding:28px 16px 48px}h1{font-size:1.5rem;line-height:1.25;margin:0 0 12px}p{margin:0 0 14px}.k{color:var(--muted);font-size:.92rem}
fieldset{min-width:0;margin:0 0 14px;padding:14px;border:0;border-radius:16px;background:var(--card)}legend{float:left;width:100%;margin:0 0 8px;font-weight:600}fieldset>:not(legend){clear:both}.nummer>*,.tag>*,.zeile>*{min-width:0}
label{display:block;margin:0 0 10px;font-size:.88rem;color:var(--muted)}label:last-child{margin-bottom:0}
input,select,textarea{display:block;width:100%;margin-top:4px;padding:12px;font:inherit;color:var(--ink);background:var(--bg);border:1px solid var(--line);border-radius:12px}
input:focus,textarea:focus{outline:2px solid var(--accent);outline-offset:1px}.zeile{display:grid;grid-template-columns:7rem 1fr;gap:10px}.strasse{display:grid;grid-template-columns:1fr 6.5rem;gap:10px}.strasse>*{min-width:0}
[hidden]{display:none!important}.fehler{display:block;margin-top:4px;color:var(--bad);font-size:.88rem}[aria-invalid=true]{border-color:var(--bad)}
.wahlen{display:flex;flex-wrap:wrap;gap:6px 18px;margin:0 0 12px}label.wahl{display:flex;align-items:center;gap:8px;margin:0;color:var(--ink);font-size:.92rem}
label.wahl input{display:inline;width:20px;height:20px;margin:0;padding:0}
.wahlen{align-items:center}.papierkorb,.zurueck{margin-left:auto;min-height:44px;padding:0 12px;background:var(--card);color:var(--ink);border:1px solid var(--line)}.papierkorb{width:44px;padding:0}
.papierkorb svg{width:20px;height:20px;vertical-align:middle;fill:none;stroke:currentColor;stroke-width:2}.geloescht input:not([type=hidden]),.geloescht select,.geloescht label.wahl{opacity:.45;text-decoration:line-through;pointer-events:none}#foto-bild{display:block;width:120px;height:auto;margin-top:10px;border-radius:12px}
.nummer{display:grid;grid-template-columns:1fr 9rem;gap:10px}.tag{display:grid;grid-template-columns:1fr 1fr 1.4fr;gap:10px}
.knoepfe{display:flex;flex-direction:column;gap:10px;margin-top:20px}
button{min-height:48px;padding:0 18px;font:inherit;font-weight:600;border:0;border-radius:14px;background:var(--accent);color:#fff}button.zweit{background:var(--card);color:var(--ink);border:1px solid var(--line)}
button[disabled]{opacity:.6}.achtung{padding:12px 14px;border-radius:12px;border:1px solid var(--warn);color:var(--warn);margin:0 0 14px}.achtung p{margin:0}</style>`
