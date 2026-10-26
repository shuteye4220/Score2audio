# The score2audio score file (.s2a)

A `.s2a` file holds one score that score2audio has already read, so it can be played elsewhere (the mobile app, or score2audio on another computer) without reading the PDF again. score2audio makes one with "Export for mobile", and opens one like any other score file.

It is an ordinary zip file containing:

- `score.json` – everything needed to play the score, described below.
- `musicxml/score.mxl` (or `.musicxml`) – the recognised music as MusicXML, for other music programs and for future versions of score2audio. A score that Audiveris split into movements has more files (`part2.mxl` and so on), in order.

Readers should ignore files and JSON fields they do not know about, so the format can grow.

## score.json

| Field | Meaning |
|---|---|
| `format` | Always `"score2audio-score"`. |
| `formatVersion` | `1`. A reader should refuse files with a higher number. |
| `appVersion` | The score2audio version that exported it. |
| `exported` | When it was exported (ISO 8601, UTC). |
| `title` | The score's title. |
| `sourceFile` | The name of the PDF or MusicXML file it was read from. |
| `skippedPages` | PDF pages that had no readable music (optional). |
| `musicxml` | Paths of the MusicXML files inside the zip. |
| `model` | The playback model (below). |
| `prefs` | The user's choices for this score (below), or `null`. |

## model

All times are in quarter notes from the start of the score, ignoring repeats. Measures are numbered from 0 in `mi` fields; `n` is the number printed on the page.

- `version` – the model version (internal; a reader may rebuild from the MusicXML if it prefers).
- `title`
- `measures` – one per measure, in printed order: `n` (printed number, a string), `t` (start), `d` (length), `beats`, `beatType`, and `sys: true` on measures that start a new line of music.
- `order` – the measures to play, as indexes into `measures`, with repeats and first and second endings expanded.
- `hasRepeats` – whether `order` differs from playing straight through.
- `tempo` – tempo changes: `t` and `bpm` (quarter notes per minute). The first entry is at 0.
- `tempoFromScore` – false when no tempo marking could be read (the first tempo is then 90).
- `lines` – the lines a user can choose, each with:
  - `id` – stable within the score, for example `p2s1l` (part 2, staff 1, lower notes).
  - `name`, and `hint` such as "usually bass" (optional).
  - `kind` – `voice`, `instrument`, `keyboard` or `other`.
  - `part` – `single` (the whole staff), or `upper`, `lower` and `all` for a staff holding two parts.
  - `primary` – lines marked primary together cover every staff once; "Choose every line" picks these.
  - `default` – switched on when the score is first opened.
  - `clef`, `low`, `high` (lowest and highest note names, as printed), `count` (number of notes).
  - `transpose` – semitones from written to sounding pitch suggested by the label (for example −2 for a B-flat clarinet), and `transposeNote` explaining it (optional).
  - `events` – the notes, in time order: `t` (start), `d` (length), `m` (MIDI note number as written; add the line's pitch shift to play), `mi` (measure index), `name` (for example "E flat 4"), `dur` (for example "dotted half"), and `ly` (lyric syllable, optional). Tied notes are joined into one event.
- `verses` – lyrics, each with `name`, `words` (`t`, `mi`, `text`; one entry per word, at the time its first syllable starts) and `lines` (`from` and `to` measure indexes and `text`, one per printed line of music).
- `warnings` – messages to show the user.

To play: walk `order`; each measure in it starts where the previous one ended, and each event in that measure (`mi`) sounds at the measure's play time plus (`t` − the measure's `t`). Convert quarter notes to seconds with `tempo`, scaled by the user's chosen tempo divided by the first tempo.

## prefs

May be `null`. When present:

- `opts` – `tempo` (beats per minute; a beat is the time signature's beat), `from` and `to` (printed measure numbers), `loop`, `countIn`, `metronome`, `repeats`, `sayMeasures`, `sayLyrics`, `verse` (index), `notesLine` (line id).
- `lines` – by line id: `enabled`, `volume` (0–100; played as (volume/100)²), `sound` (`piano`, `organ`, `flute` or `reed`), `shift` (semitones added to every note), `label` (the user's own name for the line, or empty).

## Limits

Score files can come from anyone, so programs reading them should treat every value as untrusted. score2audio (on the computer and on Android) applies these limits; values outside them are corrected or the entry is left out:

- The zip may unpack to at most 300 MB in total, 100 MB per entry, and hold at most 64 MusicXML files. A compressed MusicXML (`.mxl`) may unpack to at most 64 MB.
- At most 20,000 measures, and 20,000 entries in `order`, whose measures may add up to at most 200,000 quarter notes. `order` entries must be valid measure indexes.
- Measure lengths (`d`) and note lengths are limited to 256 quarter notes, and all times to 1,000,000 quarter notes; none may be negative, and all must be ordinary finite numbers.
- `beats` must be from 1 to 64 and `beatType` one of 1, 2, 4, 8, 16, 32 or 64 (otherwise 4/4 is assumed).
- Tempos must be above zero and are limited to 10–600 beats per minute; at most 10,000 tempo changes.
- At most 256 lines of 200,000 notes each; `m` must be 0–127 and `mi` a valid measure index.
- Lyric words and lines must refer to valid measures. Long texts are shortened.
