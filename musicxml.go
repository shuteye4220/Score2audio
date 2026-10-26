package main

// MusicXML reading: turns a .mxl / .musicxml file into the playback model
// used by the web page (measures, voice lines with timed notes, lyrics).

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

const modelVersion = 6

// ---------- raw XML structures ----------

type xScore struct {
	XMLName xml.Name
	Work    struct {
		Title string `xml:"work-title"`
	} `xml:"work"`
	MovementTitle string `xml:"movement-title"`
	PartList      struct {
		ScoreParts []struct {
			ID   string `xml:"id,attr"`
			Name string `xml:"part-name"`
		} `xml:"score-part"`
	} `xml:"part-list"`
	Parts []struct {
		ID       string     `xml:"id,attr"`
		Measures []xMeasure `xml:"measure"`
	} `xml:"part"`
}

type xMeasure struct {
	Number   string
	Implicit string
	Items    []interface{}
}

type xPitch struct {
	Step   string  `xml:"step"`
	Alter  float64 `xml:"alter"`
	Octave int     `xml:"octave"`
}

type xLyric struct {
	Number   string   `xml:"number,attr"`
	Syllabic []string `xml:"syllabic"`
	Texts    []string `xml:"text"`
}

type xTie struct {
	Type string `xml:"type,attr"`
}

type xNote struct {
	Grace    *struct{}  `xml:"grace"`
	Chord    *struct{}  `xml:"chord"`
	Rest     *struct{}  `xml:"rest"`
	Pitch    *xPitch    `xml:"pitch"`
	Duration float64    `xml:"duration"`
	Ties     []xTie     `xml:"tie"`
	Tied     []xTie     `xml:"notations>tied"`
	Voice    string     `xml:"voice"`
	Type     string     `xml:"type"`
	Dots     []struct{} `xml:"dot"`
	Staff    int        `xml:"staff"`
	TimeMod  *struct {
		Actual int `xml:"actual-notes"`
		Normal int `xml:"normal-notes"`
	} `xml:"time-modification"`
	Lyrics []xLyric `xml:"lyric"`
}

type xAttributes struct {
	Divisions float64 `xml:"divisions"`
	Keys      []struct {
		Fifths int `xml:"fifths"`
	} `xml:"key"`
	Times []struct {
		Beats    string `xml:"beats"`
		BeatType string `xml:"beat-type"`
	} `xml:"time"`
	Staves int `xml:"staves"`
	Clefs  []struct {
		Number       int    `xml:"number,attr"`
		Sign         string `xml:"sign"`
		Line         int    `xml:"line"`
		OctaveChange int    `xml:"clef-octave-change"`
	} `xml:"clef"`
	Transposes []struct {
		Chromatic    int `xml:"chromatic"`
		OctaveChange int `xml:"octave-change"`
	} `xml:"transpose"`
}

type xSound struct {
	Tempo string `xml:"tempo,attr"`
}

type xDirection struct {
	Sound      *xSound `xml:"sound"`
	Metronomes []struct {
		BeatUnit  string     `xml:"beat-unit"`
		Dots      []struct{} `xml:"beat-unit-dot"`
		PerMinute string     `xml:"per-minute"`
	} `xml:"direction-type>metronome"`
}

type xBarline struct {
	Location string `xml:"location,attr"`
	Repeat   *struct {
		Direction string `xml:"direction,attr"`
		Times     string `xml:"times,attr"`
	} `xml:"repeat"`
	Ending *struct {
		Number string `xml:"number,attr"`
		Type   string `xml:"type,attr"`
	} `xml:"ending"`
}

type xPrint struct {
	NewSystem string `xml:"new-system,attr"`
	NewPage   string `xml:"new-page,attr"`
}

type xMove struct {
	Duration float64 `xml:"duration"`
}
type xBackup struct{ xMove }
type xForward struct{ xMove }

func (m *xMeasure) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	for _, a := range start.Attr {
		switch a.Name.Local {
		case "number":
			m.Number = a.Value
		case "implicit":
			m.Implicit = a.Value
		}
	}
	for {
		tok, err := d.Token()
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			var item interface{}
			switch t.Name.Local {
			case "note":
				item = &xNote{}
			case "attributes":
				item = &xAttributes{}
			case "direction":
				item = &xDirection{}
			case "sound":
				item = &xSound{}
			case "barline":
				item = &xBarline{}
			case "print":
				item = &xPrint{}
			case "backup":
				item = &xBackup{}
			case "forward":
				item = &xForward{}
			}
			if item == nil {
				if err := d.Skip(); err != nil {
					return err
				}
				continue
			}
			if err := d.DecodeElement(item, &t); err != nil {
				return err
			}
			m.Items = append(m.Items, item)
		case xml.EndElement:
			return nil
		}
	}
}

// ---------- output model ----------

type MeasureInfo struct {
	Number    string  `json:"n"`
	T         float64 `json:"t"`
	D         float64 `json:"d"`
	Beats     int     `json:"beats"`
	BeatType  int     `json:"beatType"`
	NewSystem bool    `json:"sys,omitempty"`
}

type TempoPoint struct {
	T   float64 `json:"t"`
	BPM float64 `json:"bpm"` // quarter notes per minute
}

