package pages

import (
	"cmp"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"html"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/mdopp/homes/internal/kontaktantwort"
	"github.com/mdopp/homes/internal/teilen"
	"github.com/mdopp/homes/internal/texte"
)

// Kontakt austauschen (owner 2026-11-28): the page the other phone opens
// when two phones touch or a QR code is shown, /t/<token> on the public
// kontakt listener, without a login. It names the sharer's first name from
// the start („Michael möchte etwas mit dir teilen …“) and follows what the
// sharer releases in the app, live (/t/<token>/live, Server-Sent Events;
// reloading shows the same): their card's chosen fields with a .vcf to
// save (/t/<token>/karte.vcf), the guest Wi-Fi with „Kopieren“, and for an
// admin „Und du?“, the form of Kontaktdaten eintragen lassen, filled from
// the guest's own contact (the Contact Picker) or a .vcf file. Ended or
// over, it says so; nothing released is served then. A card at the foot
// tells what Homes is.

// TeilenPrefix is the page's path on the kontakt listener.
const TeilenPrefix = "/t/"

// teilenPing keeps the live stream open through proxies.
var teilenPing = 25 * time.Second

// Teilen serves /t/<token>, /t/<token>/live and /t/<token>/karte.vcf.
func Teilen(cfg StartConfig) http.HandlerFunc {
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
		h.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; script-src 'nonce-"+nonce+"'; connect-src 'self'; img-src blob:; form-action 'self'; frame-ancestors 'none'; base-uri 'none'")
		token, was, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, TeilenPrefix), "/")
		ungueltig := func(code int) {
			frageSeite(w, code, nonce, wort(texte.FrageUngueltig), "<p>"+wort(texte.FrageUngueltigText)+"</p>")
		}
		switch {
		case r.Method != http.MethodGet && r.Method != http.MethodHead:
			h.Set("Allow", "GET, HEAD")
			ungueltig(http.StatusMethodNotAllowed)
			return
		case !d.darf(clientIP(r), false, len(token) == 43):
			h.Set("Retry-After", "60")
			frageSeite(w, http.StatusTooManyRequests, nonce, wort(texte.FrageZuViele), "<p>"+wort(texte.FrageZuVieleText)+"</p>")
			return
		case cfg.Teilen == nil || r.URL.RawQuery != "":
			ungueltig(http.StatusNotFound)
			return
		}
		a, _, err := cfg.Teilen.Zum(token, r.Method == http.MethodGet)
		switch {
		case err != nil:
			ungueltig(http.StatusNotFound)
		case was == "":
			teilenSeite(w, r, cfg, nonce, token, a)
		case was == "live":
			teilenLive(w, r, cfg, token)
		case was == "karte.vcf" && !a.Vorbei() && a.Karte != nil:
			h.Set("Content-Type", "text/vcard; charset=utf-8")
			h.Set("Content-Disposition", teilenDatei(a.Vorname))
			w.Write([]byte(a.Karte.VCard(a.Vorname)))
		default:
			ungueltig(http.StatusNotFound)
		}
	}
}

// teilenDatei is the .vcf's Content-Disposition: the sharer's first name as its file name, quoted or
// RFC 2231-encoded by mime (no value can end the header or add a parameter), else kontakt.vcf.
func teilenDatei(vorname string) string {
	name := strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || strings.ContainsRune(`/\"`, r) {
			return -1
		}
		return r
	}, strings.TrimSpace(vorname))
	if v := mime.FormatMediaType("attachment", map[string]string{"filename": cmp.Or(name, "kontakt") + ".vcf"}); v != "" {
		return v
	}
	return `attachment; filename="kontakt.vcf"`
}

// teilenTeile is the page's parts as the live stream sends them: each one's HTML, "" none.
type teilenTeile struct {
	Titel  string `json:"titel"`
	Warten string `json:"warten"`
	Karte  string `json:"karte"`
	WLAN   string `json:"wlan"`
	Du     string `json:"du"`
	DuH    string `json:"du_h"` // the form's hash: the page reloads when it changed, a typed form is never replaced under the guest
	Vorbei bool   `json:"vorbei"`
}

