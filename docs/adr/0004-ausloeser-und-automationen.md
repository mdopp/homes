# ADR 0004 — The phone is a trigger, Home Assistant runs the automation, the Werkstatt sets it up by language

Applies when the Homes app reports something the phone noticed (an NFC tag
touched, the phone laid on its charger), when someone sets up, changes
or switches off a Home Assistant automation, or when someone asks
for "a rule" in the house that is not a single switch.

- **Status:** Accepted, 2026-11-26 (the owner: „Go. Passt alles“). Step 1 (the triggers) is built in the
  PR that adds this ADR. Amended the same day by the owner: no editor of
  building blocks on Homes; automations are set up and changed by
  language, in the Werkstatt (§3); and how tags are named (§2).
- **Deciders:** the owner (mdopp), 2026-11-26: „Ich habe einen Pixel Stand
  und die Homes App auf meinem Telefon. Und ich könnte weitere NFC Tags im
  Haus verteilen. … wenn ich mein Telefon auf den Stand stelle und es nach
  23h ist, geht mein Bürolicht aus“, and „Ich will die Home Assistant App
  nicht“: the Homes app, not HA's companion app, reports the phone; and
  „Zu dem Editor: ich würde hier eher auf Sprache setzen und wir tauschen
  uns dann dazu über Chats aus“.
- **Relates to:** ADR 0001 (modules: this is the `geraete` module),
  ADR 0003 (built-in modules are thin views over their building block),
  laib ADR 0017 (data in standard backends), laib ADR 0022 (the app is a
  widget client), laib `templates/bote` (the Automat: automations by
  sentence, confirmed with a code).

## Context

Home Assistant already runs the house's automations, and the Bote's
Automat already writes them from a sentence („sag mir Bescheid, wenn …“),
each write confirmed with a four-digit code. What is missing:

1. **The phone as a trigger.** HA learns about a phone only through its own
   companion app, which the family does not want beside the Homes app.
   The Homes app is paired, has a token, and already runs a foreground
   service; it can report an NFC tag and the charger itself.
2. **A place to see and change automations.** HA's automation editor is an
   administrator's tool; the Bote's sentence works for a new one but is a
   poor way to look at fifteen existing ones, switch one off for the
   holidays or move 23:00 to 22:30.

## Decision

### 1. The phone reports, Homes passes it to HA as an event, HA decides

The app reports two things to `POST /napi/ausloeser` (module `geraete`):

| what | body | HA event fired by Homes |
|---|---|---|
| an NFC tag touched | `{"art":"nfc","tag":"<id>"}` | `tag_scanned` `{tag_id, person}` |
| laid on / taken off the charger | `{"art":"laden","zustand":"an"\|"aus","ladeart":"kabellos"\|"kabel"\|"usb"\|""}` | `homes_laden` `{person, zustand, ladeart}` |

- `person` is the uid behind the device's bearer. Nothing else about the
  phone leaves it (not its name: one person, one phone is the household's
  case, and an automation that needs more says so later).
- NFC uses HA's own `tag_scanned`, so an automation uses HA's ordinary
  **tag trigger** and a tag appears in HA's tag list with its name.
- The charger has no HA entity of its own (that would need the companion
  app's sensor machinery); it is an event. An automation triggers on
  `homes_laden` with `event_data: {person: …, zustand: an, ladeart:
  kabellos}` and adds its own conditions (the time, who is home).
- **Homes does not decide.** It runs no rule of its own, keeps no state of
  the phone, and does not filter: it fires the event and says what it did.
  Every "if" lives in HA, where the other automations are.
- The reply says what the person will want to know on the phone:
  `{"ok":true,"name":"Bürotür"}` for a tag HA knows, `{"ok":true,
  "name":""}` for one it does not (the app says „Unbekannter Tag – auf
  Homes benennen“). A failure is `{"ok":false,"error":"<a German
  sentence>"}` with 4xx/5xx, as everywhere in `/napi`.

**Tags are named in HA.** `POST /napi/ausloeser/tag {"name":"Bürotür"}`
makes a new id (`homes-` and 12 hex digits), creates it in HA's tag
registry (`tag/create`) and answers `{"ok":true,"tag":"<id>","uri":
"cloud.dopp.homes://tag/<id>"}`; the app writes that URI onto the tag
(one NDEF URI record). `GET /napi/ausloeser/tags` lists HA's tags
`{"tags":[{"tag","name"}]}`. The names stay in HA (laib ADR 0017): Homes
stores nothing.

**Why the app's scheme on the tag.** Android opens the Homes app for its own
scheme without a chooser, also from the lock screen's "unlocked" state;
an `https://` URL would open the browser on a phone without the app. A tag
written by HA's companion app (`https://www.home-assistant.io/tag/<id>`)
is read too: the app takes the id after `/tag/`, so tags already in the
house keep working.

**The charger, honestly.** Android says *that* the phone charges and *how*
(wireless, cable, USB); it does not say *which* charger. „Auf dem Pixel
Stand“ is „lädt kabellos“; a second wireless charger in the house looks the
same. Where that matters, the automation adds a condition, or the person
puts a tag next to the stand instead. The app reports from its live
service (`realtime/LiveService.kt`), which already runs; it registers for
`ACTION_POWER_CONNECTED`/`DISCONNECTED` while it runs, and sends the one
request even with the screen off. That is a second exception to the app's
"no network while the screen is off" (the first is Daten sichern) and
the owner said yes to on 2026-11-26 (homes-android's AGENTS.md). The person turns it on
in the app (Mehr → Auslöser → „Ladestation melden“); it is off by default.

### 2. Naming tags: one name, in HA, given where the tag is in your hand

The owner, 2026-11-26: „wir brauchen ein Konzept wie ich NFC Tags benennen
kann“. A tag's name is what the person says in the Werkstatt („der Tag an
der Bürotür“) and what the phone says when it is touched („Bürotür –
gemeldet“), so it must be easy to give, to change and to find.

- **One place.** The name lives in HA's tag registry (`name` of the tag's
  id), nowhere else: HA's automations show it, the Werkstatt reads it, the
  app shows it. Homes stores nothing (laib ADR 0017).