type Event struct {
	T     float64 `json:"t"`
	D     float64 `json:"d"`
	Midi  int     `json:"m"`
	Mi    int     `json:"mi"`
	Name  string  `json:"name"`
	Dur   string  `json:"dur"`
	Lyric string  `json:"ly,omitempty"`
}

type Line struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Hint     string  `json:"hint,omitempty"`
	Kind     string  `json:"kind"` // voice | instrument | keyboard | other
	Clef     string  `json:"clef"`
	Low      string  `json:"low"`
	High     string  `json:"high"`
	Count    int     `json:"count"`
	Default  bool    `json:"default"`
	Events   []Event `json:"events"`
	StaffKey string  `json:"staff"`
	Part     string  `json:"part"` // upper | lower | all | single
	// Primary lines together cover a staff without playing notes twice.
	Primary       bool   `json:"primary"`
	Transpose     int    `json:"transpose,omitempty"`
	TransposeNote string `json:"transposeNote,omitempty"`
}

type Word struct {
	T    float64 `json:"t"`
	Mi   int     `json:"mi"`
	Text string  `json:"text"`
}

type LyricLine struct {
	From int    `json:"from"`
	To   int    `json:"to"`
	Text string `json:"text"`
}

type Verse struct {
	Name  string      `json:"name"`
	Words []Word      `json:"words"`
	Lines []LyricLine `json:"lines"`
}

type Model struct {
	Version        int           `json:"version"`
	Title          string        `json:"title"`
	Measures       []MeasureInfo `json:"measures"`
	Order          []int         `json:"order"`
	HasRepeats     bool          `json:"hasRepeats"`
	Tempo          []TempoPoint  `json:"tempo"`
	TempoFromScore bool          `json:"tempoFromScore"`
	Lines          []Line        `json:"lines"`
	Verses         []Verse       `json:"verses"`
	Warnings       []string      `json:"warnings"`
}

// ---------- loading ----------

func readMusicXMLBytes(p string) ([]byte, error) {
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	if len(data) > 2 && data[0] == 'P' && data[1] == 'K' { // compressed .mxl
		zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return nil, err
		}
		root := ""
		for _, f := range zr.File {
			if f.Name == "META-INF/container.xml" {
				rc, _ := f.Open()
				b, _ := io.ReadAll(rc)
				rc.Close()
				var c struct {
					Rootfiles []struct {
						Path string `xml:"full-path,attr"`
					} `xml:"rootfiles>rootfile"`
				}
				if xml.Unmarshal(b, &c) == nil && len(c.Rootfiles) > 0 {
					root = c.Rootfiles[0].Path
				}
			}
		}
		for _, f := range zr.File {
			if (root != "" && f.Name == root) ||
				(root == "" && !strings.HasPrefix(f.Name, "META-INF") && strings.HasSuffix(strings.ToLower(f.Name), "xml")) {
				rc, err := f.Open()
				if err != nil {
					return nil, err
				}
				defer rc.Close()
				return io.ReadAll(rc)
			}
		}
		return nil, errors.New("no music found inside the compressed MusicXML file")
	}
	return data, nil
}

func parseScoreFile(p string) (*xScore, error) {
	data, err := readMusicXMLBytes(p)
	if err != nil {
		return nil, err
	}
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.Strict = false
	dec.CharsetReader = func(label string, input io.Reader) (io.Reader, error) { return input, nil }
	var s xScore
	if err := dec.Decode(&s); err != nil {
		return nil, fmt.Errorf("could not read MusicXML: %v", err)
	}
	if s.XMLName.Local == "score-timewise" {
		return nil, errors.New("timewise MusicXML is not supported; please export partwise MusicXML")
	}
	if len(s.Parts) == 0 {
		return nil, errors.New("the file contains no parts")
	}
	return &s, nil
}

// LoadModel parses one or more MusicXML files (Audiveris may split a score
// into movements) and joins them end to end.
func LoadModel(files []string, fallbackTitle string) (*Model, error) {
	var scores []*xScore
	for _, f := range files {
		s, err := parseScoreFile(f)
		if err != nil {
			return nil, err
		}
		scores = append(scores, s)
	}
	main := scores[0]
	var warnings []string
	for i, s := range scores[1:] {
		if len(s.Parts) != len(main.Parts) {
			warnings = append(warnings, fmt.Sprintf("Section %d of the score had a different number of staves and was left out.", i+2))
			continue
		}
		for pi := range main.Parts {
			main.Parts[pi].Measures = append(main.Parts[pi].Measures, s.Parts[pi].Measures...)
		}
	}
	mergeSplitParts(main)
	m := buildModel(main, fallbackTitle)
	m.Warnings = append(append([]string{}, warnings...), m.Warnings...)
	return m, nil
}

// ---------- model building ----------

type rawLyric struct {
	verse, syllabic, text string
}

type rawNote struct {
	part, staff int
	voice       string
	mi          int
	on          float64 // offset within measure (quarter notes)
	t, d        float64 // absolute (quarter notes)
	midi        int
	name        string
	typ         string
	dots        int
	tuplet      bool
	tieStart    bool
	tieStop     bool
	lyrics      []rawLyric
	merged      bool
}

type restKey struct{ part, staff, mi int }

