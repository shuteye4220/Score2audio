package main

import (
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const repeatSample = `<?xml version="1.0" encoding="UTF-8"?>
<score-partwise version="3.1">
 <part-list><score-part id="P1"><part-name>Soprano</part-name></score-part></part-list>
 <part id="P1">
  <measure number="1">
   <barline location="left"><bar-style>heavy-light</bar-style><repeat direction="forward"/></barline>
   <attributes><divisions>6</divisions><key><fifths>-1</fifths></key><time><beats>2</beats><beat-type>4</beat-type></time><clef><sign>G</sign><line>2</line></clef></attributes>
   <note><pitch><step>F</step><octave>4</octave></pitch><duration>6</duration><voice>1</voice><type>quarter</type><lyric><syllabic>begin</syllabic><text>Al</text></lyric></note>
   <note><pitch><step>G</step><octave>4</octave></pitch><duration>2</duration><voice>1</voice><type>eighth</type><time-modification><actual-notes>3</actual-notes><normal-notes>2</normal-notes></time-modification><lyric><syllabic>middle</syllabic><text>le</text></lyric></note>
   <note><pitch><step>A</step><octave>4</octave></pitch><duration>2</duration><voice>1</voice><type>eighth</type><time-modification><actual-notes>3</actual-notes><normal-notes>2</normal-notes></time-modification><lyric><syllabic>end</syllabic><text>lu</text></lyric></note>
   <note><pitch><step>B</step><alter>-1</alter><octave>4</octave></pitch><duration>2</duration><voice>1</voice><type>eighth</type><time-modification><actual-notes>3</actual-notes><normal-notes>2</normal-notes></time-modification></note>
  </measure>
  <measure number="2">
   <note><pitch><step>C</step><octave>5</octave></pitch><duration>9</duration><voice>1</voice><type>quarter</type><dot/></note>
   <note><pitch><step>D</step><octave>5</octave></pitch><duration>3</duration><voice>1</voice><type>eighth</type></note>
  </measure>
  <measure number="3">
   <barline location="left"><ending number="1" type="start"/></barline>
   <note><pitch><step>E</step><octave>5</octave></pitch><duration>12</duration><voice>1</voice><type>half</type></note>
   <barline location="right"><bar-style>light-heavy</bar-style><ending number="1" type="stop"/><repeat direction="backward"/></barline>
  </measure>
  <measure number="4">
   <barline location="left"><ending number="2" type="start"/></barline>
   <note><pitch><step>F</step><octave>5</octave></pitch><duration>12</duration><voice>1</voice><type>half</type></note>
   <barline location="right"><ending number="2" type="discontinue"/></barline>
  </measure>
  <measure number="5">
   <note><rest/><duration>6</duration><voice>1</voice></note>
   <note><pitch><step>F</step><octave>4</octave></pitch><duration>6</duration><voice>1</voice><type>quarter</type></note>
  </measure>
 </part>
</score-partwise>`

// notationRoundTrip writes notation for every line, reads it back, and
// checks that each line's notes come back the same.
func notationRoundTrip(t *testing.T, xmlText string) (*Model, *Model) {
	t.Helper()
	dir := t.TempDir()
	src := filepath.Join(dir, "in.musicxml")
	os.WriteFile(src, []byte(xmlText), 0o644)
	m, err := LoadModel([]string{src}, "x")
	if err != nil {
		t.Fatal(err)
	}
	var whole *Model
	for _, l := range m.Lines {
		x, err := NotationXML(m, NotationRequest{Lines: []NotationLine{{ID: l.ID, Name: "Line <" + l.ID + ">"}}, From: 0, To: len(m.Measures) - 1})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(x), "Line &lt;"+l.ID+"&gt;") {
			t.Errorf("%s: the part name was not escaped", l.ID)
		}
		out := filepath.Join(dir, l.ID+".musicxml")
		os.WriteFile(out, x, 0o644)
		back, err := LoadModel([]string{out}, "y")
		if err != nil {
			t.Fatalf("%s: written notation could not be read: %v", l.ID, err)
		}
		if whole == nil {
			whole = back
		}
		key := func(e Event) [3]int64 {
			return [3]int64{int64(math.Round(e.T * 48)), int64(math.Round(e.D * 48)), int64(e.Midi)}
		}
		want := map[[3]int64]string{}
		for _, e := range l.Events {
			want[key(e)] = e.Name
		}
		got := map[[3]int64]string{}
		for _, bl := range back.Lines {
			if bl.Part == "single" || bl.Part == "all" {
				for _, e := range bl.Events {
					got[key(e)] = e.Name
				}
			}
		}
		if !reflect.DeepEqual(want, got) {
			t.Errorf("%s: notes changed\nwant %v\n got %v", l.ID, want, got)
		}
		if len(back.Measures) != len(m.Measures) {
			t.Errorf("%s: %d measures came back, want %d", l.ID, len(back.Measures), len(m.Measures))
		}
	}
	return m, whole
}

func TestNotationRoundTripVoicesAndTies(t *testing.T) {
	notationRoundTrip(t, sample)
}

func TestNotationRoundTripRepeatsTripletsLyrics(t *testing.T) {
	m, back := notationRoundTrip(t, repeatSample)
	if !reflect.DeepEqual(back.Order, m.Order) || !reflect.DeepEqual(m.Order, []int{0, 1, 2, 0, 1, 3, 4}) {
		t.Errorf("repeat route: original %v, from notation %v", m.Order, back.Order)
	}
	if back.Measures[0].Fifths != -1 {
		t.Errorf("key signature lost: %d", back.Measures[0].Fifths)
	}
	if len(back.Verses) != 1 || back.Verses[0].Words[0].Text != "Allelu" {
		t.Errorf("lyrics and their hyphenation lost: %+v", back.Verses)
	}
}

func TestNotationRefusesBadRequests(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "in.musicxml")
	os.WriteFile(src, []byte(sample), 0o644)
	m, _ := LoadModel([]string{src}, "x")
	if _, err := NotationXML(m, NotationRequest{}); err == nil {
		t.Error("accepted no lines")
	}
	if _, err := NotationXML(m, NotationRequest{Lines: []NotationLine{{ID: "nope"}}}); err == nil {
		t.Error("accepted an unknown line")
	}
	many := make([]NotationLine, maxNotationLines+1)
	for i := range many {
		many[i] = NotationLine{ID: m.Lines[0].ID}
	}
	if _, err := NotationXML(m, NotationRequest{Lines: many}); err == nil {
		t.Error("accepted too many lines")
	}
	// a range past the end is clamped, not an error
	if _, err := NotationXML(m, NotationRequest{Lines: []NotationLine{{ID: m.Lines[0].ID}}, From: -5, To: 999}); err != nil {
		t.Errorf("range not clamped: %v", err)
	}
}

func TestSplitUnits(t *testing.T) {
	for _, units := range []int{192, 144, 96, 72, 48, 36, 24, 18, 12, 6, 16, 32, 64, 8, 40, 80, 112, 120, 60, 30} {
		sum := 0
		for _, v := range splitUnits(units) {
			sum += v.units
		}
		if sum != units {
			t.Errorf("%d units written as %d", units, sum)
		}
	}
}
