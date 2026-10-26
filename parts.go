package main

// Working out what each part of a score is (a voice, an instrument, a
// keyboard), what to call it, and how a transposing instrument sounds,
// from the label printed at the start of each staff.

import (
	"regexp"
	"strings"
	"unicode"
)

type partLabel struct {
	Name          string    // display name, "" when the staff had no usable label
	Kind          string    // voice | instrument | keyboard | other | "" (unknown)
	Transpose     int       // semitones from written to sounding pitch
	TransposeNote string    // explanation of the transposition
	VoiceHints    [2]string // names for the upper and lower notes, when the label gives them
	Generic       bool      // label like "Choir", "Women", "Voices" that does not name the parts
}

// Audiveris names staves it could not label "Voice" (one staff) or "Piano"
// (two staves); those are not real labels.
var placeholderLabels = map[string]bool{"voice": true, "voice oohs": true, "": true}

var abbreviations = map[string]string{
	"s": "Soprano", "sop": "Soprano", "sopr": "Soprano", "a": "Alto", "alt": "Alto", "t": "Tenor", "ten": "Tenor",
	"b": "Bass", "bs": "Bass", "bar": "Baritone", "bari": "Baritone", "mez": "Mezzo-soprano", "ms": "Mezzo-soprano",
	"vln": "Violin", "vl": "Violin", "vn": "Violin", "vni": "Violins", "vla": "Viola", "vle": "Violas", "vc": "Cello", "vlc": "Cello",
	"cb": "Double bass", "db": "Double bass", "kb": "Double bass", "fl": "Flute", "picc": "Piccolo", "ob": "Oboe",
	"eh": "English horn", "ca": "Cor anglais", "cl": "Clarinet", "clar": "Clarinet", "bcl": "Bass clarinet", "bsn": "Bassoon", "fg": "Bassoon",
	"hn": "Horn", "hrn": "Horn", "cor": "Horn", "tpt": "Trumpet", "trp": "Trumpet", "tr": "Trumpet", "crt": "Cornet",
	"tbn": "Trombone", "trb": "Trombone", "pos": "Trombone", "euph": "Euphonium", "tba": "Tuba", "timp": "Timpani",
	"perc": "Percussion", "glk": "Glockenspiel", "glock": "Glockenspiel", "xyl": "Xylophone", "vib": "Vibraphone", "mar": "Marimba",
	"hp": "Harp", "gtr": "Guitar", "gt": "Guitar", "pno": "Piano", "pf": "Piano", "org": "Organ", "kbd": "Keyboard",
	"asx": "Alto saxophone", "tsx": "Tenor saxophone", "bsx": "Baritone saxophone", "rec": "Recorder", "acc": "Accompaniment",
	"accomp": "Accompaniment", "hb": "Handbells",
}

var (
	keyboardWords   = regexp.MustCompile(`(?i)\b(piano|pianoforte|organ|keyboards?|harpsichord|celest[ae]|synth\w*|accompaniment|rehearsal|manuals?)\b`)
	instrumentWords = regexp.MustCompile(`(?i)\b(violins?|violas?|cellos?|violoncellos?|double bass|contrabass\w*|string bass|bass guitar|electric bass|basses?\s+\(?strings|strings|flutes?|alto flute|piccolos?|oboes?|english horn|cor anglais|clarinets?|bass clarinet|bassoons?|contrabassoon|horns?|french horn|trumpets?|cornets?|flugelhorns?|trombones?|bass trombone|euphoniums?|baritone horn|tubas?|saxophones?|sax|recorders?|harps?|guitars?|mandolins?|banjos?|ukuleles?|accordions?|percussion|drums?|timpani|glockenspiel|xylophone|vibraphone|marimba|bells|handbells|chimes|cymbals?|triangle|tambourine|melodica|harmonica|whistle)\b`)
	voiceWords      = regexp.MustCompile(`(?i)\b(soprano|sopranos|mezzo-soprano|mezzo|alto|altos|contralto|tenor|tenors|baritone|baritones|bass|basses|choir|chorus|voice|voices|vocal|women|men|ladies|gentlemen|treble|trebles|cantor|descant|solo|soloist|unison|satb|ssa|ssaa|sa|sab|tb|ttb|ttbb|sat)\b`)
	genericVoice    = regexp.MustCompile(`(?i)\b(choir|chorus|voice|voices|vocal|women|men|ladies|gentlemen|unison|satb|ssa|ssaa|sa|sab|tb|ttb|ttbb|sat)\b`)
	voiceType       = regexp.MustCompile(`(?i)\b(soprano|mezzo-soprano|mezzo|alto|contralto|tenor|baritone|bass)e?s?\b`)
	inKey           = regexp.MustCompile(`(?i)\b(?:in|en)\s+([A-G])\s*(b|flat|#|sharp|s)?\b`)
	letterOnly      = regexp.MustCompile(`^[satbSATB]+$`)
)