type measureFacts struct {
	beats, beatType int
	maxLen          float64
	implicit        bool
	newSystem       bool
	repeatFwd       bool
	repeatBack      int // times, 0 = none
	endings         string
	endingType      string
	tempos          []TempoPoint // t = offset within measure
}

var stepSemis = map[string]int{"C": 0, "D": 2, "E": 4, "F": 5, "G": 7, "A": 9, "B": 11}

func pitchName(step string, alter float64, octave int) string {
	s := strings.ToUpper(step)
	switch int(math.Round(alter)) {
	case 1:
		s += " sharp"
	case 2:
		s += " double sharp"
	case -1:
		s += " flat"
	case -2:
		s += " double flat"
	}
	return fmt.Sprintf("%s %d", s, octave)
}

var sharpNames = []string{"C", "C sharp", "D", "E flat", "E", "F", "F sharp", "G", "A flat", "A", "B flat", "B"}

func midiName(m int) string {
	return fmt.Sprintf("%s %d", sharpNames[((m%12)+12)%12], m/12-1)
}

func parseBeats(s string) int {
	total := 0
	for _, p := range strings.Split(s, "+") {
		n, _ := strconv.Atoi(strings.TrimSpace(p))
		total += n
	}
	return total
}

func buildModel(s *xScore, fallbackTitle string) *Model {
	m := &Model{Version: modelVersion}
	m.Title = strings.TrimSpace(s.Work.Title)
	if m.Title == "" {
		m.Title = strings.TrimSpace(s.MovementTitle)
	}
	if m.Title == "" {
		m.Title = fallbackTitle
	}

	nMeasures := 0
	for _, p := range s.Parts {
		if len(p.Measures) > nMeasures {
			nMeasures = len(p.Measures)
		}
	}
	facts := make([]measureFacts, nMeasures)
	partNames := make([]string, len(s.Parts))
	for i, p := range s.Parts {
		for _, sp := range s.PartList.ScoreParts {
			if sp.ID == p.ID {
				partNames[i] = strings.TrimSpace(sp.Name)
			}
		}
	}

	type staffKey struct{ part, staff int }
	staffClefs := map[staffKey]map[string]int{} // clef description -> number of measures
	partStaves := make([]int, len(s.Parts))
	var notes []*rawNote
	restVoices := map[restKey]map[string]bool{}
	partHasTranspose := make([]bool, len(s.Parts))

	for pi, p := range s.Parts {
		div := 1.0
		beats, beatType := 4, 4
		transpose := 0
		curClef := map[int]string{}
		partStaves[pi] = 1
		for mi, meas := range p.Measures {
			f := &facts[mi]
			cursor, maxCursor, lastOnset := 0.0, 0.0, 0.0
			if strings.EqualFold(meas.Implicit, "yes") {
				f.implicit = true
			}
			for _, it := range meas.Items {
				switch x := it.(type) {
				case *xAttributes:
					if x.Divisions > 0 {
						div = x.Divisions
					}
					if len(x.Times) > 0 {
						if b := parseBeats(x.Times[0].Beats); b > 0 {
							beats = b
						}
						if bt, _ := strconv.Atoi(x.Times[0].BeatType); bt > 0 {
							beatType = bt
						}
					}
					if x.Staves > partStaves[pi] {
						partStaves[pi] = x.Staves
					}
					for _, c := range x.Clefs {
						n := c.Number
						if n == 0 {
							n = 1
						}
						curClef[n] = clefDescription(c.Sign, c.Line, c.OctaveChange)
					}
					if len(x.Transposes) > 0 {
						partHasTranspose[pi] = true
						transpose = x.Transposes[0].Chromatic + 12*x.Transposes[0].OctaveChange
					}
				case *xNote:
					if x.Grace != nil {
						continue
					}
					dur := x.Duration / div
					onset := cursor / div
					if x.Chord != nil {
						onset = lastOnset
					} else {
						lastOnset = onset
						cursor += x.Duration
						if cursor > maxCursor {
							maxCursor = cursor
						}
					}
					staff := x.Staff
					if staff == 0 {
						staff = 1
					}
					if staff > partStaves[pi] {
						partStaves[pi] = staff
					}
					voice := x.Voice
					if voice == "" {
						voice = "1"
					}
					if x.Rest != nil || x.Pitch == nil {
						rk := restKey{pi, staff, mi}
						if restVoices[rk] == nil {
							restVoices[rk] = map[string]bool{}
						}
						restVoices[rk][voice] = true
						continue
					}
					midi := (x.Pitch.Octave+1)*12 + stepSemis[strings.ToUpper(x.Pitch.Step)] + int(math.Round(x.Pitch.Alter)) + transpose
					rn := &rawNote{part: pi, staff: staff, voice: voice, mi: mi, on: onset, d: dur, midi: midi,
						name: pitchName(x.Pitch.Step, x.Pitch.Alter, x.Pitch.Octave), typ: x.Type, dots: len(x.Dots),
						tuplet: x.TimeMod != nil && x.TimeMod.Actual > 0 && x.TimeMod.Actual != x.TimeMod.Normal}
					if transpose != 0 {
						rn.name = midiName(midi)
					}
					for _, t := range append(x.Ties, x.Tied...) {
						switch t.Type {
						case "start":
							rn.tieStart = true
						case "stop":
							rn.tieStop = true
						}
					}
					for _, l := range x.Lyrics {
						txt := strings.TrimSpace(strings.Join(l.Texts, " "))
						if txt == "" {
							continue
						}
						syl := "single"
						if len(l.Syllabic) > 0 {
							syl = l.Syllabic[0]
						}
						v := l.Number
						if v == "" {
							v = "1"
						}
						rn.lyrics = append(rn.lyrics, rawLyric{verse: v, syllabic: syl, text: txt})
					}
					notes = append(notes, rn)
					k := staffKey{pi, staff}
					if staffClefs[k] == nil {
						staffClefs[k] = map[string]int{}
					}
					if c, ok := curClef[staff]; ok {
						staffClefs[k][c]++
					}
				case *xBackup:
					cursor -= x.Duration
					if cursor < 0 {
						cursor = 0
					}
				case *xForward:
					cursor += x.Duration
					if cursor > maxCursor {
						maxCursor = cursor
					}
				case *xDirection:
					if x.Sound != nil && x.Sound.Tempo != "" {
						if bpm, err := strconv.ParseFloat(x.Sound.Tempo, 64); err == nil && bpm > 0 {
							f.tempos = append(f.tempos, TempoPoint{T: cursor / div, BPM: bpm})
						}
					} else {
						for _, mt := range x.Metronomes {
							if bpm := metronomeToQuarter(mt.BeatUnit, len(mt.Dots), mt.PerMinute); bpm > 0 {
								f.tempos = append(f.tempos, TempoPoint{T: cursor / div, BPM: bpm})
							}
						}
					}
				case *xSound:
					if bpm, err := strconv.ParseFloat(x.Tempo, 64); err == nil && bpm > 0 {
						f.tempos = append(f.tempos, TempoPoint{T: cursor / div, BPM: bpm})
					}
				case *xBarline:
					if x.Repeat != nil {
						if x.Repeat.Direction == "forward" {
							f.repeatFwd = true
						} else if x.Repeat.Direction == "backward" {
							t, _ := strconv.Atoi(x.Repeat.Times)
							if t < 2 {
								t = 2
							}
							f.repeatBack = t
						}
					}
					if x.Ending != nil {
						if x.Ending.Type == "start" {
							f.endings = x.Ending.Number
						}
						f.endingType = x.Ending.Type
					}
				case *xPrint:
					if x.NewSystem == "yes" || x.NewPage == "yes" {
						f.newSystem = true
					}
				}
			}
			if pi == 0 || f.beats == 0 {
				f.beats, f.beatType = beats, beatType
			}
			if l := maxCursor / div; l > f.maxLen {
				f.maxLen = l
			}
		}
	}

	// measure timeline
	t := 0.0
	for i := range facts {
		f := &facts[i]
		nominal := float64(f.beats) * 4 / float64(f.beatType)
		length := nominal
		if (f.implicit || i == 0) && f.maxLen > 0 && f.maxLen < nominal-0.01 {
			length = f.maxLen // pickup measure
		}
		num := strconv.Itoa(i + 1)
		if i < len(s.Parts[0].Measures) && s.Parts[0].Measures[i].Number != "" {
			num = s.Parts[0].Measures[i].Number
		}
		m.Measures = append(m.Measures, MeasureInfo{Number: num, T: t, D: length, Beats: f.beats, BeatType: f.beatType, NewSystem: f.newSystem || i == 0})
		t += length
	}
	for _, n := range notes {
		n.t = m.Measures[n.mi].T + n.on
	}

	// tempo
	seen := map[float64]bool{}
	for i, f := range facts {
		for _, tp := range f.tempos {
			at := math.Round((m.Measures[i].T+tp.T)*1000) / 1000
			if !seen[at] {
				seen[at] = true
				m.Tempo = append(m.Tempo, TempoPoint{T: at, BPM: tp.BPM})
			}
		}
	}
	sort.Slice(m.Tempo, func(a, b int) bool { return m.Tempo[a].T < m.Tempo[b].T })
	m.TempoFromScore = len(m.Tempo) > 0
	if len(m.Tempo) == 0 || m.Tempo[0].T > 0 {
		first := 90.0
		if len(m.Tempo) > 0 {
			first = m.Tempo[0].BPM
		}
		m.Tempo = append([]TempoPoint{{T: 0, BPM: first}}, m.Tempo...)
	}

	m.Order, m.HasRepeats = performanceOrder(facts)

	// merge ties, per staff
	byStaff := map[staffKey][]*rawNote{}
	var staffKeys []staffKey
	for _, n := range notes {
		k := staffKey{n.part, n.staff}
		if _, ok := byStaff[k]; !ok {
			staffKeys = append(staffKeys, k)
		}
		byStaff[k] = append(byStaff[k], n)
	}
	sort.Slice(staffKeys, func(a, b int) bool {
		if staffKeys[a].part != staffKeys[b].part {
			return staffKeys[a].part < staffKeys[b].part
		}
		return staffKeys[a].staff < staffKeys[b].staff
	})
	for _, k := range staffKeys {
		list := byStaff[k]
		sort.SliceStable(list, func(a, b int) bool { return list[a].t < list[b].t })
		pending := map[int]*rawNote{}
		var kept []*rawNote
		for _, n := range list {
			if p, ok := pending[n.midi]; ok && n.tieStop && math.Abs(p.t+p.d-n.t) < 0.02 {
				p.d += n.d
				p.merged = true
				p.lyrics = append(p.lyrics, n.lyrics...)
				if !n.tieStart {
					delete(pending, n.midi)
				}
				continue
			}
			kept = append(kept, n)
			if n.tieStart {
				pending[n.midi] = n
			} else {
				delete(pending, n.midi)
			}
		}
		byStaff[k] = kept
	}

	// what each part is
	labels := make([]partLabel, len(s.Parts))
	lyricCount := make([]int, len(s.Parts))
	noteCount := make([]int, len(s.Parts))
	for _, n := range notes {
		noteCount[n.part]++
		if len(n.lyrics) > 0 {
			lyricCount[n.part]++
		}
	}
	for pi := range s.Parts {
		labels[pi] = classifyLabel(partNames[pi])
		if strings.EqualFold(strings.TrimSpace(partNames[pi]), "piano") && partStaves[pi] < 2 {
			labels[pi] = partLabel{} // Audiveris placeholder on a single staff
		}
		if labels[pi].Kind == "" {
			switch {
			case partStaves[pi] >= 2:
				labels[pi].Kind = "keyboard"
			case lyricCount[pi] >= 3 && float64(lyricCount[pi]) >= 0.04*float64(noteCount[pi]):
				labels[pi].Kind, labels[pi].Generic = "voice", true
			}
		}
		if partHasTranspose[pi] {
			labels[pi].Transpose, labels[pi].TransposeNote = 0, ""
		}
	}
	// an unlabelled staff right under an unlabelled voice staff is usually
	// another voice part whose words are printed with the staff above
	for pi := 1; pi < len(labels); pi++ {
		if labels[pi].Kind == "" && labels[pi].Name == "" && labels[pi-1].Kind == "voice" && labels[pi-1].Name == "" && partStaves[pi] == 1 {
			labels[pi].Kind, labels[pi].Generic = "voice", true
		}
	}

	unlabelledVoice, unlabelledOther := 0, 0
	for _, k := range staffKeys {
		list := byStaff[k]
		if len(list) == 0 {
			continue
		}
		lab := labels[k.part]
		kind := lab.Kind
		if kind == "" {
			kind = "other"
		}
		clef := mostCommon(staffClefs[k])
		withClef := func(s string) string {
			if clef != "" {
				return s + " (" + clef + ")"
			}
			return s
		}
		var base string
		switch {
		case kind == "keyboard":
			label := lab.Name
			if label == "" {
				label = "Keyboard"
			}
			if regexp.MustCompile(`(?i)^(piano|organ|keyboard|accompaniment|harpsichord)$`).MatchString(label) || lab.Name == "" {
				switch {
				case partStaves[k.part] < 2:
				case k.staff == 1:
					label += " right hand"
				case k.staff == 2 && partStaves[k.part] >= 3 && regexp.MustCompile(`(?i)organ`).MatchString(label):
					label += " left hand"
				case k.staff == 2:
					label += " left hand"
				case k.staff == 3 && regexp.MustCompile(`(?i)organ`).MatchString(label):
					label += " pedals"
				default:
					label += fmt.Sprintf(" staff %d", k.staff)
				}
			} else if partStaves[k.part] > 1 {
				label += fmt.Sprintf(", staff %d", k.staff)
			}
			base = label
		case lab.Name != "":
			base = lab.Name
			if partStaves[k.part] > 1 {
				base += fmt.Sprintf(", staff %d", k.staff)
			}
		case kind == "voice":
			unlabelledVoice++
			base = withClef(fmt.Sprintf("Voice staff %d", unlabelledVoice))
		default:
			unlabelledOther++
			base = withClef(fmt.Sprintf("Unlabelled staff %d", unlabelledOther))
		}
		id := fmt.Sprintf("p%ds%d", k.part+1, k.staff)
		info := lineInfo{kind: kind, clef: clef, transpose: lab.Transpose, transposeNote: lab.TransposeNote, staff: id}
		if !hasSimultaneous(list) {
			info.part, info.primary = "single", true
			m.Lines = append(m.Lines, makeLine(id, base, "", info, list))
			continue
		}
		up, low := splitUpperLower(list, func(mi int) map[string]bool { return restVoices[restKey{k.part, k.staff, mi}] })
		hu, hl := lab.VoiceHints[0], lab.VoiceHints[1]
		if kind == "voice" && hu == "" {
			if strings.HasPrefix(clef, "treble") {
				hu, hl = "usually soprano", "usually alto"
			} else if strings.HasPrefix(clef, "bass") {
				hu, hl = "usually tenor", "usually bass"
			}
		}
		voice := kind == "voice"
		u, l, a := info, info, info
		u.part, u.primary = "upper", voice
		l.part, l.primary = "lower", voice
		a.part, a.primary = "all", !voice
		m.Lines = append(m.Lines, makeLine(id+"u", base+", upper notes", hu, u, up))
		m.Lines = append(m.Lines, makeLine(id+"l", base+", lower notes", hl, l, low))
		m.Lines = append(m.Lines, makeLine(id+"a", base+", all notes", "", a, dedupe(list)))
	}

	// make repeated names unique
	seenName := map[string]int{}
	for _, l := range m.Lines {
		if l.Part == "single" || l.Part == "upper" {
			seenName[strings.TrimSuffix(l.Name, ", upper notes")]++
		}
	}
	dupCount := map[string]int{}
	for i := range m.Lines {
		base := strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(m.Lines[i].Name, ", upper notes"), ", lower notes"), ", all notes")
		if seenName[base] > 1 {
			if m.Lines[i].Part == "single" || m.Lines[i].Part == "upper" {
				dupCount[base]++
			}
			m.Lines[i].Name = strings.Replace(m.Lines[i].Name, base, fmt.Sprintf("%s (%d)", base, dupCount[base]), 1)
		}
	}

	// lines switched on when a score is first opened
	hasKind := map[string]bool{}
	for _, l := range m.Lines {
		hasKind[l.Kind] = true
	}
	for i := range m.Lines {
		l := &m.Lines[i]
		switch {
		case hasKind["voice"]:
			l.Default = l.Kind == "voice" && l.Primary
		case hasKind["instrument"] || hasKind["other"]:
			l.Default = (l.Kind == "instrument" || l.Kind == "other") && l.Primary
		default:
			l.Default = l.Primary
		}
	}

	m.Verses = buildLyrics(notes, m)
	if m.Verses == nil {
		m.Verses = []Verse{}
	}
	if m.Lines == nil {
		m.Lines = []Line{}
	}
	if len(m.Lines) == 0 {
		m.Warnings = append(m.Warnings, "No notes were found in this score.")
	}
	return m
}

