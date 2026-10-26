package main

import (
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
