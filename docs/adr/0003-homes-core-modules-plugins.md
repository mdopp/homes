# ADR 0003 — Homes is a small core, modules for building blocks, and plugins it can but need not carry

Applies when you add an area to Homes, when another program (the archive,
the poll, a new app) wants to appear in Homes, or when Homes' code grows
past what the household needs from it every day.

- **Status:** Proposed, 2026-11-15 (for the owner's review)
- **Deciders:** the owner (mdopp), 2026-11-15: „Homes soll anpassbar und
  erweiterbar sein. Nicht alles gehört auch dort in den Kern. Wenn möglich
  ist es eine Art Plugin. Etwas, das das sein kann, aber nicht muss.“ and
  „die Abstimmung hat nichts mit Homes zu tun, es ist eine eigene App“.
- **Relates to:** ADR 0001 (modules), laib ADR 0035 (three layers; layer 2
  without the OS).

## Context

ADR 0001 made Homes' areas modules, compiled in with an explicit list,
because nothing listed what Homes is made of. Since then Homes has grown
(pages, connectors, household), and programs of its own kind appeared
beside it in layer 2: the archive, the poll, the Bote, Frage. Some of them
already show something in Homes (the archive's counts), each in its own
way.

## Decision

1. **Homes' core is small:** login and people, Start, navigation, „Deine
   Aufgaben“, the app's pairing, and the plugin contract. It is the only
   part every Homes has.
2. **Built-in modules** stay as ADR 0001 describes: thin views over a
   building block (devices over Home Assistant, photos over Immich,
   calendar over Radicale, documents over Paperless, music, energy,
   printers). A module is on only when its building block is on the box,
   and it reaches the block only through the block's API (laib ADR 0035).
3. **Plugins are programs of their own** in layer 2, with their own repo,
   address and login. They work without Homes. They *may* register with
   Homes through one contract and then appear in it:
   - a name, an icon and the link to the program;
   - optionally a Start card, tasks for „Deine Aufgaben“, and app widgets,
     each served by the plugin over HTTP in a fixed small format;
   - Homes renders what is registered and links into the plugin for
     everything else. It holds none of the plugin's data except the tasks
     a plugin hands over: those Homes keeps with their text, so they show
     while the plugin is closed (for the archive: while the vault is
     locked).
   A plugin registers through Laib (a value or a door verb), not by Homes
   knowing it; removing the plugin removes its entry.
4. **The archive is the first plugin** (its counts and tasks already work
   this way); the poll may register, and need not.
5. **Large modules can become plugins** later (faces, printers, energy)
   when that keeps Homes' core small; each such move is its own decision.

## Consequences

- Homes' core shrinks over time; the plugin contract is versioned and
  tested in Homes and in each plugin.
- A household without some program sees no trace of it; a plugin that is
  down shows a quiet line, never breaks Homes.
- Nothing in a plugin's data passes through Homes except what the contract
  names (a title, a count, a task).
