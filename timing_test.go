package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------- measure lengths ----------

// mf builds measure facts: printed meter b/bt (0 = carried over), and each
// voice's length in quarter notes.
func mf(b, bt int, lens ...float64) measureFacts {
	return measureFacts{beats: b, beatType: bt, hasTime: b > 0, voiceLens: lens}
}

func carry(f []measureFacts) []measureFacts {
	b, bt := 4, 4
	for i := range f {
		if f[i].beats > 0 {
			b, bt = f[i].beats, f[i].beatType
		} else {
			f[i].beats, f[i].beatType = b, bt
		}
	}
	return f
}

func lengths(sh []measureShape) []float64 {
	var out []float64
	for _, s := range sh {
		out = append(out, s.length)
	}
	return out
}

func sameLengths(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !near(a[i], b[i]) {
			return false
		}
	}
	return true
}

func TestInferMeasures(t *testing.T) {
	cases := []struct {
		name string
		f    []measureFacts
		want []float64
		meta string // the meter of the measure named by the last two fields, "" to skip
		at   int
	}{
		{"music agrees with the time signature", carry([]measureFacts{mf(3, 4, 3, 3), mf(0, 0, 3), mf(0, 0, 3, 3)}),
			[]float64{3, 3, 3}, "", 0},
		{"missed change from 4/4 to 3/4", carry([]measureFacts{mf(4, 4, 4), mf(0, 0, 4), mf(0, 0, 3), mf(0, 0, 3), mf(0, 0, 3), mf(0, 0, 3)}),
			[]float64{4, 4, 3, 3, 3, 3}, "3/4", 4},
		{"3/4 read as 4/4, two voices agree", carry([]measureFacts{mf(4, 4, 3, 3), mf(0, 0, 3, 3), mf(0, 0, 3, 3)}),
			[]float64{3, 3, 3}, "3/4", 1},
		{"a missed rest is not a new meter", carry([]measureFacts{mf(4, 4, 4), mf(0, 0, 3), mf(0, 0, 4), mf(0, 0, 4)}),
			[]float64{4, 4, 4, 4}, "", 0},
		{"a short voice does not outvote a full one", carry([]measureFacts{mf(4, 4, 4, 2), mf(0, 0, 4, 3), mf(0, 0, 4, 3)}),
			[]float64{4, 4, 4}, "", 0},
		{"two short measures of one voice are not a meter", carry([]measureFacts{mf(4, 4, 4), mf(0, 0, 3), mf(0, 0, 3), mf(0, 0, 4)}),
			[]float64{4, 4, 4, 4}, "", 0},
		{"pickup and the measure completing it", carry([]measureFacts{mf(3, 4, 1), mf(0, 0, 3), mf(0, 0, 3), mf(0, 0, 2)}),
			[]float64{1, 3, 3, 2}, "", 0},
		{"one 5/8 measure among 2/4s", carry([]measureFacts{mf(2, 4, 2, 2), mf(0, 0, 2.5, 2.5), mf(0, 0, 2, 2)}),
			[]float64{2, 2.5, 2}, "5/8", 1},
		{"learned meter ends when the music fits the printed one again", carry([]measureFacts{mf(4, 4, 3, 3), mf(0, 0, 3, 3), mf(0, 0, 4, 4), mf(0, 0, 4)}),
			[]float64{3, 3, 4, 4}, "4/4", 3},
		{"8 quarters is 4/2", carry([]measureFacts{mf(4, 4, 8, 8), mf(0, 0, 8, 8)}),
			[]float64{8, 8}, "4/2", 0},
		{"no notes at all keeps the time signature", carry([]measureFacts{mf(3, 4), mf(0, 0)}),
			[]float64{3, 3}, "", 0},
	}
	for _, c := range cases {
		got := inferMeasures(c.f)
		if !sameLengths(lengths(got), c.want) {
			t.Errorf("%s: lengths %v, want %v", c.name, lengths(got), c.want)
			continue
		}
		if c.meta != "" {
			if m := fmt.Sprintf("%d/%d", got[c.at].beats, got[c.at].beatType); m != c.meta {
				t.Errorf("%s: meter %s, want %s", c.name, m, c.meta)
			}
		}
	}
}

func TestInferMeasuresSplitByRepeat(t *testing.T) {
	// a repeat sign partway through a 4/4 measure: Audiveris writes 3 + 1,
	// the second half marked incomplete
	f := carry([]measureFacts{mf(4, 4, 4), mf(0, 0, 3), mf(0, 0, 1), mf(0, 0, 4)})
	f[2].implicit = true
	if got := lengths(inferMeasures(f)); !sameLengths(got, []float64{4, 3, 1, 4}) {
		t.Fatalf("got %v", got)
	}
}

