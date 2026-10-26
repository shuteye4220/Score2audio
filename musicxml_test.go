package main

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestRepeatWithEndings(t *testing.T) {
	// 1 |: 2 3 [1. 4 :| [2. 5 ] 6
	f := make([]measureFacts, 6)
	f[1].repeatFwd = true
	f[3].endings, f[3].endingType = "1", "stop"
	f[3].repeatBack = 2
	f[4].endings, f[4].endingType = "2", "stop"
	got, rep := performanceOrder(f)
	want := []int{0, 1, 2, 3, 1, 2, 4, 5}
	if !rep || !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestEndingWithoutRepeatIsIgnored(t *testing.T) {
	f := make([]measureFacts, 4)
	f[2].endings, f[2].endingType = "1", "discontinue"
	got, rep := performanceOrder(f)
	if rep || !reflect.DeepEqual(got, []int{0, 1, 2, 3}) {
		t.Fatalf("got %v", got)
	}
}

const sample = `<?xml version="1.0" encoding="UTF-8"?>
<score-partwise version="3.1">
 <work><work-title>Test Hymn</work-title></work>
 <part-list>
  <score-part id="P1"><part-name>Women</part-name></score-part>
  <score-part id="P2"><part-name>Piano</part-name></score-part>
 </part-list>
 <part id="P1">
  <measure number="1">
   <attributes><divisions>2</divisions><time><beats>2</beats><beat-type>4</beat-type></time><clef><sign>G</sign><line>2</line></clef></attributes>
   <direction><sound tempo="120"/></direction>
   <note><pitch><step>E</step><octave>5</octave></pitch><duration>2</duration><voice>1</voice><type>quarter</type><lyric><syllabic>begin</syllabic><text>Al</text></lyric></note>
   <note><pitch><step>D</step><octave>5</octave></pitch><duration>2</duration><voice>1</voice><type>quarter</type><tie type="start"/><lyric><syllabic>end</syllabic><text>ways</text></lyric></note>
   <backup><duration>4</duration></backup>
   <note><pitch><step>C</step><octave>5</octave></pitch><duration>4</duration><voice>2</voice><type>half</type></note>
  </measure>
  <measure number="2">
   <note><pitch><step>D</step><octave>5</octave></pitch><duration>2</duration><voice>1</voice><type>quarter</type><tie type="stop"/></note>
   <note><rest/><duration>2</duration><voice>1</voice></note>
   <backup><duration>4</duration></backup>
   <note><pitch><step>B</step><alter>-1</alter><octave>4</octave></pitch><duration>4</duration><voice>2</voice><type>half</type></note>
  </measure>
 </part>
 <part id="P2">
  <measure number="1"><attributes><divisions>1</divisions><staves>2</staves></attributes>
   <note><pitch><step>C</step><octave>4</octave></pitch><duration>2</duration><staff>1</staff><type>half</type></note></measure>
  <measure number="2"><note><rest/><duration>2</duration><staff>1</staff></note></measure>
 </part>
</score-partwise>`

func TestParseSample(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.musicxml")
	os.WriteFile(p, []byte(sample), 0o644)
	m, err := LoadModel([]string{p}, "x")
	if err != nil {
		t.Fatal(err)
	}
	if m.Title != "Test Hymn" || len(m.Measures) != 2 || m.Tempo[0].BPM != 120 {
		t.Fatalf("basic facts wrong: %+v", m)
	}
	byID := map[string]Line{}
	for _, l := range m.Lines {
		byID[l.ID] = l
	}
	up, low := byID["p1s1u"], byID["p1s1l"]
	if len(up.Events) != 2 || up.Events[1].Midi != 74 || up.Events[1].D != 2 {
		t.Fatalf("upper line wrong (tie not merged?): %+v", up.Events)
	}
	if len(low.Events) != 2 || low.Events[1].Name != "B flat 4" {
		t.Fatalf("lower line wrong: %+v", low.Events)
	}
	if up.Hint != "usually soprano" || byID["p2s1"].Kind != "keyboard" {
		t.Fatalf("names wrong: %q %q", up.Hint, byID["p2s1"].Kind)
	}
	if len(m.Verses) != 1 || m.Verses[0].Words[0].Text != "Always" {
		t.Fatalf("lyrics wrong: %+v", m.Verses)
	}
}

func TestS2ARoundTrip(t *testing.T) {
	dir := t.TempDir()
	dataDir = dir
	p := filepath.Join(dir, "a.musicxml")
	os.WriteFile(p, []byte(sample), 0o644)
	os.MkdirAll(filepath.Join(dir, "scores", "20260101000000aaaa"), 0o755)
	meta := &ScoreMeta{ID: "20260101000000aaaa", Title: "Test Hymn", FileName: "hymn.pdf", Kind: "pdf"}
	os.MkdirAll(filepath.Join(scoreDir(meta.ID), "omr"), 0o755)
	data, _ := os.ReadFile(p)
	os.WriteFile(filepath.Join(scoreDir(meta.ID), "omr", "hymn.musicxml"), data, 0o644)
	if _, err := buildAndStore(meta, []string{filepath.Join(scoreDir(meta.ID), "omr", "hymn.musicxml")}); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(scoreDir(meta.ID), "prefs.json"), []byte(`{"opts":{"tempo":70}}`), 0o644)

	rec := httptest.NewRecorder()
	exportS2A(rec, meta)
	if rec.Code != 200 {
		t.Fatalf("export failed: %d %s", rec.Code, rec.Body.String())
	}
	f, music, err := readS2A(rec.Body.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if f.Title != "Test Hymn" || f.SourceFile != "hymn.pdf" || len(music) != 1 || f.Model == nil || len(f.Model.Lines) == 0 || string(f.Prefs) != `{"opts":{"tempo":70}}` {
		t.Fatalf("export content wrong: %+v (music files %d)", f, len(music))
	}
	if _, _, err := readS2A([]byte("not a zip")); err == nil {
		t.Fatal("accepted a non-score file")
	}
}
