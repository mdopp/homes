# ADR 0004 — The phone is a trigger, Home Assistant runs the automation, Homes is where the family sets it up

Applies when the Homes app reports something the phone noticed (an NFC tag
touched, the phone laid on its charger), when a page lists, creates,
changes or switches off a Home Assistant automation, or when someone asks
for "a rule" in the house that is not a single switch.

- **Status:** Proposed, 2026-11-26. Step 1 (the triggers) is built in the
  PR that adds this ADR; steps 2–4 wait for the owner's yes on the open
  questions below.
- **Deciders:** the owner (mdopp), 2026-11-26: „Ich habe einen Pixel Stand
  und die Homes App auf meinem Telefon. Und ich könnte weitere NFC Tags im
  Haus verteilen. … wenn ich mein Telefon auf den Stand stelle und es nach
  23h ist, geht mein Bürolicht aus“, and „Ich will die Home Assistant App
  nicht“: the Homes app, not HA's companion app, reports the phone.
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
needs the owner's yes in homes-android's AGENTS.md. The person turns it on
in the app (Mehr → Auslöser → „Ladestation melden“); it is off by default.

### 2. Automations are listed and switched on Homes, in German sentences

A page **Automationen** (under Geräte, admins and the owner see all, every
one else the ones they made or that name them; open question 1) reads HA's
automations (Homes' HA user is an administrator, laib
`templates/home-assistant/post-deploy`; the config API is admin-only) and
shows each as one sentence:

> **Wenn** Michaels Telefon kabellos zu laden beginnt **und** es zwischen
> 23:00 und 05:00 ist, **dann** Büro Licht aus. · *an* · zuletzt gestern
> 23:41

- A switch per automation (`automation.turn_on/off`, the pressed state,
  „läuft …“, the outcome on the row).
- „Löschen“ keeps the config and offers „Rückgängig“ on the page; the
  deleted ones are kept for 30 days (`HOMES_DATA/automationen/geloescht`),
  as the Automat keeps its own.
- An automation Homes cannot say in its words (a template, a script,
  choose/repeat) is listed with HA's alias and „In Home Assistant
  bearbeiten ›“; Homes never shows YAML.

### 3. New and changed automations from building blocks, no YAML

„Neue Automation“ is three steps, each a short list to tap, the phone
layout first:

1. **Wenn** — a trigger: *NFC-Tag berührt* (the household's tags), *Telefon
   auf die Ladestation / von der Ladestation* (the person's phone),
   *Uhrzeit*, *Sonnenauf-/untergang*, *ein Gerät geht an/aus* (the
   devices Geräte shows), *jemand kommt / geht* (HA's persons, when
   present).
2. **Und** (optional, any number) — *zwischen … und … Uhr*, *nur an
   Werktagen*, *ein Gerät ist an/aus*, *jemand ist zu Hause*.
3. **Dann** — one or more actions: *Gerät an/aus/umschalten*, *Helligkeit*,
   *Szene*, *Mitteilung an …* (`rest_command.mitteilung`, the Bote's
   delivery, laib `templates/bote` §6).

Before saving, Homes shows the sentence it will save; „Speichern“ writes
it to HA (`POST /api/config/automation/config/homes_<id>`, the id starts
with `homes_` so the page knows its own), and the row says „gespeichert,
an“. Changing one opens the same three steps filled in. Every block is a
small Go value with its HA form and its German sentence, both ways, in a
household package (`internal/automation`); one test per block pins both.

**Ready-made ones** are offered where they fit the box: after a tag is
written, „Was soll der Tag tun?“ opens step 3 with step 1 filled in; on
the phone's Auslöser screen, „Wenn das Telefon nachts auf die Ladestation
kommt …“ opens it with *Telefon auf die Ladestation* and *zwischen 22:00
und 05:00*.

### 4. A sentence still works, through the Bote

The Sprechen field and Signal stay the way to say an automation in one
sentence. The Bote's Automat writes it, with its code. Homes does not
duplicate the Automat's language understanding; its page lists the
result like every other automation.

## Consequences

- Step 1 needs no new key between services: Homes' HA user is already an
  administrator (`HOMES_HA_TOKEN`), and firing events and the tag registry
  are what it uses. laib `docs/DATA.md` gains one flow: a person's phone
  says to HA when it was charged and which tags it touched, under the
  person's uid, visible in HA's logbook to HA's administrators.
- Steps 2 and 3 give every person who may see the page the power to change
  the house's behaviour. That is a household decision (open question 1),
  not a technical one.
- The app grows one screen (Auslöser: write a tag, name it, the charger
  switch) and one activity that a tag opens, answers in a toast-sized
  line and closes.

## Open questions for the owner

1. **Who may create and change automations on Homes?** (a) only admins,
   (b) everyone, each only their own and the ones that name them, (c)
   everyone, all. Recommendation: (b).
2. **The charger report with the screen off** (above): yes or no.
3. **Step 2 and 3 now, or after a week with the triggers alone** (the
   Bote writes the first automations by sentence meanwhile)?
   Recommendation: a week first, to see which blocks are used.

## Steps

1. Triggers: `/napi/ausloeser` (+ `/tag`, `/tags`), the app's NFC activity,
   the Auslöser screen, the charger report. *(this PR, and homes-android)*
2. The Automationen page: list, sentence, switch, delete with undo.
3. The building blocks: new and change.
4. Ready-made ones after a tag and on the Auslöser screen.