// ---------- tempo ----------

func TestMatchTempoWord(t *testing.T) {
	cases := map[string]string{
		"Allegro ma non troppo": "Allegro", "Andante con moto": "Andante", "ALLEGRETTO": "Allegretto",
		"Slowly": "Slowly", "Very slowly, with feeling": "Very slowly", "Moderately fast": "Moderately fast",
		"Largo.": "Largo", "Adagio": "Adagio",
		"Allegros": "", "Lift Every Voice and Sing": "", "Fastidious": "", "cresc.": "", "": "",
		"Slowly the dawn breaks over the hills and fields": "",
	}
	for in, want := range cases {
		if got, _ := matchTempoWord(in); !strings.EqualFold(got, want) {
			t.Errorf("%q: got %q, want %q", in, got, want)
		}
	}
	_, a := matchTempoWord("Allegretto")
	_, b := matchTempoWord("Allegro")
	if a >= b {
		t.Errorf("Allegretto (%v) should be slower than Allegro (%v)", a, b)
	}
}

func TestTempoForMeter(t *testing.T) {
	cases := []struct {
		beats, beatType int
		want            float64
	}{
		{4, 4, 90}, {3, 4, 90}, {2, 2, 144}, {4, 2, 144}, {6, 8, 90}, {12, 8, 90}, {3, 8, 90},
	}
	for _, c := range cases {
		if got := defaultTempo(c.beats, c.beatType); got != c.want {
			t.Errorf("default %d/%d: got %v want %v", c.beats, c.beatType, got, c.want)
		}
	}
	if q := wordTempo(80, 4, 4); q != 80 {
		t.Errorf("Andante in 4/4: %v", q)
	}
	if q := wordTempo(128, 2, 2); q != 192 { // half note = 96
		t.Errorf("Allegro in 2/2: %v", q)
	}
	if q := wordTempo(170, 3, 8); q < 150 { // a fast 3/8 is one to the measure
		t.Errorf("Presto in 3/8 is too slow: %v", q)
	}
}

func TestSetTempoPreference(t *testing.T) {
	model := func(b, bt int) *Model {
		m := &Model{}
		for i := 0; i < 4; i++ {
			m.Measures = append(m.Measures, MeasureInfo{T: float64(i) * 4, D: 4, Beats: b, BeatType: bt})
		}
		return m
	}
	facts := make([]measureFacts, 4)

	m := model(2, 2)
	setTempo(m, facts, nil)
	if m.TempoFromScore || m.Tempo[0].BPM != 144 || m.TempoWord != "" {
		t.Errorf("no marking in 2/2: %+v", m)
	}

	facts[0].tempoWord, facts[0].tempoBPM = "Andante", 80
	m = model(4, 4)
	setTempo(m, facts, nil)
	if m.TempoFromScore || m.Tempo[0].BPM != 80 || m.TempoWord != "Andante" {
		t.Errorf("word: %+v", m)
	}

	// a mark read from the page beats a word; its unit decides the speed
	m = model(2, 2)
	setTempo(m, facts, []omrTempo{{measure: 0, unit: 2, number: 80}, {measure: 2, unit: 2, number: 60}})
	if !m.TempoFromScore || len(m.Tempo) != 2 || m.Tempo[0].BPM != 160 || m.Tempo[1].T != 8 || m.Tempo[1].BPM != 120 {
		t.Errorf("marks: %+v", m.Tempo)
	}

	// a mark in the MusicXML beats them all
	facts[1].tempos = []TempoPoint{{T: 0, BPM: 100}}
	m = model(4, 4)
	setTempo(m, facts, []omrTempo{{measure: 0, unit: 1, number: 60}})
	if !m.TempoFromScore || len(m.Tempo) != 2 || m.Tempo[0].BPM != 100 || m.Tempo[1].BPM != 100 {
		t.Errorf("MusicXML mark: %+v", m.Tempo)
	}
}

func TestMisreadMetronomeIgnored(t *testing.T) {
	// Audiveris sometimes makes "whole note = 4" out of other symbols
	x := strings.Replace(sample, `<direction><sound tempo="120"/></direction>`,
		`<direction><direction-type><metronome><beat-unit>whole</beat-unit><per-minute>4</per-minute></metronome></direction-type><sound tempo="16"/></direction>`, 1)
	p := filepath.Join(t.TempDir(), "a.musicxml")
	os.WriteFile(p, []byte(x), 0o644)
	m, err := LoadModel([]string{p}, "x")
	if err != nil {
		t.Fatal(err)
	}
	if m.TempoFromScore || m.Tempo[0].BPM != 90 {
		t.Fatalf("misread mark used: %+v", m.Tempo)
	}
}

