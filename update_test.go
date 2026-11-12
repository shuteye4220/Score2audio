package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// ---------- downloads ----------

func pinFor(b []byte, url string) pinnedFile {
	h := sha256.Sum256(b)
	return pinnedFile{what: "the test file", url: url, size: int64(len(b)), sha256: hex.EncodeToString(h[:])}
}

func TestDownloadChecksTheFile(t *testing.T) {
	good := bytes.Repeat([]byte("score2audio "), 50000)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/good.bin", "/tampered.bin":
			b := good
			if r.URL.Path == "/tampered.bin" {
				b = append([]byte("X"), good[1:]...)
			}
			w.Write(b)
		case "/longer.bin":
			w.Write(append(good, 'x'))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	dir := t.TempDir()
	var said []string
	say := func(s string) { said = append(said, s) }

	p, err := download(context.Background(), pinFor(good, srv.URL+"/good.bin"), dir, say)
	if err != nil || filepath.Base(p) != "good.bin" {
		t.Fatalf("good download: %v %s", err, p)
	}
	if len(said) < 3 || !strings.Contains(said[len(said)-1], "checked") {
		t.Errorf("progress not reported: %q", said)
	}
	for _, name := range []string{"tampered.bin", "longer.bin", "missing.bin"} {
		pin := pinFor(good, srv.URL+"/"+name)
		if _, err := download(context.Background(), pin, dir, say); err == nil {
			t.Errorf("%s was accepted", name)
		}
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			t.Errorf("%s was left behind", name)
		}
		if _, err := os.Stat(filepath.Join(dir, name+".download")); err == nil {
			t.Errorf("%s.download was left behind", name)
		}
	}
	// a file already downloaded and checked is used without downloading again
	srv.Close()
	if _, err := download(context.Background(), pinFor(good, srv.URL+"/good.bin"), dir, say); err != nil {
		t.Errorf("the checked file was not reused: %v", err)
	}
}

func TestPinnedDownloads(t *testing.T) {
	for _, p := range []pinnedFile{audiverisWindows, audiverisMacARM, audiverisMacIntel, audiverisUbuntu24, audiverisUbuntu22, ffmpegWindows} {
		if !strings.HasPrefix(p.url, "https://github.com/") || p.size < 1<<20 || len(p.sha256) != 64 || strings.ToLower(p.sha256) != p.sha256 {
			t.Errorf("bad pin: %+v", p)
		}
	}
}

func TestUnpackFFmpeg(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range map[string]string{
		"ffmpeg-8.1.2-essentials_build/bin/ffmpeg.exe":  "MZ ffmpeg",
		"ffmpeg-8.1.2-essentials_build/bin/ffplay.exe":  "MZ ffplay",
		"ffmpeg-8.1.2-essentials_build/LICENSE":         "GPL",
		"ffmpeg-8.1.2-essentials_build/README.txt":      "readme",
		"ffmpeg-8.1.2-essentials_build/../../evil.exe":  "evil",
		"ffmpeg-8.1.2-essentials_build/bin/../../x.exe": "evil",
	} {
		w, _ := zw.Create(name)
		w.Write([]byte(body))
	}
	zw.Close()
	dir := t.TempDir()
	zp := filepath.Join(dir, "ff.zip")
	os.WriteFile(zp, buf.Bytes(), 0o600)
	dest := filepath.Join(dir, "app", "ffmpeg")
	if err := unpackFFmpeg(zp, []string{dest}, func(string) {}); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadDir(dest)
	var names []string
	for _, e := range got {
		names = append(names, e.Name())
	}
	if strings.Join(names, ",") != "LICENSE.txt,README.txt,ffmpeg.exe" {
		t.Fatalf("unpacked %v", names)
	}
	if _, err := os.Stat(filepath.Join(dir, "evil.exe")); err == nil {
		t.Fatal("a file escaped the folder")
	}
}