- **Named before or while it is written.** On the app's Auslöser screen:
  „Neuen Tag beschreiben“ → the name first (a field, with the house's
  rooms from HA as suggestions: „Büro“, „Bad“, and a few places: „Tür“,
  „Bett“, „Schreibtisch“, „Ladestation“) → „Halte den Tag ans Telefon …“ →
  written and named, one line: „‚Bürotür‘ ist fertig. Kleb ihn hin und
  sag in der Werkstatt, was er tun soll.“ The id is never shown.
- **A tag that has an id but no name** (written by HA's companion app, or
  named nowhere): touching it answers „Unbekannter Tag“ with a button
  „Benennen“, which opens the same name field and names *that* id; the tag
  is not written again, so an automation that already uses it keeps
  working.
- **A blank tag** (nothing on it) does nothing while the app is closed —
  Android does not open an app for it. With the Auslöser screen open it is
  offered for „Neuen Tag beschreiben“.
- **Rename** on the Auslöser screen: tap a tag, change the name, „Sichern“.
  The id stays, so every automation keeps working; only HA's name changes.
- **Remove**: „Entfernen“ deletes the name and the id from HA after one
  question („Automationen, die ‚Bürotür‘ nutzen, laufen dann nicht mehr“);
  the tag itself can be written again as a new one.
- **Rules for a name:** 1–60 characters, trimmed; unique among the tags,
  case aside („Es gibt schon einen Tag ‚Bürotür‘ – nimm einen anderen
  Namen“), because a name that is said must find one tag.
- **On the tag itself**: the app suggests writing the name on the sticker;
  the name is not stored on the tag (it would go stale on a rename).

Server, additively: `POST /napi/ausloeser/tag` takes an optional `"tag"`:
without it a new id is made (as now), with it that id is named or renamed
(`tag/create` when HA lacks it, else `tag/update`); `DELETE
/napi/ausloeser/tag/{tag}` removes it (`tag/delete`). Duplicate names are
refused with 409 and the sentence above.

### 3. Automations are set up and changed by language, in the Werkstatt

There is no editor on Homes (the owner, 2026-11-26: „ich würde hier eher
auf Sprache setzen und wir tauschen uns dann dazu über Chats aus. Das
sollte die Werkbank können“ — the Werkstatt, the box's workshop where
the owner talks with Claude Code or pi). The owner says what they want,
and the conversation goes on until the automation is right:

> „Wenn ich mein Telefon nach 23 Uhr auf den Stand lege, soll das
> Bürolicht ausgehen.“ — „So würde ich es anlegen: Wenn Michaels Telefon
> kabellos zu laden beginnt und es zwischen 23:00 und 05:00 ist, dann
> Büro Licht aus. Bestätige mit 4711.“ — „Lieber schon ab 22:30.“ — …

- **The Werkstatt has no way into HA today** (no token, no tool; checked
  2026-11-26). It gets one through Laib, not a token of its own: Laib's
  MCP offers the claude and pi hands the Automat's tools (laib
  `templates/bote` §5: `entitaeten`, `automationen`, `automation_lesen`,
  `automation_schreiben`, `automation_loeschen`, `automation_schalten`),
  plus the tag registry by name and the `homes_laden` event's fields. The
  Automat keeps its HA token; the Werkstatt holds none (Werkstatt rule 5).
