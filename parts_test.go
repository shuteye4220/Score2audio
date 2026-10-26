package main

import "testing"

func TestClassifyLabel(t *testing.T) {
	cases := []struct {
		in, name, kind string
		transpose      int
		hints          [2]string
	}{
		{"Voice", "", "", 0, [2]string{}},
		{"l", "", "", 0, [2]string{}},
		{"CELLO", "Cello", "instrument", 0, [2]string{}},
		{"Vln. I", "Violin I", "instrument", 0, [2]string{}},
		{"S. A.", "Soprano and Alto", "voice", 0, [2]string{"soprano", "alto"}},
		{"TENOR BASS", "Tenor Bass", "voice", 0, [2]string{"tenor", "bass"}},
		{"Women", "Women", "voice", 0, [2]string{}},
		{"Bass", "Bass", "voice", 0, [2]string{"probably first basses", "probably second basses"}},
		{"Clarinet in B♭", "Clarinet in Bb", "instrument", -2, [2]string{}},
		{"Clarinet in A", "Clarinet in A", "instrument", -3, [2]string{}},
		{"Trumpet in C", "Trumpet in C", "instrument", 0, [2]string{}},
		{"Horn in F", "Horn in F", "instrument", -7, [2]string{}},
		{"Horn in E♭", "Horn in Eb", "instrument", -9, [2]string{}},
		{"Alto Sax", "Alto Sax", "instrument", -9, [2]string{}},
		{"Tenor Saxophone", "Tenor Saxophone", "instrument", -14, [2]string{}},
		{"Double Bass", "Double Bass", "instrument", -12, [2]string{}},
		{"Flute", "Flute", "instrument", 0, [2]string{}},
		{"Organ", "Organ", "keyboard", 0, [2]string{}},
		{"Pno.", "Piano", "keyboard", 0, [2]string{}},
		{"Melody", "Melody", "other", 0, [2]string{}},
	}
	for _, c := range cases {
		p := classifyLabel(c.in)
		if p.Name != c.name || p.Kind != c.kind || p.Transpose != c.transpose || p.VoiceHints != c.hints {
			t.Errorf("%q: got name=%q kind=%q transpose=%d hints=%v", c.in, p.Name, p.Kind, p.Transpose, p.VoiceHints)
		}
	}
}
