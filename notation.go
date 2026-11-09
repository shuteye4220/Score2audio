package main

// Notation for the video: MusicXML of only the lines the user chose, made
// from the same notes score2audio plays, so what is seen matches what is
// heard. The web page engraves it with Verovio.

import (
	"encoding/xml"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// div is the MusicXML divisions per quarter note: it divides evenly into
// 32nd notes (6) and every triplet value down to triplet sixteenths (8).
const div = 48

type NotationLine struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type NotationRequest struct {
	Lines []NotationLine `json:"lines"`
	Verse int            `json:"verse"`
	From  int            `json:"from"` // measure index, inclusive
	To    int            `json:"to"`   // measure index, inclusive
}

const maxNotationLines = 16

// a written duration: note type, dots, and whether it is a triplet value
type noteValue struct {
	units   int
	typ     string
	dots    int
	triplet bool
}

var normalValues = []noteValue{
	{192, "whole", 0, false}, {144, "half", 1, false}, {96, "half", 0, false}, {72, "quarter", 1, false},
	{48, "quarter", 0, false}, {36, "eighth", 1, false}, {24, "eighth", 0, false}, {18, "16th", 1, false},
	{12, "16th", 0, false}, {6, "32nd", 0, false},
}

var tripletValues = []noteValue{
	{64, "half", 0, true}, {32, "quarter", 0, true}, {16, "eighth", 0, true}, {8, "16th", 0, true},
}

// splitUnits writes a length as note values that add up to it: triplet
// values first while the rest cannot be written without them, then the
// ordinary values from the longest down.
func splitUnits(units int) []noteValue {
	var out []noteValue
	for units%6 != 0 && units >= 8 {
		for _, v := range tripletValues {
			if v.units <= units {
				out = append(out, v)
				units -= v.units
				break
			}
		}
	}
	for units >= 6 {
		for _, v := range normalValues {
			if v.units <= units {
				out = append(out, v)
				units -= v.units
				break
			}
		}
	}
	return out // anything shorter than a 32nd is too short to write and is dropped
}

var pitchNameRe = regexp.MustCompile(`^([A-G])(?: (sharp|flat|double sharp|double flat))? (-?\d+)$`)

// spell turns a note name such as "E flat 4" back into step, alter and octave.
func spell(name string, midi int) (step string, alter, octave int) {
	if m := pitchNameRe.FindStringSubmatch(strings.TrimSpace(name)); m != nil {
		octave, _ = strconv.Atoi(m[3])
		alter = map[string]int{"sharp": 1, "flat": -1, "double sharp": 2, "double flat": -2}[m[2]]
		return m[1], alter, octave
	}
	steps := []string{"C", "C", "D", "E", "E", "F", "F", "G", "A", "A", "B", "B"}
	alters := []int{0, 1, 0, -1, 0, 0, 1, 0, -1, 0, -1, 0}
	pc := ((midi % 12) + 12) % 12
	return steps[pc], alters[pc], int(math.Floor(float64(midi)/12)) - 1
}

// clefFor turns a clef description back into MusicXML; with no clef known,
// it picks treble or bass from the line's notes.
func clefFor(desc string, events []Event) (sign string, line, octave int) {
	switch {
	case strings.HasPrefix(desc, "treble clef"):
		sign, line = "G", 2
	case strings.HasPrefix(desc, "bass clef"):
		sign, line = "F", 4
	case strings.HasPrefix(desc, "alto clef"):
		sign, line = "C", 3
	case strings.HasPrefix(desc, "tenor clef"):
		sign, line = "C", 4
	default:
		sum := 0
		for _, e := range events {
			sum += e.Midi
		}
		if len(events) > 0 && sum/len(events) < 57 {
			sign, line = "F", 4
		} else {
			sign, line = "G", 2
		}
	}
	if strings.Contains(desc, "octave lower") {
		octave = -1
	} else if strings.Contains(desc, "octave higher") {
		octave = 1
	}
	return
}

// a piece of a note inside one measure
type piece struct {
	start, units     int // within the measure
	midi             int
	name             string
	tieStop, tieNext bool // continues from the previous measure, continues into the next
	at               float64
}

// a chord or single note at one moment, within one voice
type group struct {
	start, units int
	notes        []piece
}

// one written note or rest
type item struct {
	start  int
	value  noteValue
	notes  []piece // empty for a rest
	first  bool    // first written value of its notes (carries the lyric)
	last   bool    // last written value of its notes
	at     float64
	beam   string
	tuplet string
	hidden bool
}

func xmlText(s string) string {
	var b strings.Builder
	xml.EscapeText(&b, []byte(s))
	return b.String()
}

// NotationXML writes MusicXML for the requested lines and measures.
func NotationXML(m *Model, req NotationRequest) ([]byte, error) {
	if len(m.Measures) == 0 {
		return nil, fmt.Errorf("this score has no measures")
	}
	if len(req.Lines) == 0 {
		return nil, fmt.Errorf("no lines are chosen")
	}
	if len(req.Lines) > maxNotationLines {
		return nil, fmt.Errorf("choose at most %d lines for a video", maxNotationLines)
	}
	from, to := req.From, req.To
	if from < 0 {
		from = 0
	}
	if to >= len(m.Measures) || to < 0 {
		to = len(m.Measures) - 1
	}
	if to < from {
		from, to = to, from
	}
	byID := map[string]*Line{}
	for i := range m.Lines {
		byID[m.Lines[i].ID] = &m.Lines[i]
	}
	var syls []Syllable
	if req.Verse >= 0 && req.Verse < len(m.Verses) {
		syls = m.Verses[req.Verse].Syllables
	}
	sylAt := map[int64]Syllable{}
	for _, s := range syls {
		sylAt[int64(math.Round(s.T*div))] = s
	}

	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<score-partwise version="4.0">` + "\n<part-list>\n")
	for i, nl := range req.Lines {
		l := byID[nl.ID]
		if l == nil {
			return nil, fmt.Errorf("the score has no line %q", nl.ID)
		}
		name := strings.TrimSpace(nl.Name)
		if name == "" {
			name = l.Name
		}
		if len([]rune(name)) > 100 {
			name = string([]rune(name)[:100])
		}
		fmt.Fprintf(&b, "<score-part id=\"P%d\"><part-name>%s</part-name></score-part>\n", i+1, xmlText(name))
	}
	b.WriteString("</part-list>\n")

	for i, nl := range req.Lines {
		l := byID[nl.ID]
		fmt.Fprintf(&b, "<part id=\"P%d\">\n", i+1)
		writePart(&b, m, l, from, to, sylAt)
		b.WriteString("</part>\n")
	}
	b.WriteString("</score-partwise>\n")
	return []byte(b.String()), nil
}

func writePart(b *strings.Builder, m *Model, l *Line, from, to int, sylAt map[int64]Syllable) {
	if l.Kind == "instrument" || l.Kind == "keyboard" {
		sylAt = nil // words go under the voices only
	}
	// cut every note into pieces, one per measure it sounds in
	pieces := make(map[int][]piece)
	for _, e := range l.Events {
		start, end := e.T, e.T+e.D
		for mi := e.Mi; mi < len(m.Measures) && mi <= to; mi++ {
			ms := m.Measures[mi]
			if mi > e.Mi && ms.T >= end-0.001 {
				break
			}
			s := math.Max(start, ms.T)
			en := math.Min(end, ms.T+ms.D)
			if en-s < 0.001 {
				continue
			}
			if mi < from {
				continue
			}
			u0 := int(math.Round((s - ms.T) * div))
			u1 := int(math.Round((en - ms.T) * div))
			if u1 <= u0 {
				continue
			}
			pieces[mi] = append(pieces[mi], piece{start: u0, units: u1 - u0, midi: e.Midi, name: e.Name,
				tieStop: s > start+0.001, tieNext: en < end-0.001, at: s})
		}
	}

	clefSign, clefLine, clefOct := clefFor(l.Clef, l.Events)
	prevFifths, prevBeats, prevBeatType := math.MinInt32, 0, 0
	for mi := from; mi <= to; mi++ {
		ms := m.Measures[mi]
		munits := int(math.Round(ms.D * div))
		nominal := ms.Beats * 4 * div / max(1, ms.BeatType)
		implicit := ""
		if munits < nominal && (mi == from || mi == 0) {
			implicit = ` implicit="yes"`
		}
		fmt.Fprintf(b, "<measure number=\"%s\"%s>\n", xmlText(ms.Number), implicit)
		if mi == from || ms.Fifths != prevFifths || ms.Beats != prevBeats || ms.BeatType != prevBeatType {
			b.WriteString("<attributes>")
			if mi == from {
				fmt.Fprintf(b, "<divisions>%d</divisions>", div)
			}
			if ms.Fifths != prevFifths {
				fmt.Fprintf(b, "<key><fifths>%d</fifths></key>", ms.Fifths)
			}
			if ms.Beats != prevBeats || ms.BeatType != prevBeatType {
				fmt.Fprintf(b, "<time><beats>%d</beats><beat-type>%d</beat-type></time>", ms.Beats, ms.BeatType)
			}
			if mi == from {
				fmt.Fprintf(b, "<clef><sign>%s</sign><line>%d</line>", clefSign, clefLine)
				if clefOct != 0 {
					fmt.Fprintf(b, "<clef-octave-change>%d</clef-octave-change>", clefOct)
				}
				b.WriteString("</clef>")
			}
			b.WriteString("</attributes>\n")
			prevFifths, prevBeats, prevBeatType = ms.Fifths, ms.Beats, ms.BeatType
		}
		if ms.RepeatStart || (ms.Ending != "" && ms.EndingFirst) {
			b.WriteString(`<barline location="left">`)
			if ms.RepeatStart {
				b.WriteString("<bar-style>heavy-light</bar-style>")
			}
			if ms.Ending != "" && ms.EndingFirst {
				fmt.Fprintf(b, `<ending number="%s" type="start">%s.</ending>`, xmlText(ms.Ending), xmlText(ms.Ending))
			}
			if ms.RepeatStart {
				b.WriteString(`<repeat direction="forward"/>`)
			}
			b.WriteString("</barline>\n")
		}
		voices := assignVoices(pieces[mi])
		if len(voices) == 0 {
			fmt.Fprintf(b, "<note><rest measure=\"yes\"/><duration>%d</duration><voice>1</voice></note>\n", munits)
		}
		beat := div
		if ms.BeatType == 8 && ms.Beats%3 == 0 && ms.Beats > 3 {
			beat = div * 3 / 2
		}
		for vi, groups := range voices {
			if vi > 0 {
				fmt.Fprintf(b, "<backup><duration>%d</duration></backup>\n", munits)
			}
			items := layOut(groups, munits, vi > 0)
			markTuplets(items)
			markBeams(items, beat)
			stem := ""
			if len(voices) > 1 {
				stem = map[bool]string{true: "up", false: "down"}[vi == 0]
			}
			for _, it := range items {
				writeItem(b, it, vi+1, stem, sylAt)
			}
		}
		if ms.RepeatEnd > 0 || (ms.Ending != "" && ms.EndingLast != "") {
			b.WriteString(`<barline location="right">`)
			if ms.RepeatEnd > 0 {
				b.WriteString("<bar-style>light-heavy</bar-style>")
			}
			if ms.Ending != "" && ms.EndingLast != "" {
				fmt.Fprintf(b, `<ending number="%s" type="%s"/>`, xmlText(ms.Ending), ms.EndingLast)
			}
			if ms.RepeatEnd > 0 {
				times := ""
				if ms.RepeatEnd > 2 {
					times = fmt.Sprintf(` times="%d"`, ms.RepeatEnd)
				}
				fmt.Fprintf(b, `<repeat direction="backward"%s/>`, times)
			}
			b.WriteString("</barline>\n")
		}
		b.WriteString("</measure>\n")
	}
}

// assignVoices puts notes that start together with the same length into
// chords, and overlapping chords into separate voices.
func assignVoices(ps []piece) [][]group {
	sort.SliceStable(ps, func(a, b int) bool {
		if ps[a].start != ps[b].start {
			return ps[a].start < ps[b].start
		}
		return ps[a].midi > ps[b].midi
	})
	var groups []group
	for _, p := range ps {
		if n := len(groups); n > 0 && groups[n-1].start == p.start && groups[n-1].units == p.units {
			dup := false
			for _, q := range groups[n-1].notes {
				if q.midi == p.midi {
					dup = true
				}
			}
			if !dup {
				groups[n-1].notes = append(groups[n-1].notes, p)
			}
			continue
		}
		groups = append(groups, group{start: p.start, units: p.units, notes: []piece{p}})
	}
	var voices [][]group
	var ends []int
	for _, g := range groups {
		placed := false
		for v := range voices {
			if ends[v] <= g.start {
				voices[v] = append(voices[v], g)
				ends[v] = g.start + g.units
				placed = true
				break
			}
		}
		if !placed && len(voices) < 4 {
			voices = append(voices, []group{g})
			ends = append(ends, g.start+g.units)
		}
	}
	// the voice with the highest notes first, so its stems go up
	if len(voices) > 1 {
		avg := func(gs []group) float64 {
			s, n := 0, 0
			for _, g := range gs {
				for _, p := range g.notes {
					s += p.midi
					n++
				}
			}
			return float64(s) / float64(max(1, n))
		}
		sort.SliceStable(voices, func(a, b int) bool { return avg(voices[a]) > avg(voices[b]) })
	}
	return voices
}

// layOut writes a voice as notes and rests that fill the measure.
func layOut(groups []group, munits int, hideRests bool) []item {
	var items []item
	rest := func(start, units int) {
		pos := start
		for _, v := range splitUnits(units) {
			items = append(items, item{start: pos, value: v, hidden: hideRests})
			pos += v.units
		}
	}
	cursor := 0
	for _, g := range groups {
		if g.start > cursor {
			rest(cursor, g.start-cursor)
		}
		values := splitUnits(g.units)
		pos := g.start
		for i, v := range values {
			last := i == len(values)-1
			items = append(items, item{start: pos, value: v, notes: g.notes, first: i == 0, last: last, at: g.notes[0].at})
			pos += v.units
		}
		cursor = g.start + g.units
	}
	if cursor < munits {
		rest(cursor, munits-cursor)
	}
	return items
}

// markTuplets brackets triplet values in groups that fill a beat (or two
// beats for triplet halves and quarters).
func markTuplets(items []item) {
	open := -1
	sum := 0
	for i := range items {
		v := items[i].value
		if !v.triplet {
			if open >= 0 {
				items[i-1].tuplet = "stop"
				if i-1 == open {
					items[open].tuplet = ""
				}
				open = -1
			}
			continue
		}
		if open < 0 {
			open, sum = i, 0
			items[i].tuplet = "start"
		}
		sum += v.units
		span := div
		if v.units >= 32 {
			span = 2 * div
		}
		if sum%span == 0 {
			if i == open {
				items[i].tuplet = "single"
			} else {
				items[i].tuplet = "stop"
			}
			open = -1
		}
	}
	if open >= 0 {
		last := len(items) - 1
		if last == open {
			items[last].tuplet = "single"
		} else {
			items[last].tuplet = "stop"
		}
	}
}

// markBeams joins eighths and shorter notes that fall in the same beat.
func markBeams(items []item, beat int) {
	beamable := func(it item) bool {
		return len(it.notes) > 0 && !it.hidden && (it.value.typ == "eighth" || it.value.typ == "16th" || it.value.typ == "32nd")
	}
	i := 0
	for i < len(items) {
		if !beamable(items[i]) {
			i++
			continue
		}
		j := i
		for j+1 < len(items) && beamable(items[j+1]) && items[j+1].start/beat == items[i].start/beat {
			j++
		}
		if j > i {
			items[i].beam = "begin"
			for k := i + 1; k < j; k++ {
				items[k].beam = "continue"
			}
			items[j].beam = "end"
		}
		i = j + 1
	}
}

func writeItem(b *strings.Builder, it item, voice int, stem string, sylAt map[int64]Syllable) {
	if len(it.notes) == 0 {
		hidden := ""
		if it.hidden {
			hidden = ` print-object="no"`
		}
		fmt.Fprintf(b, "<note%s><rest/><duration>%d</duration><voice>%d</voice><type>%s</type>", hidden, it.value.units, voice, it.value.typ)
		writeValueTail(b, it)
		b.WriteString("</note>\n")
		return
	}
	notes := append([]piece(nil), it.notes...)
	sort.SliceStable(notes, func(a, c int) bool { return notes[a].midi < notes[c].midi })
	for ni, p := range notes {
		b.WriteString("<note>")
		if ni > 0 {
			b.WriteString("<chord/>")
		}
		step, alter, octave := spell(p.name, p.midi)
		fmt.Fprintf(b, "<pitch><step>%s</step>", step)
		if alter != 0 {
			fmt.Fprintf(b, "<alter>%d</alter>", alter)
		}
		fmt.Fprintf(b, "<octave>%d</octave></pitch><duration>%d</duration>", octave, it.value.units)
		// each note of a chord has its own ties: one may carry over the
		// barline while another stops
		tieStop := !it.first || p.tieStop
		tieStart := !it.last || p.tieNext
		if tieStop {
			b.WriteString(`<tie type="stop"/>`)
		}
		if tieStart {
			b.WriteString(`<tie type="start"/>`)
		}
		fmt.Fprintf(b, "<voice>%d</voice><type>%s</type>", voice, it.value.typ)
		writeDots(b, it)
		if it.value.triplet {
			b.WriteString("<time-modification><actual-notes>3</actual-notes><normal-notes>2</normal-notes></time-modification>")
		}
		if stem != "" {
			fmt.Fprintf(b, "<stem>%s</stem>", stem)
		}
		if ni == 0 && it.beam != "" {
			fmt.Fprintf(b, `<beam number="1">%s</beam>`, it.beam)
		}
		var nots []string
		if tieStop {
			nots = append(nots, `<tied type="stop"/>`)
		}
		if tieStart {
			nots = append(nots, `<tied type="start"/>`)
		}
		if ni == 0 && it.tuplet != "" && it.tuplet != "single" {
			nots = append(nots, fmt.Sprintf(`<tuplet type="%s" bracket="no"/>`, it.tuplet))
		}
		if len(nots) > 0 {
			b.WriteString("<notations>" + strings.Join(nots, "") + "</notations>")
		}
		// the lyric goes under the first voice, on a newly sung note, not on
		// held continuations
		if ni == 0 && voice == 1 && it.first && !it.notes[0].tieStop {
			if s, ok := syllableAt(sylAt, it.at); ok {
				fmt.Fprintf(b, `<lyric number="1"><syllabic>%s</syllabic><text>%s</text></lyric>`, s.Syllabic, xmlText(s.Text))
			}
		}
		b.WriteString("</note>\n")
	}
}

// syllableAt finds the syllable sung at time t, allowing for rounding.
func syllableAt(sylAt map[int64]Syllable, t float64) (Syllable, bool) {
	k := int64(math.Round(t * div))
	for _, d := range []int64{0, -1, 1} {
		if s, ok := sylAt[k+d]; ok {
			return s, true
		}
	}
	return Syllable{}, false
}

func writeDots(b *strings.Builder, it item) {
	for d := 0; d < it.value.dots; d++ {
		b.WriteString("<dot/>")
	}
}

func writeValueTail(b *strings.Builder, it item) {
	writeDots(b, it)
	if it.value.triplet {
		b.WriteString("<time-modification><actual-notes>3</actual-notes><normal-notes>2</normal-notes></time-modification>")
	}
}