- **Every write is confirmed with the Automat's code**, said by the owner
  in the Werkstatt's conversation, as on Signal. Reading needs no code.
  Since this changes the house's behaviour from the Werkstatt, the new
  tool is a Laib change the owner approves (Werkstatt rule 1).
- **The phone's triggers in words.** „Wenn ich den Tag ‚Bürotür‘ berühre“
  becomes HA's tag trigger on that tag's id; „wenn mein Telefon auf den
  Stand kommt“ an event trigger on `homes_laden` with `person` and
  `zustand: an` (and `ladeart: kabellos` for the stand); every condition
  is HA's own (time, weekday, who is home).
- **Changing one** is the same conversation: „Welche Automationen gibt es
  fürs Büro?“, „Schalt die Nachtlicht-Automation bis Montag ab“, „Mach
  daraus 22:30“.
- The Bote's sentence on Signal keeps working for the rest of the family
  (the same Automat).

### 4. A tag can do something of Homes' own: the scanner

The owner, 2026-11-26: „Können wir … das Scannen in meinen persönlichen
Paperless Account (also privat, nicht Familie) starten? Oder einen
weiteren, der das für die Familie macht?“ Homes already starts a stack
(the Scannen buttons on Dokumente, `laib scan`), so a tag is one more
button, not an HA automation (that would need HA to call Homes back, a new
key, and „for whom“ would go round in a circle).

- When a tag is named, the app asks „Was soll der Tag tun?“: *Home
  Assistant melden* (§1, the default), *Scannen – für mich*, *Scannen – für
  den Haushalt* (`aktionen` in `GET /napi/ausloeser/tags`; the scan ones
  only where Homes has Laib).
- **„Für mich“ is always the toucher's**: the stack belongs to the person
  whose paired phone touched the tag (`laib scan start` with `person` and
  `fuer: person`, as the button „für mich“), never to the tag's maker. One
  tag at the scanner serves everyone, and nobody scans into another
  person's Paperless. A phone without a login of its own is refused in
  words. „Für den Haushalt“ sends no `fuer`.
- A tag with an action reports nothing to HA. The phone says „Der Scanner
  startet – nur für dich.“ (or „– für den Haushalt.“, or why not); the
  stack's closing message comes as the buttons' does, on the person's chat
  channel.
- Which tag does what is Homes' behaviour, so Homes keeps it
  (`internal/tagaktion`, `HOMES_DATA/tag-aktionen.json`, tag id → action);
  the name stays in HA. Removing a tag removes its action.

## Amendment 2026-11-26 (evening): Home Assistant's standards, end to end

The owner asked whether this follows standard patterns („Nutzen wir
Standard pattern?“), wanted scanning through Home Assistant („Ich hatte
eher gedacht, dass das Scannen auch über Home Assistant“), an address of
the box's own on the tag rather than home-assistant.io, and a tag that
offers existing automations and new ones. Decided the same evening:
„Finde ich gut“, „1b, aber Bestätigung über Signal-Abfrage an die
Admins, 2 natürlich“. §5–§8 replace the parts of §1 and §4 they name.

### 5. The tag carries the box's own address (an Android App Link)

A new tag holds one NDEF URI record `<HOMES_PUBLIC_URL>/tag/<id>`
(e.g. `https://www.dopp.cloud/tag/homes-3f9a1c7e02b4`), not
`home-assistant.io` and not the app's scheme.