// ---------- marks on the page ----------

// The test images are the strip above the first staff of pages engraved from
// public-domain scores (see tools/bench), interline 21 pixels; the top staff
// line is 7 interlines below the top of the image.
const testIL = 21

func loadTestPNG(t *testing.T, name string) image.Image {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", "tempo-marks", name))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func TestFindMetronomeMarks(t *testing.T) {
	cases := []struct {
		file   string
		unit   float64
		number int
	}{
		{"quarter-72.png", 1, 72},
		{"half-80-scan.png", 2, 80},
		{"dotted-quarter-54.png", 1.5, 54},
		{"dotted-quarter-46.png", 1.5, 46},
		{"eighth-70.png", 0.5, 70},
	}
	for _, c := range cases {
		img := loadTestPNG(t, c.file)
		w := img.Bounds().Dx()
		top := int(7 * testIL)
		got := findMetronomeMarks(img, 0, 0, w, top-testIL/5, testIL, nil, nil)
		if len(got) != 1 || got[0].unit != c.unit || got[0].number != c.number {
			t.Errorf("%s: got %+v, want one mark of %v = %d", c.file, got, c.unit, c.number)
		}
		// Audiveris's reading of the number wins over ours
		if len(got) == 1 {
			b := omrBounds{X: got[0].x + testIL/2, Y: float64(top) - 6*testIL, W: 8 * testIL, H: 6 * testIL}
			again := findMetronomeMarks(img, 0, 0, w, top-testIL/5, testIL, []omrBounds{b}, []int{99})
			if len(again) != 1 || again[0].number != 99 {
				t.Errorf("%s: Audiveris's number not used: %+v", c.file, again)
			}
		}
		// the music itself has no marks in it
		if got := findMetronomeMarks(img, 0, top, w, img.Bounds().Dy(), testIL, nil, nil); len(got) != 0 {
			t.Errorf("%s: marks found among the notes: %+v", c.file, got)
		}
	}
}

func TestReadOMRTempos(t *testing.T) {
	// a two-page Audiveris project: page 1 has no music, page 2 has two
	// systems, the mark is in the second measure of the second system
	img := loadTestPNG(t, "half-80-scan.png")
	var png1 bytes.Buffer
	png.Encode(&png1, img)
	mark := findMetronomeMarks(img, 0, 0, img.Bounds().Dx(), 7*testIL-4, testIL, nil, nil)
	if len(mark) != 1 {
		t.Fatalf("mark not found: %+v", mark)
	}
	x := mark[0].x
	sheet := fmt.Sprintf(`<sheet><scale><interline main="21"/></scale><page>
 <system><stack left="0" right="1000"/><stack left="1000" right="2000"/>
  <part><staff id="1"><lines><line><point x="0" y="%d"/><point x="2000" y="%d"/></line></lines></staff></part></system>
 <system><stack left="0" right="%f"/><stack left="%f" right="2200"/>
  <part><staff id="9"><lines><line><point x="0" y="%d"/><point x="2190" y="%d"/></line></lines></staff></part></system>
</page></sheet>`, 2000, 2000, x-50, x-50, 7*testIL, 7*testIL)
	// the first system's staff is far below the image, so it has no marks
	path := filepath.Join(t.TempDir(), "book.omr")
	f, _ := os.Create(path)
	zw := zip.NewWriter(f)
	w, _ := zw.Create("sheet#1/sheet#1.xml")
	w.Write([]byte(`<sheet><scale><interline main="21"/></scale><page/></sheet>`))
	w, _ = zw.Create("sheet#2/sheet#2.xml")
	w.Write([]byte(sheet))
	w, _ = zw.Create("sheet#2/BINARY.png")
	w.Write(png1.Bytes())
	zw.Close()
	f.Close()

	got := readOMRTempos(path, []int{1})
	if len(got) != 1 || got[0].measure != 3 || got[0].unit != 2 || got[0].number != 80 {
		t.Fatalf("got %+v", got)
	}
	if got := readOMRTempos(filepath.Join(t.TempDir(), "missing.omr"), nil); got != nil {
		t.Fatalf("missing file: %+v", got)
	}
}