func clefDescription(sign string, line, octave int) string {
	s := ""
	switch strings.ToUpper(sign) {
	case "G":
		s = "treble clef"
	case "F":
		s = "bass clef"
	case "C":
		switch line {
		case 3:
			s = "alto clef"
		case 4:
			s = "tenor clef"
		default:
			s = "C clef"
		}
	case "PERCUSSION":
		s = "percussion clef"
	default:
		return ""
	}
	if octave == -1 {
		s += ", octave lower"
	} else if octave == 1 {
		s += ", octave higher"
	}
	return s
}

func metronomeToQuarter(unit string, dots int, perMinute string) float64 {
	pm, err := strconv.ParseFloat(strings.TrimSpace(perMinute), 64)
	if err != nil || pm <= 0 {
		return 0
	}
	q := map[string]float64{"whole": 4, "half": 2, "quarter": 1, "eighth": 0.5, "16th": 0.25}[unit]
	if q == 0 {
		return 0
	}
	if dots > 0 {
		q *= 1.5
	}
	return pm * q
}

func mostCommon(m map[string]int) string {
	best, bn := "", -1
	for k, v := range m {
		if v > bn || (v == bn && k < best) {
			best, bn = k, v
		}
	}
	return best
}

func hasSimultaneous(list []*rawNote) bool {
	for i, a := range list {
		for j := i + 1; j < len(list); j++ {
			b := list[j]
			if b.t >= a.t+a.d-0.01 {
				break
			}
			if b.midi != a.midi {
				return true
			}
		}
	}
	return false
}

