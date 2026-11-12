package main

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAudiverisEnv(t *testing.T) {
	env := audiverisEnv([]string{"PATH=/bin", "JAVA_TOOL_OPTIONS=-Dfoo=1", "TESSDATA_PREFIX=/old"}, "/new/tessdata")
	joined := strings.Join(env, "\n")
	if !strings.Contains(joined, "JAVA_TOOL_OPTIONS=-Dfoo=1 -Djava.awt.headless=true") {
		t.Fatalf("existing Java options lost or headless missing:\n%s", joined)
	}
	if strings.Count(joined, "JAVA_TOOL_OPTIONS=") != 1 || strings.Count(joined, "TESSDATA_PREFIX=") != 1 || !strings.Contains(joined, "TESSDATA_PREFIX=/new/tessdata") {
		t.Fatalf("duplicate or wrong variables:\n%s", joined)
	}
	env = audiverisEnv([]string{"PATH=/bin"}, "")
	if strings.Join(env, "\n") != "PATH=/bin\nJAVA_TOOL_OPTIONS=-Djava.awt.headless=true" {
		t.Fatalf("got %v", env)
	}
}

func TestHasLegacyOCR(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, offsets []int64) string {
		b := make([]byte, 4+8*len(offsets))
		binary.LittleEndian.PutUint32(b, uint32(len(offsets)))
		for i, o := range offsets {
			binary.LittleEndian.PutUint64(b[4+8*i:], uint64(o))
		}
		p := filepath.Join(dir, name)
		os.WriteFile(p, b, 0o644)
		return p
	}
	offs := make([]int64, 24)
	for i := range offs {
		offs[i] = -1
	}
	offs[17] = 200 // the newer recogniser only, like a "fast" file
	if hasLegacyOCR(write("fast", offs)) {
		t.Error("a file without the legacy recogniser was accepted")
	}
	offs[3] = 100
	if !hasLegacyOCR(write("full", offs)) {
		t.Error("a file with the legacy recogniser was refused")
	}
	if hasLegacyOCR(filepath.Join(dir, "missing")) || hasLegacyOCR(write("junk", []int64{1})) {
		t.Error("a missing or broken file was accepted")
	}
}