// teilenStand is a's parts for the guest.
func teilenStand(r *http.Request, cfg StartConfig, token string, a teilen.Ansicht) teilenTeile {
	sp := texte.Von(r)
	t := func(id texte.ID, args ...any) string { return html.EscapeString(fmt.Sprintf(sp.T(id), args...)) }
	var p teilenTeile
	switch {
	case a.Beendet:
		p.Titel, p.Warten, p.Vorbei = t(texte.TeilenBeendet, a.Vorname), "<p>"+t(texte.TeilenBeendetText)+"</p>", true
	case a.Abgelaufen:
		p.Titel, p.Warten, p.Vorbei = t(texte.TeilenAbgelaufen), "<p>"+t(texte.TeilenAbgelaufenText, a.Vorname)+"</p>", true
	default:
		p.Titel = t(texte.TeilenTitel, a.Vorname)
		if a.Wartet() {
			p.Warten = `<p class=wartet><span class=punkte aria-hidden=true><i></i><i></i><i></i></span>` + t(texte.TeilenWartet, a.Vorname) + `</p>`
		}
		p.Karte, p.WLAN, p.Du = teilenKarte(sp, token, a), teilenWLAN(sp, a.WLAN), teilenDu(r, cfg, a)
	}
	f := fnv.New32a()
	f.Write([]byte(p.Du))
	p.DuH = strconv.FormatUint(uint64(f.Sum32()), 36)
	return p
}

// genitiv is a first name's genitive as German writes it before a noun: „Michaels“, „Hans'“.
func genitiv(v string) string {
	if l := strings.ToLower(v); strings.HasSuffix(l, "s") || strings.HasSuffix(l, "ß") || strings.HasSuffix(l, "x") || strings.HasSuffix(l, "z") || strings.HasSuffix(l, "ce") {
		return v + "’"
	}
	return v + "s"
}

// teilenKarte is the released card: its fields as text, and the .vcf.
func teilenKarte(sp texte.Sprache, token string, a teilen.Ansicht) string {
	k := a.Karte
	if k == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("<h2>" + html.EscapeString(fmt.Sprintf(sp.T(texte.TeilenKarte), genitiv(a.Vorname))) + "</h2><dl class=felder>")
	t, m, j := tagTeile(k.Geburtstag)
	geb := ""
	if n, err := strconv.Atoi(m); err == nil && n >= 1 && n <= 12 {
		geb = strings.TrimSpace(t + ". " + months[n-1] + " " + j)
	}
	for _, f := range [][2]string{{sp.T(texte.ArtName), k.Name}, {sp.T(texte.TeilenMobil), k.Mobil}, {sp.T(texte.ArtTelefon), k.Telefon}, {sp.T(texte.ArtMail), k.Mail},
		{sp.T(texte.ArtAdresse), k.Adresse}, {sp.T(texte.ArtGeburtstag), geb}, {sp.T(texte.ArtFirma), k.Firma}} {
		if strings.TrimSpace(f[1]) != "" {
			b.WriteString("<dt>" + html.EscapeString(f[0]) + "</dt><dd>" + html.EscapeString(f[1]) + "</dd>")
		}
	}
	b.WriteString(`</dl><p class=knoepfe><a class=knopf href="` + TeilenPrefix + html.EscapeString(token) + `/karte.vcf" download>` + html.EscapeString(sp.T(texte.TeilenKarteKnopf)) +
		`</a></p><p class=k>` + html.EscapeString(sp.T(texte.TeilenKarteWie)) + `</p>`)
	return b.String()
}

// teilenWLAN is the guest Wi-Fi with its „Kopieren“ buttons (no QR: the guest is on this very phone).
func teilenWLAN(sp texte.Sprache, wl *teilen.WLAN) string {
	if wl == nil {
		return ""
	}
	zeile := func(label, wert string) string {
		return `<div class=kopie><span class=k>` + html.EscapeString(label) + `</span><code>` + html.EscapeString(wert) + `</code><button type=button class=zweit data-kopie="` +
			html.EscapeString(wert) + `" data-kopiert="` + html.EscapeString(sp.T(texte.TeilenKopiert)) + `" data-markiert="` + html.EscapeString(sp.T(texte.TeilenMarkiert)) + `">` +
			html.EscapeString(sp.T(texte.TeilenKopieren)) + `</button></div>`
	}
	out := "<h2>" + html.EscapeString(sp.T(texte.TeilenWLAN)) + "</h2>" + zeile(sp.T(texte.TeilenWLANName), wl.SSID)
	if wl.Offen || wl.Kennwort == "" {
		return out + `<p class=k>` + html.EscapeString(sp.T(texte.TeilenOffen)) + `</p>`
	}
	return out + zeile(sp.T(texte.TeilenKennwort), wl.Kennwort)
}

