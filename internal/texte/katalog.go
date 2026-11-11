package texte

// The catalog: every word's id and its German (owner 2026-11-04: „Texte
// aus Übersetzungen sind kein Code“). Words only, no logic; the other
// languages come as tables beside it. scripts/loc.sh does not count it.

// The module titles are "modul.<name>"; Titel finds them.
const (
	// Daten sichern (owner 2026-11-08: the one overview; the app keeps only what needs the phone)
	DatenInDerApp      ID = "daten.in-der-app"    // a phone source with nothing yet: where to switch it on
	DatenTeilen        ID = "daten.teilen"        // Facebook and LinkedIn with nothing yet: share the export to the app
	DatenMail          ID = "daten.mail"          // E-Mail's line: Homes cannot see the mailbox, so the way
	DatenKeinKalender  ID = "daten.kein-kalender" // the person sees no calendar yet, not even the household's
	DatenTermine       ID = "daten.termine"       // after the count of calendars
	DatenSchluessel    ID = "daten.schluessel"    // the button into the archive's /tresor
	DatenWelche        ID = "daten.welche"        // its quiet line: which keys
	DatenBitwarden     ID = "daten.bitwarden"     // the keys belong in Bitwarden too
	DatenNieTitel      ID = "daten.nie.titel"     // the <details>: what never reaches the archive
	DatenNie           ID = "daten.nie"
	DatenWechsel       ID = "daten.wechsel" // the phone change (haus#209, homes-android#197): three steps, one a line
	DatenWechselMedien ID = "daten.wechsel.medien"
	// Meine Daten (homes#342): every source of a person on one page, with its last arrival
	MeineDatenTitel     ID = "meine-daten.titel"
	MeineDatenMehr      ID = "meine-daten.mehr"     // its line on Mehr
	MeineDatenDa        ID = "meine-daten.da"       // the card of the sources that came
	MeineDatenWege      ID = "meine-daten.wege"     // the card of the sources not set up, and the ways in
	MeineDatenNachTag   ID = "meine-daten.nach"     // a threshold: „still nach %s“
	MeineDatenSelbst    ID = "meine-daten.selbst"   // a source that comes when the person sends it: never quiet
	MeineDatenZahl      ID = "meine-daten.zahl"     // a source Homes only counts
	MeineDatenWartet    ID = "meine-daten.wartet"   // after „n Dateien warten“: the next step
	MeineDatenFehler    ID = "meine-daten.fehler"   // before the archive's own sentence
	MeineDatenStumm     ID = "meine-daten.stumm"    // the archive's status could not be read
	MeineDatenScannen   ID = "meine-daten.scannen"  // the scanner, never used: the way
	MeineDatenOrdner    ID = "meine-daten.ordner"   // the inbox folder: „Dateien in den Ordner „%s“ …“
	MeineDatenMailDok   ID = "meine-daten.mail-dok" // documents by mail: „an %s schicken“
	MeineDatenTeilen    ID = "meine-daten.teilen"   // Share → Homes
	MeineDatenTakeout   ID = "meine-daten.takeout"  // a Google location history
	SpurFehlt           ID = "spur.fehlt"           // the box does not run the module's backend
	SpurAus             ID = "spur.aus"             // the owner switched it off
	SpurAngefordert     ID = "spur.angefordert"     // a wish is open
	SpurWartet          ID = "spur.wartet"          // a proposal for it waits in the owner's Inbox
	SpurAbgelehnt       ID = "spur.abgelehnt"       // the proposal was rejected, or the wish closed without it
	SpurEingerichtet    ID = "spur.eingerichtet"    // the proposal was approved, the module is not on yet
	Anfordern           ID = "anfordern"            // the trace's button
	Anfordernd          ID = "anfordern.laeuft"     // the pressed button
	SpurenTitel         ID = "spuren.titel"         // Mehr's card of traces
	SpurenHinweis       ID = "spuren.hinweis"       // its quiet line
	SpurenStandNicht    ID = "spuren.stand-nicht"
	AnfordernNicht      ID = "anfordern.nicht"        // Laib refused or failed
	AnfordernStumm      ID = "anfordern.stumm"        // Laib did not answer in time
	AnfordernGibtsNicht ID = "anfordern.gibts-nicht"  // no such module, or it is on
	AnfordernKeinLaib   ID = "anfordern.kein-laib"    // this Laib takes no wish yet
	GrundAus            ID = "grund.aus"              // why a module is off: the owner switched it off
	GrundBraucht        ID = "grund.braucht"          // … because the module it needs is off: „%s“ is its title
	ModulSeiteTitel     ID = "modul.seite.titel"      // /module/<name>, admins
	ModulAn             ID = "modul.an"               // a module that is on
	ModulDienst         ID = "modul.dienst"           // the row naming its backend
	ModulStand          ID = "modul.stand"            // the row saying on or off
	ModulWunsch         ID = "modul.wunsch"           // the row with its wish
	WunschVon           ID = "wunsch.von"             // „%s, angefragt von %s seit %s“
	AusVorschlagen      ID = "schalten.aus"           // the admin's button: switch it off
	AnVorschlagen       ID = "schalten.an"            // the admin's button: switch it on again
	Vorschlagend        ID = "schalten.laeuft"        // the pressed button
	Vorgeschlagen       ID = "schalten.vorgeschlagen" // the proposal waits in Laib's Inbox
	ZurInbox            ID = "schalten.inbox"         // the link to it
	VorschlagNicht      ID = "schalten.nicht"         // Laib refused or failed
	VorschlagStumm      ID = "schalten.stumm"         // Laib did not answer in time
	VorschlagKeinLaib   ID = "schalten.kein-laib"     // this Laib takes no such proposal yet
	GrundVorschlagAus   ID = "schalten.grund.aus"     // the proposal's reason, the owner's notification: „%s verschwindet …“
	GrundVorschlagAn    ID = "schalten.grund.an"
	NichtSchaltbar      ID = "schalten.nicht-schaltbar"
	ModuleSeite         ID = "box.module" // the Box pill
	// Start's „Deine Geräte“ as the app's tiles (homes-android#245, owner 2026-11-04)
	Geraete             ID = "geraete.titel"
	GeraeteAlle         ID = "geraete.alle"     // the head's link to Räume
	GeraeteHinweis      ID = "geraete.hinweis"  // what a tap and a long press do
	GeraeteSchaltet     ID = "geraete.schaltet" // a tile right after a tap
	BoxWeg              ID = "verbindung.weg"   // the page cannot reach the box
	Stand               ID = "verbindung.stand" // „Stand %s“: since when the values are
	TorOeffnen          ID = "tor.frage"        // „%s öffnen?“, asked before a garage opens
	TorJa               ID = "tor.ja"
	TorNein             ID = "tor.nein"
	StandortOeffnen     ID = "standort.oeffnen"     // the main button, into Reitti
	StandortZuletzt     ID = "standort.zuletzt"     // „… %s“: when the newest point came
	StandortEinschalten ID = "standort.einschalten" // the step while no point has come
	// each module's setup, a task per person (owner 2026-11-04); the steps
	// are the catalog's, "schritt.<…>" with {adresse} and {name}
	Aufgabe       ID = "einrichten.aufgabe" // „%s auf dem Handy einrichten“
	MitApp        ID = "einrichten.mit-app" // the task's line: „Mit der App „%s“.“
	HomesApp      ID = "einrichten.homes"   // the Homes app, in a task's title
	AppLaden      ID = "einrichten.app"     // the first step: „Lade die App „%s“ aufs Handy.“
	Laden         ID = "einrichten.laden"   // its link to the store
	Oeffnen       ID = "einrichten.oeffnen" // the module's address
	Anleitung     ID = "einrichten.anleitung"
	Eingerichtet  ID = "einrichten.fertig"   // the person's tick, where nothing can tell
	Hakt          ID = "einrichten.hakt"     // the pressed tick
	Weiteres      ID = "einrichten.weiteres" // what is left of the block once it is set up
	HakenNicht    ID = "einrichten.nicht"    // the tick could not be kept
	Offen         ID = "aufgaben.offen"      // the counter's words: „%d offen“
	Hallo         ID = "willkommen.hallo"    // „Hallo %s“
	Benutzername  ID = "willkommen.benutzername"
	Schritte      ID = "willkommen.schritte"
	AllesFertig   ID = "willkommen.alles"
	ZurStartseite ID = "willkommen.weiter"
	GrundDienst   ID = "grund.dienst" // a link module's service is not on the box: „%s“ is its name
	// Musik, managing the music (owner 2026-11-04: „Musik verwalten“, not what is playing)
	MusikDazu             ID = "musik.dazu"              // the section how music gets in
	MusikHandy            ID = "musik.handy"             // from the phone: Teilen → Homes
	MusikComputer         ID = "musik.computer"          // from a computer at home; its word „Netzlaufwerk“ links to the page
	MusikWirdEinsortiert  ID = "musik.wird-einsortiert"  // a file beets has not finished
	MusikDauert           ID = "musik.dauert"            // an upload waits longer than ten minutes
	MusikFertig           ID = "musik.fertig"            // „%s“ what went in
	MusikNicht            ID = "musik.nicht"             // a file beets kept back; its reason after
	MusikPlatten          ID = "musik.platten"           // the records
	MusikPlatteErfassen   ID = "musik.platte-erfassen"   // its button
	MusikWunschliste      ID = "musik.wunschliste"       // the music wish list's link, its open wishes after
	MusikDiscogsVerbunden ID = "musik.discogs-verbunden" // „%s“ the Discogs account
	MusikDiscogsVerbinden ID = "musik.discogs-verbinden" // not connected yet: the quiet way to connect
	MusikDiscogsBringt    ID = "musik.discogs-bringt"    // what connecting brings
	// „Web zeigt, Handy bearbeitet“ (owner 2026-11-05): the calendar and the contacts say where to edit, alike
	KalenderBearbeiten ID = "kalender.bearbeiten"
	KontakteBearbeiten ID = "kontakte.bearbeiten"
	KontakteSuchen     ID = "kontakte.suchen"     // the search field: name, number, e-mail
	KontakteLeer       ID = "kontakte.leer"       // the address book has no card
	KontakteKeiner     ID = "kontakte.keiner"     // „%s“: no card matches the search
	KontakteAdressbuch ID = "kontakte.adressbuch" // the card's title while the list is not grouped by letter
	// the Kontakte area's segments and path (owner 2026-11-08)
	KontakteHaushalt ID = "kontakte.seg.haushalt"
	KontakteMeine    ID = "kontakte.seg.meine"
	KontakteWie      ID = "kontakte.wie"  // the <details> that holds a page's explanation
	KontaktePfad     ID = "kontakte.pfad" // the path's name for a screen reader
	// a contact's two fixed links (owner 2026-11-05): the page's buttons, the phone book's labels, what a link says instead
	KontaktDokumente       ID = "kontakt.dokumente"        // the button to Paperless
	KontaktNachrichten     ID = "kontakt.nachrichten"      // the button to the viewer's archive
	KontaktDokumenteKurz   ID = "kontakt.dokumente-kurz"   // the web address's label in the phone book
	KontaktNachrichtenKurz ID = "kontakt.nachrichten-kurz" // the same
	KontaktKeinPaperless   ID = "kontakt.kein-paperless"   // the box has no Paperless (any more)
	KontaktKeinArchiv      ID = "kontakt.kein-archiv"      // the viewer has no message archive
	KontaktArchivDaheim    ID = "kontakt.archiv-daheim"    // the archive opens at home only, and the viewer is away
	// Immich for the contacts (owner 2026-11-05): the third link, and the Kontakte page's suggestions
	KontaktFotos            ID = "kontakt.fotos"             // the button to the Immich person
	KontaktFotosKurz        ID = "kontakt.fotos-kurz"        // the web address's label in the phone book
	KontaktKeinImmich       ID = "kontakt.kein-immich"       // the box has no Immich (any more)
	KontaktKeineFotos       ID = "kontakt.keine-fotos"       // a firm or nobody Immich knows: no photos to show
	KontaktKeineNachrichten ID = "kontakt.keine-nachrichten" // no number, no e-mail: nothing to search the archive for
	MeineKontakte           ID = "meine-kontakte"            // the page, and its line under Kontakte
	MeineKontakteHinweis    ID = "meine-kontakte.hinweis"    // the page's quiet line
	MeineKontakteLeer       ID = "meine-kontakte.leer"       // the phone sent none yet
	MeineKontakteWeg        ID = "meine-kontakte.weg"        // a uid not in the book
	MeineKontakteRecht      ID = "meine-kontakte.recht"      // the key lacks „%s“ (one or a list)
	Dieselbe                ID = "dieselbe"                  // the page „Dieselbe Person?“ and its link
	DieselbeHinweis         ID = "dieselbe.hinweis"          // the page's quiet line
	DieselbeLeer            ID = "dieselbe.leer"             // no pair to ask about
	DieselbeOhneName        ID = "dieselbe.ohne-name"        // a face without a name
	DieselbeJa              ID = "dieselbe.ja"               // the buttons, and each pressed
	DieselbeJaLaeuft        ID = "dieselbe.ja.laeuft"
	DieselbeNein            ID = "dieselbe.nein"
	DieselbeNeinLaeuft      ID = "dieselbe.nein.laeuft"
	DieselbeSpaeter         ID = "dieselbe.spaeter"
	DieselbeSpaeterLauft    ID = "dieselbe.spaeter.laeuft"
	DieselbeZusammen        ID = "dieselbe.zusammen"    // merged: „%s“ is the name kept
	DieselbeGemerkt         ID = "dieselbe.gemerkt"     // „Nein“ kept
	DieselbeHinten          ID = "dieselbe.hinten"      // skipped
	DieselbeWeg             ID = "dieselbe.weg"         // the pair is no longer offered
	DieselbeWeiter          ID = "dieselbe.weiter"      // back to the list of contacts with a question
	DieselbeNichtGemacht    ID = "dieselbe.nicht"       // Immich merged neither: one is gone
	DieselbeStumm           ID = "dieselbe.stumm"       // Immich did not answer
	DieselbeOrte            ID = "dieselbe.orte"        // under a face: „%s“ the places, then the years
	DieselbeMit             ID = "dieselbe.mit"         // under a face: „%s“ the named persons in the same photos
	DieselbeInImmich        ID = "dieselbe.immich"      // a face's link: „%s“ the name, all its photos in Immich
	DieselbeInImmichOhne    ID = "dieselbe.immich.ohne" // the same for a face without a name
	DieselbeFoto            ID = "dieselbe.foto"        // a small face's link: the whole photo in Immich
	DieselbeAufgabe         ID = "dieselbe.aufgabe"     // the key owner's task: „%s“ how many, „3 Personen“
	DieselbeAufgabeWarum    ID = "dieselbe.aufgabe.warum"
	// „Ist das …?“: a contact's picture beside the person Immich gave its face
	IstDas                ID = "ist-das"             // the section
	IstDasHinweis         ID = "ist-das.hinweis"     // its quiet line
	IstDasFrage           ID = "ist-das.frage"       // „%s“ is the contact's name
	IstDasKontaktbild     ID = "ist-das.kontaktbild" // under the contact's picture
	IstDasJa              ID = "ist-das.ja"          // „%s“ is the contact's name
	IstDasKeinFoto        ID = "ist-das.kein-foto"   // „%s“ is the contact's name
	IstDasKeinFotoLaeuft  ID = "ist-das.kein-foto.laeuft"
	IstDasNeinGemerkt     ID = "ist-das.nein.gemerkt"      // „Nein“ kept
	IstDasKeinFotoGemerkt ID = "ist-das.kein-foto.gemerkt" // „Kein Foto“ kept
	IstDasStumm           ID = "ist-das.stumm"             // Immich did not answer
	// „Wer ist das?“ (owner 2026-11-09): unnamed faces to name, on „Dieselbe Person?“
	WerIstDas          ID = "wer-ist-das"
	WerHinweis         ID = "wer-ist-das.hinweis"
	WerName            ID = "wer-ist-das.name" // the field's label
	WerSpeichern       ID = "wer-ist-das.speichern"
	WerSpeichernLaeuft ID = "wer-ist-das.speichern.laeuft"
	WerUnbekannt       ID = "wer-ist-das.unbekannt" // „Kenne ich nicht“
	WerUnbekanntLauft  ID = "wer-ist-das.unbekannt.laeuft"
	// „Wer steht wem nahe“ (owner 2026-11-09): the contact net, for the Immich key's owner
	Netz              ID = "netz"         // the page, its link on Kontakte
	NetzNahe          ID = "netz.nahe"    // the section on a contact's page
	NetzLeer          ID = "netz.leer"    // nothing computed yet
	NetzKeine         ID = "netz.keine"   // this person has no tie shown
	NetzBild          ID = "netz.bild"    // the picture's label: „%s“ the centre
	NetzMehr          ID = "netz.mehr"    // widens the picture
	NetzMitte         ID = "netz.mitte"   // a row's link that makes it the centre: „%s“ the name
	NetzKontakt       ID = "netz.kontakt" // a row's link to its contact
	NetzAus           ID = "netz.aus"     // Ausblenden, and pressed
	NetzAusLaeuft     ID = "netz.aus.laeuft"
	NetzNicht         ID = "netz.nicht" // Nicht berechnen, and pressed
	NetzNichtLaeuft   ID = "netz.nicht.laeuft"
	NetzZurueck       ID = "netz.zurueck" // restore, and pressed
	NetzZurueckLaeuft ID = "netz.zurueck.laeuft"
	NetzVerborgen     ID = "netz.verborgen"       // the list „Ausgeblendet“
	NetzNichtArt      ID = "netz.verborgen.nicht" // a row there that is not computed
	NetzGesagtAus     ID = "netz.gesagt.aus"      // „%s“ the name
	NetzGesagtNicht   ID = "netz.gesagt.nicht"    // „%s“ the name
	NetzGesagtZurueck ID = "netz.gesagt.zurueck"  // „%s“ the name
	NetzFehler        ID = "netz.fehler"          // the choice was not kept
	NetzHinweis       ID = "netz.hinweis"         // how it is computed, folded away
	NetzFotos         ID = "netz.fotos"           // a reason: „%s“ the days, „%d“ the first year
	NetzAdresse       ID = "netz.adresse"         // a reason: the same postal address
	NetzFirma         ID = "netz.firma"           // a reason: the same company
	NetzBeziehung     ID = "netz.beziehung"       // a reason: „%s“ the relation the centre's card names
	NetzBeziehungVon  ID = "netz.beziehung.von"   // a reason: „%s“ the relation, „%s“ whose card names it
	NetzVerwandt      ID = "netz.verwandt"        // a reason: a relation without a kind
	// a contact's relations (owner 2026-11-09); the kinds are netz.art.<kind>
	NetzGesagt      ID = "netz.gesagt"          // a reason: „%s“ the kind the owner gave
	NetzIst         ID = "netz.ist"             // the choice's label: „%s“ X
	NetzSpeichern   ID = "netz.speichern"       // and pressed
	NetzKeineBez    ID = "netz.keine-beziehung" // and pressed
	NetzOhneWahl    ID = "netz.ohne-wahl"       // Speichern without a kind chosen
	NetzGesagtArt   ID = "netz.gesagt.art"      // „%s“ X, „%s“ the kind, „%s“ Y
	NetzFremd       ID = "netz.fremd"           // the choice „Kennen sich nicht“ (netz.Fremd)
	NetzGesagtFremd ID = "netz.gesagt.fremd"    // Fremd kept
	NetzNichtImBuch ID = "netz.nicht-im-buch"   // kept, but the address book not written
	// Änderbar (owner 2026-11-09): a contact's relations
	NetzBeziehungen   ID = "netz.beziehungen"      // the section on a contact's page
	NetzVergessen     ID = "netz.vergessen"        // a „Keine Beziehung“ forgotten, and pressed
	NetzGesagtWeg     ID = "netz.gesagt.weg"       // „%s“ and „%s“ the two: taken away
	NetzGesagtVergess ID = "netz.gesagt.vergessen" // forgotten
	NetzTelefonbuch   ID = "netz.telefonbuch"      // a relation's source, under its name
	NetzImArchiv      ID = "netz.im-archiv"        // the link to the questions in the person's archive (archiv#27)
	// Ins Adressbuch übernehmen (archiv#27 step 2, ADR 0002): an answer from the person's archive, confirmed here
	// Gruppen (owner 2026-11-10, ADR 0002): a contact's own groups, as the card's CATEGORIES
	Gruppen            ID = "gruppen"           // the section on a contact's page, the filter on Kontakte
	GruppenKeine       ID = "gruppen.keine"     // the contact is in none
	GruppenAlle        ID = "gruppen.alle"      // the filter's chip for every contact
	GruppenLeer        ID = "gruppen.leer"      // nobody in the group chosen
	GruppeDazu         ID = "gruppe.dazu"       // the field's label
	GruppeBeispiel     ID = "gruppe.beispiel"   // its placeholder
	GruppeDazuKnopf    ID = "gruppe.dazu.knopf" // and pressed
	GruppeDazuLauf     ID = "gruppe.dazu.laeuft"
	GruppeWeg          ID = "gruppe.weg" // a chip's ✕: „%s“ the group
	GruppeWegLauf      ID = "gruppe.weg.laeuft"
	GruppeDazuGesagt   ID = "gruppe.dazu.gesagt" // „%s“ the group
	GruppeWegGesagt    ID = "gruppe.weg.gesagt"  // „%s“ the group
	GruppeOhneName     ID = "gruppe.ohne-name"
	GruppeZuLang       ID = "gruppe.zu-lang"
	GruppeFirma        ID = "gruppe.firma" // „Firma“ is Homes' marker of a firm's card
	GruppeNurAdmins    ID = "gruppe.nur-admins"
	GruppeFehler       ID = "gruppe.fehler"
	WerGenannt         ID = "wer-ist-das.genannt" // saved: „%s“ is the name
	WerGemerkt         ID = "wer-ist-das.gemerkt" // „Kenne ich nicht“ kept
	WerOhneName        ID = "wer-ist-das.ohne-name"
	WerStumm           ID = "wer-ist-das.stumm"
	WerDann            ID = "wer-ist-das.dann" // after a pair's Nein: its unnamed face to name (owner 2026-11-11)
	WerDannHinweis     ID = "wer-ist-das.dann.hinweis"
	WerDoch            ID = "wer-ist-das.doch"     // the named side's own name typed: merged after all, „%s“ the name
	DieselbeKontakt    ID = "dieselbe.kontakt"     // under a named face: its contact's page
	DieselbeKontaktVon ID = "dieselbe.kontakt.von" // the link's label: „%s“ the name
	// Kontakt prüfen lassen (owner 2026-11-07): the start, the answers' review, the page without a login
	PruefenErstelltLaeuft ID = "pruefen.erstellen.laeuft"
	PruefenText           ID = "pruefen.text"        // the prepared message: the first name, the link
	PruefenTeilen         ID = "pruefen.teilen"      // the contact's button: the link made and shared
	PruefenWozu           ID = "pruefen.wozu"        // under it: what the link does
	PruefenTeilenFuer     ID = "pruefen.teilen-fuer" // a row's icon, for a screen reader: „%s“ the name
	PruefenNochmal        ID = "pruefen.nochmal"     // the link made, the share sheet needs one more tap
	KontaktAntwort        ID = "kontakt.antwort"     // the answer's heading on the contact
	PruefenKopiert        ID = "pruefen.kopiert"
	PruefenWeg            ID = "pruefen.weg"
	PruefenNicht          ID = "pruefen.nicht"
	PruefenNurAdmins      ID = "pruefen.nur-admins"
	AntwortenAufgabe      ID = "antworten.aufgabe" // the task: „%s“ how many, „2 Kontakt-Antworten“
	AntwortenAufgabeWarum ID = "antworten.aufgabe.warum"
	AntwortWeg            ID = "antwort.weg"
	AntwortVon            ID = "antwort.von" // the date, the book
	BuchHaus              ID = "buch.haus"
	BuchHandy             ID = "buch.handy"
	AntwortNurBestaetigt  ID = "antwort.nur-bestaetigt"
	AntwortNeu            ID = "antwort.neu"
	AntwortEntfernt       ID = "antwort.entfernt"
	AntwortAuswahl        ID = "antwort.auswahl" // the buttons, and pressed
	AntwortAlle           ID = "antwort.alle"
	AntwortLaeuft         ID = "antwort.laeuft"
	AntwortErledigt       ID = "antwort.erledigt"
	AntwortVerwerfen      ID = "antwort.verwerfen"
	AntwortVerwirft       ID = "antwort.verwirft"
	AntwortVerwerfenFrage ID = "antwort.verwerfen.frage"
	AntwortVerworfen      ID = "antwort.verworfen" // what came of it
	AntwortGeschrieben    ID = "antwort.geschrieben"
	AntwortZumHandy       ID = "antwort.zum-handy"
	AntwortUebersprungen  ID = "antwort.uebersprungen"
	AntwortKontaktWeg     ID = "antwort.kontakt-weg"
	AntwortNicht          ID = "antwort.nicht"
	AntwortErledigtGesagt ID = "antwort.erledigt.gesagt"
	StandUebernommen      ID = "stand.uebernommen" // what became of a change
	StandAbgelehnt        ID = "stand.abgelehnt"
	StandUebersprungen    ID = "stand.uebersprungen"
	StandAngenommen       ID = "stand.angenommen"
	ArtName               ID = "art.name" // the fields
	ArtTelefon            ID = "art.telefon"
	ArtMail               ID = "art.mail"
	ArtAdresse            ID = "art.adresse"
	ArtGeburtstag         ID = "art.geburtstag"
	ArtVorname            ID = "art.vorname"
	ArtNachname           ID = "art.nachname"
	ArtFirma              ID = "art.firma"
	ArtPosition           ID = "art.position"
	NummerMobil           ID = "nummer.mobil"
	NummerPrivat          ID = "nummer.privat"
	NummerGeschaeftlich   ID = "nummer.geschaeftlich"
	FrageNummerArt        ID = "frage.nummer-art"
	FrageTag              ID = "frage.tag"
	FrageMonat            ID = "frage.monat"
	FrageJahr             ID = "frage.jahr"
	FrageBeruf            ID = "frage.beruf"
	FrageTitel            ID = "frage" // the page without a login, /k/<token>
	FrageHinweis          ID = "frage.hinweis"
	FrageNummern          ID = "frage.nummern"
	FrageWeitereNummer    ID = "frage.weitere-nummer"
	FrageWeitereMail      ID = "frage.weitere-mail"
	FrageWeitereAdresse   ID = "frage.weitere-adresse"
	FrageStrasse          ID = "frage.strasse"
	FragePLZ              ID = "frage.plz"
	FrageOrt              ID = "frage.ort"
	FrageLand             ID = "frage.land"
	FrageHausnummer       ID = "frage.hausnummer"
	FrageHauptNummer      ID = "frage.haupt-nummer" // the main number, the main e-mail address (vCard PREF)
	FrageHauptMail        ID = "frage.haupt-mail"
	FrageNummerLoeschen   ID = "frage.nummer-loeschen" // a card's row's trash can, its label
	FrageMailLoeschen     ID = "frage.mail-loeschen"
	FrageAdresseLoeschen  ID = "frage.adresse-loeschen"
	FrageRueckgaengig     ID = "frage.rueckgaengig"
	FrageFoto             ID = "frage.foto"
	FrageFotoWahl         ID = "frage.foto.wahl"
	FrageFotoKeins        ID = "frage.foto.keins"
	FrageFotoGross        ID = "frage.foto.gross"
	FrageFotoNochmal      ID = "frage.foto.nochmal"
	FrageNachricht        ID = "frage.nachricht"
	FrageNachrichtAn      ID = "frage.nachricht.an" // „%s“ is the first name of who sent the link
	FrageDir              ID = "frage.dir"
	FrageFehler           ID = "frage.fehler" // the form again: what is wrong is said beside each field
	FrageNameForm         ID = "frage.form.name"
	FrageNummerForm       ID = "frage.form.nummer"
	FrageMailForm         ID = "frage.form.mail"
	FrageStrasseForm      ID = "frage.form.strasse"
	FrageHausnummerForm   ID = "frage.form.hausnummer"
	FragePLZForm          ID = "frage.form.plz"
	FrageOrtForm          ID = "frage.form.ort"
	FrageLandForm         ID = "frage.form.land"
	FrageTagForm          ID = "frage.form.tag"
	FrageJahrForm         ID = "frage.form.jahr"
	NummerAndere          ID = "nummer.andere"
	ArtFoto               ID = "art.foto"
	ArtNotiz              ID = "art.notiz"
	AntwortNachricht      ID = "antwort.nachricht" // the message in the review: „%s“ the contact's name
	AntwortNotiz          ID = "antwort.notiz"     // its tick
	AntwortHaupt          ID = "antwort.haupt"     // „Haupt-Nummer“ in a number's kind
	FrageSenden           ID = "frage.senden"
	FrageStimmt           ID = "frage.stimmt"
	FrageSendet           ID = "frage.sendet"
	FrageBis              ID = "frage.bis"
	FrageDanke            ID = "frage.danke"
	FrageDankeText        ID = "frage.danke.text"
	FrageUngueltig        ID = "frage.ungueltig" // the one page for every link that is none
	FrageUngueltigText    ID = "frage.ungueltig.text"
	FrageZuViele          ID = "frage.zu-viele"
	FrageZuVieleText      ID = "frage.zu-viele.text"
	FrageZuGross          ID = "frage.zu-gross"
	FrageDatum            ID = "frage.datum"
	FrageNicht            ID = "frage.nicht"
	// Zugang anfragen (owner 2026-11-08): the public teaser's form on the domain itself
	ZugangOhneName    ID = "zugang.ohne-name"
	ZugangOhneKontakt ID = "zugang.ohne-kontakt"
	ZugangZuLang      ID = "zugang.zu-lang"
	ZugangSpaeter     ID = "zugang.spaeter" // over the limit, or the store did not take it
	ZugangFremd       ID = "zugang.fremd"   // a POST another site sent
	ZugangNichtDa     ID = "zugang.nicht-da"
	ZugangMeldung     ID = "zugang.meldung" // the Signal message to the admin: name, contact, message
	ZugangAnfragen    ID = "zugang.anfragen"
	ZugangAnfrage     ID = "zugang.anfrage" // „Zugangsanfrage von %s“
	ZugangKeine       ID = "zugang.keine"
	ZugangErledigt    ID = "zugang.erledigt"
	ZugangHinweis     ID = "zugang.hinweis"
)

