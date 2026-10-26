package main

// The score2audio score file (.s2a): one file holding a score that has
// already been read, ready to play on another computer or on the mobile app
// without reading the PDF again. It is a zip file containing:
//
//	score.json        everything needed to play the score (see S2AFile)
//	musicxml/*.mxl    the recognised music, for other programs or later versions
//
// The format is described for app writers in docs/s2a-format.md.

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	s2aFormat        = "score2audio-score"
	s2aFormatVersion = 1
)

type S2AFile struct {
	Format        string          `json:"format"`
	FormatVersion int             `json:"formatVersion"`
	AppVersion    string          `json:"appVersion"`
	Exported      time.Time       `json:"exported"`
	Title         string          `json:"title"`
	SourceFile    string          `json:"sourceFile"`
	SkippedPages  []int           `json:"skippedPages,omitempty"`
	MusicXML      []string        `json:"musicxml"`
	Model         *Model          `json:"model"`
	Prefs         json.RawMessage `json:"prefs"`
}

// storedModel loads a score's playback model, rebuilding it from the stored
// MusicXML when it was made by an older version of score2audio.
func storedModel(meta *ScoreMeta) (*Model, error) {
	dir := scoreDir(meta.ID)
	b, err := os.ReadFile(filepath.Join(dir, "model.json"))
	var m Model
	if err == nil {
		err = json.Unmarshal(b, &m)
	}
	if err == nil && m.Version == modelVersion {
		return &m, nil
	}
	return buildAndStore(meta, scoreMusicXML(dir))
}

func scoreMusicXML(dir string) []string {
	files := mxlFiles(filepath.Join(dir, "omr"))
	for _, ext := range []string{".mxl", ".musicxml", ".xml"} {
		if isFile(filepath.Join(dir, "source"+ext)) {
			files = []string{filepath.Join(dir, "source"+ext)}
		}
	}
	return files
}

func safeFileName(title string) string {
	s := regexp.MustCompile(`[\\/:*?"<>|\x00-\x1f]+`).ReplaceAllString(title, " ")
	s = strings.Join(strings.Fields(s), " ")
	if s == "" {
		s = "score"
	}
	return s
}

func exportS2A(w http.ResponseWriter, meta *ScoreMeta) {
	m, err := storedModel(meta)
	if err != nil {
		httpErr(w, 500, "Could not prepare the score: "+err.Error())
		return
	}
	dir := scoreDir(meta.ID)
	prefs := json.RawMessage("null")
	if p, err := os.ReadFile(filepath.Join(dir, "prefs.json")); err == nil && json.Valid(p) {
		prefs = p
	}
	f := S2AFile{Format: s2aFormat, FormatVersion: s2aFormatVersion, AppVersion: appVersion, Exported: time.Now().UTC(),
		Title: meta.Title, SourceFile: meta.FileName, SkippedPages: meta.SkippedPages, Model: m, Prefs: prefs, MusicXML: []string{}}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for i, src := range scoreMusicXML(dir) {
		data, err := os.ReadFile(src)
		if err != nil {
			continue
		}
		ext := strings.ToLower(filepath.Ext(src))
		name := fmt.Sprintf("musicxml/part%d%s", i+1, ext)
		if i == 0 {
			name = "musicxml/score" + ext
		}
		method := zip.Deflate
		if ext == ".mxl" {
			method = zip.Store // already compressed
		}
		zf, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: method, Modified: time.Now()})
		if err != nil {
			continue
		}
		zf.Write(data)
		f.MusicXML = append(f.MusicXML, name)
	}
	js, _ := json.Marshal(f)
	zf, _ := zw.CreateHeader(&zip.FileHeader{Name: "score.json", Method: zip.Deflate, Modified: time.Now()})
	zf.Write(js)
	if err := zw.Close(); err != nil {
		httpErr(w, 500, "Could not prepare the score: "+err.Error())
		return
	}
	name := safeFileName(meta.Title) + ".s2a"
	ascii := regexp.MustCompile(`[^\x20-\x7e]`).ReplaceAllString(name, "_")
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, strings.ReplaceAll(ascii, `"`, ""), url.PathEscape(name)))
	w.Write(buf.Bytes())
}

