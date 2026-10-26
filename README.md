# score2audio

score2audio turns a PDF of sheet music into sound. It reads the printed music, splits it into its parts, and plays any combination of them with correct pitch and timing. It works with any kind of score: choir music, songs, band and orchestra parts, chamber music, piano music. Parts are named from the labels printed at the start of each staff, such as "Soprano", "Violin I" or "Clarinet in B♭".

It was made so that musicians who cannot read print music can learn their parts. The whole program is used through an accessible web page that works with screen readers such as NVDA, JAWS, Narrator, VoiceOver and Orca, and with the keyboard alone. The page aims to meet WCAG 2.1 level AA.

score2audio runs on Windows, macOS and Linux. Everything happens on your own computer; your scores are not sent anywhere.

## What you can do

- Open a PDF score (or a MusicXML file) and have it read automatically.
- Choose any combination of lines to play, with a volume control for each, so you can make your own part louder than the others.
- Lines are grouped into voices, instruments and keyboards, with buttons to choose a whole group at once.
- When a staff holds two parts (for example sopranos and altos, or first and second flutes), play its upper notes, its lower notes, or all of it.
- Choose a sound for each line: piano, organ, soft flute or clarinet. Organ and flute hold each note steadily, which makes note lengths easy to hear.
- Transposing instruments are played at sounding pitch: a part labelled "Clarinet in B♭" automatically sounds a whole step lower than written, "Horn in F" a fifth lower, and so on. Each line's Pitch setting can change this, or move a line up or down by octaves.
- Set the tempo, play a range of measures, repeat a passage until you stop it, count in a measure, and add metronome clicks.
- Hear the starting notes of the chosen lines, lowest first, then together.
- Read the lyrics, line by line as they appear on the page, with a "Play from measure" button for each line.
- Read the notes of any line as a table, one row per measure (for example "F 3, half; E flat 3, quarter"), and play any single measure.
- Save the chosen lines as a WAV audio file to practise on a phone or other player, with the "Save as audio file" button next to Play.
- Export a score that has been read as a score2audio score file (`.s2a`), with "Export for mobile". It holds the recognised music and your line choices, so it plays on the score2audio mobile app, or on another computer, without reading the PDF again. Anyone you share it with does not need Audiveris.
- Keep a library of the scores you have opened, so each PDF only needs to be read once. Each score remembers your line, volume and tempo choices.

## Installing

score2audio needs Audiveris, a free program that reads music from PDF files. Install it first, from https://github.com/Audiveris/audiveris/releases (under the newest release, choose the file for your system as described below). MusicXML files can be opened without it.

Then download score2audio for your system from the Releases page of this project.

### Windows

1. Install Audiveris: download the file whose name ends in `windowsConsole-x86_64.msi` and run it. The default choices are fine.
2. Download `score2audio-windows.zip` and unzip it into a folder of your choice. Keep the `tessdata` folder next to `score2audio.exe`; it is used to read lyrics.
3. Start `score2audio.exe`. A small console window opens and your web browser opens the score2audio page. If the browser does not open, the console window shows the address to open.

Windows may warn that the program is from an unknown publisher, because it is not signed. Choose "More info", then "Run anyway".

### Mac

1. Install Audiveris: download the file ending in `macosx-arm64.dmg` for a Mac with Apple silicon (M1 or newer), or `macosx-x86_64.dmg` for an Intel Mac. Open it and drag Audiveris to your Applications folder.
2. Download `score2audio-mac.zip` and unzip it. You get a `score2audio` folder; move it wherever you like.
3. In that folder, open `start-score2audio.command`. The first time, macOS will say it cannot verify the developer. Choose Done, then open System Settings, Privacy and Security, and near the bottom choose "Open Anyway" next to the message about start-score2audio.command. After that it opens normally.

A Terminal window opens and shows score2audio's messages, and your web browser opens the page. The same download works on both Apple silicon and Intel Macs.

### Linux

1. Install Audiveris from your distribution (for example the `audiveris` package in the Arch User Repository), from Flathub, or from the `.deb` file on the Audiveris downloads page for Debian and Ubuntu.
2. Download `score2audio-linux-x86_64.tar.gz`, unpack it, and run `./score2audio` from the `score2audio` folder. Or build it yourself; see Building from source.