func dedupe(list []*rawNote) []*rawNote {
	seen := map[string]bool{}
	var out []*rawNote
	for _, n := range list {
		k := fmt.Sprintf("%.3f:%d", n.t, n.midi)
		if !seen[k] {
			seen[k] = true
			out = append(out, n)
		}
	}
	return out
}

// splitUpperLower picks, measure by measure, the highest and lowest voice
// (by average pitch), and from chords the top or bottom note. When a measure
// has a single voice, it remembers which voice was upper or lower before,
// and does not copy notes into a line that is still holding a note.
func splitUpperLower(list []*rawNote, restsIn func(mi int) map[string]bool) (up, low []*rawNote) {
	byMeasure := map[int][]*rawNote{}
	var mis []int
	for _, n := range list {
		if _, ok := byMeasure[n.mi]; !ok {
			mis = append(mis, n.mi)
		}
		byMeasure[n.mi] = append(byMeasure[n.mi], n)
	}
	sort.Ints(mis)
	prevUp, prevLow := "", ""
	upEnd, lowEnd := -1.0, -1.0
	lastEnd := func(ns []*rawNote) float64 {
		e := -1.0
		for _, n := range ns {
			if n.t+n.d > e {
				e = n.t + n.d
			}
		}
		return e
	}
	for _, mi := range mis {
		ns := byMeasure[mi]
		voices := map[string][]*rawNote{}
		var vnames []string
		for _, n := range ns {
			if _, ok := voices[n.voice]; !ok {
				vnames = append(vnames, n.voice)
			}
			voices[n.voice] = append(voices[n.voice], n)
		}
		avg := func(v string) float64 {
			s := 0.0
			for _, n := range voices[v] {
				s += float64(n.midi)
			}
			return s / float64(len(voices[v]))
		}
		sort.SliceStable(vnames, func(a, b int) bool { return avg(vnames[a]) > avg(vnames[b]) })
		first := ns[0].t
		for _, n := range ns {
			if n.t < first {
				first = n.t
			}
		}
		var u, l []*rawNote
		if len(vnames) >= 2 {
			u = pickByOnset(voices[vnames[0]], true)
			l = pickByOnset(voices[vnames[len(vnames)-1]], false)
			prevUp, prevLow = vnames[0], vnames[len(vnames)-1]
		} else {
			v := vnames[0]
			chords := hasSimultaneous(voices[v])
			upBusy := upEnd > first+0.01
			lowBusy := lowEnd > first+0.01
			toUp, toLow := true, true
			// another voice that only has rests in this measure means that
			// part is resting, so the notes belong to one line only
			restOnly := ""
			for rv := range restsIn(mi) {
				if rv != v && (restOnly == "" || rv < restOnly) {
					restOnly = rv
				}
			}
			if !chords && restOnly != "" {
				vi, _ := strconv.Atoi(v)
				ri, _ := strconv.Atoi(restOnly)
				if ri > vi {
					toLow = false
				} else {
					toUp = false
				}
			} else if !chords {
				if v == prevLow && v != prevUp {
					toUp = false
				}
				if upBusy {
					toUp = false
				}
				if lowBusy && toUp {
					toLow = false
				}
			}
			if toUp {
				u = pickByOnset(voices[v], true)
			}
			if toLow {
				l = pickByOnset(voices[v], false)
			}
		}
		up = append(up, u...)
		low = append(low, l...)
		if e := lastEnd(u); e > upEnd {
			upEnd = e
		}
		if e := lastEnd(l); e > lowEnd {
			lowEnd = e
		}
	}
	return
}