// teilenDu is „Und du?“: the form of Kontaktdaten eintragen lassen, or the thanks once it is answered.
func teilenDu(r *http.Request, cfg StartConfig, a teilen.Ansicht) string {
	if !a.Eintragen || a.Einladung.Token == "" || cfg.Antworten == nil {
		return ""
	}
	sp := texte.Von(r)
	out := "<h2>" + html.EscapeString(sp.T(texte.TeilenDu)) + "</h2>"
	f, err := cfg.Antworten.Zum(a.Einladung.Token)
	if err != nil { // answered (a link works once), or gone
		return out + `<p role=status>` + html.EscapeString(fmt.Sprintf(sp.T(texte.TeilenDuDanke), a.Vorname)) + `</p>`
	}
	return out + `<div class=quelle><button type=button class=zweit id=t-picker hidden>` + html.EscapeString(sp.T(texte.TeilenPicker)) + `</button>` +
		`<label class=datei>` + html.EscapeString(sp.T(texte.TeilenDatei)) + `<input type=file id=t-vcf accept=".vcf,text/vcard,text/x-vcard" data-keine="` + html.EscapeString(sp.T(texte.TeilenKeineKarte)) +
		`" data-gut="` + html.EscapeString(sp.T(texte.TeilenUebernommen)) + `"></label><p class=k>` + html.EscapeString(sp.T(texte.TeilenQuelleWie)) + `</p><p id=t-quelle role=status></p></div>` +
		frageForm(a.Einladung.Token, f, frageStand{f: kontaktantwort.Sauber(f.Vorher)}, "")
}

// teilenWerbung is the foot's one card about Homes: its public teaser where the box has one, else the project.
func teilenWerbung(sp texte.Sprache, cfg StartConfig) string {
	ziel := "https://github.com/mdopp/homes"
	if cfg.TeaserHost != "" {
		ziel = "https://" + cfg.TeaserHost + "/"
	}
	return `<aside class=werbung><p>` + html.EscapeString(sp.T(texte.TeilenWerbung)) + `</p><a href="` + html.EscapeString(ziel) + `" rel="noopener noreferrer">` + html.EscapeString(sp.T(texte.TeilenMehr)) + `</a></aside>`
}

// teilenSeite is the whole page, its parts as the live stream will replace them.
func teilenSeite(w http.ResponseWriter, r *http.Request, cfg StartConfig, nonce, token string, a teilen.Ansicht) {
	p := teilenStand(r, cfg, token, a)
	teil := func(id, inhalt string) string {
		return `<section id=t-` + id + ` class=teil` + map[bool]string{true: " hidden"}[inhalt == ""] + `>` + inhalt + `</section>`
	}
	var b strings.Builder
	b.WriteString(`<!doctype html><html lang=de><meta charset=utf-8><meta name=viewport content="width=device-width,initial-scale=1"><meta name=color-scheme content="light dark">` +
		`<meta name=robots content="noindex,nofollow"><title>` + p.Titel + `</title>` + frageStyle + teilenStyle + `<main data-live="` + TeilenPrefix + html.EscapeString(token) + `/live"><h1 id=t-titel>` + p.Titel + `</h1>` +
		`<div id=t-warten>` + p.Warten + `</div>` + teil("karte", p.Karte) + teil("wlan", p.WLAN) + `<section id=t-du class=teil data-h="` + p.DuH + `"` + map[bool]string{true: " hidden"}[p.Du == ""] + `>` + p.Du + `</section>` +
		teilenWerbung(texte.Von(r), cfg) + `</main><script nonce="` + nonce + `">` + frageScript + ";" + teilenScript + `</script>`)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(b.String()))
}

