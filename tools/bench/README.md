# Timing bench

Checks how well score2audio gets the timing of real scores right: measure
lengths, time signatures, tempo marks and where each note falls.

The music is public domain, from the corpus that comes with
[music21](https://web.mit.edu/music21/): folk tunes, hymns, Bach, Handel,
Mozart, Beethoven, trecento pieces. Each case takes a piece (or a stretch of
one), prints a tempo mark on it where the case asks for one, engraves it to
PDF with [Verovio](https://www.verovio.org/) in one of several music fonts
(some made to look scanned), and keeps the exact timing as the truth to
compare against. The cases cover cut time, 4/2, 3/8, 6/8 and 9/8, changes of
meter, pickups, repeats partway through measures, triplets, and metronome
marks counting quarter, half, eighth and dotted-quarter notes.

    pip install music21 verovio pypdfium2 Pillow    # and: npm install playwright
    python3 make_cases.py cases                     # about a minute
    AUDIVERIS_EXE=... TESSDATA_PREFIX=... ./run_omr.sh cases ../../score2audio   # 15-45 s a case
    python3 compare.py cases ../../score2audio

`compare.py` prints, for each case: measures found, time signatures and
measure lengths that match, total length in quarter notes, whether the tempo
was read right (or doubled, halved…), playing time in seconds against the
truth, and how many note onsets and notes (onset and pitch) match.

Results in November 2026 (20 cases, before and after the timing work): every one of the 12 printed tempo
marks read right (none before), playing time within 10% for 10 of them (none
before), 312 of 344 measure lengths right (243 before), note onsets matching
0.84 (0.75 before). What remains is mostly Audiveris misreading the page:
9/8 read as 6/8, a time signature read as notes, a measure dropped, and a
page it cannot read at all (Aloha 'Oe, page 2).
