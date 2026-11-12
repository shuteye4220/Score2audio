# score2audio

score2audio turns a PDF of sheet music into sound. It reads the printed music, splits it into its parts, and plays any combination of them with correct pitch and timing. It works with any kind of score: choir music, songs, band and orchestra parts, chamber music, piano music. Parts are named from the labels printed at the start of each staff, such as "Soprano", "Violin I" or "Clarinet in B♭".

It was made so that musicians who cannot read print music can learn their parts. The whole program is used through an accessible web page that works with screen readers such as NVDA, JAWS, Narrator, VoiceOver and Orca, and with the keyboard alone. The page aims to meet WCAG 2.1 level AA.

score2audio runs on Windows, macOS and Linux, and scores it has read can be played on Android with the score2audio app. Everything happens on your own computer; your scores are not sent anywhere.

## What you can do

- Open a PDF score (or a MusicXML file) and have it read automatically.
- Choose any combination of lines to play, with a volume control for each, so you can make your own part louder than the others.
- Lines are grouped into voices, instruments and keyboards, with buttons to choose a whole group at once.
- When a staff holds two parts (for example sopranos and altos, or first and second flutes), play its upper notes, its lower notes, or all of it.
- Choose a sound for each line: piano, organ, soft flute or clarinet. Organ and flute hold each note steadily, which makes note lengths easy to hear.
- Transposing instruments are played at sounding pitch: a part labelled "Clarinet in B♭" automatically sounds a whole step lower than written, "Horn in F" a fifth lower, and so on. Each line's Pitch setting can change this, or move a line up or down by octaves.
- Set the tempo, play a range of measures, repeat a passage until you stop it, count in a measure, and add metronome clicks.
- Hear the starting notes of the chosen lines, lowest first, then together.
- Read the lyrics, line by line as they appear on the page, with a "Play from measure" button for each line, and save them all as a plain text file with "Save lyrics as text".
- Read the notes of any line as a table, one row per measure (for example "F 3, half; E flat 3, quarter"), and play any single measure.
- Save the chosen lines as a WAV audio file (or an MP3 file, when FFmpeg is installed) to practise on a phone or other player, with the "Save as audio file" button next to Play.
- Save the chosen lines as an MP4 video to send to others, with "Save as video": the lines as sheet music that scrolls with the playing, the current measure highlighted, the words under the notes and in large type underneath, and the same sound as the audio file. Choose only the alto line, for example, and the video shows and plays the alto part through the whole piece (or the measures chosen). Nothing else needs installing: the video is made by your web browser. See "Videos" below.
- Export a score that has been read as a score2audio score file (`.s2a`), with "Export for mobile". It holds the recognised music and your line choices, so it plays on the score2audio mobile app, or on another computer, without reading the PDF again. Anyone you share it with does not need Audiveris.
- Keep a library of the scores you have opened, so each PDF only needs to be read once. Each score remembers your line, volume and tempo choices.
- Keep score2audio up to date by itself: new versions are installed automatically (or, if you prefer, when you choose). See "Updates" below.

## Installing

score2audio uses two free programs: Audiveris, which reads the music in PDF files (MusicXML files can be opened without it), and, optionally, FFmpeg, for MP3 files and for video sound that plays everywhere. The Windows installer installs both for you, and on Mac and Linux score2audio's setup does. Each download comes from its publisher and is checked against its known fingerprint (SHA-256) before it is used.

Download score2audio for your system from the Releases page of this project.

### Windows

1. Download `score2audio-windows-setup.exe` and run it. It installs score2audio for you only, so it does not need an administrator, and puts it in the Start menu and in Apps in Settings.
2. Leave "Audiveris and FFmpeg" checked on the Components page, and the installer downloads and installs them (about 200 MB). Windows asks whether to allow the Audiveris installer to make changes; answer Yes. Each step is listed on the installer's progress page.
3. Start score2audio from the Start menu. A small console window opens and your web browser opens the score2audio page. If the browser does not open, the console window shows the address to open.

Windows may warn that the installer is from an unknown publisher, because it is not signed. Choose "More info", then "Run anyway".

To uninstall, use Apps in Settings (or Add or remove programs). Your scores are kept.

If you would rather not install anything, `score2audio-windows.zip` has the same program: unzip it, keep the `tessdata` and `verovio` folders next to `score2audio.exe`, and start `score2audio.exe`. Its Setup section has a button that installs Audiveris and FFmpeg.