func TestLinuxPlans(t *testing.T) {
	with := func(cmds ...string) func(string) bool {
		return func(n string) bool {
			for _, c := range cmds {
				if c == n {
					return true
				}
			}
			return false
		}
	}
	arch := osRelease{id: "arch"}
	if p := linuxPlanFor(arch, "Audiveris", with("pacman", "yay")); strings.Join(p.cmd, " ") != "yay -S --needed audiveris" || p.sudo {
		t.Errorf("Arch with yay: %+v", p)
	}
	if p := linuxPlanFor(osRelease{id: "endeavouros", like: "arch"}, "Audiveris", with("pacman")); p.manual == "" || !strings.Contains(p.manual, "AUR") {
		t.Errorf("Arch without an AUR helper: %+v", p)
	}
	if p := linuxPlanFor(arch, "FFmpeg", with("pacman")); strings.Join(p.cmd, " ") != "pacman -S --needed ffmpeg" || !p.sudo {
		t.Errorf("Arch FFmpeg: %+v", p)
	}
	if runtime.GOARCH == "amd64" { // the .deb packages are for x86_64 only
		if p := linuxPlanFor(osRelease{id: "ubuntu", like: "debian", version: "24.04"}, "Audiveris", with("apt-get")); p.download == nil || p.download.url != audiverisUbuntu24.url || !p.sudo {
			t.Errorf("Ubuntu 24.04: %+v", p)
		}
		if p := linuxPlanFor(osRelease{id: "debian", version: "12"}, "Audiveris", with("apt-get")); p.download == nil || p.download.url != audiverisUbuntu22.url {
			t.Errorf("Debian 12: %+v", p)
		}
	}
	if p := linuxPlanFor(osRelease{id: "fedora"}, "Audiveris", with("dnf", "flatpak")); !strings.Contains(strings.Join(p.cmd, " "), "org.audiveris.audiveris") {
		t.Errorf("Fedora: %+v", p)
	}
	if p := linuxPlanFor(osRelease{id: "fedora"}, "FFmpeg", with("dnf")); strings.Join(p.cmd, " ") != "dnf install -y ffmpeg-free" {
		t.Errorf("Fedora FFmpeg: %+v", p)
	}
	if p := linuxPlanFor(osRelease{id: "slackware"}, "Audiveris", with()); p.manual == "" {
		t.Errorf("unknown distribution: %+v", p)
	}
}

// ---------- converting with FFmpeg ----------

func TestConvertArgs(t *testing.T) {
	args := convertArgs(conversions["mp3"], "/tmp/x")
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-i /tmp/x/in.wav") || !strings.HasSuffix(joined, "/tmp/x/out.mp3") || !strings.Contains(joined, "-nostdin") {
		t.Fatalf("mp3 args: %s", joined)
	}
	if v := strings.Join(convertArgs(conversions["video-aac"], "d"), " "); !strings.Contains(v, "-c:v copy") || !strings.Contains(v, "-c:a aac") {
		t.Fatalf("video args: %s", v)
	}
}

func TestConvertWithFFmpeg(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("FFmpeg is not installed")
	}
	// one second of a 440 Hz tone, as the page would send it
	ff, _ := exec.LookPath("ffmpeg")
	wav, err := exec.Command(ff, "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "sine=frequency=440:duration=1", "-f", "wav", "-").Output()
	if err != nil {
		t.Skip("FFmpeg cannot make test sound:", err)
	}
	rec := httptest.NewRecorder()
	handleConvert(rec, httptest.NewRequest(http.MethodPost, "/api/convert?to=mp3", bytes.NewReader(wav)))
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "audio/mpeg" || rec.Body.Len() < 1000 {
		t.Fatalf("mp3: %d %s %d bytes: %s", rec.Code, rec.Header().Get("Content-Type"), rec.Body.Len(), rec.Body.String()[:min(200, rec.Body.Len())])
	}
	b := rec.Body.Bytes()
	if !(bytes.HasPrefix(b, []byte("ID3")) || (b[0] == 0xFF && b[1]&0xE0 == 0xE0)) {
		t.Fatalf("not an MP3 file: % x", b[:4])
	}
	rec = httptest.NewRecorder()
	handleConvert(rec, httptest.NewRequest(http.MethodPost, "/api/convert?to=exe", bytes.NewReader(wav)))
	if rec.Code != 400 {
		t.Fatalf("unknown conversion accepted: %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	handleConvert(rec, httptest.NewRequest(http.MethodPost, "/api/convert?to=mp3", strings.NewReader("not a sound")))
	if rec.Code != 500 {
		t.Fatalf("junk converted: %d", rec.Code)
	}
}

// ---------- updates ----------

func TestNewerVersion(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"1.1.0", "1.0.1", true}, {"1.0.10", "1.0.9", true}, {"2.0.0", "1.99.99", true},
		{"1.0.1", "1.0.1", false}, {"1.0.0", "1.0.1", false}, {"1.1.0", "dev-abc1234", false},
		{"01.1.0x", "1.0.0", false},
		{"1.1.0", "1.1.0-rc1", true}, {"1.1.0-rc1", "1.1.0", false}, {"1.1.0-rc2", "1.1.0-rc1", true}, {"1.1.0-rc1", "1.0.1", true},
		{"1.1.0", "1.1.0-beta", false},
	}
	for _, c := range cases {
		if got := newerVersion(c.a, c.b); got != c.want {
			t.Errorf("newerVersion(%s, %s) = %v", c.a, c.b, got)
		}
	}
}