// default transpositions (written to sounding) for instruments that are
// normally written at a different pitch than they sound
var defaultTranspose = []struct {
	re   *regexp.Regexp
	semi int
	note string
}{
	{regexp.MustCompile(`(?i)bass clarinet`), -14, "B-flat bass clarinet"},
	{regexp.MustCompile(`(?i)clarinet`), -2, "B-flat clarinet"},
	{regexp.MustCompile(`(?i)(soprano sax|soprano saxophone)`), -2, "B-flat soprano saxophone"},
	{regexp.MustCompile(`(?i)(alto sax|alto saxophone)`), -9, "E-flat alto saxophone"},
	{regexp.MustCompile(`(?i)(tenor sax|tenor saxophone)`), -14, "B-flat tenor saxophone"},
	{regexp.MustCompile(`(?i)(baritone sax|baritone saxophone|bari sax)`), -21, "E-flat baritone saxophone"},
	{regexp.MustCompile(`(?i)(trumpet|cornet|flugelhorn)`), -2, "B-flat trumpet"},
	{regexp.MustCompile(`(?i)(english horn|cor anglais)`), -7, "English horn in F"},
	{regexp.MustCompile(`(?i)horn`), -7, "horn in F"},
	{regexp.MustCompile(`(?i)alto flute`), -5, "alto flute in G"},
	{regexp.MustCompile(`(?i)piccolo`), 12, "piccolo"},
	{regexp.MustCompile(`(?i)contrabassoon`), -12, "contrabassoon"},
	{regexp.MustCompile(`(?i)(double bass|contrabass|string bass|bass guitar|electric bass)`), -12, "bass"},
	{regexp.MustCompile(`(?i)guitar`), -12, "guitar"},
	{regexp.MustCompile(`(?i)glockenspiel`), 24, "glockenspiel"},
	{regexp.MustCompile(`(?i)(xylophone|celest)`), 12, "instrument"},
}

var keyPC = map[string]int{"C": 0, "D": 2, "E": 4, "F": 5, "G": 7, "A": 9, "B": 11}

func intervalWords(semi int) string {
	names := map[int]string{1: "a half step", 2: "a whole step", 3: "a minor third", 4: "a major third", 5: "a perfect fourth",
		7: "a perfect fifth", 8: "a minor sixth", 9: "a major sixth", 12: "an octave", 14: "an octave and a whole step",
		15: "an octave and a minor third", 21: "an octave and a major sixth", 24: "two octaves"}
	a := semi
	if a < 0 {
		a = -a
	}
	n := names[a]
	if n == "" {
		return ""
	}
	if semi < 0 {
		return n + " lower"
	}
	return n + " higher"
}

func titleIfShouting(s string) string {
	letters, upper := 0, 0
	for _, r := range s {
		if unicode.IsLetter(r) {
			letters++
			if unicode.IsUpper(r) {
				upper++
			}
		}
	}
	if letters < 4 || upper != letters {
		return s
	}
	words := strings.Fields(strings.ToLower(s))
	for i, w := range words {
		if isRoman(w) {
			words[i] = strings.ToUpper(w)
			continue
		}
		r := []rune(w)
		r[0] = unicode.ToUpper(r[0])
		words[i] = string(r)
	}
	return strings.Join(words, " ")
}

func isRoman(w string) bool {
	switch w {
	case "i", "ii", "iii", "iv", "v", "vi":
		return true
	}
	return false
}

