#!/usr/bin/env sh
# One budget per area of Homes, as laib's scripts/loc.sh has one per program
# (owner, 2026-11-03: "Code sollte immer möglichst sauber sein"). Production
# lines only (not *_test.go), counted as gofmt writes them; tests are not
# capped. Each cap was set at the area's size that day plus 5 %.
#
# Crossing a cap means raising it in the same PR, with its reason next to the
# number and the owner's approval; at 98 % this warns (95 % would warn from
# the first day, the caps being 5 % above it). Every package under
# internal/ is in exactly one area: a new package names its area here.
#
# The areas, and who may import whom (production imports, `go list`):
# - connectors: a client of one other service. It imports only shared
#   packages: it knows its service, nothing of the household.
# - shared: the small shapes everyone uses (the card, Markdown, the app's
#   names, the guarded web fetch, a name compared). It imports no Homes
#   package.
# - household: the household's own logic, on top of connectors and shared.
# - napi: the app's API; pages: the family's pages; cmd: the wiring.
#   Only cmd imports pages; only cmd and pages import napi.
# - A fake (a package whose directory ends in "fake") is for the tests and
#   cmd/homesdemo only.
set -e
cd "$(dirname "$0")/.."
bad=0
p() { for x in "$@"; do printf 'internal/%s ' "$x"; done; }
connectors=$(p abs bote caldav discogs ha immich jellyfin judge laib musicbrainz paperless plugin qmd reitti samba werkstatt)
shared=$(p abruf app assistent cards code128 erreichbar jsonfile markdown mcpserver modul namen texte)
household=$(p ansicht devices dokumente drucker einkauf einrichten energie favoriten gesehen gesichter haushalt kontaktantwort library merken musik netz notes papierkorb podcasts schalter tagaktion takeout telefon uebernahme usage zugang)