func pickByOnset(ns []*rawNote, highest bool) []*rawNote {
	groups := map[float64]*rawNote{}
	var keys []float64
	for _, n := range ns {
		k := math.Round(n.t*1000) / 1000
		cur, ok := groups[k]
		if !ok {
			keys = append(keys, k)
			groups[k] = n
			continue
		}
		if (highest && n.midi > cur.midi) || (!highest && n.midi < cur.midi) {
			groups[k] = n
		}
	}
	sort.Float64s(keys)
	out := make([]*rawNote, 0, len(keys))
	for _, k := range keys {
		out = append(out, groups[k])
	}
	return out
}

func durationName(n *rawNote) string {
	if !n.merged && n.typ != "" {
		s := n.typ
		if s == "16th" {
			s = "sixteenth"
		} else if s == "32nd" {
			s = "thirty-second"
		}
		switch n.dots {
		case 1:
			s = "dotted " + s
		case 2:
			s = "double-dotted " + s
		}
		if n.tuplet {
			s = "triplet " + s
		}
		return s
	}
	return qnName(n.d)
}

func qnName(q float64) string {
	names := []struct {
		q float64
		s string
	}{{8, "two whole notes"}, {6, "dotted whole"}, {4, "whole"}, {3, "dotted half"}, {2, "half"}, {1.5, "dotted quarter"},
		{1, "quarter"}, {0.75, "dotted eighth"}, {0.5, "eighth"}, {0.25, "sixteenth"}, {2.0 / 3, "triplet quarter"}, {1.0 / 3, "triplet eighth"}}
	for _, x := range names {
		if math.Abs(q-x.q) < 0.02 {
			return x.s
		}
	}
	return strconv.FormatFloat(math.Round(q*100)/100, 'f', -1, 64) + " beats"
}

