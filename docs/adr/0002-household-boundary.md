# ADR 0002 — Homes shows the household's shared state; what one person may see, or what infers new facts, lives with its data

Applies when you add something to Homes that reads a person's private data
(their photos' faces, their messages, their mail), that infers facts nobody
entered (who is close to whom, how two people are related, which group
someone belongs to), or that shows an outsider how the household recorded
them. Also applies when you review an existing Homes page against this rule.

- **Status:** Accepted, 2026-11-10. The cut of the form's „Beziehungen“
  and self-entered groups are built in the PR that adds this ADR. The
  move of the contact network: on 2026-11-11 (owner) Homes' page
  `/kontakte/netz`, *Steht nahe* and the photo read behind them were
  removed; Kontakte and a contact's page link to the archive („Dein
  Netzwerk im Archiv ›“). A contact's *Beziehungen* stay for now.
- **Deciders:** the owner (mdopp), 2026-11-10, after an analysis that
  ended: „Homes ist nicht mehr ‚dein Zuhause klar im Blick‘, sondern
  Sherlock mit einem Lichtschalter.“
- **Relates to:** ADR 0001 (modules), laib ADR 0014 (three programs),
  laib ADR 0017 (data in standard backends), mdopp/archiv (the person's
  message archive, behind the person's vault).

## Context

Homes is the family's surface: the switches, the shopping list, the
calendar, the shared address book. Within a few days (homes#329, #330,
#331) it also grew a contact network (`internal/netz`): „Steht nahe“ on a
contact's page, the graph on `/kontakte/netz`, questions about how two
people are related, suggestions from the key owner's Immich photos, and a
„Beziehungen“ section on the public contact form („Daten prüfen lassen“,
`kontakt_frage.go`, `internal/kontaktantwort`).

That network reads one person's private data (the faces in their photos)
and infers facts nobody entered (who stands close to whom). The form's
section went further: it showed an outsider, on a page without a login
behind a link, how the household had recorded their relations.

## Decision

**The boundary rule (owner, 2026-11-10):** „Homes zeigt den gemeinsamen
Stand des Haushalts und ändert ihn mit einem Tipp. Was nur eine Person
sehen darf oder neue Fakten erschließt, gehört zu dem Dienst, dem die Daten
gehören; Homes bekommt dorthin einen Link.“

Under that rule the owner decided, 2026-11-10:

1. **The contact network moves into the person's message archive**
   (mdopp/archiv, behind the person's vault): „Steht nahe“, the
   `/kontakte/netz` graph, the relation questions, and what was planned on
   top of them (a pair page, group suggestions, chats and mail). That move
   is a later PR; `internal/netz` stays in Homes until then.
2. **Homes keeps a read-only Immich key**; the archive gets its own.
3. **Self-entered groups live in Homes** as vCard `CATEGORIES` on the
   shared contact cards („Stammtisch“, „Kickermannschaft Renegades“; a
   person can be in several). A group is shared household state, entered by
   hand, changed with one tap: Homes' kind of thing. Suggestions of groups
   come from the archive later.
4. **The form's „Beziehungen“ is cut.** The public contact form no longer
   shows or asks for relations. An answer stored while it was there still
   loads; its relation changes are dropped.

## What moves later, and what stays

Moves to the archive (a later PR, with the archive's side first):

- `internal/netz` and its pages: „Steht nahe“, `/kontakte/netz`,
  „Beziehungen klären“ and its task on Deine Aufgaben, the relation
  section on a contact's page that edits the net's answers.
- The `gesichter` question flows that infer facts from one person's photos
  („Ist das …?“, „Dieselbe Person?“, „Wer ist das?“) as far as they serve
  the network rather than the shared address book.

Stays in Homes:

- Writing `RELATED` into a shared card (`caldav.Korrektur`, Prop
  `RELATED`) as a plain vCard property, whoever decided it.
- The phone sync of relations as plain vCard
  (`gesichter.HandyAenderung` Mit/Art, `kontaktantwort.Store.Handy`), so a
  relation reaches the person's own phone contacts.
- Groups (`CATEGORIES`): shown on a shared card's page and above
  Kontakte, edited there by who may correct a shared card on its page
  (an admin, as „Daten prüfen lassen“).
- The household's contact photos in Immich (owner 2026-11-11, homes#352):
  the photos on the household's shared cards go into one archived Immich
  album, and *Wer ist das?* offers a shared card's name for the face Immich
  matches. The owner confirms it with a tap. Only household cards are read
  and only household names are offered, never a person's phone book or
  archive. The owner accepted that faces of people outside the family then
  live in Immich. This needs write permissions on Homes' key (an upload, an
  album, a delete), beyond the read-only key of point 2.

## Candidates for a later review

Named under the rule, **not decided**; each needs its own look and the
owner's word:

- *Box-Auslastung* and *Befunde*: the box's state is Laib's; Homes may only
  need a link.
- *Kontakt prüfen lassen*: an outsider's answer about themselves, kept in
  Homes; is that the household's shared state or the person's?
- *Discogs*: one person's collection and wantlist.
- Parts of *Dokumente*: what only one person may see among the household's
  documents.

## Consequences

- Homes loses the „Beziehungen“ section of the public form, its parsing,
  its review rows and the code only it used (`netzUebernehmen`,
  `formBeziehungen`, `netz.Uebernehmen`). A link's answer kept from before
  loads with its relation changes dropped; nothing pending breaks.
- Homes gains groups: `caldav` writes one `CATEGORIES` value at a time
  (`korrigiertGruppe`), `haushalt.Gruppen` reads them, the contact page
  and Kontakte show them. Phones that sync the shared address book see
  `CATEGORIES` as the card carries it; Android's own contact groups need an
  app change (homes-android), not part of this decision's first PR.
- A feature that reads one person's private data or infers new facts is
  built in the service that owns that data; Homes links there. A reviewer
  can point at this ADR.