func importS2A(w http.ResponseWriter, r *http.Request, uploadName string) {
	data, err := io.ReadAll(io.LimitReader(r.Body, 200<<20))
	if err != nil || len(data) == 0 {
		httpErr(w, 400, "The file could not be received, or it was empty.")
		return
	}
	f, files, err := readS2A(data)
	if err != nil {
		httpErr(w, 400, err.Error())
		return
	}
	id := newID()
	dir := scoreDir(id)
	omr := filepath.Join(dir, "omr")
	os.MkdirAll(omr, 0o755)
	var stored []string
	for i, x := range files {
		p := filepath.Join(omr, fmt.Sprintf("score%02d%s", i+1, x.ext))
		if err := os.WriteFile(p, x.data, 0o644); err == nil {
			stored = append(stored, p)
		}
	}
	title := strings.TrimSpace(f.Title)
	if title == "" {
		title = strings.TrimSuffix(uploadName, filepath.Ext(uploadName))
	}
	src := f.SourceFile
	if src == "" {
		src = uploadName
	}
	meta := &ScoreMeta{ID: id, Title: title, FileName: src, Kind: "import", Created: time.Now(), SkippedPages: f.SkippedPages}
	if f.Model != nil && f.Model.Version == modelVersion {
		b, _ := json.Marshal(f.Model)
		os.WriteFile(filepath.Join(dir, "model.json"), b, 0o644)
		meta.Ready, meta.Measures = true, len(f.Model.Measures)
		writeMeta(meta)
	} else if len(stored) > 0 {
		writeMeta(meta)
		if _, err := buildAndStore(meta, stored); err != nil {
			os.RemoveAll(dir)
			httpErr(w, 400, "The music in that score file could not be read: "+err.Error())
			return
		}
	} else {
		os.RemoveAll(dir)
		httpErr(w, 400, "That score file does not contain any music.")
		return
	}
	if len(f.Prefs) > 0 && json.Valid(f.Prefs) && string(f.Prefs) != "null" {
		os.WriteFile(filepath.Join(dir, "prefs.json"), f.Prefs, 0o644)
	}
	writeJSON(w, map[string]string{"scoreId": id})
}

type s2aMusic struct {
	ext  string
	data []byte
}

// readS2A unpacks and checks a score file.
func readS2A(data []byte) (*S2AFile, []s2aMusic, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, nil, fmt.Errorf("That is not a score2audio score file.")
	}
	var f *S2AFile
	var music []s2aMusic
	for _, zf := range zr.File {
		if zf.UncompressedSize64 > 100<<20 {
			return nil, nil, fmt.Errorf("That score file is too large.")
		}
		name := path.Clean(strings.ReplaceAll(zf.Name, `\`, "/"))
		read := func() ([]byte, error) {
			rc, err := zf.Open()
			if err != nil {
				return nil, err
			}
			defer rc.Close()
			return io.ReadAll(io.LimitReader(rc, 100<<20))
		}
		switch {
		case name == "score.json":
			b, err := read()
			if err != nil {
				return nil, nil, fmt.Errorf("That score file is damaged.")
			}
			var x S2AFile
			if err := json.Unmarshal(b, &x); err != nil || x.Format != s2aFormat {
				return nil, nil, fmt.Errorf("That is not a score2audio score file.")
			}
			if x.FormatVersion > s2aFormatVersion {
				return nil, nil, fmt.Errorf("That score file was made by a newer version of score2audio. Please update score2audio to open it.")
			}
			f = &x
		case strings.HasPrefix(name, "musicxml/"):
			ext := strings.ToLower(path.Ext(name))
			if ext != ".mxl" && ext != ".musicxml" && ext != ".xml" {
				continue
			}
			b, err := read()
			if err == nil {
				music = append(music, s2aMusic{ext: ext, data: b})
			}
		}
	}
	if f == nil {
		return nil, nil, fmt.Errorf("That is not a score2audio score file.")
	}
	return f, music, nil
}