budget() { # budget <name> <cap> <dir…>
  name=$1 cap=$2; shift 2
  prod=$(find "$@" -name '*.go' ! -name '*_test.go' -exec cat {} + 2>/dev/null | wc -l)
  test=$(find "$@" -name '*_test.go' -exec cat {} + 2>/dev/null | wc -l)
  echo "$name production: $prod  tests: $test  (cap $cap)"
  [ "$prod" -lt "$cap" ] || { echo "$name over $cap lines"; bad=1; }
  [ "$((prod * 100))" -lt "$((cap * 98))" ] || [ "$prod" -ge "$cap" ] || echo "warning: $name at $prod of $cap lines, 98 % or more"
}
# area       cap    dirs              reason (2026-11-03: the size that day, plus 5 %)
# 2026-11-05, owner 2026-11-05: contact jump links (15,415 -> 15,626): /kontakte/<uid>/dokumente and
# /nachrichten to Paperless and the viewer's archive, the contact page's buttons, the phone book's URLs
# 2026-11-05, owner 2026-11-05: So hängt alles zusammen (15,626 -> 15,740): the family's page under
# Mehr, the web of what links their data and the „Das geht“ moments, each shown while its modules are on
# 2026-11-05, owner 2026-11-05 yes: Immich for the contacts (15,740 -> 15,949): the
# „Fotos“ link and the face in the phones' cards (kontakt_sprung.go), the suggestions on Kontakte with anlegen
# and ausblenden (kontakte_immich.go), Fotos in „So hängt alles zusammen“
# 2026-11-06, owner approved 2026-11-06: Box → Auslastung (15,949 -> 16,090): the tab, its bars,
# rings and temperatures, its values fetched again in place every 5 s (box_auslastung.go, laib#1076)
# 2026-11-06, owner approved 2026-11-06: Dokumente-Suche (16,089 -> 16,371): Finden on Dokumente and /dokumente/suche (dokumente_suche.go): the search field fetched
# while typing, the Bereich tiles, type and person chips, Versicherungen and Garantie läuft noch, the rows
# 2026-11-06, owner approved 2026-11-06: Dieselbe Person: „Dieselbe Person?“ (16,341 -> 16,497):
# /kontakte/dieselbe (kontakte_dieselbe.go): pairs of faces, ten a page, Ja/Nein/Überspringen, the outcome on the page, the quiet links
# 2026-11-06, owner approved 2026-11-06: Kontaktfotos: „Ist das …?“ (16,497 -> 16,555):
# a contact's picture beside the person Immich gave its face, Ja/Nein/Kein Foto (kontakte_ist.go), on „Dieselbe Person?“
# 2026-11-07, owner approved 2026-11-07: Befunde in der Box-Übersicht (16,555 -> 16,574): laib befunde
# (laib#1085) instead of diag, „Befunde gemessen vor X min“ (seit), the line before diag's first run
# 2026-11-07, owner approved 2026-11-07: Dieselbe Person – Kontext und Aufgabe (16,574 -> 16,579): each face a
# link into Immich, a grey line for its places and its company, the questions as one task on Deine Aufgaben
# 2026-11-07, owner approved 2026-11-07: Kontakt prüfen lassen (16,579 -> 17,411): the start and the link
# with its share sheet (kontakt_pruefen.go), the review before → after, and the page without a login on its own listener (kontakt_frage.go)
# 2026-11-08, owner approved 2026-11-08: Kontakt-Formular v2 (17,404 -> 17,655): the public form's
# patterns, autofill and German errors beside each field, street and number apart, kinds and the main one, „entfernen“, the photo and
# the message, the multipart read in memory (kontakt_frage.go); the review's removal, photo and note (kontakt_pruefen.go)
# 2026-11-08, owner approved 2026-11-08: Kontakt-Formular Mülleimer (17,655 -> 17,709): a trash button per
# existing number, e-mail and address with „Rückgängig“, its no-JS fallback (kontakt_frage.go)
# 2026-11-08, owner approved 2026-11-08: Kontakte-Navigation (17,709 -> 17,761): the area's segments
# Haushalt · Meine · Fragen n · Antworten n and the path back on every Kontakte page (kontakte_nav.go), the actions at the top
# 2026-11-09, owner approved 2026-11-09: the teaser on the apex host and Zugang anfragen (pages 17,753 -> 18,023): the public
# page on the shared listener told apart by Host, the request's form, throttle and Danke, the admins' /zugang and the rows on Start (teaser.go)
# 2026-11-09, owner approved 2026-11-09: Wer steht wem nahe (pages 18,100 -> 18,350; 18,315 used): kontakte_netz.go,
# „Steht nahe“ on the contact page, the link on Kontakte
# 2026-11-09, owner approved 2026-11-09: Beziehungen klären (pages 18,350 -> 18,480; 18,446 used): the questions on
# /kontakte/netz and on the contact page
# 2026-11-10, owner approved 2026-11-09: Beziehungen änderbar, aufs Handy, in Deine Aufgaben und im Kontaktformular (homes#330) (18,480 -> 18,600): edit/remove on the contact page, the form's Beziehungen section and its review
# 2026-11-10, owner approved 2026-11-10: Beziehungen als eine Zeile je Person, beim Wählen gespeichert, und Handy-Kontakte als Namensvorschläge bei Wer ist das? (pages 18,600 -> 18,620; 18,613 used); shrinks when the questions move to the archive (archiv#27)
# 2026-11-11, owner approved 2026-11-11: Dieselbe Person – drei Gesichter je Seite (pages 18,620 -> 18,640; 18,627 used): the
# small faces under each face of a pair, each a link to its photo in Immich (kontakte_dieselbe.go)
# 2026-11-11, owner asked 2026-11-11: Dieselbe Person – bei Nein gleich den richtigen Namen (pages 18,640 -> 18,680; 18,671 used):
# „Wer ist das dann?“ after a pair's Nein, the same card as „Wer ist das?“ (kontakte_wer.go)
# 2026-11-11, owner approved 2026-11-11 („den Rest“): Meine Daten (pages 18,680 -> 18,900; 18,896 used): every source of a
# person on one page with its last arrival, state and threshold (meine_daten.go, homes#342); Daten sichern's and Mehr's own state lines go
# 2026-11-11, owner decided 2026-11-11 (the needed raise OK'd): Kontaktfotos helfen Immich (pages 18,900 -> 18,910; 18,904 used):
# „Sieht aus wie … aus deinen Kontakten“ with its one tap on „Wer ist das?“ (kontakte_wer.go, homes#352)
# 2026-11-11, owner OK'd the needed raise 2026-11-11: a „nur für mich“ stack on Letzte Stapel (pages 18,910 -> 18,990; 18,980 used):
#   the owner's own reads „Nur für dich“ with the way into Paperless instead of „Nichts mehr in Paperless“, another person's is not shown.
# 2026-11-11, owner OK'd needed raises for the UX pass 2026-11-11: Dieselbe Person – eine Frage auf einmal (pages 18,990 -> 19,030; 19,012 used):
# one card under one heading, „Frage 2 von 6 · Nächste Frage · Alle zeigen“ (kontakte_dieselbe.go)
# 2026-11-11, owner decided 2026-11-11 (ADR 0002): the net into the archive (pages 19,030 -> 18,844, ratcheted down by the
# 186 lines removed: kontakte_netz.go, „Steht nahe“; one link „Dein Netzwerk im Archiv ›“ instead), and one tap first on the faces
# 2026-11-11, owner OK'd the needed raise 2026-11-11: Energie – what the chosen hour used (pages 18,844 -> 19,110; 19,097 used): energie_stunde.go, the line under
# the Tag chart for the tapped hour, its devices, the Verbrauch column, the script choosing an hour; the scale's 0 and its negative bottom
# 2026-11-11, owner's wish 2026-11-11 „Das sollte bei jedem stehen“ (pages 19,110 -> 19,118; 19,117 used): the chosen bar's line,
# its devices and the Verbrauch column on Woche, Monat and Jahr too, from the daily statistics
# 2026-11-12, owner's request homes#362 (needed raise OK'd with it): Deine Aufgaben, the one list (pages 19,118 -> 19,220; 19,217 used;
# +99 net after the page Medien, the Box tab Aufgaben and Daten sichern's own page went, -190): the list per viewer in the owner's order
# (decisions, keys and permissions, questions, setup), the keys' and the archive's rows, the Box filter, AppListe for /napi/einrichtung,
# Scannen and Scans zuordnen on Dokumente, the answers' row on Kontakte
# 2026-11-12, owner's request homes#362 with archiv#60 (the count is the archive's owner's only, not in Laib's status): the archive's
# counts from the viewer's own archive (pages 19,220 -> 19,257; 19,256 used, the minimum): GET /intern/aufgaben with X-Archiv-Homes,
# the missing key's row, the clean-up's own link, „Stand: <Tag>“ while the vault is closed; the laib-status fields went (-4)
# 2026-11-12, owner's request (after contact-form spam): Zugang anfragen only at home (pages 19,257 -> 19,274; 19,273 used, the
# minimum): zuhause (the address Caddy saw, private or link-local), the sentence outside in the section's place, POST 403 (teaser.go)
# 2026-11-12, owner's wish 2026-11-12 („ein Ausgabegerät in einem Raum zum Standard erheben … konfigurierbar im System“, the needed
# raise OK'd with it): Standard-Lautsprecher per room on Geräte (pages 19,274 -> 19,414; 19,413 used, the minimum): standard.go, the
# room's Music Assistant players, the admins' choice and its POST, the family's line; raeume.go's page split for the answer
# 2026-11-13, owner's request 2026-11-13 („sieht noch nicht harmonisch aus … braucht man die Wahl nur, wenn es mehrere Geräte
# im Raum gibt“, which counts as the OK; pages 19,414 -> 19,425; 19,424 used, the minimum): the standard speaker as the room card's
# quiet last row (its style and pictogram), shown only with two or more speakers; a failed self-sending select goes back (skript.go)
# 2026-11-14, owner's request 2026-11-14 („Können wir ihn [den Epson-Drucker] über Home Assistant überwachen und in der App
# darstellen?“, with „neu verbinden“, „ob ein Ping funktioniert“ and „einen Button auf der Karte … ‚Prüfen und neu verbinden‘“;
# the raise to be OK'd in the PR; pages 19,425 -> 20,075; 20,068 used): the printer's row on its room's card (drucker.go) with
# its button while unreachable, its colours and the connection's check on its own page (drucker_pruefen.go: reload_config_entry,
# the fresh state, the address from the config entry or the configuration_url, the network's answer, kept for the row),
# roomCards' row split out, nachladen by POST, the host HA's diagnostics last gave kept in HOMES_DATA/drucker.json
# 2026-11-14, the same request's follow-up („Wird auch der Status ‚druckt‘ dargestellt?“, „Bitte in der App und auf der Webseite
# gleich“): pages 20,075 -> 19,950, ratcheted down by the printer's reading moved to internal/drucker (household)
# 2026-11-14, owner's request 2026-11-14 („In der Liste hat der Drucker keinen Stern für Favoriten“; pages 19,950 -> 19,970;
# 19,965 used, to be OK'd in the PR): the printer row's ☆ (roomThing.ownRow split from cardRow), Start's tile in its words
# 2026-11-14, owner's request 2026-11-14 („Kein Mensch“ on „Wer ist das?“: a toy's face asked as a person; pages 19,970 -> 19,990;
# 19,984 used, to be OK'd in the PR): the card's third choice and „Ausgeblendet.“ with its Rückgängig (kontakte_wer.go)
# 2026-11-15, owner's request homes#375 (ADR 0003 accepted 2026-11-15, „setze es so um“; pages 19,990 -> 20,035; 20,031 used, the
# minimum, to be OK'd in the PR): the plugin contract on the pages (plugins.go): a plugin's entry, Start card, handed-over tasks with
# „Stand: <Tag>“ and the quiet line; the archive's own counts went (archivZahlen, its rows, its nav entry, -70)
# 2026-11-18, owner's decision 2026-11-18 (Homes in the client's language: „ja, baue es so“; pages 20,035 -> 20,057; 20,056 used, the minimum,
# to be OK'd in the PR): <html lang> and the selection in the reader's language (navWortIn), Start's words and date in it, texte.Von(r) at
# every catalog word a page has its request for
# 2026-11-26, owner approved 2026-11-26, ADR 0004 §5 §8 (pages 20,057 -> 20,420; 20,419 used, the minimum): a tag's own page /tag/<id> (tag.go: the button
# as the signed-in person, the automations with Lösen, linking and a new one, the sentence before saving, the wish followed in place),
# the admins' answer /tag/freigabe/<id> (tag_freigabe.go), /.well-known/assetlinks.json (assetlinks.go), the wish on Deine Aufgaben
# 2026-11-27, owner approved 2026-11-27 („Alles OK. Go ahead“), ADR 0004 §9 (pages 20,420 -> 20,431; 20,430 used, the minimum): the line after Auslösen, „Zuletzt …“ (tag.go)
# 2026-11-27, owner's request 2026-11-27 („Deinem Zuhause geht es gut.“), owner approved 2026-11-27 „Bitte umsetzen“ (pages 20,431 -> 20,439; 20,438 used, the minimum): the claim as Laib says
# all is well, swapped once by StartBoxPath's answer (start.go, start_box.go)
# 2026-11-27, owner's request 2026-11-27 (Rezepte: „werden die mir nicht mehr in der App angezeigt“), owner approved 2026-11-27 („Bau alles“) (pages 20,439 -> 20,525; 20,524 used, the minimum): /rezepte (rezepte.go: the recipes newest
# first, the search, the way into Einkauf's choice, empty and failing in words), the link on Notizen
# 2026-11-28, owner's request 2026-11-28 (Zugangsanfragen ablehnen), owner approved 2026-11-28 „Ja zu allen“ (pages 20,525 -> 20,589; 20,588 used, the minimum):
# /zugang's Ablehnen beside Erledigt, the outcome said on top, „Abgelehnt: N“ folded with Wieder zulassen, 403 for a non-admin's POST (teaser.go)
# 2026-11-28, owner's request 2026-11-28 (Kontakte fragen auffindbar), owner approved 2026-11-28 „Ja zu allen“ (pages 20,589 -> 20,720; 20,719 used, the minimum): kontakte_fehlt.go: what a contact lacks at the top of its page with „Anna selbst fragen“, Kontakte's chip Unvollständig (n) with „Fragen“ and the grey line; pruefenForm shared
budget pages 20720 internal/pages     # the family's pages, one per file: 14,684
# 2026-11-05, owner 2026-11-05: writable Homes-Konto (5,033 -> 5,121): PUT and DELETE of one shared
# appointment or card for the phones, the Papierkorb on delete, the feed for a phone that writes back
# 2026-11-07, owner approved 2026-11-07: Kontakt prüfen lassen (5,114 -> 5,224): the phone's accepted
# corrections of its own contacts, GET and POST /napi/kontakte/korrekturen (korrekturen.go)
# 2026-11-14, owner's request 2026-11-14 („Bitte in der App und auf der Webseite gleich“; napi 5,225 -> 5,280; 5,274 used, to be
# OK'd in the PR): a card's text, a printer's words on the favorites, its state and the stream (withText, live.card)
# 2026-11-14, owner's request 2026-11-14 („Build the sealed-archive path for the phone's WhatsApp and Signal backups“,
# archiv#64; napi 5,280 -> 5,340; 5,337 used): only age-encrypted uploads (ageLeser), a medium sent before still counts in
# /fehlend, every .age file of letzte/ back through the open archive (ausArchiv, ageKlar)
# 2026-11-18, owner's decision 2026-11-18 (Homes in the client's language: „ja, baue es so“; napi 5,340 -> 5,342; 5,341 used): /napi/kann's titles in the phone's language
# 2026-11-26, owner approved 2026-11-26 (ADR 0004, the phone as a trigger without HA's app; napi 5,342 -> 5,538; 5,537 used, the
# minimum): POST /napi/ausloeser (an NFC tag as HA's tag_scanned, the charger as homes_laden), /tag, /tags;
# naming (ADR 0004 §2): an existing id named or renamed in place, unique names, DELETE, the name field's suggestions
# 2026-11-26, owner's go 2026-11-26 („Go. Passt alles“, the needed raise with it; ADR 0004 §4, napi 5,538 -> 5,629; 5,628 used, the minimum):
# the scan tag in /napi/ausloeser: „für mich“ the toucher's stack, the action kept and offered (aktionen)
# 2026-11-26, owner approved 2026-11-26, ADR 0004 §5 §8 (napi 5,629 -> 5,670; 5,669 used, the minimum): GET /napi/automationen, POST …/tag/{tag}/verbinden|loesen|neu,
# GET …/freigabe/{id} (ausloeser_automationen.go); the tag's action moved to tagaktion.Dienst (shared with the page)
# 2026-11-26, owner approved 2026-11-26, ADR 0004 §7 (napi 5,670 -> 5,698; 5,697 used, the minimum): the phone behind the bearer as HA's device,
# a tag as its scan_tag, the charger as its sensors (ausloeser.go)
# 2026-11-27, owner approved 2026-11-27 („Alles OK. Go ahead“), ADR 0004 §9 (napi 5,698 -> 5,716; 5,715 used, the minimum): ergebnisse and text after nfc, zuletzt per tag
# 2026-11-27, owner 2026-11-27: Notizen- und Dokumente-Widget ausgebaut (napi 5,716 -> 5,656; 5,655 used): /napi/portal/notes and
# /napi/portal/documents and their catalog entries gone (tools.go)
# 2026-11-27, owner approved the app's start screen 2026-11-27 („Bitte umsetzen“, homes-android#290), owner approved 2026-11-27 („Bau alles“) (napi 5,656 -> 5,682; 5,681 used, the minimum): achtung on a device's card (home.go)
# 2026-11-28, owner's request 2026-11-28 (Kontakte fragen auffindbar), owner approved 2026-11-28 „Ja zu allen“ (napi 5,682 -> 5,828; 5,827 used, the minimum): kontakte_fragen.go: POST /napi/kontakte/fragen, the household's card for a shared vCard (number, e-mail, name), the link and message, 404/409, anlegen
budget napi 5828 internal/napi        # the app's widgets' and share sheet's API: 4,793
# 2026-11-04, homes ADR 0001 step H1 (owner: „ganz durchziehen“): the module catalog,
# cmd/homes/module.go, 1,150 with it; plus 5 %. H3–H6 move the routes out of main.go into it
# 2026-11-04, step H8 (§10, owner: „Keine Funktion soll versteckt werden“): the routes of the traces
# and closing a wish once its module is on, 1,209; plus 5 %
# 2026-11-04, step H3: HOMES_MODULE_AUS switches the leaf modules off, 1,298; plus 5 %
# 2026-11-04, steps H4–H6: HOMES_MODULE_AUS switches every module off (the switches, the
# collections per module, the background work after the list), 1,379; plus 5 %
# 2026-11-04, the owner (the caps count production code only): cmd/homesdemo is the demo on
# fakes for the browser tests and the pictures, never run on a box; it is not counted. The
# cap stays as it was, now for cmd/homes alone
# 2026-11-12, owner's request homes#362: the app's Deine Aufgaben wired to the page's list and the admins seen (1,448 -> 1,452; 1,451 used)
# 2026-11-14, owner's request 2026-11-14 (the printer; 1,452 -> 1,455; 1,452 used): HOMES_DATA handed to the pages, where
# drucker.json keeps each printer's last known address
# 2026-11-15, owner's request homes#375 (1,455 -> 1,460; 1,458 used): the plugins from HOMES_PLUGIN_*, kept in HOMES_DATA, asked every 5 minutes
# 2026-11-18, owner's decision 2026-11-18 (Homes in the client's language: „ja, baue es so“; 1,460 -> 1,462; 1,461 used): Vary: Accept-Language on every answer
# 2026-11-26, owner's go 2026-11-26 („Go. Passt alles“, the needed raise with it; ADR 0004 §4, cmd 1,462 -> 1,474; 1,473 used, the minimum):
# tagAktionen in the wiring, out of main (gocognit)
# 2026-11-26, owner approved 2026-11-26, ADR 0004 §5 §8 (cmd 1,474 -> 1,493; 1,492 used, the minimum): the Dienst wired for the app and the pages, its admins
# and Bote, the daily drop of old wishes, HOMES_APP_SHA256's route
# 2026-11-26, owner approved 2026-11-26, ADR 0004 §7 (cmd 1,493 -> 1,520; 1,519 used, the minimum): telefoneAnschliessen: the registrations, removed on
# unpairing, the hourly look
# 2026-11-27, owner approved 2026-11-27 („Alles OK. Go ahead“), ADR 0004 §9 (cmd 1,520 -> 1,530; 1,529 used, the minimum): whose phone a scan's HA device is, wired
# 2026-11-27, owner's request 2026-11-27 (Rezepte: „werden die mir nicht mehr in der App angezeigt“), owner approved 2026-11-27 („Bau alles“) (cmd 1,530 -> 1,542; 1,541 used, the minimum): the app's Rezepte row after Notizen, with how many;
# the widgets' Angebot gone
# 2026-11-27, owner approved the app's start screen 2026-11-27 („Bitte umsetzen“, homes-android#290), owner approved 2026-11-27 („Bau alles“) (cmd 1,542 -> 1,580; 1,579 used, the minimum): bereiche: the tags, the archive's line, Paperless's week
# 2026-11-28, owner 2026-11-28: Energie-Widget ausgebaut (cmd 1,580 -> 1,579; 1,578 used): the energie module offers no widget
budget cmd 1579 cmd/homes             # homes' wiring: 1,018 with the demo then
# 2026-11-04, the caps moved with the packages, none raised (sum 24,134 as before): abs and
# musicbrainz (1,227 lines, 1,289 of cap) from household to connectors once merken's fetcher
# was internal/abruf (235 lines of merken, 247 of cap, to shared) and jellyfin.Plain
# internal/namen (11 lines, 12 of cap, to shared)
# 2026-11-05, owner 2026-11-05: writable Homes-Konto (13,171 -> 13,467): the logic of that napi raise
# lives in caldav (schreiben.go: find by UID, write over If-Match keeping what the phone does not show,
# delete, put back; the feed of whole series with their ETags), so this part of the budget moved with it
# 2026-11-05, owner 2026-11-05 yes: Immich for the contacts (13,466 -> 13,688):
# internal/immich, the people and their faces with the owner's key, and its fake (immichfake); caldav takes a
# picture the phone sends (sentOnly)
# 2026-11-06, owner approved 2026-11-06: Box → Auslastung (13,670 -> 13,714): laib.Auslastung,
# the shape of laib auslastung's JSON and its call (laib#1076)
# 2026-11-06, owner approved 2026-11-06: Dokumente-Suche (13,714 -> 14,098): paperless.Finde and Ordnung
# (suche.go: Bereiche, types, Person; the number fields by custom_field_query, sender, text, merged and kept a
# minute; Vertraege shares its reading, -64), and paperlessfake's custom_field_query (AND/OR/NOT, icontains, gt,
# exists), text, a sender's name, document counts and a select's options (+95)
# 2026-11-06, owner approved 2026-11-06: Kontakte → Immich, every contact a person in Immich (14,098 -> 14,288): internal/immich
# searches people by name (faceless and hidden too), makes one, sets its name and birthday, says whose
# key it is and which permission it lacks (107 -> 190); immichfake answers all of it, keeping a faceless
# person out of its list as Immich does, with the key's permissions (99 -> 206)
# 2026-11-06, owner approved 2026-11-06: Dieselbe Person: Immich merges (14,288 -> 14,434):
# immich.Zusammenfuehren (POST /api/people/merge), Naechste (closestPersonId), Fotos (statistics), Rechte (api-keys/me), and immichfake's
# 2026-11-06, owner approved 2026-11-06: Kontaktfotos: contacts' pictures (14,434 -> 14,575):
# immich.Hochladen (archived), Funde (GET /api/faces), Entfernen (DELETE /api/assets), multipart and 204; immichfake's
# 2026-11-07, owner approved 2026-11-07: Befunde in der Box-Übersicht (14,575 -> 14,579): laib.Befunde,
# the last diag run's time and findings, in place of laib.Diag
# 2026-11-07, owner approved 2026-11-07: Dieselbe Person – Kontext und Aufgabe (14,579 -> 14,642): immich.Umfeld (POST
# /api/search/metadata with personIds, exif and people) and immichfake's search over a library
# 2026-11-07, owner approved 2026-11-07: Kontakt prüfen lassen (14,642 -> 15,006): a card's taken fields
# written line by line over the version read (caldav/korrektur.go), N, ORG, TITLE and the numbers' kinds read
# 2026-11-08, owner approved 2026-11-08: Kontakt-Formular v2 (15,006 -> 15,120): caldav reads an e-mail's kind and
# the main number and address (PREF=1, 3.0 TYPE=PREF) and writes them as the card's version does, the contact's photo and the note
# 2026-11-09, owner approved 2026-11-09: Firmenkontakte aufräumen (15,120 -> 15,138): a card's PRODID says whether it is
# still as Homes wrote it, and a card removed over the ETag read (carddav.go AsWritten, RemoveCard)
# 2026-11-09, owner approved 2026-11-09: Wer steht wem nahe (connectors 15,140 -> 15,220; 15,202 used): immich.Aufnahmen,
# the photos with their people page by page, and its fake
# 2026-11-10, owner approved 2026-11-09: Beziehungen änderbar, aufs Handy, in Deine Aufgaben und im Kontaktformular (homes#330) (15,220 -> 15,240): the relation in the phone-correction payload
# 2026-11-11, owner approved 2026-11-11: Dieselbe Person – drei Gesichter je Seite (connectors 15,240 -> 15,290; 15,281 used): immich.Vorschau
# (a photo's preview, asset.view), the face's box in Funde, and immichfake's library faces and previews
# 2026-11-11, owner decided 2026-11-11 (the needed raise OK'd): Kontaktfotos helfen Immich (connectors 15,290 -> 15,425; 15,420 used):
# immich.Album and InsAlbum (album.create, album.read, albumAsset.create), Homes' uploads left out of Umfeld and Aufnahmen
# (originalFileName), and immichfake's albums and its search finding the uploads as Immich finds archived photos (homes#352)
# 2026-11-11, owner decided 2026-11-11 (ADR 0002): the net into the archive (connectors 15,425 -> 15,373, ratcheted down by the
# 52 lines removed: immich.Aufnahmen, the photo read only the net used)
# 2026-11-11, owner asked 2026-11-11 („Viele Kontakte haben mehrfach die gleiche Nummer“): one line per number
# (connectors 15,373 -> 15,500; 15,496 used): caldav/doppelt.go, a number compared with its country code (0171 … = +49 171 …),
# the phone's mirror stored with each number once, a correction that never writes one twice, and the counts and
# writes of `homes nummern-bereinigen`
# 2026-11-11, owner OK'd the needed raise 2026-11-11: Energie – what the chosen hour used (connectors 15,500 -> 15,610; 15,597 used): ha.EnergyPrefs (energy/get_prefs:
# the battery's counters and the devices), one command on its own connection, and hafake's energy dashboard
# 2026-11-12, owner's wish 2026-11-12, the needed raise OK'd with it: Standard-Lautsprecher per room (connectors 15,610 -> 15,777;
# 15,776 used, the minimum): ha.SetStandard (HA's label registry, the entity's labels and area), the labels and area ids in the
# registry, and hafake's label registry and entity get/update
# 2026-11-14, owner's request 2026-11-14 (the printer, its address; connectors 15,777 -> 15,830; 15,824 used): the device
# registry's configuration_url (Registry.ConfigURLs), the entity's config entry (Registry.Entries) and its host from
# HA's diagnostics (ha.EntryHost), hafake's diagnostics
# 2026-11-14, owner's request 2026-11-14 („Kein Mensch“ on „Wer ist das?“: a toy's face asked as a person; connectors 15,830 -> 15,850;
# 15,847 used, to be OK'd in the PR): immich.Verstecken (PUT isHidden), immichfake taking isHidden and a field left out
# 2026-11-15, owner's request homes#375 (ADR 0003 §3, connectors 15,850 -> 16,230; 16,224 used, the minimum, to be OK'd in the PR):
# internal/plugin, the contract's client: HOMES_PLUGIN_* as Laib renders them, the answer checked against contract 1, what Homes keeps
# of each plugin per person (plugins.json), the background ask
# 2026-11-26, owner approved 2026-11-26 (ADR 0004; connectors 16,230 -> 16,363; 16,362 used, the minimum): ha.Fire, ha.Tags, ha.CreateTag, ha.UpdateTag, ha.DeleteTag (tags.go) and the fake's events and tag registry
# 2026-11-26, owner approved 2026-11-26, ADR 0004 §5 §8 (connectors 16,363 -> 16,532; 16,531 used, the minimum): ha.Automations, AutomationConfig,
# SetAutomationConfig (automationen.go), the registry's unique_ids; the fakes: hafake's automation config API, laibfake's tell and scan
# 2026-11-26, owner approved 2026-11-26, ADR 0004 §7 (connectors 16,532 -> 16,759; 16,758 used, the minimum): ha.RegisterMobileApp, MobileWebhook,
# MobileAppEntries, DeleteConfigEntry (mobile.go); hafake's registrations, webhooks and config entry delete
# 2026-11-27, owner approved 2026-11-27 („Alles OK. Go ahead“), ADR 0004 §9 (connectors 16,759 -> 16,973; 16,972 used, the minimum): ha.Laeufe, ha.Spur (traces.go), ha.MobileAppDevices;
# hafake's trace/list, trace/get and runs on a scanned tag
# 2026-11-27, owner's request 2026-11-27 („Deinem Zuhause geht es gut.“), owner approved 2026-11-27 „Bitte umsetzen“ (connectors 16,973 -> 16,979; 16,978 used, the minimum): laib.Gut, one rule for the page and the app
# 2026-11-28, field failure 2026-11-28 (HA took the phone's registration without os_version and failed to set it up; a touched tag
# was lost), owner approved 2026-11-28 „Ja zu allen“ (connectors 16,979 -> 17,051; 17,050 used, the minimum): ha.Registrierung's os_version, ha.MobileAppBroken; the fake's failed setup
budget connectors 17051 $connectors   # one client per service, with its fake: 11,327, 12,543 with abs and musicbrainz
# 2026-11-04, internal/jsonfile (47 lines, 50 of cap) from household's cap to shared's: it
# replaces nine copies of a save, 62 of their lines in household
# 2026-11-05, owner 2026-11-05 yes: Immich for the contacts (9,583 -> 9,874):
# internal/gesichter, a contact matched to its Immich person, the faces fetched in the background, made
# 96 px and kept in HOMES_DATA, the suggestions and who was hidden from them
# 2026-11-06, owner approved 2026-11-06: Kontakte → Immich, every contact a person in Immich (9,874 -> 10,025): internal/gesichter
# matches the Haushalts-Kontakte and the key owner's Handy-Kontakte to Immich's people hourly, makes the
# missing ones, follows the ones it made, keeps the mapping, learns the owner's login (abgleich.go, 139);
# the suggestions and their hidden list go (gesichter.go 291 -> 303)
# 2026-11-06, owner approved 2026-11-06: Dieselbe Person: one name one person, „Dieselbe Person?“
# (10,025 -> 10,510): gesichter/dieselbe.go merges the persons of one name hourly and follows the mapping, asks Immich for the closest
# faces in the background, ranks the pairs, keeps „Nein“; gesichter.go keeps the key's permissions for the missing-permission line
# 2026-11-06, owner approved 2026-11-06: Kontaktfotos: „Ist das …?“ (10,510 -> 10,872):
# gesichter/kontaktbilder.go uploads the owner's contacts' pictures, follows what Immich made of each, asks, merges only on Ja
# 2026-11-07, owner approved 2026-11-07: Dieselbe Person – Kontext und Aufgabe (10,872 -> 11,088):
# a pair or question only with both faces kept as JPEGs and in a photo; gesichter/umfeld.go reads each shown
# person's places, years and companions at start and daily; Offen and einrichten.Fragen make the open
# questions one task; Öffnen and the icon per module in /napi/einrichtung
# 2026-11-07, owner approved 2026-11-07: Kontakt prüfen lassen (11,088 -> 12,059): kontaktantwort, the
# links' hashes and the answers, the comparison field by field and the checks (number, e-mail and its MX, birthday, name)
# 2026-11-08, owner approved 2026-11-08: Kontakt-Formular v2 (12,059 -> 12,516): kontaktantwort's form of each
# field (format.go), street and number and the PLZ by country, the PLZ↔Ort check against the open table shipped with Homes
# (adresse.go), the photo decoded and made again (foto.go), kinds, main ones, removals and the message (felder.go)
# 2026-11-08, owner approved 2026-11-08: the owner's answer wins over a background run that read before it (12,520 -> 12,560): gesichter
# 2026-11-09, owner approved 2026-11-09: the teaser on the apex host and Zugang anfragen (household 12,557 -> 12,741): internal/zugang,
# the requests kept with a salted hash of the address, the file as the throttle's memory (one per address an hour, twenty a day)
# 2026-11-09, owner approved 2026-11-09: Firmenkontakte aufräumen (12,756 -> 12,851): the Haushalt tools firmenkontakte and
# kontakt_entfernen for Dokumente's kontakte_aufraeumen.py (haushalt/firmenkontakte.go)
# 2026-11-09, owner approved 2026-11-09: handy_firmen, firm cards from the key owner's own phone contacts for kontakte_nachlesen
# (household 12,849 -> 12,898): the Haushalt tool handy_firmen (haushalt/firmenkontakte.go), the card shared with firmenkontakte
# 2026-11-09, owner approved 2026-11-09: Telefonnummern einheitlich (household 12,898 -> 13,019): haushalt.Telefon, a number in
# DIN 5008's international form with the German area codes' table, and the Haushalt tool telefon_vereinheitlichen (haushalt/telefon.go)
# 2026-11-09, owner approved 2026-11-09: Immich: no named pairs, Kontakt link, Wer ist das?, renames follow into household
# contacts (household 13,020 -> 13,200; 13,194 used): gesichter/wer.go, abgleich.go folgen, dieselbe.go and umfeld.go
# 2026-11-09, owner approved 2026-11-09: Immich names to the owner's phone (new contact, rename) (household 13,200 -> 13,330):
# gesichter/handy.go (Neuanlegen), folgen asking the phone, kontaktantwort.Store.Handy
# 2026-11-09, owner approved 2026-11-09: Wer steht wem nahe (household 13,330 -> 14,100; 14,036 used): internal/netz,
# the contact net from the key owner's photos and phone books
# 2026-11-09, owner approved 2026-11-09: Beziehungen klären (household 14,100 -> 14,420; 14,374 used): netz's
# suggestions, answers and their RELATED lines
# 2026-11-10, owner approved 2026-11-09: Beziehungen änderbar, aufs Handy, in Deine Aufgaben und im Kontaktformular (homes#330) (14,420 -> 14,620): changing and removing answers, relations to the phone, the task count
# 2026-11-11, owner approved 2026-11-11: Dieselbe Person – drei Gesichter je Seite (household 14,620 -> 14,760; 14,748 used):
# gesichter/ausschnitte.go cuts each pair's person's face from three photos in the daily run, and a face that says little is replaced
# 2026-11-11, owner asked 2026-11-11: Dieselbe Person – bei Nein gleich den richtigen Namen (household 14,760 -> 14,810; 14,807 used):
# gesichter.Dann, Benennen for a Nein's face, the named side's own name a merge after all (wer.go, dieselbe.go zusammen)
# 2026-11-11, owner decided 2026-11-11 (the needed raise OK'd): Kontaktfotos helfen Immich (household 14,810 -> 15,035; 15,029 used):
# gesichter/kontaktfotos.go puts each household photo up once by its hash, archived, into one album, replaces and deletes it, reads its
# face's person and suggests the name on „Wer ist das?“; counts leave Homes' uploads out (homes#352)
# 2026-11-11, owner decided 2026-11-11 (ADR 0002): the net into the archive (household 15,035 -> 14,635, ratcheted down by the
# 400 lines removed: internal/netz's ties from photos, their ranking, hiding and reasons; the relations stay)
# 2026-11-11, owner OK'd the needed raise 2026-11-11: Energie – what the chosen hour used (household 14,635 -> 14,700; 14,695 used): energie.Stunde, what the house used
# in an hour with the battery, and its three devices that used most
# 2026-11-12, owner's request homes#362: einrichten.Admins (who Authelia said is an admin, kept for the app, which has no groups) and
# the page's rows in /napi/einrichtung before and after the modules (household 14,700 -> 14,780; 14,776 used)
# 2026-11-14, owner's request 2026-11-14 („Wird auch der Status ‚druckt‘ dargestellt?“, „… gleich“; household 14,780 -> 15,035;
# 15,030 used, to be OK'd in the PR): internal/drucker, the printers read from HA's IPP sensors (moved from pages, -122 there),
# their state in words, IPP's state reasons in short German, one line for the app
# 2026-11-14, owner's request 2026-11-14 („Kein Mensch“ on „Wer ist das?“: a toy's face asked as a person; household 15,035 -> 15,095;
# 15,092 used, to be OK'd in the PR): gesichter.Ausblenden and Einblenden (Rückgängig), the hidden kept with their faces
# 2026-11-26, owner's go 2026-11-26 („Go. Passt alles“, the needed raise with it; ADR 0004 §4, household 15,095 -> 15,180; 15,179 used, the minimum):
# internal/tagaktion, which tag starts a scan (HOMES_DATA/tag-aktionen.json)
# 2026-11-26, owner approved 2026-11-26, ADR 0004 §5 §8 (household 15,180 -> 16,122; 16,121 used, the minimum): tagaktion's Dienst (the scanner's modes through
# HA's buttons or laib scan, the old names read as new), a tag's automations (found, linked, loosed back to exactly what was, new by the
# Bote) and the wishes waiting for an admin (told, tasked, first answer counts, dropped after 7 days); einrichten.AdminListe
# 2026-11-26, owner approved 2026-11-26, ADR 0004 §7 (household 16,122 -> 16,415; 16,414 used, the minimum): internal/telefon (a reporting phone registered
# with HA's mobile_app, its sensors, scan_tag, removed when unpaired, telefone.json), tagaktion.AusloesenMit, devices' Weg and Gibt
# 2026-11-27, owner approved 2026-11-27 („Alles OK. Go ahead“), ADR 0004 §9 (household 16,415 -> 16,756; 16,755 used, the minimum): tagaktion's ergebnis.go: the runs after a
# touch, waited for, a condition in words, each tag's last run, the traces kept 10 s; telefon.Person: a scan's HA device → its person
# 2026-11-27, owner's request 2026-11-27 (Rezepte: „werden die mir nicht mehr in der App angezeigt“), owner approved 2026-11-27 („Bau alles“) (household 16,756 -> 16,763; 16,762 used, the minimum): einrichten's rows after a module (Nach), anzahl
# 2026-11-27, owner approved the app's start screen 2026-11-27 („Bitte umsetzen“, homes-android#290), owner approved 2026-11-27 („Bau alles“) (household 16,763 -> 16,857; 16,856 used, the minimum): einrichten's bereiche, neu and dringend,
# tagaktion.Stand, drucker.Achtung
# 2026-11-28, field failure 2026-11-28 (HA took the phone's registration without os_version and failed to set it up; a touched tag
# was lost), owner approved 2026-11-28 „Ja zu allen“ (household 16,857 -> 16,885; 16,884 used, the minimum): telefon: a registration whose device HA did not set up counts as none; the failed entry removed first
# 2026-11-28, owner's request 2026-11-28 (Zugangsanfragen ablehnen), owner approved 2026-11-28 „Ja zu allen“ (household 16,885 -> 17,021; 17,020 used, the minimum):
# zugang's refused senders: Normal (an e-mail without +tag, gmail without dots; a phone its digits), Ablehnen, Abgelehnte, Zulassen, a refused sender's request only a throttle trace
# 2026-11-28, owner's request 2026-11-28 (Kontakte fragen auffindbar), owner approved 2026-11-28 „Ja zu allen“ (household 17,021 -> 17,176; 17,175 used, the minimum): kontaktantwort/fragen.go: Fehlt and FehltWorte, Mobil, Finden, Anlegen, Store.Link
budget household 17176 $household   # the household's own logic: 10,638, 9,176 without them and the fetcher
# 2026-11-04, homes ADR 0001 step H1 (owner: „ganz durchziehen“): internal/modul, what an area
# says about itself and the list Homes builds from them, 1,472 with it; plus 5 %
# 2026-11-04, step H8 (§10): the traces and wishes in internal/modul, and internal/texte, the one
# table of the new family words (owner: Homes becomes multilingual), 1,709; plus 5 %
# 2026-11-04, the owner: „Texte aus Übersetzungen sind kein Code. Also raus aus der Grenze.“ The
# text catalog (internal/texte/katalog.go: the family's words by id, and later each language's
# table, de/en/es/fr) is words, not code, and multiplies with every language: not counted. The
# package stays shared for the import rules; its logic (texte.go: T, In, Titel) is counted here
# 2026-11-14, owner's request 2026-11-14 („wird angezeigt, ob ein Ping auf die Adresse funktioniert“; shared 1,795 -> 1,920;
# 1,914 used): internal/erreichbar, a ping over an unprivileged ICMP socket, else a TCP connect to 631 or 80, within 2 s, a
# name that does not resolve said apart; cards.Row.Warn, a detail drawn as a warning; cards.Action's Path and Laeuft
# 2026-11-18, owner's decision 2026-11-18 (Homes in the client's language: „ja, baue es so“; shared 1,920 -> 1,960; 1,959 used): texte.Sprache, Von and Waehle,
# Accept-Language read by its weights (RFC 9110), and In finding the translations
budget shared 1960 $(echo "$shared" | sed 's#internal/texte ##') internal/texte/texte.go # the card, Markdown, the app's names: 1,019; abruf, namen: 1,287; jsonfile: 1,334

