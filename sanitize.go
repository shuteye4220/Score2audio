package main

// sanitizeModel checks a playback model before it is stored or played.
//
// Models come from Audiveris' reading of a PDF, from MusicXML files, and from
// .s2a score files that may have been made by someone else. The page and
// the Android app trust the numbers in a model (they schedule one sound per
// note and one click per beat), so a damaged or deliberately crafted file
// with, say, a tempo of zero, a note lasting a billion beats, a beat type of
// a million or a measure number that does not exist could freeze or crash
// them, or make "Save as audio file" try to write an enormous file.
//
// Values that can be repaired are; entries that cannot are dropped. A
// model with no measures at all is an error.

import (
	"errors"
	"math"
	"sort"
	"unicode/utf8"
)

const (
	maxMeasures      = 20000  // also the limit for the expanded repeat order
	maxMeasureQN     = 256.0  // longest single measure, in quarter notes
	maxScoreQN       = 1e6    // latest time in the score, in quarter notes
	maxPerformanceQN = 200000 // total length with repeats (~37 hours at 90 bpm)
	maxLines         = 256
	maxEventsPerLine = 200000
	maxTempoPoints   = 10000
	minBPM, maxBPM   = 10.0, 600.0
	maxVerses        = 64
	maxWordsPerVerse = 200000
	maxWarnings      = 50
	maxShortText     = 200
	maxLongText      = 5000
	defaultBPM       = 90.0
)

var errNoMeasures = errors.New("the score contains no measures")

func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

// clampF returns v limited to [lo, hi], or def when v is not a finite number.
func clampF(v, lo, hi, def float64) float64 {
	if !finite(v) {
		return def
	}
	return math.Max(lo, math.Min(hi, v))
}

func clampI(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// cut shortens s to at most n characters without splitting a character.
func cut(s string, n int) string {
	if !utf8.ValidString(s) {
		s = string([]rune(s)) // replaces invalid bytes
	}
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

func validBeatType(v int) bool {
	switch v {
	case 1, 2, 4, 8, 16, 32, 64:
		return true
	}
	return false
}

func sanitizeModel(m *Model) error {
	if m == nil || len(m.Measures) == 0 {
		return errNoMeasures
	}
	m.Title = cut(m.Title, maxShortText)

	if len(m.Measures) > maxMeasures {
		m.Measures = m.Measures[:maxMeasures]
	}
	nm := len(m.Measures)
	for i := range m.Measures {
		ms := &m.Measures[i]
		ms.Number = cut(ms.Number, 20)
		ms.T = clampF(ms.T, 0, maxScoreQN, 0)
		ms.D = clampF(ms.D, 0, maxMeasureQN, 0)
		if ms.Beats < 1 || ms.Beats > 64 {
			ms.Beats = 4
		}
		if !validBeatType(ms.BeatType) {
			ms.BeatType = 4
		}
	}

	// The playing order (repeats expanded) may only name real measures, and
	// may not add up to an absurd length.
	order := make([]int, 0, len(m.Order))
	total := 0.0
	for _, mi := range m.Order {
		if mi < 0 || mi >= nm || len(order) >= maxMeasures {
			continue
		}
		total += m.Measures[mi].D
		if total > maxPerformanceQN {
			break
		}
		order = append(order, mi)
	}
	if len(order) == 0 {
		order = make([]int, nm)
		for i := range order {
			order[i] = i
		}
		m.HasRepeats = false
	}
	m.Order = order

	tempo := make([]TempoPoint, 0, len(m.Tempo))
	for _, tp := range m.Tempo {
		if !finite(tp.T) || !finite(tp.BPM) || tp.T < 0 || tp.BPM <= 0 || len(tempo) >= maxTempoPoints {
			continue
		}
		tempo = append(tempo, TempoPoint{T: math.Min(tp.T, maxScoreQN), BPM: clampF(tp.BPM, minBPM, maxBPM, defaultBPM)})
	}
	sort.SliceStable(tempo, func(a, b int) bool { return tempo[a].T < tempo[b].T })
	if len(tempo) == 0 || tempo[0].T > 0 {
		bpm := defaultBPM
		if len(tempo) > 0 {
			bpm = tempo[0].BPM
		}
		tempo = append([]TempoPoint{{T: 0, BPM: bpm}}, tempo...)
	}
	m.Tempo = tempo

	if len(m.Lines) > maxLines {
		m.Lines = m.Lines[:maxLines]
	}
	for li := range m.Lines {
		l := &m.Lines[li]
		l.ID = cut(l.ID, 64)
		l.Name, l.Hint = cut(l.Name, maxShortText), cut(l.Hint, maxShortText)
		l.Kind, l.Part, l.StaffKey = cut(l.Kind, 20), cut(l.Part, 20), cut(l.StaffKey, 64)
		l.Clef, l.Low, l.High = cut(l.Clef, 40), cut(l.Low, 40), cut(l.High, 40)
		l.Transpose = clampI(l.Transpose, -48, 48)
		l.TransposeNote = cut(l.TransposeNote, maxShortText)
		events := l.Events[:0]
		for _, e := range l.Events {
			if len(events) >= maxEventsPerLine || e.Mi < 0 || e.Mi >= nm || e.Midi < 0 || e.Midi > 127 ||
				!finite(e.T) || !finite(e.D) || e.T < 0 {
				continue
			}
			e.T = math.Min(e.T, maxScoreQN)
			e.D = clampF(e.D, 0, maxMeasureQN, 0)
			e.Name, e.Dur, e.Lyric = cut(e.Name, 40), cut(e.Dur, 80), cut(e.Lyric, maxShortText)
			events = append(events, e)
		}
		l.Events = events
		l.Count = len(events)
	}

	if len(m.Verses) > maxVerses {
		m.Verses = m.Verses[:maxVerses]
	}
	for vi := range m.Verses {
		v := &m.Verses[vi]
		v.Name = cut(v.Name, maxShortText)
		words := v.Words[:0]
		for _, w := range v.Words {
			if len(words) >= maxWordsPerVerse || !finite(w.T) || w.T < 0 || w.Mi < 0 || w.Mi >= nm {
				continue
			}
			w.Text = cut(w.Text, maxShortText)
			words = append(words, w)
		}
		v.Words = words
		lines := v.Lines[:0]
		for _, ll := range v.Lines {
			if ll.From < 0 || ll.To >= nm || ll.From > ll.To || len(lines) >= maxMeasures {
				continue
			}
			ll.Text = cut(ll.Text, maxLongText)
			lines = append(lines, ll)
		}
		v.Lines = lines
	}

	if len(m.Warnings) > maxWarnings {
		m.Warnings = m.Warnings[:maxWarnings]
	}
	for i := range m.Warnings {
		m.Warnings[i] = cut(m.Warnings[i], 1000)
	}
	return nil
}
