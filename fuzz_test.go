package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// FuzzLoadModel feeds random MusicXML to the parser. Any panic is a bug: a
// crafted score (or a misreading by Audiveris) must never crash score2audio.
func FuzzLoadModel(f *testing.F) {
	f.Add([]byte(sample))
	f.Add([]byte(strings.ReplaceAll(sample, "<divisions>2</divisions>", "<divisions>0</divisions>")))
	f.Add([]byte(strings.ReplaceAll(sample, `tempo="120"`, `tempo="0"`)))
	f.Add([]byte(strings.ReplaceAll(sample, "<beats>2</beats>", "<beats>0</beats>")))
	f.Add([]byte(strings.ReplaceAll(sample, "<beat-type>4</beat-type>", "<beat-type>0</beat-type>")))
	f.Add([]byte(strings.ReplaceAll(sample, "<duration>4</duration>", "<duration>-99999999999</duration>")))
	f.Add([]byte(strings.ReplaceAll(sample, "<staves>2</staves>", "<staves>100000000</staves>")))
	f.Add([]byte(strings.ReplaceAll(sample, "<octave>5</octave>", "<octave>9999999999</octave>")))
	f.Fuzz(func(t *testing.T, data []byte) {
		// A score that takes this long to read is a denial of service.
		watchdog := time.AfterFunc(5*time.Second, func() { panic("reading this input took more than 5 seconds") })
		defer watchdog.Stop()
		p := filepath.Join(t.TempDir(), "f.musicxml")
		if err := os.WriteFile(p, data, 0o644); err != nil {
			t.Skip()
		}
		LoadModel([]string{p}, "fuzz")
	})
}