type lineInfo struct {
	kind, clef, part, staff string
	primary                 bool
	transpose               int
	transposeNote           string
}

func makeLine(id, name, hint string, info lineInfo, list []*rawNote) Line {
	l := Line{ID: id, Name: name, Hint: hint, Kind: info.kind, Clef: info.clef, StaffKey: info.staff, Part: info.part,
		Primary: info.primary, Transpose: info.transpose, TransposeNote: info.transposeNote}
	lo, hi := -1, -1
	for i, n := range list {
		ly := ""
		if len(n.lyrics) > 0 {
			ly = n.lyrics[0].text
		}
		l.Events = append(l.Events, Event{T: round3(n.t), D: round3(n.d), Midi: n.midi, Mi: n.mi, Name: n.name, Dur: durationName(n), Lyric: ly})
		if lo < 0 || n.midi < list[lo].midi {
			lo = i
		}
		if hi < 0 || n.midi > list[hi].midi {
			hi = i
		}
	}
	if lo >= 0 {
		l.Low, l.High = list[lo].name, list[hi].name
	}
	l.Count = len(list)
	return l
}

func round3(f float64) float64 { return math.Round(f*1000) / 1000 }

var dynamicsRe = regexp.MustCompile(`(?i)^(p|pp|ppp|mp|mf|f|ff|fff|sf|sfz|fp|rit\.?|cresc\.?|dim\.?|slower|faster|a tempo)$|[0-9]|^[^aAiIoO]$|^[^\pL]+$`)