# every package in one area
for d in internal/*/; do
  d=${d%/}
  n=0
  for a in $connectors $shared $household internal/pages internal/napi; do [ "$a" = "$d" ] && n=$((n + 1)); done
  [ "$n" = 1 ] || { echo "$d is in $n areas, it belongs in exactly one (scripts/loc.sh)"; bad=1; }
done

# who imports whom
mod=github.com/mdopp/homes/
imports=$(go list -f '{{.ImportPath}} {{join .Imports " "}}' ./... | sed "s#$mod##g")
uses() { # uses <importer-regex> <imported-regex>: the lines that break the rule
  echo "$imports" | awk -v from="^($1)\$" -v to="^($2)\$" '$1 ~ from { for (i = 2; i <= NF; i++) if ($i ~ to) print $1 " imports " $i }'
}
re() { echo "$@" | sed 's/ *$//; s/ \{1,\}/|/g'; }
leaf=$(uses "($(re $shared))(/.*)?" '(internal|cmd)/.*')
[ -z "$leaf" ] || { echo "a shared package imports Homes:"; echo "$leaf"; bad=1; }
conn=$(uses "($(re $connectors))(/.*)?" '(internal|cmd)/.*' | grep -vE " imports ($(re $shared))\$" || true)
[ -z "$conn" ] || { echo "a connector imports more of Homes than shared packages:"; echo "$conn"; bad=1; }
surface=$(uses '.*' 'internal/(pages|napi)' | grep -vE '^cmd/|^internal/pages imports internal/napi$' || true)
[ -z "$surface" ] || { echo "only cmd imports pages, only cmd and pages import napi:"; echo "$surface"; bad=1; }
fakes=$(uses '.*' 'internal/.*fake' | grep -v '^cmd/homesdemo ' || true)
[ -z "$fakes" ] || { echo "a fake is for tests and cmd/homesdemo:"; echo "$fakes"; bad=1; }
exit $bad