func TestVerifyManifest(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	key := base64.StdEncoding.EncodeToString(pub)
	data := []byte(`{"version":"1.2.0","files":{"windows-installer":{"name":"score2audio-windows-setup.exe","size":123,"sha256":"` + strings.Repeat("ab", 32) + `"}}}`)
	sig := []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(priv, data)) + "\n")

	m, err := verifyManifest(data, sig, key)
	if err != nil || m.Version != "1.2.0" || m.Files["windows-installer"].Size != 123 {
		t.Fatalf("good manifest: %v %+v", err, m)
	}
	changed := bytes.Replace(data, []byte("1.2.0"), []byte("1.3.0"), 1)
	if _, err := verifyManifest(changed, sig, key); err == nil {
		t.Fatal("a changed manifest was trusted")
	}
	other, _, _ := ed25519.GenerateKey(rand.Reader)
	if _, err := verifyManifest(data, sig, base64.StdEncoding.EncodeToString(other)); err == nil {
		t.Fatal("a manifest signed with another key was trusted")
	}
	if _, err := verifyManifest(data, nil, key); !errors.Is(err, errUnsigned) {
		t.Fatalf("unsigned: %v", err)
	}
	for _, bad := range []string{
		`{"version":"1.2.0","files":{"mac":{"name":"../../evil","size":1,"sha256":"` + strings.Repeat("ab", 32) + `"}}}`,
		`{"version":"1.2.0","files":{"mac":{"name":"score2audio-mac.zip","size":1,"sha256":"xyz"}}}`,
		`{"version":"1.2.0","files":{"mac":{"name":"score2audio-mac.zip","size":0,"sha256":"` + strings.Repeat("ab", 32) + `"}}}`,
		`{"version":"latest","files":{}}`,
		`not json`,
	} {
		b := []byte(bad)
		s := []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(priv, b)))
		if _, err := verifyManifest(b, s, key); err == nil {
			t.Errorf("bad manifest trusted: %s", bad)
		}
	}
	if pk, err := base64.StdEncoding.DecodeString(updatePublicKey); err != nil || len(pk) != ed25519.PublicKeySize {
		t.Fatal("the built-in public key is not an Ed25519 key")
	}
}

func TestUpdatePaths(t *testing.T) {
	ok := map[[2]string]string{
		{"mac", "score2audio/score2audio"}:                        "score2audio",
		{"linux-x86_64", "score2audio/verovio/COPYING"}:           "verovio/COPYING",
		{"linux-arm64", "./score2audio/tessdata/eng.traineddata"}: "tessdata/eng.traineddata",
		{"windows-zip", "verovio/verovio-toolkit-wasm.js"}:        "verovio/verovio-toolkit-wasm.js",
		{"mac", "score2audio/Install helpers.command"}:            "Install helpers.command",
	}
	for in, want := range ok {
		if got, ok := updatePaths(in[0], in[1]); !ok || got != want {
			t.Errorf("%v: got %q %v", in, got, ok)
		}
	}
	for _, in := range [][2]string{
		{"mac", "score2audio/../../.bashrc"}, {"mac", "/etc/passwd"}, {"linux-x86_64", "score2audio/verovio/../../x"},
		{"linux-x86_64", "score2audio/verovio/sub/x.js"}, {"windows-zip", "evil.dll"}, {"mac", "other/score2audio"},
		{"windows-zip", "tessdata/../score2audio.exe/x"},
	} {
		if got, ok := updatePaths(in[0], in[1]); ok {
			t.Errorf("%v allowed as %q", in, got)
		}
	}
}

