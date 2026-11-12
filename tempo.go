package main

// Choosing the starting tempo.
//
// In order of preference: a metronome mark in the MusicXML; a metronome mark
// read from the page image (Audiveris reads the number but leaves the mark
// out of its MusicXML, see omrtempo.go); a tempo word such as "Andante"; and
// last a usual tempo for the meter. Without the meter, a piece in 2/2 or 4/2
// would start at half the speed it should, which is the most common reason a
// score sounded "twice too slow".

import (
	"math"
	"sort"
	"strings"
	"unicode"
)

// beatQN is the beat the player counts, in quarter notes: the same rule the
// web page and the Android app use, so "beats per minute" means one thing.
func beatQN(beats, beatType int) float64 {
	if beatType == 8 && beats%3 == 0 && beats > 3 {
		return 1.5
	}
	if beatType <= 0 {
		return 1
	}
	return 4 / float64(beatType)
}

// tempoWords gives a usual number of beats a minute for common tempo words,
// counting quarter-note beats. Longer beats (half notes, dotted quarters) are
// counted a little slower (see wordTempo).
var tempoWords = []struct {
	word string
	bpm  float64
}{
	{"prestissimo", 190}, {"presto", 170}, {"vivacissimo", 160}, {"vivace", 150}, {"vivo", 150},
	{"allegrissimo", 150}, {"allegretto", 108}, {"allegro", 128}, {"moderato", 100}, {"andantino", 88},
	{"andante", 80}, {"adagietto", 72}, {"adagio", 66}, {"larghetto", 60}, {"largo", 50}, {"lento", 52},
	{"grave", 42}, {"maestoso", 84}, {"tempo di valse", 160}, {"valse", 160}, {"marcia", 112},
	// English, as hymnals and songbooks print them
	{"very slowly", 50}, {"slowly", 60}, {"slow", 60}, {"gently", 72}, {"reverently", 66}, {"prayerfully", 66},
	{"moderately slow", 76}, {"moderately fast", 116}, {"moderately", 96}, {"steadily", 92}, {"with motion", 100},
	{"flowing", 100}, {"joyfully", 120}, {"brightly", 132}, {"lively", 132}, {"briskly", 132},
	{"quickly", 144}, {"fast", 144}, {"very fast", 176},
	// German and French
	{"langsam", 60}, {"mässig", 96}, {"massig", 96}, {"bewegt", 116}, {"schnell", 144}, {"lebhaft", 132},
	{"lent", 52}, {"modéré", 96}, {"modere", 96}, {"animé", 120}, {"anime", 120}, {"vif", 144}, {"vite", 160},
}

// matchTempoWord finds a tempo word at the start of a direction ("Allegro ma
// non troppo" is Allegro). Long texts are not tempo words, they are titles,
// instructions or lyrics.
func matchTempoWord(text string) (string, float64) {
	t := strings.ToLower(strings.TrimSpace(text))
	t = strings.TrimFunc(t, func(r rune) bool { return !unicode.IsLetter(r) })
	if t == "" || len(strings.Fields(t)) > 5 || len(t) > 40 {
		return "", 0
	}
	best, bpm := "", 0.0
	for _, w := range tempoWords {
		if strings.HasPrefix(t, w.word) && len(w.word) > len(best) {
			rest := t[len(w.word):]
			if rest == "" || !unicode.IsLetter([]rune(rest)[0]) {
				best, bpm = w.word, w.bpm
			}
		}
	}
	if best == "" {
		return "", 0
	}
	// the word as printed, without what follows it
	words := strings.Fields(strings.TrimSpace(text))
	n := len(strings.Fields(best))
	if n > len(words) {
		n = len(words)
	}
	return strings.TrimRightFunc(strings.Join(words[:n], " "), func(r rune) bool { return !unicode.IsLetter(r) }), bpm
}

// wordTempo turns a tempo word's beats per minute into quarter notes per
// minute for the meter it is in.
func wordTempo(bpm float64, beats, beatType int) float64 {
	b := beatQN(beats, beatType)
	if beatType == 8 && beats == 3 && bpm >= 120 {
		b = 1.5 // a fast 3/8 is felt one beat to the measure
	}
	if b > 1 {
		bpm *= 0.75 // long beats go by more slowly than quarter notes
	}
	return math.Round(bpm * b)
}

// defaultTempo is a usual tempo, in quarter notes per minute, for a meter
// when the score gives no tempo at all: 90 quarter-note beats, 72 half-note
// beats (2/2, 4/2), 60 dotted-quarter beats (6/8, 9/8, 12/8).
func defaultTempo(beats, beatType int) float64 {
	if b := beatQN(beats, beatType); b >= 2 {
		return 72 * b
	}
	return 90 // 90 quarter notes, or 60 dotted quarters

}

// setTempo fills in m.Tempo once the measures are laid out.
func setTempo(m *Model, facts []measureFacts, marks []omrTempo) {
	m.Tempo = nil
	seen := map[float64]bool{}
	add := func(i int, off, qbpm float64) {
		at := math.Round((m.Measures[i].T+off)*1000) / 1000
		if !seen[at] && qbpm >= 10 && qbpm <= 1000 {
			seen[at] = true
			m.Tempo = append(m.Tempo, TempoPoint{T: at, BPM: qbpm})
		}
	}
	for i, f := range facts {
		for _, tp := range f.tempos {
			add(i, tp.T, tp.BPM)
		}
	}
	if len(m.Tempo) == 0 {
		for _, mk := range marks {
			if mk.measure >= 0 && mk.measure < len(m.Measures) {
				add(mk.measure, 0, float64(mk.number)*mk.unit)
			}
		}
	}
	m.TempoFromScore = len(m.Tempo) > 0
	if !m.TempoFromScore {
		for i, f := range facts {
			if f.tempoWord != "" && i < len(m.Measures) {
				mi := m.Measures[i]
				q := wordTempo(f.tempoBPM, mi.Beats, mi.BeatType)
				if n := len(m.Tempo); n > 0 && m.Tempo[n-1].BPM == q {
					continue // the same word again (often at the top of each page)
				}
				add(i, 0, q)
				if m.TempoWord == "" {
					m.TempoWord = f.tempoWord
				}
			}
		}
	}
	sort.Slice(m.Tempo, func(a, b int) bool { return m.Tempo[a].T < m.Tempo[b].T })
	if len(m.Tempo) == 0 || m.Tempo[0].T > 0 {
		first := 90.0
		switch {
		case len(m.Tempo) > 0:
			first = m.Tempo[0].BPM
		case len(m.Measures) > 0:
			// the first full measure's meter (a pickup may be written oddly)
			mi := m.Measures[0]
			if len(m.Measures) > 1 {
				mi = m.Measures[1]
			}
			first = defaultTempo(mi.Beats, mi.BeatType)
		}
		m.Tempo = append([]TempoPoint{{T: 0, BPM: first}}, m.Tempo...)
	}
}