var de = map[ID]string{
	"meine-daten.titel":          "Meine Daten",
	"meine-daten.mehr":           "Was von dir auf die Box kommt, und wann zuletzt",
	"meine-daten.da":             "Was ankommt",
	"meine-daten.wege":           "So kommt mehr dazu",
	"meine-daten.nach":           "still nach %s",
	"meine-daten.selbst":         "kommt, wenn du etwas schickst",
	"meine-daten.zahl":           "die Box zählt nur, sie sieht keine Uhrzeit",
	"meine-daten.wartet":         "öffne dein Archiv, dann liest es sie",
	"meine-daten.fehler":         "hängt: ",
	"meine-daten.stumm":          "Dein Archiv sagt gerade nicht, wann zuletzt etwas kam. Später noch einmal schauen.",
	"meine-daten.scannen":        "Dokumente → Stapel scannen",
	"meine-daten.ordner":         "Dateien in den Ordner „%s“ auf dem Netzlaufwerk legen",
	"meine-daten.mail-dok":       "Briefe und Rechnungen per Mail an %s schicken",
	"meine-daten.teilen":         "Ein Foto oder PDF auf dem Handy teilen → Homes",
	"meine-daten.takeout":        "Den Standortverlauf aus Google Takeout auf dem Handy teilen → Homes",
	"meine-daten.still.whatsapp": "die Homes-App öffnen: läuft die Sicherung noch?",
	"meine-daten.still.signal":   "die Homes-App öffnen: läuft die Sicherung noch?",
	"meine-daten.still.sms":      "die Homes-App öffnen: läuft die Sicherung noch?",
	"meine-daten.still.mail":     "im Archiv das Postfach prüfen",
	"meine-daten.still.standort": "auf dem Handy prüfen, ob der Standort an ist",
	"daten.in-der-app":           "in der Homes-App einschalten",
	"daten.teilen":               "Export auf dem Handy teilen → „Ins Archiv“",
	"daten.mail":                 "Trag dein Postfach einmal im Archiv ein; es holt dann jede Stunde neue Mails.",
	"daten.kein-kalender":        "noch kein Kalender",
	"daten.termine":              "das Archiv übernimmt sie, während es offen ist",
	"daten.schluessel":           "Schlüssel im Archiv eintragen",
	"daten.welche":               "Signal-Passphrase, WhatsApp-Schlüssel und Postfach",
	"daten.bitwarden":            "Leg Archiv-Passwort, Signal-Passphrase und WhatsApp-Schlüssel auch in Bitwarden ab: wer einen verliert, kann diese Sicherungen nicht mehr lesen.",
	"daten.nie.titel":            "Was nie ins Archiv kommt",
	"daten.wechsel":              "Auf dem neuen Handy die Homes-App installieren und mit deinem Namen koppeln.\nIn der Homes-App: Mehr → Daten sichern → „Sicherung zurückholen“. Für Signal einen Ordner wählen, etwa „Dokumente/Signal“. WhatsApp vorher installieren, aber noch nicht öffnen, und den Ordner „Android/media/com.whatsapp/WhatsApp“ freigeben (gibt es ihn noch nicht: „Android/media“).\nSignal neu installieren, bei der Einrichtung „Aus Sicherung wiederherstellen“ wählen, die Datei auswählen und die 30-stellige Passphrase aus Bitwarden eingeben. WhatsApp öffnen, mit derselben Nummer anmelden und „Wiederherstellen“ wählen.",
	"daten.wechsel.medien":       "Fotos und Sprachnachrichten kommen nur zurück, während dein Archiv offen ist. Die App überschreibt nichts, ohne zu fragen.",
	"daten.nie":                  "Nachrichten, die sich selbst löschen, und Fotos oder Videos zur einmaligen Ansicht: sie stehen in keiner Sicherung.",
	"modul.geraete":              "Geräte",
	"modul.energie":              "Energie",
	"modul.sprechen":             "Sprechen",
	"modul.kalender":             "Kalender",
	"modul.kontakte":             "Kontakte",
	"modul.konto":                "Homes-Konto",
	"modul.einkauf":              "Einkauf",
	"modul.notizen":              "Notizen",
	"modul.dokumente":            "Dokumente",
	"modul.musik":                "Musik",
	"modul.podcasts":             "Podcasts",
	"modul.hoerbuecher":          "Hörbücher und Podcasts",
	"modul.gesehen":              "Gesehen & Gehört",
	"modul.sicherung":            "Daten sichern",
	"modul.standort":             "Standortverlauf",
	"modul.netzlaufwerk":         "Netzlaufwerk",
	"modul.werkstatt":            "Werkstatt",
	"modul.nutzung":              "Was wird genutzt",
	"modul.fotos":                "Fotos",
	"modul.passwoerter":          "Passwörter",
	"modul.filme":                "Filme und Musik",

	SpurFehlt:           "auf dieser Box nicht eingerichtet",
	SpurAus:             "ausgeschaltet",
	SpurAngefordert:     "angefordert",
	SpurWartet:          "wartet auf Freigabe",
	SpurAbgelehnt:       "abgelehnt",
	SpurEingerichtet:    "wird eingerichtet",
	Anfordern:           "Hier anfordern",
	Anfordernd:          "Wird angefordert …",
	SpurenTitel:         "Gibt es hier noch nicht",
	SpurenHinweis:       "Das kann Homes, auf dieser Box aber noch nicht. Fordere es an, dann sieht es der Admin.",
	SpurenStandNicht:    "Ob es schon jemand angefordert hat, lässt sich gerade nicht lesen.",
	AnfordernNicht:      "Die Anfrage ließ sich gerade nicht aufschreiben. Versuch es später noch einmal.",
	AnfordernStumm:      "Die Box hat nicht rechtzeitig geantwortet. Schau gleich nach, ob die Anfrage angekommen ist, bevor du es noch einmal versuchst.",
	AnfordernGibtsNicht: "Das gibt es hier schon, oder es gibt es gar nicht.",
	AnfordernKeinLaib:   "Diese Box nimmt noch keine Anfragen an.",
	GrundAus:            "Auf dieser Box ausgeschaltet.",
	GrundBraucht:        "Ausgeschaltet, weil es „%s“ braucht, und das ist hier ausgeschaltet.",
	ModulSeiteTitel:     "Was Homes kann",
	ModulAn:             "an",
	ModulDienst:         "Dienst",
	ModulStand:          "Stand",
	ModulWunsch:         "Anfrage",
	WunschVon:           "%s, angefragt von %s seit %s",
	AusVorschlagen:      "Ausschalten vorschlagen",
	AnVorschlagen:       "Wieder einschalten vorschlagen",
	Vorschlagend:        "Wird vorgeschlagen …",
	Vorgeschlagen:       "Vorgeschlagen. Der Vorschlag wartet in Laibs Inbox auf Freigabe.",
	ZurInbox:            "Zur Inbox",
	VorschlagNicht:      "Laib hat den Vorschlag nicht angenommen. Versuch es später noch einmal; sonst weiß laib diag mehr.",
	VorschlagStumm:      "Laib hat nicht rechtzeitig geantwortet. Schau in Laibs Inbox nach, ob der Vorschlag angekommen ist, bevor du es noch einmal versuchst.",
	VorschlagKeinLaib:   "Diese Box nimmt diesen Vorschlag noch nicht an: Laib braucht dafür ein Update.",
	GrundVorschlagAus:   "%s verschwindet aus Homes und der App.",
	GrundVorschlagAn:    "%s kommt in Homes und die App zurück.",
	NichtSchaltbar:      "Dieses Modul lässt sich noch nicht ausschalten.",
	ModuleSeite:         "Module",
	Geraete:             "Deine Geräte",
	GeraeteAlle:         "Alle →",
	GeraeteHinweis:      "Tippen schaltet, lange drücken öffnet das Gerät.",
	GeraeteSchaltet:     "schaltet …",
	BoxWeg:              "Box nicht erreichbar – versucht es wieder …",
	Stand:               "Stand %s",
	TorOeffnen:          "%s öffnen?",
	TorJa:               "Öffnen",
	TorNein:             "Abbrechen",
	StandortOeffnen:     "Zeitleiste öffnen",
	StandortZuletzt:     "Verbunden · zuletzt aufgezeichnet %s",
	StandortEinschalten: "Noch ist kein Punkt angekommen. Schalte in der Homes-App unter „Standortverlauf“ die Aufzeichnung ein.",
	Aufgabe:             "%s auf dem Handy einrichten",
	MitApp:              "Mit der App „%s“.",
	HomesApp:            "Homes-App",
	AppLaden:            "Lade die App „%s“ aufs Handy.",
	Laden:               "App laden",
	Oeffnen:             "Öffnen",
	Anleitung:           "Ausführliche Anleitung",
	Eingerichtet:        "Ist eingerichtet",
	Hakt:                "Hakt ab …",
	Weiteres:            "Auf einem weiteren Gerät einrichten",
	HakenNicht:          "Das ließ sich gerade nicht abhaken. Versuch es gleich noch einmal.",
	Offen:               "%d offen",
	Hallo:               "Hallo %s",
	Benutzername:        "Dein Benutzername:",
	Schritte:            "Deine nächsten Schritte",
	AllesFertig:         "Alles eingerichtet.",
	ZurStartseite:       "Weiter zur Startseite",
	GrundDienst:         "Auf dieser Box läuft %s nicht.",

	"schritt.anmelden":             "Trag als Server-Adresse {adresse} ein und melde dich mit deinem Benutzernamen und Passwort an.",
	"schritt.fotos":                "Trag als Server-Adresse {adresse} ein, tippe auf „Mit Familienkonto anmelden“ und melde dich an.",
	"schritt.fotos-sichern":        "Schalte unter „Sicherung“ ein, dass neue Fotos von selbst auf die Box kommen.",
	"schritt.tresor":               "Wähle „Selbst gehostet“, trag {adresse} ein und melde dich mit „Single Sign-On“ und deinem Familienkonto an.",
	"schritt.tresor-passwort":      "Beim ersten Mal legst du ein Master-Passwort fest. Das kennst nur du, niemand kann es zurücksetzen: Schreib es auf.",
	"schritt.signal":               "Tippe unten auf „Öffnen“ und schreib „Hallo“. {name} antwortet, sobald ein Admin dich einmal freigegeben hat.",
	"schritt.homes":                "Öffne die App, trag {adresse} ein und melde dich an. Die App richtet sich dann selbst ein.",
	"schritt.homes-iphone":         "iPhone: Öffne {adresse} in Safari, tippe auf „Teilen“ und dann auf „Zum Home-Bildschirm“.",
	"musik.dazu":                   "Hinzufügen",
	"musik.handy":                  "Auf dem Handy: Lieder oder ein ganzes Album als ZIP teilen und Homes wählen.",
	"musik.computer":               "Am Computer, zu Hause: in den Ordner „musik-eingang“ auf dem Netzlaufwerk legen.",
	"musik.wird-einsortiert":       "wird einsortiert …",
	"musik.dauert":                 "Das dauert länger als sonst; sobald es fertig ist, bekommst du eine Nachricht.",
	"musik.fertig":                 "fertig: %s in der Musik",
	"musik.nicht":                  "nicht übernommen",
	"musik.platten":                "Platten",
	"musik.platte-erfassen":        "Platte erfassen",
	"musik.wunschliste":            "Wunschliste",
	"musik.discogs-verbunden":      "Discogs: verbunden mit „%s“",
	"musik.discogs-verbinden":      "Mit Discogs verbinden",
	"musik.discogs-bringt":         "dann erfasst du Platten mit einem Foto in deine Sammlung.",
	"kalender.bearbeiten":          "Neue Termine und Änderungen machst du auf dem Handy im Haushalts-Kalender oder über den Boten.",
	"kontakte.bearbeiten":          "Neue Kontakte und Änderungen machst du auf dem Handy in den Haushalts-Kontakten oder über den Boten.",
	"kontakte.suchen":              "Name, Nummer, E-Mail",
	"kontakte.leer":                "Im Adressbuch steht noch niemand.",
	"kontakte.keiner":              "Kein Kontakt passt zu „%s“.",
	"kontakte.seg.haushalt":        "Haushalt",
	"kontakte.seg.meine":           "Meine",
	"kontakte.wie":                 "Wie das funktioniert",
	"kontakte.pfad":                "Wo du bist",
	"kontakte.adressbuch":          "Adressbuch",
	"kontakt.dokumente":            "Dokumente in Paperless",
	"kontakt.nachrichten":          "Nachrichten im Archiv",
	"kontakt.dokumente-kurz":       "Dokumente",
	"kontakt.nachrichten-kurz":     "Nachrichten",
	"kontakt.kein-paperless":       "Auf dieser Box gibt es kein Paperless.",
	"kontakt.kein-archiv":          "Für dich gibt es auf dieser Box noch kein Archiv für Nachrichten.",
	"kontakt.archiv-daheim":        "Dein Archiv öffnet sich nur zu Hause im WLAN; gerade bist du unterwegs.",
	"kontakt.fotos":                "Fotos in Immich",
	"kontakt.fotos-kurz":           "Fotos",
	"kontakt.kein-immich":          "Auf dieser Box gibt es kein Immich.",
	"kontakt.keine-nachrichten":    "Zu diesem Kontakt ist keine Nummer und keine Mail-Adresse bekannt, also keine Nachrichten.",
	"kontakt.keine-fotos":          "Zu diesem Kontakt gibt es in Immich keine Person, also keine Fotos.",
	"meine-kontakte":               "Meine Kontakte",
	"meine-kontakte.hinweis":       "Deine eigenen Handy-Kontakte; nur du siehst sie. Ändern kannst du sie auf dem Handy.",
	"meine-kontakte.leer":          "Dein Handy hat noch keine Kontakte geschickt. In der Homes-App: Kontakte übertragen.",
	"meine-kontakte.weg":           "Diesen Kontakt gibt es in deinen Handy-Kontakten nicht mehr.",
	"meine-kontakte.recht":         "Dem Immich-Schlüssel von Homes fehlt diese Berechtigung: %s. Ohne sie bekommen neue Kontakte keine Person in Immich, doppelte Personen bleiben getrennt, oder die Bilder deiner Kontakte kommen nicht als Vorschlag. In Immich: oben rechts dein Bild → Kontoeinstellungen → API-Schlüssel, beim Schlüssel „Homes“ auf Bearbeiten tippen und sie ankreuzen.",
	"dieselbe":                     "Dieselbe Person?",
	"dieselbe.hinweis":             "Immich hält diese Gesichter für ähnlich. Sagst du Ja, führt Homes beide in Immich zu einer Person zusammen. Ein Nein merkt sich Homes, dann fragt es nicht wieder. Nur du siehst diese Seite.",
	"dieselbe.leer":                "Gerade gibt es nichts zu fragen. Homes schaut jede Stunde in Immich nach ähnlichen Gesichtern.",
	"dieselbe.ohne-name":           "ohne Namen",
	"dieselbe.ja":                  "Ja, dieselbe Person",
	"dieselbe.ja.laeuft":           "Führt zusammen …",
	"dieselbe.nein":                "Nein",
	"dieselbe.nein.laeuft":         "Merkt es sich …",
	"dieselbe.spaeter":             "Überspringen",
	"dieselbe.spaeter.laeuft":      "Schiebt nach hinten …",
	"dieselbe.zusammen":            "Zusammengeführt: Die Fotos gehören jetzt zu %s.",
	"dieselbe.gemerkt":             "Gemerkt: Nach diesen beiden fragt Homes nicht wieder.",
	"dieselbe.hinten":              "Übersprungen: Dieser Vorschlag kommt am Ende wieder.",
	"dieselbe.weiter":              "‹ Zurück zur Liste",
	"dieselbe.weg":                 "Diesen Vorschlag gibt es nicht mehr. Hier sind die aktuellen.",
	"dieselbe.nicht":               "Immich hat die beiden nicht zusammengeführt, weil es eine davon nicht mehr gibt. Nichts wurde geändert.",
	"dieselbe.stumm":               "Immich hat gerade nicht geantwortet, nichts wurde zusammengeführt. Versuch es gleich noch einmal.",
	"dieselbe.orte":                "Orte: %s",
	"dieselbe.mit":                 "Auch dabei: %s",
	"dieselbe.immich":              "Alle Fotos von %s in Immich",
	"dieselbe.immich.ohne":         "Alle Fotos dieses Gesichts in Immich",
	"dieselbe.foto":                "Das ganze Foto in Immich",
	"dieselbe.aufgabe":             "%s in Immich prüfen",
	"dieselbe.aufgabe.warum":       "Immich hält Gesichter für dieselbe Person oder fragt, ob ein Kontaktbild passt. Ein Ja führt sie zusammen.",
	"ist-das":                      "Ist das …?",
	"ist-das.hinweis":              "Das Bild aus deinem Handy-Kontakt, daneben die Person, der Immich das Gesicht darauf zuordnet. Erst dein Ja führt sie zusammen.",
	"ist-das.frage":                "Ist das %s?",
	"ist-das.kontaktbild":          "Bild im Kontakt",
	"ist-das.ja":                   "Ja, das ist %s",
	"ist-das.kein-foto":            "Kein Foto von %s",
	"ist-das.kein-foto.laeuft":     "Nimmt das Bild heraus …",
	"ist-das.nein.gemerkt":         "Gemerkt: Homes hat das Bild aus Immich genommen und fragt nach dieser Person nicht wieder.",
	"ist-das.kein-foto.gemerkt":    "Gemerkt: Homes hat das Bild aus Immich genommen und nimmt es nicht wieder.",
	"ist-das.stumm":                "Immich hat gerade nicht geantwortet, nichts wurde geändert. Versuch es gleich noch einmal.",
	"wer-ist-das":                  "Wer ist das?",
	"wer-ist-das.hinweis":          "Gesichter, die Immich oft sieht und noch nicht kennt. Ein Name gilt in Immich; heißt schon jemand so, führt Homes beide in der nächsten Stunde zusammen.",
	"wer-ist-das.name":             "Name",
	"wer-ist-das.speichern":        "Speichern",
	"wer-ist-das.speichern.laeuft": "Speichert …",
	"wer-ist-das.unbekannt":        "Kenne ich nicht",
	"wer-ist-das.unbekannt.laeuft": "Merkt es sich …",
	"netz":                         "Wer steht wem nahe",
	"netz.nahe":                    "Steht nahe",
	"netz.leer":                    "Noch keine Verbindungen. Homes rechnet einmal am Tag aus deinen Fotos und den Telefonbüchern; schau morgen wieder.",
	"netz.keine":                   "Zu dieser Person gibt es noch keine Verbindung.",
	"netz.bild":                    "Wer %s nahe steht",
	"netz.mehr":                    "Mehr zeigen",
	"netz.mitte":                   "%s in die Mitte",
	"netz.kontakt":                 "Kontakt",
	"netz.aus":                     "Ausblenden",
	"netz.aus.laeuft":              "Blendet aus …",
	"netz.nicht":                   "Nicht berechnen",
	"netz.nicht.laeuft":            "Nimmt heraus …",
	"netz.zurueck":                 "Zurückholen",
	"netz.zurueck.laeuft":          "Holt zurück …",
	"netz.verborgen":               "Ausgeblendet",
	"netz.verborgen.nicht":         "nicht berechnet",
	"netz.gesagt.aus":              "%s ist ausgeblendet. Unter „Ausgeblendet“ lässt es sich zurückholen.",
	"netz.gesagt.nicht":            "%s wird nicht mehr berechnet, auch nicht in den Verbindungen der anderen.",
	"netz.gesagt.zurueck":          "%s ist wieder da.",
	"netz.fehler":                  "Das hat nicht geklappt: Homes konnte es nicht speichern. Versuch es gleich noch einmal.",
	"netz.hinweis":                 "Homes zählt einmal am Tag, an wie vielen Tagen zwei Menschen zusammen auf deinen Fotos sind; ein Tag an einem Ort zählt einmal, ein großes Gruppenfoto wenig, Altes weniger als Neues. Dazu kommt, was die Telefonbücher sagen: dieselbe Adresse, dieselbe Firma, eine eingetragene Beziehung. „Nicht berechnen“ nimmt jemanden ganz heraus, etwa Freunde der Kinder. Nur du siehst diese Seite.",
	"netz.fotos":                   "%s auf deinen Fotos seit %d",
	"netz.adresse":                 "gleiche Adresse",
	"netz.firma":                   "gleiche Firma",
	"netz.beziehung":               "%s (Telefonbuch)",
	"netz.beziehung.von":           "%s von %s (Telefonbuch)",
	"netz.verwandt":                "im Telefonbuch verknüpft",
	"netz.gesagt":                  "%s (von dir angegeben)",
	"netz.ist":                     "%s ist …",
	"netz.speichern":               "Speichern",
	"netz.keine-beziehung":         "Keine Beziehung",
	"netz.ohne-wahl":               "Bitte wähle zuerst, wie die beiden zueinander stehen.",
	"netz.gesagt.art":              "Gespeichert: %s ist %s von %s.",
	"netz.fremd":                   "Kennen sich nicht",
	"netz.gesagt.fremd":            "Gemerkt: Die beiden kennen sich nicht. Homes zeigt keine Verbindung mehr zwischen ihnen und fragt nicht wieder.",
	"netz.nicht-im-buch":           "Ins Adressbuch konnte Homes es gerade nicht schreiben; hier ist es gespeichert.",
	"netz.beziehungen":             "Beziehungen",
	"netz.vergessen":               "Vergessen",
	"netz.gesagt.weg":              "Entfernt: %s und %s stehen in keiner Beziehung. Homes fragt danach nicht wieder.",
	"netz.gesagt.vergessen":        "Vergessen. Homes fragt vielleicht später wieder danach.",
	"netz.telefonbuch":             "aus dem Telefonbuch",
	"netz.im-archiv":               "Wie die Menschen zueinander stehen, klärst du jetzt in deinem Archiv",
	"gruppen":                      "Gruppen",
	"gruppen.keine":                "Noch in keiner Gruppe.",
	"gruppen.alle":                 "Alle",
	"gruppen.leer":                 "In dieser Gruppe ist niemand.",
	"gruppe.dazu":                  "Zu Gruppe hinzufügen",
	"gruppe.beispiel":              "z. B. Stammtisch",
	"gruppe.dazu.knopf":            "Hinzufügen",
	"gruppe.dazu.laeuft":           "Fügt hinzu …",
	"gruppe.weg":                   "Aus „%s“ entfernen",
	"gruppe.weg.laeuft":            "…",
	"gruppe.dazu.gesagt":           "Hinzugefügt: „%s“.",
	"gruppe.weg.gesagt":            "Aus „%s“ entfernt.",
	"gruppe.ohne-name":             "Bitte gib den Namen der Gruppe ein.",
	"gruppe.zu-lang":               "Der Name der Gruppe ist zu lang (höchstens 60 Zeichen).",
	"gruppe.firma":                 "„Firma“ nutzt Homes schon für Firmen-Kontakte. Wähle bitte einen anderen Namen.",
	"gruppe.nur-admins":            "Gruppen eines Haushalts-Kontakts ändert nur ein Admin.",
	"gruppe.fehler":                "Ins Adressbuch konnte Homes es gerade nicht schreiben. Versuch es gleich noch einmal.",
	"frage.beziehung.ist":          "%s ist für dich",
	"frage.beziehung.name":         "Weitere Person (Name)",
	"frage.beziehung.art":          "ist für dich",
	"frage.beziehung.loeschen":     "Beziehung entfernen",
	"frage.beziehung.ohne-art":     "Bitte wähle, was diese Person für dich ist.",
	// the kinds a question offers (netz.Arten): „X ist <kind> von Y“
	"netz.art.partner":         "Partner/in",
	"netz.art.expartner":       "Ex-Partner/in",
	"netz.art.eltern":          "Mutter/Vater (Eltern)",
	"netz.art.kind":            "Kind",
	"netz.art.geschwister":     "Geschwister",
	"netz.art.grosseltern":     "Großeltern",
	"netz.art.enkel":           "Enkel/in",
	"netz.art.onkel":           "Onkel/Tante",
	"netz.art.neffe":           "Neffe/Nichte",
	"netz.art.cousin":          "Cousin/e",
	"netz.art.schwiegereltern": "Schwiegereltern",
	"netz.art.schwiegerkind":   "Schwiegerkind",
	"netz.art.schwager":        "Schwager/Schwägerin",
	"netz.art.freund":          "Freund/in",
	"netz.art.kollege":         "Kollege/Kollegin",
	"netz.art.nachbar":         "Nachbar/in",
	"netz.art.bekannt":         "Bekannte/r",
	"netz.art.verwandt":        "Sonstige verwandt",
	// a relation's kind as phones write it (netz.label): vCard's RELATED TYPE, iOS's labels, Android's types
	"netz.rel.spouse":          "Ehepartner/in",
	"netz.rel.partner":         "Partner/in",
	"netz.rel.child":           "Kind",
	"netz.rel.son":             "Sohn",
	"netz.rel.daughter":        "Tochter",
	"netz.rel.parent":          "Elternteil",
	"netz.rel.mother":          "Mutter",
	"netz.rel.father":          "Vater",
	"netz.rel.sibling":         "Geschwister",
	"netz.rel.brother":         "Bruder",
	"netz.rel.sister":          "Schwester",
	"netz.rel.friend":          "Freund/in",
	"netz.rel.relative":        "Verwandt",
	"netz.rel.kin":             "Verwandt",
	"netz.rel.colleague":       "Kollege/Kollegin",
	"netz.rel.coworker":        "Kollege/Kollegin",
	"netz.rel.manager":         "Vorgesetzte/r",
	"netz.rel.assistant":       "Assistenz",
	"netz.rel.neighbor":        "Nachbar/in",
	"wer-ist-das.genannt":      "Gespeichert: Das Gesicht heißt in Immich jetzt %s.",
	"wer-ist-das.gemerkt":      "Gemerkt: Nach diesem Gesicht fragt Homes nicht wieder.",
	"wer-ist-das.ohne-name":    "Der Name fehlt. Schreib ihn ins Feld, oder tippe auf „Kenne ich nicht“.",
	"wer-ist-das.stumm":        "Immich hat gerade nicht geantwortet, nichts wurde gespeichert. Versuch es gleich noch einmal.",
	"wer-ist-das.dann":         "Wer ist das dann?",
	"wer-ist-das.dann.hinweis": "Kennst du das Gesicht, schreib seinen Namen dazu. Er gilt dann in Immich.",
	"wer-ist-das.doch":         "Das ist derselbe Name wie links, also doch dieselbe Person: Die Fotos gehören jetzt zu %s.",
	"dieselbe.kontakt":         "Kontakt",
	"dieselbe.kontakt.von":     "Kontakt von %s öffnen",

	// „So hängt alles zusammen“ (owner 2026-11-05): the page, its web's nodes ("knoten.<…>", a word and a line under it), its moments ("<…>" and "<…>.so")
	"zusammen.titel":                    "So hängt alles zusammen",
	"zusammen.mehr":                     "Von der Rechnung zum Anruf: was deine Daten miteinander können",
	"archiv.titel":                      "Dein Archiv",
	"archiv.mehr":                       "Deine Nachrichten, Kontakte und Mails, gesichert und durchsuchbar",
	"zusammen.kurz":                     "Kontakte, Dokumente, Nachrichten und Termine kennen einander. Das macht vieles leichter.",
	"zusammen.bild":                     "In der Mitte ein Haushalts-Kontakt, verbunden mit Dokumenten, Nachrichten, Terminen, Bank, Telefonbuch und seinen Fotos; drumherum Zeitleiste, Musik und Geräte.",
	"zusammen.mitte":                    "Haushalts-",
	"zusammen.mitte.unter":              "Kontakt",
	"zusammen.knoten.termine":           "Termine",
	"zusammen.knoten.termine.unter":     "Haushalts-Kalender",
	"zusammen.knoten.bank":              "Bank & IBAN",
	"zusammen.knoten.bank.unter":        "aus der Rechnung",
	"zusammen.knoten.telefonbuch":       "Telefonbuch",
	"zusammen.knoten.telefonbuch.unter": "auf jedem Handy",
	"zusammen.knoten.nachrichten":       "Nachrichten",
	"zusammen.knoten.nachrichten.unter": "dein Archiv",
	"zusammen.knoten.dokumente":         "Dokumente",
	"zusammen.knoten.dokumente.unter":   "Paperless",
	"zusammen.knoten.zeitleiste":        "Zeitleiste",
	"zusammen.knoten.zeitleiste.unter":  "Standortverlauf",
	"zusammen.knoten.musik":             "Musik",
	"zusammen.knoten.musik.unter":       "Wunschliste",
	"zusammen.knoten.fotos":             "Fotos",
	"zusammen.knoten.fotos.unter":       "Immich",
	"zusammen.knoten.geraete":           "Geräte",
	"zusammen.knoten.geraete.unter":     "per Signal",
	"zusammen.geht":                     "Das geht",
	"zusammen.rechnung":                 "Von der Rechnung zum Anruf",
	"zusammen.rechnung.so":              "Eine Rechnung wird gescannt, die Firma landet mit ihrer IBAN in den Haushalts-Kontakten, und du rufst sie aus dem Telefonbuch an.",
	"zusammen.tipp":                     "Ein Tipp zu allen Dokumenten eines Kontakts",
	"zusammen.tipp.so":                  "Beim Kontakt auf „Dokumente“ tippen: alles, was er geschickt hat, in Paperless.",
	"zusammen.gesichter":                "Ein Tipp vom Kontakt zu allen Fotos dieser Person",
	"zusammen.gesichter.so":             "Beim Kontakt auf „Fotos“ tippen: Immich zeigt alle Bilder, auf denen es dieses Gesicht erkannt hat.",
	"zusammen.termin":                   "Termin auf dem Handy verschieben, alle sehen es",
	"zusammen.termin.so":                "Im Haushalts-Kalender geändert, kurz darauf auf allen Handys.",
	"zusammen.zeitachse":                "Google-Zeitachse teilen, die Zeitleiste füllt sich",
	"zusammen.zeitachse.so":             "Den Takeout mit der Homes-App teilen: Jahre voller Orte in deinem Standortverlauf, nur für dich.",
	"zusammen.waschmaschine":            "Waschmaschine fertig, Nachricht in Signal",
	"zusammen.waschmaschine.so":         "Geräte im Haus sagen Bescheid, der Bote schreibt dir.",
	"zusammen.fotos":                    "Fotos vom Handy landen von selbst in Immich",
	"zusammen.fotos.so":                 "Einmal in der Immich-App die Sicherung einschalten; ein Tipp öffnet deine Fotos.",
	"zusammen.fehlt":                    "Etwas fehlt? Hier anfordern",
	"zusammen.fehlt.so":                 "Was diese Box noch nicht kann, steht unten auf Mehr. Eine Anfrage, und der Admin sieht es.",
	// Box → Auslastung (owner 2026-11-06, laib auslastung)
	"auslastung.titel":         "Auslastung",
	"auslastung.kein-laib":     "Laib gibt diese Werte noch nicht her. Nach dem nächsten Laib-Update wartet in der Inbox die Freigabe „Neue Standard-Verben“; danach stehen sie hier.",
	"auslastung.speicher":      "Speicher",
	"auslastung.platte.system": "System",
	"auslastung.platte.data":   "Daten",
	"auslastung.platte.boot":   "Startbereich",
	"auslastung.belegt":        "%s von %s belegt · %.0f %% · %s frei",
	"auslastung.raid":          "Ob der Spiegel der Datenplatten heil ist, steht auf der Übersicht, sobald ihm etwas fehlt.",
	"auslastung.rechner":       "Rechner",
	"auslastung.cpu":           "Prozessor",
	"auslastung.last":          "Last %s · %s · %s, %d Kerne",
	"auslastung.ram":           "Arbeitsspeicher",
	"auslastung.gpu":           "Grafikkarte",
	"auslastung.von":           "%s von %s",
	"auslastung.swap":          "Auslagerung:",
	"auslastung.seit":          "läuft seit %s",
	"auslastung.tage":          "%d Tagen",
	"auslastung.stunden":       "%d Stunden",
	"auslastung.watt":          "%.0f von %.0f Watt",
	"auslastung.haushalt":      "Gerade frei für den Haushalt, Modell %s.",
	"auslastung.gehalten":      "Gerade belegt von %s (Fenster %s).",
	"auslastung.temperaturen":  "Temperaturen",
	"auslastung.temp.cpu":      "Prozessor",
	"auslastung.temp.platte":   "Platte %s",
	"auslastung.fehlt":         "Nicht messbar:",
	"auslastung.gemessen":      "Gemessen um %s, alle 5 Sekunden neu",
	"auslastung.alt":           "gerade keine neuen Werte",
	// Box → Übersicht, „Was braucht mich?“ (owner 2026-11-06)
	"box.ruhig":            "Alles in Ordnung.",
	"box.gesichert":        "Letzte Sicherung %s, an %s",
	"box.keine-sicherung":  "Noch keine gültige Sicherung",
	"box.geaendert":        "Letzte Änderung: %s (%s)",
	"box.faellig":          "Box-Aufgabe fällig",
	"box.faellig.n":        "Box-Aufgaben fällig",
	"box.wartet":           "Entscheidung wartet",
	"box.wartet.n":         "Entscheidungen warten",
	"box.drift":            "%d Dateien auf der Box weichen von Laibs Plan ab",
	"box.diag-stumm":       "Die Befunde von laib diag lassen sich gerade nicht lesen.",
	"box.diag-fehlt":       "Gelbe Befunde (laib diag) zeigt Homes, sobald Laib das Verb befunde kennt (ein Laib-Update); bis dahin nur die roten aus laib status.",
	"box.diag-nie":         "laib diag ist noch nicht gelaufen: Befunde gibt es noch keine.",
	"box.gemessen":         "Befunde gemessen %s",
	"box.inbox-stumm":      "Laibs Inbox antwortet gerade nicht: ob Entscheidungen warten, ist offen.",
	"operator.alle-gut":    "Alle Skills liefen zuletzt gut.",
	"operator.alle-zeigen": "Alle Skills zeigen",
	// Box → Haushalt: the five rows and their state (owner 2026-11-06)
	"haushalt.karte":         "Was zum Haushalt gehört",
	"haushalt.einladen":      "Jemanden einladen",
	"haushalt.einladung":     "Einladung offen",
	"haushalt.einladung.n":   "Einladungen offen",
	"haushalt.telefon":       "Telefon gekoppelt",
	"haushalt.telefon.n":     "Telefone gekoppelt",
	"haushalt.stumm":         "lässt sich gerade nicht lesen",
	"haushalt.adresse":       "Adresse gesetzt",
	"haushalt.keine-adresse": "Noch keine Adresse",
	"haushalt.module":        "%d von %d Modulen an",
	"haushalt.nutzung":       "Homes zählt seit %s",
	// Dokumente: finden (owner 2026-11-06: „Kategorien und Rechnungsnummern/Versicherungsart+Nr/gekaufte Produkte … suchbar“)
	"dok.finden":                 "Finden",
	"dok.suchen":                 "Suchen",
	"dok.sucht":                  "Sucht …",
	"dok.suchen.feld":            "Name, Nummer, Produkt",
	"dok.versicherungen":         "Versicherungen",
	"dok.garantie":               "Garantie läuft noch",
	"dok.fuer":                   "Für %s",
	"dok.alle":                   "Alle Dokumente",
	"dok.gesucht":                "Gesucht: „%s“",
	"dok.zurueck":                "Dokumente",
	"dok.fehler":                 "Paperless antwortet gerade nicht. Später noch einmal versuchen.",
	"dok.keine":                  "Nichts gefunden für „%s“. Ein Stück der Nummer reicht, oder ein Wort aus dem Titel.",
	"dok.leer":                   "Hier liegt noch nichts.",
	"dok.versicherungen.leer":    "Keine Versicherung gefunden. Hier stehen Versicherungsscheine und Verträge mit dem Bereich „Versicherung“.",
	"dok.garantie.leer":          "Keine Garantie, die noch läuft. Hier steht, was im Feld „Garantie bis“ ein Datum in der Zukunft hat.",
	"dok.mehr":                   "Die ersten %d. Mit einem Wort mehr wird es genauer.",
	"dok.nr.Rechnungsnummer":     "Rechnung %s",
	"dok.nr.Versicherungsnummer": "Vers.-Nr. %s",
	"dok.nr.Vertragsnummer":      "Vertrag %s",
	"dok.nr.Kundennummer":        "Kunde %s",
	"dok.garantie.bis":           "Garantie bis %s",
	"dok.kuendbar":               "kündbar bis %s",
	"dok.termin":                 "Kündigungstermin %s",
	"pruefen.erstellen.laeuft":   "Erstellt den Link …",
	"pruefen.teilen-fuer":        "Daten von %s prüfen lassen",
	"pruefen.nochmal":            "Jetzt teilen",
	"kontakt.antwort":            "Antwort auf deinen Link",
	"pruefen.text":               "Hallo %s, ich möchte deine Kontaktdaten in unserem Adressbuch aktuell halten. Magst du kurz prüfen, ob alles stimmt? Der Link gilt 14 Tage und nur einmal: %s",
	"pruefen.teilen":             "Daten prüfen lassen",
	"pruefen.wozu":               "%s bekommt einen Link und bestätigt Telefon, Adresse und Co. selbst.",
	"pruefen.kopiert":            "Link kopiert",
	"pruefen.weg":                "Diesen Kontakt gibt es nicht mehr.",
	"pruefen.nicht":              "Der Link ließ sich nicht erstellen. Versuch es gleich noch einmal; hilft das nicht, sag dem Admin Bescheid.",
	"pruefen.nur-admins":         "Kontakte des Hauses lässt nur ein Admin prüfen. Deine eigenen Handy-Kontakte kannst du prüfen lassen.",
	"antworten.aufgabe":          "%s prüfen",
	"antworten.aufgabe.warum":    "Jemand hat seine Kontaktdaten geprüft. Übernimm, was stimmt.",
	"antwort.weg":                "Diese Antwort gibt es nicht (mehr).",
	"antwort.von":                "Antwort vom %s · %s",
	"buch.haus":                  "Kontakt des Hauses",
	"buch.handy":                 "dein Handy-Kontakt",
	"antwort.nur-bestaetigt":     "%s hat bestätigt: alles stimmt so.",
	"antwort.neu":                "neu",
	"antwort.entfernt":           "entfernen",
	"antwort.auswahl":            "Ausgewählte übernehmen",
	"antwort.alle":               "Alle übernehmen",
	"antwort.laeuft":             "Übernimmt …",
	"antwort.erledigt":           "Erledigt",
	"antwort.verwerfen":          "Verwerfen",
	"antwort.verwirft":           "Verwirft …",
	"antwort.verwerfen.frage":    "Diese Antwort verwerfen? Nichts davon wird übernommen.",
	"antwort.verworfen":          "Verworfen: nichts wurde geändert.",
	"antwort.geschrieben":        "%s ins Adressbuch geschrieben; die Handys bekommen sie beim nächsten Abgleich.",
	"antwort.zum-handy":          "%s gehen an dein Handy: die Homes-App trägt sie beim nächsten Sichern in den Kontakt ein, sofort unter Daten sichern → Jetzt sichern.",
	"antwort.uebersprungen":      "%s nicht geschrieben: das Feld war inzwischen anders. Sieh dir den Kontakt an.",
	"antwort.kontakt-weg":        "Den Kontakt gibt es im Adressbuch nicht mehr; nichts wurde geschrieben.",
	"antwort.nicht":              "Das Adressbuch hat nicht geantwortet; nichts wurde geschrieben. Versuch es gleich noch einmal.",
	"antwort.erledigt.gesagt":    "Erledigt.",
	"stand.uebernommen":          "übernommen",
	"stand.abgelehnt":            "nicht übernommen",
	"stand.uebersprungen":        "nicht geschrieben: inzwischen anders, oder der Kontakt fehlt",
	"stand.angenommen":           "wartet auf dein Handy",
	"art.name":                   "Name",
	"art.telefon":                "Telefon",
	"art.mail":                   "E-Mail",
	"art.adresse":                "Adresse",
	"art.geburtstag":             "Geburtstag",
	"frage":                      "Stimmen deine Kontaktdaten?",
	"frage.hinweis":              "So stehen deine Angaben in unserem Adressbuch. Ändere oder ergänze, was nicht stimmt, und schick sie ab. Du siehst hier nur deine eigenen Angaben.",
	"frage.nummern":              "Telefonnummern",
	"frage.weitere-nummer":       "Weitere Nummer",
	"frage.weitere-mail":         "Weitere E-Mail",
	"frage.weitere-adresse":      "Weitere Adresse",
	"frage.strasse":              "Straße",
	"frage.hausnummer":           "Hausnummer",
	"frage.haupt-nummer":         "Haupt-Nummer",
	"frage.haupt-mail":           "Haupt-Adresse",
	"frage.nummer-loeschen":      "Nummer löschen",
	"frage.mail-loeschen":        "E-Mail löschen",
	"frage.adresse-loeschen":     "Adresse löschen",
	"frage.rueckgaengig":         "Rückgängig",
	"frage.foto":                 "Dein Foto",
	"frage.foto.wahl":            "Foto wählen (wenn du magst)",
	"frage.foto.keins":           "Dieses Bild lässt sich nicht öffnen. Wähle ein Foto als JPEG, PNG, HEIC oder WebP.",
	"frage.foto.gross":           "Das Foto ist zu groß: höchstens 5 MB.",
	"frage.foto.nochmal":         "Wähle das Foto bitte noch einmal.",
	"frage.nachricht":            "Nachricht",
	"frage.nachricht.an":         "Nachricht an %s (wenn du magst)",
	"frage.dir":                  "die Person, die dir den Link geschickt hat",
	"frage.fehler":               "Ein paar Angaben passen so noch nicht. Was fehlt, steht beim Feld.",
	"frage.form.name":            "Ein Name hat nur Buchstaben, Leerzeichen, Bindestriche und Apostrophe.",
	"frage.form.nummer":          "So ist es keine Telefonnummer: Ziffern mit Vorwahl, etwa 0171 2345678 oder +49 171 2345678.",
	"frage.form.mail":            "So ist die E-Mail-Adresse nicht gültig, etwa name@beispiel.de.",
	"frage.form.strasse":         "Eine Straße hat nur Buchstaben, Ziffern, Leerzeichen und . , - ' / ( ).",
	"frage.form.hausnummer":      "Eine Hausnummer beginnt mit einer Ziffer, etwa 5, 12a oder 3-5.",
	"frage.form.plz":             "So sieht hier keine Postleitzahl aus; in Deutschland sind es fünf Ziffern.",
	"frage.form.ort":             "Ein Ort hat nur Buchstaben, Leerzeichen und . - ' / ( ).",
	"frage.form.land":            "Ein Land hat nur Buchstaben, Leerzeichen und Bindestriche.",
	"frage.form.tag":             "Der Tag als Zahl, 1 bis 31.",
	"frage.form.jahr":            "Das Jahr mit vier Ziffern, oder leer.",
	"nummer.andere":              "andere",
	"art.foto":                   "Foto",
	"art.notiz":                  "Notiz",
	"antwort.nachricht":          "Nachricht von %s",
	"antwort.notiz":              "An die Notiz des Kontakts anhängen",
	"antwort.haupt":              "Haupt-Nummer",
	"frage.plz":                  "PLZ",
	"frage.ort":                  "Ort",
	"frage.land":                 "Land",
	"frage.geburtstag":           "Tag, Monat, Jahr (TT.MM.JJJJ)",
	"frage.senden":               "Änderungen senden",
	"frage.stimmt":               "Alles stimmt so",
	"frage.sendet":               "Wird gesendet …",
	"frage.bis":                  "Der Link gilt bis %s und nur einmal.",
	"frage.danke":                "Danke!",
	"frage.danke.text":           "Deine Angaben sind angekommen. Bevor sich im Adressbuch etwas ändert, sieht sie sich jemand an. Du kannst die Seite jetzt schließen.",
	"frage.ungueltig":            "Link ungültig oder abgelaufen",
	"frage.ungueltig.text":       "Frag bei der Person nach, die dir den Link geschickt hat.",
	"frage.zu-viele":             "Zu viele Versuche",
	"frage.zu-viele.text":        "Bitte warte ein paar Minuten und versuch es dann noch einmal.",
	"frage.zu-gross":             "Das waren zu viele Angaben auf einmal. Lass ein paar leere Felder weg und schick es noch einmal.",
	"frage.datum":                "Der Geburtstag ist so kein Tag im Kalender: Tag und Monat als Zahl, das Jahr mit vier Ziffern oder leer.",
	"frage.nicht":                "Das ließ sich gerade nicht speichern. Versuch es in ein paar Minuten noch einmal.",
	"zugang.ohne-name":           "Sag uns bitte, wie du heißt (höchstens 80 Zeichen).",
	"zugang.ohne-kontakt":        "Wie erreichen wir dich? Eine Telefonnummer oder E-Mail-Adresse (höchstens 120 Zeichen).",
	"zugang.zu-lang":             "Die Nachricht ist zu lang, höchstens 300 Zeichen.",
	"zugang.spaeter":             "Gerade geht es nicht, versuch es später noch einmal.",
	"zugang.fremd":               "Bitte schick die Anfrage von dieser Seite aus.",
	"zugang.nicht-da":            "Diese Seite gibt es nicht.",
	"zugang.meldung":             "Zugangsanfrage von %s (%s)%s",
	"zugang.anfragen":            "Zugangsanfragen",
	"zugang.anfrage":             "Zugangsanfrage von %s",
	"zugang.keine":               "Gerade fragt niemand nach Zugang.",
	"zugang.erledigt":            "Erledigt",
	"zugang.hinweis":             "Wer die Box ohne Anmeldung aufruft, kann hier um Zugang bitten. Melde dich bei der Person; ein Login legst du direkt an der Box an. „Erledigt“ nimmt die Anfrage von der Startseite.",
	"art.vorname":                "Vorname",
	"art.nachname":               "Nachname",
	"art.firma":                  "Firma",
	"art.position":               "Position",
	"nummer.mobil":               "mobil",
	"nummer.privat":              "privat",
	"nummer.geschaeftlich":       "geschäftlich",
	"frage.nummer-art":           "Art",
	"frage.tag":                  "Tag",
	"frage.monat":                "Monat",
	"frage.jahr":                 "Jahr (wenn du magst)",
	"frage.beruf":                "Firma und Position",
}