func TestReplaceOwnFiles(t *testing.T) {
	dir := t.TempDir()
	app := filepath.Join(dir, "app")
	os.MkdirAll(filepath.Join(app, "verovio"), 0o755)
	os.MkdirAll(filepath.Join(app, "tessdata"), 0o755)
	os.WriteFile(filepath.Join(app, "tessdata", "eng.traineddata"), []byte("data"), 0o644)
	os.WriteFile(filepath.Join(app, "score2audio"), []byte("old program"), 0o755)
	os.WriteFile(filepath.Join(app, "verovio", "verovio-toolkit-wasm.js"), []byte("old engraver"), 0o644)
	os.WriteFile(filepath.Join(app, "notes.txt"), []byte("mine"), 0o644)

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	add := func(name, body string, mode int64) {
		tw.WriteHeader(&tar.Header{Name: name, Mode: mode, Size: int64(len(body)), Typeflag: tar.TypeReg})
		tw.Write([]byte(body))
	}
	add("score2audio/score2audio", "new program", 0o755)
	add("score2audio/verovio/verovio-toolkit-wasm.js", "new engraver", 0o644)
	add("score2audio/../outside", "evil", 0o644)
	add("score2audio/notes.txt", "replaced", 0o644)
	tw.Close()
	gz.Close()
	archive := filepath.Join(dir, "score2audio-linux-x86_64.tar.gz")
	os.WriteFile(archive, buf.Bytes(), 0o600)

	if err := replaceOwnFiles(app, "linux-x86_64", archive); err != nil {
		t.Fatal(err)
	}
	read := func(p string) string { b, _ := os.ReadFile(filepath.Join(app, p)); return string(b) }
	if read("score2audio") != "new program" || read("verovio/verovio-toolkit-wasm.js") != "new engraver" || read("notes.txt") != "mine" {
		t.Fatalf("files: %q %q %q", read("score2audio"), read("verovio/verovio-toolkit-wasm.js"), read("notes.txt"))
	}
	if st, _ := os.Stat(filepath.Join(app, "score2audio")); st.Mode()&0o111 == 0 {
		t.Fatal("the new program cannot be run")
	}
	if _, err := os.Stat(filepath.Join(dir, "outside")); err == nil {
		t.Fatal("a file escaped the program's folder")
	}
	leftovers, _ := filepath.Glob(filepath.Join(app, "*.new"))
	if len(leftovers) > 0 {
		t.Fatalf("left behind: %v", leftovers)
	}

	// installed with "make install": only the program is replaced
	bin := filepath.Join(dir, "bin")
	os.MkdirAll(bin, 0o755)
	os.WriteFile(filepath.Join(bin, "score2audio"), []byte("old program"), 0o755)
	os.WriteFile(filepath.Join(bin, "README.md"), []byte("someone else's"), 0o644)
	if err := replaceOwnFiles(bin, "linux-x86_64", archive); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(bin, "score2audio")); string(b) != "new program" {
		t.Fatal("program not replaced in a bin folder")
	}
	if _, err := os.Stat(filepath.Join(bin, "verovio")); err == nil {
		t.Fatal("other files were put in a bin folder")
	}

	// an archive without the program is refused, and nothing is changed
	var empty bytes.Buffer
	zw := zip.NewWriter(&empty)
	w, _ := zw.Create("README.txt")
	w.Write([]byte("hello"))
	zw.Close()
	zp := filepath.Join(dir, "score2audio-windows.zip")
	os.WriteFile(zp, empty.Bytes(), 0o600)
	if err := replaceOwnFiles(app, "windows-zip", zp); err == nil {
		t.Fatal("an update without the program was installed")
	}
}

func TestPendingUpdateIsCheckedAgain(t *testing.T) {
	old := dataDir
	dataDir = t.TempDir()
	defer func() { dataDir = old }()
	oldVersion := appVersion
	appVersion = "1.0.0"
	defer func() { appVersion = oldVersion }()

	key, _ := updateKind()
	body := []byte("the new version")
	h := sha256.Sum256(body)
	dir := filepath.Join(dataDir, "updates")
	os.MkdirAll(dir, 0o700)
	os.WriteFile(filepath.Join(dir, "score2audio-test.zip"), body, 0o600)
	write := func(version, sum string) {
		os.MkdirAll(dir, 0o700)
		os.WriteFile(filepath.Join(dir, "score2audio-test.zip"), body, 0o600)
		os.WriteFile(filepath.Join(dir, "pending.json"), []byte(`{"version":"`+version+`","kind":"`+key+`","file":{"name":"score2audio-test.zip","size":15,"sha256":"`+sum+`"}}`), 0o600)
	}
	write("1.1.0", hex.EncodeToString(h[:]))
	if v, f, ok := pendingUpdate(); !ok || v != "1.1.0" || filepath.Base(f) != "score2audio-test.zip" {
		t.Fatalf("good pending update: %v %v %v", v, f, ok)
	}
	write("1.1.0", strings.Repeat("0", 64))
	if _, _, ok := pendingUpdate(); ok {
		t.Fatal("a changed download was installed")
	}
	write("0.9.0", hex.EncodeToString(h[:]))
	if _, _, ok := pendingUpdate(); ok {
		t.Fatal("an older version was installed")
	}
}