If score2audio cannot find Audiveris, the Setup section of the page explains what to do, and lets you type where Audiveris is installed.

## Using it

The page is divided by headings, so you can move between sections with your screen reader's heading keys.

1. Under "Open a score", choose the PDF file and press "Read score". Reading takes roughly 10 to 20 seconds per page. Progress is announced page by page, and the score opens by itself when it is ready.
2. Under "Lines to play", check the lines you want. Each line says what it is, its clef, and its lowest and highest notes. You can give any line your own name, change its sound or pitch, under "More settings".
3. Under "Play", press Play. The same button pauses and continues. Changes to lines, volumes and sounds take effect while the music plays.

The current measure is shown next to the Play button. If you check "Announce measure numbers while playing" or "Announce lyrics while playing", your screen reader will speak them as the music goes; these are off at first because speech can talk over the music.

To quit, use the "Quit score2audio" button at the bottom of the page, or close the console or Terminal window.

## How accurate is it?

Music recognition is good on clean, printed scores such as publishers' editions, but it is not perfect. Occasionally a note, rhythm, tie or lyric syllable is misread, and lyrics and labels are read by text recognition, so some words may be misspelled or missing. If something sounds wrong, check it with a sighted reader. "Read the PDF again" in the score's options repeats the recognition, for example after installing a newer Audiveris.

When a staff has no readable label, score2audio works out what it is: a staff with lyrics is a voice, a pair of staves joined by a brace is a keyboard, and anything else is listed under "Other lines".

Pages without readable music, such as title pages and pages of notes, are skipped, and the page says which pages were left out.

Scanned photocopies, handwritten music and very crowded scores give poorer results.

Repeat signs with first and second endings are followed. "D.S.", "D.C." and "To Coda" directions are not yet followed; play those sections using the measure range instead. Tempo changes written as words, such as "rit." or "A little slower", are not applied.

## Other ways to open music

A score2audio score file (`.s2a`), exported from score2audio on this or another computer, opens straight away, with the line names, volumes and tempo it was exported with. The format is described in [docs/s2a-format.md](docs/s2a-format.md).

If you already have a score as MusicXML (a `.mxl`, `.musicxml` or `.xml` file, for example exported from MuseScore, Finale, Sibelius or Dorico, or downloaded from a music library), open it directly. Audiveris is not needed for these, and the result is exact.

## Building from source

You need Go 1.22 or newer (on Arch Linux: `sudo pacman -S go`; on a Mac with Homebrew: `brew install go`) and, for `make dist`, `curl` and `zip`.

```
make            # build for this computer
make test       # run the tests
make run        # build and start it
make install    # Linux: install into ~/.local, with a menu entry
make dist       # build the Windows, Mac and Linux downloads into dist/
```

The web page lives in the `web` folder and is built into the program. Pushing a tag such as `v1.0.1` to GitHub builds all three downloads automatically and attaches them to a new release.

Settings for testing, as environment variables:

- `AUDIVERIS_EXE` – path to the Audiveris program.
- `SCORE2AUDIO_DATA` – folder for the score library. By default it is `%LOCALAPPDATA%\score2audio` on Windows, `~/Library/Application Support/score2audio` on a Mac, and `~/.local/share/score2audio` on Linux.
- `SCORE2AUDIO_PORT` – a fixed port instead of a random one.
- `SCORE2AUDIO_NO_BROWSER` – do not open a browser.

`score2audio --dump file.mxl` prints what the program understood from a MusicXML file.

## How it works

1. Audiveris reads the PDF and writes MusicXML, a standard text format for sheet music.
2. score2audio reads the MusicXML, works out timing, ties and repeats, joins up parts that Audiveris split because their labels are abbreviated later in the score ("Flute" on the first line, "Fl." after), identifies each part from its label, and splits staves that hold two parts.
3. The web page plays the lines with sounds made by your browser, tuned to A = 440 Hz.

## Licence and credits

score2audio is released under the MIT licence; see the LICENSE file.

- [Audiveris](https://github.com/Audiveris/audiveris), the open source music recognition program (AGPL licence), installed separately.
- [Tesseract](https://github.com/tesseract-ocr/tessdata) English language data, used by Audiveris to read lyrics and labels (Apache 2.0 licence), included in the downloads.