### Mac

1. Download `score2audio-mac.zip` and unzip it. You get a `score2audio` folder; move it wherever you like.
2. In that folder, open `Install helpers.command`. The first time, macOS will say it cannot verify the developer. Choose Done, then open System Settings, Privacy and Security, and near the bottom choose "Open Anyway" next to the message about it. It downloads Audiveris (for Apple silicon or Intel, whichever your Mac has) and copies it into the Applications folder in your home folder. FFmpeg is installed too if you have Homebrew; it is optional.
3. Open `start-score2audio.command` (allowing it the same way the first time).

A Terminal window opens and shows score2audio's messages, and your web browser opens the page. The same download works on both Apple silicon and Intel Macs. The Setup section of the page also has a button that installs Audiveris and FFmpeg.

### Linux

1. Download `score2audio-linux-x86_64.tar.gz` (for PCs) or `score2audio-linux-arm64.tar.gz` (for ARM computers such as a Raspberry Pi 4 or 5 running a 64-bit system), unpack it, and open a terminal in the `score2audio` folder.
2. Run `./setup.sh` to install Audiveris and FFmpeg. It uses your distribution's own packages where it has them, and says what it will run and asks before anything that needs your password:
   - Arch Linux and relatives: Audiveris from the AUR with `yay` or `paru` if you have one, FFmpeg with pacman.
   - Debian and Ubuntu (on PCs): the Audiveris `.deb` for your release, checked, then installed with apt; FFmpeg with apt.
   - Fedora, openSUSE and others: Audiveris from Flathub if Flatpak is installed; FFmpeg with dnf or zypper.
3. Run `./score2audio`.

Audiveris is not published for ARM Linux, so on a Raspberry Pi read your PDFs on another computer, or install Audiveris from Flathub if it is offered there for your system; MusicXML and `.s2a` files open without it.

Or build score2audio yourself; see Building from source. If score2audio cannot find Audiveris, the Setup section of the page explains what to do, and lets you type where Audiveris is installed.

### Android app (beta)

The score2audio app plays scores on an Android phone or braille notetaker (Android 10 or newer). It does not read PDFs itself: read the PDF with score2audio on a computer, choose "Export for mobile", and copy the `.s2a` file to the device.

If you installed a test version of the app from before version 1.0.0, uninstall it first: those were signed with a different key, so Android will not install the new version over them. Uninstalling removes the scores stored in the app, so add your `.s2a` files again afterwards.

1. Download `score2audio-android.apk` from the Releases page onto the device, or copy it there.
2. Open it from your file manager. Android will ask you to allow that file manager to install apps; allow it, then go back and install.
3. Open score2audio, choose "Add a score file", and pick the `.s2a` file. You can also open `.s2a` files straight from a file manager.

The app is built only from standard Android controls, one task per screen, so it works well with screen readers and braille displays:

- Your scores: the scores on the device, and Add a score file.
- A score: Play (which becomes Pause and Continue), Stop, Give starting notes, Save as audio file, the tempo and measure range, and the same play options as on a computer.
- Lines to play: a checkbox for each line, with a Settings button for its volume, sound, pitch and your own name for it.
- Lyrics: one item per printed line of music; choosing one plays from there.
- Read the notes: one item per measure, as text; choosing one plays that measure of the line.

Audio files are saved in the Music/score2audio folder.

## Updates

score2audio checks once a day whether a new version has been released, by asking this project's Releases page on GitHub (nothing about you or your scores is sent). In the Setup section of the page, under Updates, choose what happens when there is one:

- Install it automatically (the first choice): the new version is downloaded in the background and installed the next time you start score2audio.
- Tell me, and install it when I choose: the Updates section says a new version is available, with an "Update now" button and a link to what is new.
- Do not check for new versions.

"Check for a new version now" checks straight away. A new version is only installed if the list of its files carries the project's signature, and every file it downloads matches that list, so a changed or tampered download is never installed. A copy installed with the Windows installer is updated by running the new installer, without any questions, after which score2audio starts again by itself; any other copy replaces its own files and starts again. A copy in a folder it cannot change (for example one installed by a package manager) only tells you about the new version.

The Android app does not update itself yet: install the new `score2audio-android.apk` over the old one (your scores are kept).

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

