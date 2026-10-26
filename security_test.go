package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// zipOf builds a zip in memory; each entry is compressed with Deflate.
func zipOf(t *testing.T, entries map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, data := range entries {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate})
		if err != nil {
			t.Fatal(err)
		}
		w.Write(data)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestMXLZipBombIsRefused(t *testing.T) {
	// 80 MB of zeros compresses to well under 1 MB.
	bomb := zipOf(t, map[string][]byte{"score.xml": make([]byte, 80<<20)})
	if len(bomb) > 1<<20 {
		t.Fatalf("test zip unexpectedly large: %d", len(bomb))
	}
	p := filepath.Join(t.TempDir(), "bomb.mxl")
	os.WriteFile(p, bomb, 0o600)
	if _, err := readMusicXMLBytes(p); err != errMusicXMLTooLarge {
		t.Fatalf("expected errMusicXMLTooLarge, got %v", err)
	}
}

func TestMXLStillReadsNormalFiles(t *testing.T) {
	mxl := zipOf(t, map[string][]byte{
		"META-INF/container.xml": []byte(`<container><rootfiles><rootfile full-path="music/score.xml"/></rootfiles></container>`),
		"music/score.xml":        []byte(sample),
	})
	p := filepath.Join(t.TempDir(), "ok.mxl")
	os.WriteFile(p, mxl, 0o600)
	m, err := LoadModel([]string{p}, "x")
	if err != nil || m.Title != "Test Hymn" {
		t.Fatalf("normal .mxl not read: %v", err)
	}
}

func TestS2ATooManyOrTooLargeEntries(t *testing.T) {
	score := []byte(`{"format":"score2audio-score","formatVersion":1,"title":"x"}`)
	many := map[string][]byte{"score.json": score}
	for i := 0; i < maxS2AMusicFiles+1; i++ {
		many[filepath.ToSlash(filepath.Join("musicxml", strings.Repeat("a", i+1)+".xml"))] = []byte("<x/>")
	}
	if _, _, err := readS2A(zipOf(t, many)); err == nil {
		t.Fatal("accepted a score file with too many music files")
	}
	big := zipOf(t, map[string][]byte{"score.json": score, "musicxml/a.xml": make([]byte, maxS2AEntryBytes+1)})
	if _, _, err := readS2A(big); err == nil {
		t.Fatal("accepted a score file with an oversized entry")
	}
}

func craftedModel() *Model {
	return &Model{
		Version:  modelVersion,
		Title:    strings.Repeat("T", 10000),
		Measures: []MeasureInfo{{Number: "1", T: 0, D: 1e12, Beats: 0, BeatType: 1000000}, {Number: "2", T: math.Inf(1), D: math.NaN(), Beats: 4, BeatType: 4}},
		Order:    []int{0, 1, 5, -3, 1},
		Tempo:    []TempoPoint{{T: 0, BPM: 0}, {T: 1, BPM: math.NaN()}, {T: 2, BPM: 1e9}},
		Lines: []Line{{ID: "l1", Events: []Event{
			{T: 0, D: 1e15, Midi: 60, Mi: 0},
			{T: 1, D: 1, Midi: 300, Mi: 0},
			{T: 1, D: 1, Midi: 60, Mi: 99},
			{T: math.NaN(), D: 1, Midi: 60, Mi: 1},
		}}},
		Verses: []Verse{{Name: "V", Words: []Word{{T: 0, Mi: 7, Text: "x"}}, Lines: []LyricLine{{From: 1, To: 0}, {From: 0, To: 1, Text: "ok"}}}},
	}
}

