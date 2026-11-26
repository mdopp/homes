# ADR 0004 — The phone is a trigger, Home Assistant runs the automation, a chat sets it up

Applies when the Homes app reports something the phone noticed (an NFC tag
touched, the phone laid on its charger), when someone sets up, changes
or switches off a Home Assistant automation, or when someone asks
for "a rule" in the house that is not a single switch.

- **Status:** Proposed, 2026-11-26. Step 1 (the triggers) is built in the
  PR that adds this ADR. Amended the same day by the owner: no editor of
  building blocks on Homes; automations are set up and changed by
  language, in a chat (§2).
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
needs the owner's yes in homes-android's AGENTS.md. The person turns it on
in the app (Mehr → Auslöser → „Ladestation melden“); it is off by default.

### 2. Automations are set up and changed by language, in a chat

There is no editor on Homes (the owner, 2026-11-26). A person says what
they want, in a chat, and the conversation goes on until the automation is
right:

> „Wenn ich mein Telefon nach 23 Uhr auf den Stand lege, mach das
> Bürolicht aus.“ — „So würde ich es anlegen: Wenn Michaels Telefon
> kabellos zu laden beginnt und es zwischen 23:00 und 05:00 ist, dann
> Büro Licht aus. Bestätige mit 4711.“ — „Lieber schon ab 22:30.“ — …

- **Who writes it** is the Bote's Automat (laib `templates/bote` §5): it
  reads HA's automations, drafts, shows the sentence and writes only with
  the four-digit code; a deleted one is kept for restore. Homes does not
  duplicate it. The chat is wherever the person talks to the assistant:
  Signal, or the Sprechen field on Homes.
- **Changing one** is the same conversation: „Welche Automationen
  gibt es fürs Büro?“, „Schalt die Nachtlicht-Automation bis Montag ab“,
  „Mach daraus 22:30“.
- **The Automat must know the phone's triggers.** Today it knows HA's
  entities, not the tags or `homes_laden`. Laib's Automat gains this
  (a laib change, its own PR): HA's tag registry by name (`tag/list`),
  and the `homes_laden` event with its fields, so „wenn ich den Tag an der
  Bürotür berühre“ becomes a tag trigger on that tag's id and „wenn mein
  Telefon auf den Stand kommt“ an event trigger on `homes_laden` with
  the person's uid.
- **Homes shows the way in, not an editor.** After a tag is written, and
  on the app's Auslöser screen, a line says what to tell the assistant:
  „Sag Homes: Wenn ich den Tag ‚Bürotür‘ berühre, …“ (the assistant's
  name from `internal/assistent`), and on Homes it opens the Sprechen
  field with that start filled in.

## Consequences

- Step 1 needs no new key between services: Homes' HA user is already an
  administrator (`HOMES_HA_TOKEN`), and firing events and the tag registry
  are what it uses. laib `docs/DATA.md` gains one flow: a person's phone
  says to HA when it was charged and which tags it touched, under the
  person's uid, visible in HA's logbook to HA's administrators.
- Who may change the house's automations is the Automat's rule, as today:
  every write is confirmed with its code in the chat, and the Bote talks
  only to the people in box.yml `people`.
- The app grows one screen (Auslöser: write a tag, name it, the charger
  switch) and one activity that a tag opens, answers in one line and
  closes.

## Open questions for the owner

1. **The charger report with the screen off** (§1): yes or no.

## Steps

1. Triggers: `/napi/ausloeser` (+ `/tag`, `/tags`), the app's NFC activity,
   the Auslöser screen, the charger report. *(this PR, and homes-android)*
2. Laib's Automat knows the tags and `homes_laden`. *(mdopp/laib)*
3. The line „Sag Homes: …“ after a tag is written and on the Auslöser
   screen, opening the Sprechen field on Homes.