Timing: Audiveris sometimes misses or misreads a time signature, especially a change of meter partway through a piece. score2audio then follows the notes: where several measures in a row agree on a different length, that is taken as the new meter, and the page lists how many measures were corrected this way. Printed metronome marks such as "♩ = 72", "𝅗𝅥 = 60" or "♩. = 54" are read from the page itself (Audiveris leaves them out), including which note counts the beat, so a piece is not played at double or half speed. A tempo word without a number ("Andante", "Slowly", "Allegro") gives a usual tempo for that word, and with no marking at all the time signature decides (90 quarter notes a minute, 72 half notes in 2/2 or 4/2, 60 dotted quarters in 6/8). The tempo can always be changed under Play. The tests in [tools/bench](tools/bench) measure all of this on 20 public-domain scores.

## Other ways to open music

A score2audio score file (`.s2a`), exported from score2audio on this or another computer, opens straight away, with the line names, volumes and tempo it was exported with. The format is described in [docs/s2a-format.md](docs/s2a-format.md).

If you already have a score as MusicXML (a `.mxl`, `.musicxml` or `.xml` file, for example exported from MuseScore, Finale, Sibelius or Dorico, or downloaded from a music library), open it directly. Audiveris is not needed for these, and the result is exact.

## Building from source

The Android app is in the `android` folder; build it with Gradle 8.10 or Android Studio (`gradle assembleRelease`). A build on your own computer is signed with Android's debug key. Published builds are signed on GitHub with the project's private key, which is kept in two repository secrets and never in the repository: `SIGNING_STORE_BASE64` (the PKCS12 key store, base64-encoded) and `SIGNING_STORE_PASSWORD` (its password; the key's alias is `score2audio` and its password is the same, unless `SIGNING_KEY_ALIAS` and `SIGNING_KEY_PASSWORD` say otherwise). GitHub refuses to publish a release without them, and checks that the app was signed with the key whose SHA-256 fingerprint is `81d60d76fccc52cbfc9b33f1d17aef664c16d013e1bb45c20a4897565d9bfaac`. Keep a safe copy of the key store and its password: if they are lost, the app can never be updated in place again.

For the desktop program you need Go 1.24 or newer (on Arch Linux: `sudo pacman -S go`; on a Mac with Homebrew: `brew install go`) and, for `make dist`, `curl` and `zip`, and NSIS for the Windows installer (`sudo pacman -S nsis` or `sudo apt install nsis`; without it the installer is skipped). The downloads are built with Go 1.27, whose standard library has security fixes that older versions of Go lack; if your Go is older, it fetches Go 1.27 by itself the first time you build.

```
make            # build for this computer
make test       # run the tests
make vulncheck  # check the code and Go for known security vulnerabilities
make run        # build and start it
make install    # Linux: install into ~/.local, with a menu entry
make dist       # build the Windows, Mac and Linux downloads into dist/
```

The web page lives in the `web` folder and is built into the program. To publish a new version, change the number in the `VERSION` file (and optionally add release notes as `release-notes/v<version>.md`) and push to `main`: GitHub then builds the Windows, Mac, Linux and Android downloads and publishes them as a new release, tagged with that version. Pushing a tag such as `v1.0.1` does the same.

Settings for testing, as environment variables:

- `AUDIVERIS_EXE` – path to the Audiveris program.
- `SCORE2AUDIO_DATA` – folder for the score library. By default it is `%LOCALAPPDATA%\score2audio` on Windows, `~/Library/Application Support/score2audio` on a Mac, and `~/.local/share/score2audio` on Linux.
- `SCORE2AUDIO_PORT` – a fixed port instead of a random one.
- `SCORE2AUDIO_NO_BROWSER` – do not open a browser.

`score2audio --dump file.mxl` prints what the program understood from a MusicXML file. `score2audio --read file.pdf folder` reads a PDF with Audiveris exactly as the page does and leaves the results in the folder. `score2audio --setup` installs Audiveris and FFmpeg (add `--yes` to answer yes to every question).

## Security

score2audio works entirely on your computer and never sends your scores anywhere. Its page is served by score2audio itself, only to your own computer, on a different port each time, and every request must carry a secret that changes each time score2audio starts, so other web pages and other programs cannot use it. Your score library is kept where only your user account can read it.

Score files (`.s2a`), MusicXML files and PDFs can come from anyone, so score2audio checks everything it reads from them: compressed files may not unpack to more than a set size, and the music in a score file is checked (for example for a tempo of zero, impossibly long notes, or measures that do not exist) before it is stored or played, on the computer and on Android.

Every change to the project is checked automatically: the tests run, `govulncheck` looks for known vulnerabilities in the code and in Go, and the downloads are built with the newest Go 1.27. The English language data for reading lyrics is downloaded from a fixed version and checked against its fingerprint (SHA-256) before it is packaged. Audiveris and FFmpeg are downloaded from fixed versions and checked the same way before they are installed.

Updates are signed: each release's list of files is signed with a private key that only the release build has (the `UPDATE_SIGNING_KEY` secret of this repository; its public key is in update.go), and score2audio installs an update only if that signature and every file's SHA-256 are right. A release built without the key can still be downloaded by hand, but is never installed automatically. To make a new key, run `go run ./tools/updatekey new key.txt`, put the printed public key in update.go, and the contents of key.txt in the secret.

To report a security problem, please open an issue on GitHub (or, for anything sensitive, contact the maintainer privately through GitHub).

## Videos

"Save as video" makes an MP4 video, 1280 by 720 pixels, of the lines that are chosen, over the chosen measures, at the chosen tempo. It shows:

- the title, the names of the lines, and the measure being played;
- the chosen lines engraved as sheet music, one staff per line, with the words under the notes, repeat signs and first and second endings; the music scrolls a line at a time and the measure being played is highlighted;
- the lyrics in large type, with the word being sung highlighted and the next line underneath;
- a bar showing how far through the piece the video is.

The sound is the same as "Save as audio file": the chosen lines with their volumes, sounds and pitch settings, and metronome clicks if those are switched on. A video of a whole piece usually takes well under a minute or two to make; a progress bar shows how far it has got, and it can be cancelled.

The video uses the line names: a line you have named under More settings (for example "Alto") is called that, and an unnamed line is called by the part it usually is.

The notation is made from the notes score2audio plays, so what is seen matches what is heard; that also means any mistakes in reading the PDF show in the music. Lyrics only go under voices, not instruments.

Videos are made by the web browser. Microsoft Edge and Google Chrome on Windows and Mac make videos with AAC sound, which plays everywhere. Other browsers may make videos with Opus sound, which a few older phones cannot play; when FFmpeg is installed, score2audio converts that sound to AAC by itself. If a browser cannot make videos at all, score2audio says so, and on Windows offers to open the page in Microsoft Edge.

## How it works

1. Audiveris reads the PDF and writes MusicXML, a standard text format for sheet music.
2. score2audio reads the MusicXML, works out timing, ties and repeats, joins up parts that Audiveris split because their labels are abbreviated later in the score ("Flute" on the first line, "Fl." after), identifies each part from its label, and splits staves that hold two parts.
3. The web page plays the lines with sounds made by your browser, tuned to A = 440 Hz.
4. For a video, score2audio writes the chosen lines as fresh MusicXML; the web page engraves it with Verovio, draws each frame, and encodes the picture (H.264) and sound (AAC) with the browser's own encoders (WebCodecs) into an MP4 file.

## Licence and credits

score2audio is released under the MIT licence; see the LICENSE file.

- [Audiveris](https://github.com/Audiveris/audiveris), the open source music recognition program (AGPL licence), installed separately (the installer and setup download it from its releases page).
- [FFmpeg](https://ffmpeg.org/), optional, for MP3 files and video sound (LGPL/GPL licences), installed separately; on Windows the "essentials" build from [gyan.dev](https://www.gyan.dev/ffmpeg/builds/) is downloaded, with its licence.
- [NSIS](https://nsis.sourceforge.io/), which builds the Windows installer (zlib licence).
- [Tesseract](https://github.com/tesseract-ocr/tessdata) English language data, used by Audiveris to read lyrics and labels (Apache 2.0 licence), included in the downloads.
- [Verovio](https://www.verovio.org/), which engraves the music in videos (LGPL licence, version 3 or later). It is included unmodified in the downloads as its own file, in the `verovio` folder next to the program, with its licence; you can replace that file with another build of Verovio. See third_party/verovio/README.md.
- [mp4-muxer](https://github.com/Vanilagy/mp4-muxer) by Vanilagy, which writes the MP4 file (MIT licence; web/vendor/mp4-muxer.LICENSE.txt).