- **With the Homes app** Android opens the app directly: Homes serves
  `/.well-known/assetlinks.json` (the app's package and its signing
  certificate's SHA-256, from `HOMES_APP_SHA256`), the app declares the
  link with `autoVerify`. Nothing goes to anyone but the box.
- **Without the app** (a guest, a laptop, an iPhone) the browser opens
  Homes' page `/tag/<id>` behind the login: it shows the tag's name and
  what it does, and one button („Auslösen“, or the scanner's choice, §8)
  that acts as the signed-in person. A GET never acts (no side effect on
  a link preview); the button posts.
- Tags already written (`cloud.dopp.homes://tag/<id>`, HA's
  `https://www.home-assistant.io/tag/<id>`) keep working.
- `POST /napi/ausloeser/tag` answers the new `uri`; the field's name and
  everything else stay.

### 6. The scanner is a device in Home Assistant (MQTT discovery)

The scan service announces itself to Home Assistant through MQTT
discovery on the box's Mosquitto: a device „Scanner“ with one button
„Scannen – Haushalt“ (`unique_id` `laib-scan-haushalt`), one button per
person whose stacks are theirs alone, „Scannen – <Anzeigename>“
(`unique_id` `laib-scan-person-<login>`), and a sensor for its state
(`unique_id` `laib-scan-zustand`: bereit, scannt, wartet, fertig,
fehler). Pressing a button is `laib scan start` (with `fuer: person` for a
person's button). So scanning is an ordinary Home Assistant action, for
automations, voice and dashboards. This is a Laib change (the scan
template); Homes finds the buttons by their `unique_id` in HA's entity
registry.

### 7. Each paired phone is a device in Home Assistant (mobile_app)

Homes registers each paired phone with Home Assistant's `mobile_app`
integration, the open API HA's own app uses
(`POST /api/mobile_app/registrations`, then the registration's webhook),
under Homes' HA user, the device named after the person and the phone
(„Michaels Pixel 9“). Then:

- a touched tag is HA's own scan (`scan_tag` through the webhook): HA
  records „zuletzt gescannt“ and which device, and `trigger: tag` with
  `device_id` works;
- the charger is two sensors of that device, `binary_sensor.<gerät>_laedt`
  and `sensor.<gerät>_ladeart` (kabellos, kabel, usb, keine), with HA's
  history and ordinary state triggers;
- `homes_laden` and the raw `tag_scanned` go once nothing uses them
  (Homes fires both meanwhile, so no automation breaks during the move).

The webhook ids are secrets Homes keeps in `HOMES_DATA`; a phone
unpaired is a registration removed. The app's requests do not change.

### 8. A tag offers what it can do: the scanner's choice, existing automations, a new one

After naming, „Was soll ‚<Name>‘ tun?“:

- **At the scanner**: *Fragen* (the default: the tag opens Homes' Scannen
  section, „Für mich“ / „Für den Haushalt“ and the stack's progress, in
  the app or the browser), *Direkt für mich* (HA's button of the person
  who touched it, §6), *Direkt für den Haushalt*.
- **Any other tag**:
  - *Mit einer Automation verbinden*: HA's automations by name,
    searchable; Homes adds the tag as one more trigger
    (`trigger: tag`, `tag_id`) to that automation through HA's config
    API (Homes' HA user is an administrator), shows the outcome as a
    sentence first, and keeps the automation's previous config so
    „Lösen“ brings exactly that back. Under each tag: the automations
    that trigger on it (read from HA), each with „Lösen“.
  - *Neue Automation*: one sentence („Was soll passieren, wenn jemand
    ‚Bürotür‘ berührt?“), passed with the tag to the Bote; the Automat
    drafts and writes it with its code (laib `templates/bote` §5).
- **Who**: everyone in the household may (the owner, 1b). A change by an
  admin is confirmed in place. A change by anyone else waits: Homes tells
  every admin on their chat channel (`laib tell`, Signal first) „Anna
  möchte ‚Bürotür‘ mit ‚Gute Nacht‘ verbinden“ with the link to the
  decision on Homes (a task in „Deine Aufgaben“, Freigeben / Ablehnen);
  the first admin's answer counts, the person sees „wartet auf
  Freigabe“, then what came of it. A new automation by a non-admin goes
  to the admins the same way as a wish: the admin continues with the Bote
  (who drafts it with its code for the admin). Nothing waits longer than
  7 days; then it is dropped and the person is told.

## Consequences

- Step 1 needs no new key between services: Homes' HA user is already an
  administrator (`HOMES_HA_TOKEN`), and firing events and the tag registry
  are what it uses. laib `docs/DATA.md` gains one flow: a person's phone
  says to HA when it was charged and which tags it touched, under the
  person's uid, visible in HA's logbook to HA's administrators.
- Who may change the house's automations: the owner in the Werkstatt, and
  the people the Bote talks to (box.yml `people`); every write is
  confirmed with the Automat's code.
- The app grows one screen (Auslöser: write a tag, name it, the charger
  switch) and one activity that a tag opens, answers in one line and
  closes.

## Questions the owner answered

Both answered by the owner on 2026-11-26 („Go. Passt alles“):

1. **The charger report with the screen off** (§1): yes, behind its
   switch, off by default.
2. **The Automat's tools for the Werkstatt** (§3): yes, built in Laib;
   the box changes only when the owner approves it in the Inbox.

## Steps

(§5–§8, 2026-11-26 evening: 5. the tag's own address and App Link; 6. the
scanner as an HA device, in Laib; 7. phones as HA devices; 8. what a tag
does, with the admins' approval on Signal.)


1. Triggers: `/napi/ausloeser` (+ `/tag`, `/tags`), the app's NFC activity,
   the Auslöser screen, the charger report. *(this PR, and homes-android)*
2. Naming (§2): name an existing id, rename, remove, unique names; the
   app's „Benennen“ on an unknown tag, rename and remove on the Auslöser
   screen. *(homes, homes-android)*
3. The Automat's tools in the Werkstatt through Laib, with the tags and
   `homes_laden`. *(mdopp/laib, a proposal)*
4. A tag's action of Homes' own: the scanner, for the toucher or the
   household (§4). *(homes, homes-android)*
