package main

// Audiveris treats a staff labelled "Flute" on the first line of music and
// "Fl." on later lines as two different parts, each resting where the other
// plays. mergeSplitParts joins such parts back together.

import (
	"strings"
	"unicode"
)

type partState struct {
	div   float64
	clefs map[int]struct {
		sign      string
		line, oct int
	}
	beats, bt string
	fifths    int
	hasKey    bool
	transpose *struct{ chromatic, octave int }
}

func normLabel(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(expandLabel(s)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func labelsCompatible(a, b string) bool {
	if placeholderLabels[strings.ToLower(strings.TrimSpace(a))] || placeholderLabels[strings.ToLower(strings.TrimSpace(b))] {
		return false
	}
	x, y := normLabel(a), normLabel(b)
	if len(x) < 2 || len(y) < 2 {
		return false
	}
	if len(y) > len(x) {
		x, y = y, x
	}
	// "flu" and "flute", "violin" and "violin i", "cello" and "violoncello"
	return strings.HasPrefix(x, y) || (len(y) >= 4 && strings.Contains(x, y))
}

func measureHasNotes(m xMeasure) bool {
	for _, it := range m.Items {
		if n, ok := it.(*xNote); ok && n.Pitch != nil && n.Grace == nil {
			return true
		}
	}
	return false
}

func partStaffCount(p []xMeasure) int {
	n := 1
	for _, m := range p {
		for _, it := range m.Items {
			if a, ok := it.(*xAttributes); ok && a.Staves > n {
				n = a.Staves
			}
		}
	}
	return n
}

// mergeSplitParts returns the number of parts that were joined.
func mergeSplitParts(s *xScore) int {
	nParts := len(s.Parts)
	if nParts < 2 {
		return 0
	}
	nMeas := len(s.Parts[0].Measures)
	// system number of each measure
	system := make([]int, nMeas)
	sys := 0
	for mi := 0; mi < nMeas; mi++ {
		newSys := false
		for _, p := range s.Parts {
			if mi < len(p.Measures) {
				for _, it := range p.Measures[mi].Items {
					if pr, ok := it.(*xPrint); ok && (pr.NewSystem == "yes" || pr.NewPage == "yes") {
						newSys = true
					}
				}
			}
		}
		if newSys && mi > 0 {
			sys++
		}
		system[mi] = sys
	}
	nSys := sys + 1
	names := make([]string, nParts)
	for i, p := range s.Parts {
		for _, sp := range s.PartList.ScoreParts {
			if sp.ID == p.ID {
				names[i] = sp.Name
			}
		}
	}
	content := make([][]bool, nParts) // [part][system]
	first := make([]int, nParts)
	staves := make([]int, nParts)
	for pi, p := range s.Parts {
		content[pi] = make([]bool, nSys)
		first[pi] = -1
		for mi, m := range p.Measures {
			if mi < nMeas && measureHasNotes(m) {
				content[pi][system[mi]] = true
			}
		}
		for si := 0; si < nSys; si++ {
			if content[pi][si] {
				first[pi] = si
				break
			}
		}
		staves[pi] = partStaffCount(p.Measures)
	}

	type chain struct {
		parts []int
		sys   []bool
	}
	var chains []*chain
	for si := 0; si < nSys; si++ {
		lastUsed := -1
		for pi := 0; pi < nParts; pi++ {
			if first[pi] != si {
				continue
			}
			matched := -1
			if si > 0 {
				for ci := lastUsed + 1; ci < len(chains); ci++ {
					c := chains[ci]
					head := c.parts[0]
					if staves[head] != staves[pi] || !labelsCompatible(names[head], names[pi]) {
						continue
					}
					overlap := false
					for k := 0; k < nSys; k++ {
						if c.sys[k] && content[pi][k] {
							overlap = true
						}
					}
					if !overlap {
						matched = ci
						break
					}
				}
			}
			if matched >= 0 {
				c := chains[matched]
				c.parts = append(c.parts, pi)
				for k := 0; k < nSys; k++ {
					c.sys[k] = c.sys[k] || content[pi][k]
				}
				lastUsed = matched
			} else {
				c := &chain{parts: []int{pi}, sys: append([]bool(nil), content[pi]...)}
				chains = append(chains, c)
			}
		}
	}
	// parts with no notes at all stay as they are
	merged := 0
	for _, c := range chains {
		merged += len(c.parts) - 1
	}
	if merged == 0 {
		return 0
	}

	// attribute state of every part at the start of every measure
	states := make([][]partState, nParts)
	for pi, p := range s.Parts {
		st := partState{div: 1, clefs: map[int]struct {
			sign      string
			line, oct int
		}{}}
		states[pi] = make([]partState, len(p.Measures))
		for mi, m := range p.Measures {
			cp := st
			cp.clefs = map[int]struct {
				sign      string
				line, oct int
			}{}
			for k, v := range st.clefs {
				cp.clefs[k] = v
			}
			states[pi][mi] = cp
			for _, it := range m.Items {
				a, ok := it.(*xAttributes)
				if !ok {
					continue
				}
				if a.Divisions > 0 {
					st.div = a.Divisions
				}
				for _, c := range a.Clefs {
					n := c.Number
					if n == 0 {
						n = 1
					}
					st.clefs[n] = struct {
						sign      string
						line, oct int
					}{c.Sign, c.Line, c.OctaveChange}
				}
				if len(a.Times) > 0 {
					st.beats, st.bt = a.Times[0].Beats, a.Times[0].BeatType
				}
				if len(a.Keys) > 0 {
					st.fifths, st.hasKey = a.Keys[0].Fifths, true
				}
				if len(a.Transposes) > 0 {
					st.transpose = &struct{ chromatic, octave int }{a.Transposes[0].Chromatic, a.Transposes[0].OctaveChange}
				}
			}
		}
	}
	stateAttrs := func(st partState) *xAttributes {
		a := &xAttributes{Divisions: st.div}
		for n, c := range st.clefs {
			a.Clefs = append(a.Clefs, struct {
				Number       int    `xml:"number,attr"`
				Sign         string `xml:"sign"`
				Line         int    `xml:"line"`
				OctaveChange int    `xml:"clef-octave-change"`
			}{n, c.sign, c.line, c.oct})
		}
		if st.transpose != nil {
			a.Transposes = append(a.Transposes, struct {
				Chromatic    int `xml:"chromatic"`
				OctaveChange int `xml:"octave-change"`
			}{st.transpose.chromatic, st.transpose.octave})
		}
		return a
	}

	type newPart struct {
		ID       string
		Measures []xMeasure
	}
	var parts []newPart
	var scoreParts = s.PartList.ScoreParts[:0:0]
	for _, c := range chains {
		head := c.parts[0]
		// the fullest label names the part
		best := names[head]
		for _, pi := range c.parts {
			if len(normLabel(names[pi])) > len(normLabel(best)) {
				best = names[pi]
			}
		}
		owner := make([]int, nSys)
		for k := range owner {
			owner[k] = head
			for _, pi := range c.parts {
				if content[pi][k] {
					owner[k] = pi
				}
			}
		}
		var ms []xMeasure
		prevOwner := head
		for mi := 0; mi < nMeas; mi++ {
			o := owner[system[mi]]
			if mi >= len(s.Parts[o].Measures) {
				o = head
			}
			if mi >= len(s.Parts[o].Measures) {
				break
			}
			m := s.Parts[o].Measures[mi]
			if o != prevOwner {
				items := append([]interface{}{stateAttrs(states[o][mi])}, m.Items...)
				m = xMeasure{Number: m.Number, Implicit: m.Implicit, Items: items}
				prevOwner = o
			}
			ms = append(ms, m)
		}
		parts = append(parts, newPart{ID: s.Parts[head].ID, Measures: ms})
		scoreParts = append(scoreParts, struct {
			ID   string `xml:"id,attr"`
			Name string `xml:"part-name"`
		}{s.Parts[head].ID, best})
	}
	// keep the original top-to-bottom order of the first part of each chain
	s.Parts = s.Parts[:0]
	for _, p := range parts {
		s.Parts = append(s.Parts, struct {
			ID       string     `xml:"id,attr"`
			Measures []xMeasure `xml:"measure"`
		}{p.ID, p.Measures})
	}
	s.PartList.ScoreParts = scoreParts
	return merged
}