// expandLabel turns abbreviations such as "Vln. I" or "S. A." into words.
func expandLabel(raw string) string {
	s := strings.NewReplacer("♭", "b", "♯", "#", " ", " ").Replace(strings.TrimSpace(raw))
	compact := strings.NewReplacer(".", "", " ", "", ",", "", "&", "", "/", "").Replace(s)
	fields := regexp.MustCompile(`[\s.,/&]+`).Split(strings.Trim(s, " .,/&"), -1)
	allSingle := len(fields) > 1
	for _, f := range fields {
		if len([]rune(f)) != 1 {
			allSingle = false
		}
	}
	shortCaps := len(fields) == 1 && compact == strings.ToUpper(compact) && !strings.EqualFold(compact, "bass")
	if letterOnly.MatchString(compact) && len(compact) <= 4 && (allSingle || shortCaps) {
		// "S A T B", "S.A.", "TB": a list of voice parts
		var parts []string
		for _, c := range strings.ToLower(compact) {
			parts = append(parts, abbreviations[string(c)])
		}
		return strings.Join(parts, " and ")
	}
	tokens := regexp.MustCompile(`[\s.,/&]+`).Split(s, -1)
	var out []string
	for i, t := range tokens {
		if t == "" {
			continue
		}
		low := strings.ToLower(t)
		prevIn := i > 0 && strings.EqualFold(tokens[i-1], "in")
		if e, ok := abbreviations[low]; ok && len(low) > 1 && !prevIn {
			out = append(out, e)
			continue
		}
		out = append(out, t)
	}
	return strings.Join(out, " ")
}

func classifyLabel(raw string) partLabel {
	trimmed := strings.TrimSpace(raw)
	if placeholderLabels[strings.ToLower(trimmed)] {
		return partLabel{}
	}
	name := titleIfShouting(expandLabel(trimmed))
	letters := 0
	for _, r := range name {
		if unicode.IsLetter(r) {
			letters++
		}
	}
	if letters < 3 {
		return partLabel{} // OCR noise
	}
	p := partLabel{Name: name}
	switch {
	case instrumentWords.MatchString(name) && !keyboardWords.MatchString(name):
		p.Kind = "instrument"
	case keyboardWords.MatchString(name):
		p.Kind = "keyboard"
		if regexp.MustCompile(`(?i)celest`).MatchString(name) {
			p.Transpose, p.TransposeNote = 12, "Celesta: sounds an octave higher than written."
		}
	case voiceWords.MatchString(name):
		p.Kind = "voice"
		types := voiceType.FindAllStringSubmatch(name, -1)
		var distinct []string
		for _, t := range types {
			v := strings.ToLower(t[1])
			if v == "mezzo-soprano" {
				v = "mezzo"
			}
			if len(distinct) == 0 || distinct[len(distinct)-1] != v {
				distinct = append(distinct, v)
			}
		}
		if len(distinct) >= 2 {
			p.VoiceHints = [2]string{distinct[0], distinct[len(distinct)-1]}
		} else if len(distinct) == 1 {
			p.VoiceHints = [2]string{"probably first " + plural(distinct[0]), "probably second " + plural(distinct[0])}
		}
		if genericVoice.MatchString(name) && len(distinct) < 2 {
			p.Generic = true
			p.VoiceHints = [2]string{}
		}
	default:
		p.Kind = "other"
	}
	if p.Kind == "instrument" {
		def, defNote := 0, ""
		for _, d := range defaultTranspose {
			if d.re.MatchString(name) {
				def, defNote = d.semi, d.note
				break
			}
		}
		if m := inKey.FindStringSubmatch(name); m != nil {
			pc := keyPC[strings.ToUpper(m[1])]
			switch strings.ToLower(m[2]) {
			case "b", "flat", "s":
				pc--
			case "#", "sharp":
				pc++
			}
			pc = (pc + 12) % 12
			best := pc
			for _, c := range []int{pc, pc - 12, pc - 24} {
				if abs(c-def) < abs(best-def) {
					best = c
				}
			}
			if best != def {
				key := strings.ToUpper(m[1])
				switch strings.ToLower(m[2]) {
				case "b", "flat", "s":
					key += "-flat"
				case "#", "sharp":
					key += "-sharp"
				}
				defNote = "instrument in " + key
			}
			def = best
		}
		if def != 0 {
			p.Transpose = def
			p.TransposeNote = strings.ToUpper(defNote[:1]) + defNote[1:] + ": sounds " + intervalWords(def) + " than written."
		}
	}
	return p
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func plural(v string) string {
	switch v {
	case "bass":
		return "basses"
	case "mezzo":
		return "mezzos"
	}
	return v + "s"
}