// teilenLive is the live stream: the parts now, again on every change and at the session's end,
// a comment every teilenPing between; it ends with the session.
func teilenLive(w http.ResponseWriter, r *http.Request, cfg StartConfig, token string) {
	rc := http.NewResponseController(w)
	rc.SetReadDeadline(time.Now().Add(teilen.Dauer + time.Minute)) // the listener's own 20 s would end the stream
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	now := time.Now
	if cfg.Now != nil {
		now = cfg.Now
	}
	ping := time.NewTicker(teilenPing)
	defer ping.Stop()
	send := func(s string) bool {
		rc.SetWriteDeadline(time.Now().Add(30 * time.Second))
		_, err := w.Write([]byte(s))
		return err == nil && rc.Flush() == nil
	}
	if !send("retry: 5000\n\n") {
		return
	}
	for {
		a, wechsel, err := cfg.Teilen.Zum(token, true)
		if err != nil {
			return
		}
		if !send(teilenFrame(teilenStand(r, cfg, token, a))) || a.Vorbei() {
			return
		}
		ende := time.NewTimer(max(a.Bis.Sub(now()), 0) + 50*time.Millisecond)
	warte:
		for {
			select {
			case <-r.Context().Done():
				ende.Stop()
				return
			case <-wechsel:
				break warte
			case <-ende.C:
				break warte
			case <-ping.C:
				if !send(": ping\n\n") {
					return
				}
			}
		}
		ende.Stop()
	}
}

// teilenFrame is one event of the live stream: the parts as one line of JSON (it escapes every
// newline, < > & and U+2028), so no value ends the frame or starts a field of its own.
func teilenFrame(p teilenTeile) string {
	b, _ := json.Marshal(p)
	return "data: " + string(b) + "\n\n"
}

// teilenScript follows the live stream (a part replaced in place; the form only by a reload, as
// its own script binds it at load; the stream closed once it is over), copies with „Kopieren“, and
// fills the form from the guest's own contact (navigator.contacts) or a .vcf file. No EventSource:
// the page reloads while it waits.
const teilenScript = `(function(){var m=document.querySelector('main[data-live]');if(!m)return;
function setz(id,v){var e=document.getElementById(id);if(!e)return;if(e.innerHTML!==v)e.innerHTML=v;if(e.tagName==='SECTION')e.hidden=!v}
if(window.EventSource){var es=new EventSource(m.getAttribute('data-live'));es.onmessage=function(ev){var p=JSON.parse(ev.data),du=document.getElementById('t-du');
if(du.getAttribute('data-h')!==p.du_h){es.close();location.reload();return}
setz('t-titel',p.titel);document.title=document.getElementById('t-titel').textContent;setz('t-warten',p.warten);setz('t-karte',p.karte);setz('t-wlan',p.wlan);if(p.vorbei)es.close()}}
else if(document.querySelector('.wartet'))setTimeout(function(){location.reload()},10000);
document.addEventListener('click',function(e){var k=e.target.closest('button[data-kopie]');if(!k)return;var alt=k.getAttribute('data-alt')||k.textContent,v=k.getAttribute('data-kopie');k.setAttribute('data-alt',alt);
function ok(t){k.textContent=t;setTimeout(function(){k.textContent=alt},2500)}
function markiere(){var c=k.parentNode.querySelector('code'),s=getSelection(),rg=document.createRange();rg.selectNodeContents(c);s.removeAllRanges();s.addRange(rg);ok(k.getAttribute('data-markiert'))}
if(navigator.clipboard&&navigator.clipboard.writeText)navigator.clipboard.writeText(v).then(function(){ok(k.getAttribute('data-kopiert'))},markiere);else markiere()});
var f=document.querySelector('#t-du form'),st=document.getElementById('t-quelle'),datei=document.getElementById('t-vcf'),pk=document.getElementById('t-picker');if(!f)return;
function feld(n,v){var e=f.querySelector('[name="'+n+'"]');if(!e||v==null||v==='')return;e.value=String(v).trim();e.dispatchEvent(new Event('input',{bubbles:true}));e.dispatchEvent(new Event('change',{bubbles:true}))}
function fuell(o){var name=(o.fn||'').trim(),i=name.lastIndexOf(' ');if(!o.vorname&&!o.nachname&&name){o.vorname=i>0?name.slice(0,i):name;o.nachname=i>0?name.slice(i+1):''}
feld('vorname',o.vorname);feld('nachname',o.nachname);feld('tel',o.tel);feld('mail',o.mail);feld('adr-strasse',o.strasse);feld('adr-plz',o.plz);feld('adr-ort',o.ort);feld('adr-land',o.land);feld('firma',o.firma);
var g=(o.geb||'').replace(/^(\d{4})(\d{2})(\d{2})$/,'$1-$2-$3').match(/^(\d{4}|-)-?(\d{2})-(\d{2})/);if(g){feld('geb-tag',+g[3]);feld('geb-monat',+g[2]);if(g[1]!=='-')feld('geb-jahr',g[1])}
st.textContent=datei.getAttribute('data-gut')}
function vcf(t){var o={},u=function(s){return s.replace(/\\n/gi,' ').replace(/\\([,;\\])/g,'$1').trim()};
t.replace(/\r?\n[ \t]/g,'').split(/\r?\n/).forEach(function(l){var i=l.indexOf(':');if(i<0)return;var k=l.slice(0,i).split(';')[0].toUpperCase().replace(/^ITEM\d+\./,''),v=l.slice(i+1),a;
if(k==='N'&&o.nachname==null){a=v.split(';');o.nachname=u(a[0]||'');o.vorname=u(a[1]||'')}else if(k==='FN')o.fn=u(v);else if(k==='TEL'&&!o.tel)o.tel=u(v);else if(k==='EMAIL'&&!o.mail)o.mail=u(v);
else if(k==='ADR'&&!o.strasse){a=v.split(';');o.strasse=u(a[2]||'');o.ort=u(a[3]||'');o.plz=u(a[5]||'');o.land=u(a[6]||'')}else if(k==='BDAY')o.geb=v.trim();else if(k==='ORG')o.firma=u(v.split(';')[0])});return o}
datei.addEventListener('change',function(){var d=datei.files&&datei.files[0];if(!d)return;d.text().then(function(t){if(!/BEGIN:VCARD/i.test(t))throw 0;fuell(vcf(t))}).catch(function(){st.textContent=datei.getAttribute('data-keine')}).then(function(){datei.value=''})});
if(pk&&navigator.contacts&&navigator.contacts.select){pk.hidden=false;pk.addEventListener('click',function(){var w=['name','tel','email','address'];
(navigator.contacts.getProperties?navigator.contacts.getProperties():Promise.resolve(w)).then(function(ps){return navigator.contacts.select(w.filter(function(p){return ps.indexOf(p)>=0}),{multiple:false})})
.then(function(cs){var c=cs&&cs[0];if(!c)return;var a=c.address&&c.address[0]||{};fuell({fn:c.name&&c.name[0],tel:c.tel&&c.tel[0],mail:c.email&&c.email[0],strasse:(a.addressLine||[]).join(' '),plz:a.postalCode,ort:a.city,land:a.country})}).catch(function(){})})}})()`