func buildLyrics(notes []*rawNote, m *Model) []Verse {
	type syl struct {
		t              float64
		mi             int
		ord            int
		syllabic, text string
	}
	byVerse := map[string][]syl{}
	for _, n := range notes {
		for _, l := range n.lyrics {
			core := strings.TrimFunc(l.text, func(r rune) bool { return !unicode.IsLetter(r) })
			if core == "" || dynamicsRe.MatchString(core) {
				continue
			}
			byVerse[l.verse] = append(byVerse[l.verse], syl{t: n.t, mi: n.mi, ord: n.part*10 + n.staff, syllabic: l.syllabic, text: l.text})
		}
	}
	var names []string
	for k := range byVerse {
		names = append(names, k)
	}
	sort.Slice(names, func(a, b int) bool {
		x, e1 := strconv.Atoi(names[a])
		y, e2 := strconv.Atoi(names[b])
		if e1 == nil && e2 == nil {
			return x < y
		}
		return names[a] < names[b]
	})
	var verses []Verse
	for _, v := range names {
		list := byVerse[v]
		sort.SliceStable(list, func(a, b int) bool {
			if math.Abs(list[a].t-list[b].t) > 0.001 {
				return list[a].t < list[b].t
			}
			return list[a].ord < list[b].ord
		})
		// keep one syllable per moment (the highest staff that has one)
		var uniq []syl
		for _, s := range list {
			if len(uniq) > 0 && math.Abs(uniq[len(uniq)-1].t-s.t) < 0.001 {
				continue
			}
			uniq = append(uniq, s)
		}
		var words []Word
		var cur *Word
		for _, s := range uniq {
			txt := strings.Trim(s.text, "-_ ")
			if txt == "" {
				continue
			}
			switch s.syllabic {
			case "begin":
				if cur != nil {
					words = append(words, *cur)
				}
				cur = &Word{T: round3(s.t), Mi: s.mi, Text: txt}
			case "middle":
				if cur == nil {
					cur = &Word{T: round3(s.t), Mi: s.mi}
				}
				cur.Text += txt
			case "end":
				if cur == nil {
					cur = &Word{T: round3(s.t), Mi: s.mi}
				}
				cur.Text += txt
				words = append(words, *cur)
				cur = nil
			default:
				if cur != nil {
					words = append(words, *cur)
					cur = nil
				}
				words = append(words, Word{T: round3(s.t), Mi: s.mi, Text: txt})
			}
		}
		if cur != nil {
			words = append(words, *cur)
		}
		if len(words) == 0 {
			continue
		}
		name := v
		if len(names) > 1 {
			name = "Verse " + v
		} else {
			name = "Lyrics"
		}
		verse := Verse{Name: name, Words: words}
		// group into printed systems
		start := 0
		flush := func(end int) {
			var parts []string
			for _, w := range words {
				if w.Mi >= start && w.Mi <= end {
					parts = append(parts, w.Text)
				}
			}
			if len(parts) > 0 {
				verse.Lines = append(verse.Lines, LyricLine{From: start, To: end, Text: strings.Join(parts, " ")})
			}
		}
		for i := 1; i < len(m.Measures); i++ {
			if m.Measures[i].NewSystem {
				flush(i - 1)
				start = i
			}
		}
		flush(len(m.Measures) - 1)
		verses = append(verses, verse)
	}
	return verses
}

// performanceOrder expands repeat signs and first/second endings.
func performanceOrder(f []measureFacts) ([]int, bool) {
	n := len(f)
	plain := make([]int, n)
	for i := range plain {
		plain[i] = i
	}
	hasBack := false
	for _, x := range f {
		if x.repeatBack > 0 {
			hasBack = true
		}
	}
	if !hasBack {
		return plain, false
	}
	// spread ending numbers over the measures they cover
	ending := make([]map[int]bool, n)
	var open map[int]bool
	for i, x := range f {
		if x.endings != "" {
			open = map[int]bool{}
			for _, p := range strings.FieldsFunc(x.endings, func(r rune) bool { return r == ',' || r == ' ' }) {
				if v, err := strconv.Atoi(strings.TrimSpace(p)); err == nil {
					open[v] = true
				}
			}
		}
		if open != nil {
			ending[i] = open
			if x.endingType == "stop" || x.endingType == "discontinue" {
				open = nil
			}
		}
	}
	var order []int
	counts := map[int]int{}
	start, pass := 0, 1
	jumped := false
	for i := 0; i < n && len(order) < 20000; {
		if f[i].repeatFwd && !jumped {
			start, pass = i, 1
		}
		jumped = false
		if ending[i] != nil && !ending[i][pass] {
			i++
			continue
		}
		order = append(order, i)
		if f[i].repeatBack > 0 && counts[i] < f[i].repeatBack-1 {
			counts[i]++
			pass++
			i = start
			jumped = true
			continue
		}
		if f[i].repeatBack > 0 {
			counts[i] = 0
			start, pass = i+1, 1
		}
		i++
	}
	return order, true
}

// mxlFiles returns the MusicXML files Audiveris wrote, in movement order.
func mxlFiles(dir string) []string {
	entries, _ := os.ReadDir(dir)
	var out []string
	for _, e := range entries {
		n := strings.ToLower(e.Name())
		if !e.IsDir() && (strings.HasSuffix(n, ".mxl") || strings.HasSuffix(n, ".musicxml")) {
			out = append(out, filepath.Join(dir, e.Name()))
		}
	}
	mvt := regexp.MustCompile(`\.mvt(\d+)\.`)
	sort.Slice(out, func(a, b int) bool {
		ma, mb := mvt.FindStringSubmatch(out[a]), mvt.FindStringSubmatch(out[b])
		if ma != nil && mb != nil {
			x, _ := strconv.Atoi(ma[1])
			y, _ := strconv.Atoi(mb[1])
			return x < y
		}
		return out[a] < out[b]
	})
	return out
}