func TestSanitizeModel(t *testing.T) {
	m := craftedModel()
	if err := sanitizeModel(m); err != nil {
		t.Fatal(err)
	}
	ms := m.Measures
	if ms[0].D != maxMeasureQN || ms[0].Beats != 4 || ms[0].BeatType != 4 || ms[1].D != 0 || ms[1].T != 0 {
		t.Fatalf("measures not repaired: %+v", ms)
	}
	if len(m.Order) != 3 || m.Order[2] != 1 {
		t.Fatalf("order not filtered: %v", m.Order)
	}
	// 0 and NaN are dropped, 1e9 is limited, and a starting tempo is added.
	if len(m.Tempo) != 2 || m.Tempo[0].T != 0 || m.Tempo[0].BPM != maxBPM || m.Tempo[1].BPM != maxBPM {
		t.Fatalf("tempo not repaired: %+v", m.Tempo)
	}
	if ev := m.Lines[0].Events; len(ev) != 1 || ev[0].D != maxMeasureQN {
		t.Fatalf("events not filtered: %+v", ev)
	}
	if len(m.Verses[0].Words) != 0 || len(m.Verses[0].Lines) != 1 {
		t.Fatalf("lyrics not filtered: %+v", m.Verses[0])
	}
	if len([]rune(m.Title)) != maxShortText {
		t.Fatalf("title not shortened")
	}
	if err := sanitizeModel(&Model{}); err == nil {
		t.Fatal("accepted a model with no measures")
	}
	// The result must still be valid JSON (NaN and Inf are not).
	if _, err := json.Marshal(m); err != nil {
		t.Fatal(err)
	}
}

func TestImportedS2AModelIsSanitized(t *testing.T) {
	dataDir = t.TempDir()
	os.MkdirAll(filepath.Join(dataDir, "scores"), 0o700)
	m := craftedModel()
	m.Tempo = []TempoPoint{{T: 0, BPM: 0}}
	m.Measures[1].T, m.Measures[1].D = 0, 0 // JSON cannot carry Inf/NaN
	m.Lines[0].Events = m.Lines[0].Events[:3]
	js, err := json.Marshal(S2AFile{Format: s2aFormat, FormatVersion: 1, Title: "Crafted", SourceFile: `..\..\evil.pdf`, Model: m})
	if err != nil {
		t.Fatal(err)
	}
	body := zipOf(t, map[string][]byte{"score.json": js})
	req := httptest.NewRequest(http.MethodPost, "/api/upload?name=x.s2a", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	importS2A(rec, req, "x.s2a")
	if rec.Code != 200 {
		t.Fatalf("import failed: %d %s", rec.Code, rec.Body.String())
	}
	var res map[string]string
	json.Unmarshal(rec.Body.Bytes(), &res)
	meta, err := readMeta(res["scoreId"])
	if err != nil {
		t.Fatal(err)
	}
	if meta.FileName != "evil.pdf" {
		t.Fatalf("source file name not reduced to a plain name: %q", meta.FileName)
	}
	stored, err := storedModel(meta)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Tempo[0].BPM < minBPM || stored.Measures[0].BeatType != 4 || len(stored.Lines[0].Events) != 1 {
		t.Fatalf("stored model not sanitized: tempo %+v measures %+v", stored.Tempo, stored.Measures)
	}
	if st, err := os.Stat(filepath.Join(scoreDir(meta.ID), "model.json")); err != nil || st.Mode().Perm()&0o077 != 0 {
		t.Fatalf("library file readable by other users: %v", st.Mode())
	}
}

func TestHostGuard(t *testing.T) {
	token = "secret-token-for-test"
	h := hostGuard("127.0.0.1:1234", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	try := func(host, tok string) int {
		r := httptest.NewRequest(http.MethodGet, "/api/status", nil)
		r.Host = host
		if tok != "" {
			r.Header.Set("X-Token", tok)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		return rec.Code
	}
	if c := try("127.0.0.1:1234", token); c != 200 {
		t.Fatalf("own page refused: %d", c)
	}
	if c := try("localhost:1234", token); c != 200 {
		t.Fatalf("localhost refused: %d", c)
	}
	if c := try("evil.example:1234", token); c != 403 {
		t.Fatalf("DNS-rebinding host allowed: %d", c)
	}
	if c := try("127.0.0.1:1234", ""); c != 403 {
		t.Fatalf("missing token allowed: %d", c)
	}
	if c := try("127.0.0.1:1234", token+"x"); c != 403 {
		t.Fatalf("wrong token allowed: %d", c)
	}
}