// teilenStyle adds the page's own parts to the form's look: each part a card, the waiting dots, the copy rows, the foot.
const teilenStyle = `<style>.teil{margin:0 0 14px;padding:16px;border-radius:16px;background:var(--card)}.teil h2{font-size:1.15rem;margin:0 0 10px}
.wartet{display:flex;gap:12px;align-items:center;color:var(--muted)}.punkte{display:inline-flex;gap:5px;flex:none}.punkte i{width:8px;height:8px;border-radius:50%;background:var(--accent);opacity:.3;animation:puls 1.4s infinite}
.punkte i:nth-child(2){animation-delay:.2s}.punkte i:nth-child(3){animation-delay:.4s}@keyframes puls{50%{opacity:1}}@media (prefers-reduced-motion:reduce){.punkte i{animation:none;opacity:.6}}
.felder{display:grid;grid-template-columns:auto 1fr;gap:4px 14px;margin:0 0 12px}.felder dt{color:var(--muted);font-size:.88rem;padding-top:2px}.felder dd{margin:0;overflow-wrap:anywhere}
a.knopf{display:flex;align-items:center;justify-content:center;min-height:48px;padding:0 18px;font-weight:600;border-radius:14px;background:var(--accent);color:var(--accent-ink);text-decoration:none}
.kopie{display:grid;grid-template-columns:1fr auto;gap:2px 10px;align-items:center;margin:0 0 12px}.kopie .k{grid-column:1/-1}.kopie code{font-size:1.05rem;overflow-wrap:anywhere;user-select:all}
.quelle{display:flex;flex-direction:column;gap:10px;margin:0 0 14px}label.datei input{padding:10px}#t-quelle:empty{display:none}
.werbung{margin:28px 0 0;padding:16px;border-radius:16px;border:1px solid var(--edge);color:var(--muted);font-size:.92rem}.werbung p{margin:0 0 6px}.werbung a{color:var(--accent);font-weight:600}</style>`
