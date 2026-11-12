package main

// Updating score2audio itself.
//
// Each release carries latest.json, which lists its downloads with their
// sizes and SHA-256, and latest.json.sig, an Ed25519 signature of it made
// with a key only the release build has (the UPDATE_SIGNING_KEY secret).
// score2audio checks for a newer release at most once a day, and installs
// one only if the signature is right and every downloaded byte matches the
// list. Downloads come only from this project's releases on GitHub.
//
// A copy installed with the Windows installer is updated by running the new
// installer; any other copy (the zip and tar files) by replacing its own
// files. The user chooses in the Setup section: install updates
// automatically (the default), ask first, or never check.

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	updateRepo      = "shuteye4220/Score2audio"
	updatePublicKey = "W2GYmZKqX3mcJjnU6LY+OSyXpIhVrhxyrv29O/Yg+j0="
	updateBase      = "https://github.com/" + updateRepo + "/releases"
	maxUpdateSize   = 400 << 20
)

type updateFile struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

type updateManifest struct {
	Version string                `json:"version"`
	Files   map[string]updateFile `json:"files"`
}

// updateInfo is what the page is told about updates.
type updateInfo struct {
	Mode      string `json:"mode"` // auto | ask | off
	Checked   string `json:"checked,omitempty"`
	Available string `json:"available,omitempty"` // a newer version
	NotesURL  string `json:"notesUrl,omitempty"`
	CanApply  bool   `json:"canApply"`          // signed, and this copy can update itself
	Why       string `json:"why,omitempty"`     // why it cannot
	Ready     bool   `json:"ready"`             // downloaded and checked, installs at the next start
	Working   string `json:"working,omitempty"` // what it is doing now
	Error     string `json:"error,omitempty"`
}

var (
	updateMu    sync.Mutex
	updateState updateInfo
	updateFound *updateManifest
	updateBusy  bool
)

// versionRe is a release ("1.2.3"); appVersionRe also takes a test build
// for one ("1.2.3-rc1"), which updates to that release when it is out.
var versionRe = regexp.MustCompile(`^(\d{1,4})\.(\d{1,4})\.(\d{1,6})$`)
var appVersionRe = regexp.MustCompile(`^(\d{1,4})\.(\d{1,4})\.(\d{1,6})(?:-rc\.?(\d{1,4}))?$`)
var updateNameRe = regexp.MustCompile(`^score2audio-[A-Za-z0-9._-]{1,80}$`)

// newerVersion tells whether version a is newer than b ("1.2.3", or
// "1.2.3-rc1", which comes before 1.2.3).
func newerVersion(a, b string) bool {
	ma, mb := appVersionRe.FindStringSubmatch(a), appVersionRe.FindStringSubmatch(b)
	if ma == nil || mb == nil {
		return false
	}
	for i := 1; i <= 3; i++ {
		x, _ := strconv.Atoi(ma[i])
		y, _ := strconv.Atoi(mb[i])
		if x != y {
			return x > y
		}
	}
	// same numbers: a release is newer than its test builds
	switch {
	case ma[4] == "" && mb[4] != "":
		return true
	case ma[4] != "" && mb[4] != "":
		x, _ := strconv.Atoi(ma[4])
		y, _ := strconv.Atoi(mb[4])
		return x > y
	}
	return false
}

// updateKind is which download updates this copy, and how.
func updateKind() (key string, installer bool) {
	switch runtime.GOOS {
	case "windows":
		if isFile(filepath.Join(exeDir(), "uninstall.exe")) {
			return "windows-installer", true
		}
		return "windows-zip", false
	case "darwin":
		return "mac", false
	case "linux":
		if runtime.GOARCH == "arm64" {
			return "linux-arm64", false
		}
		return "linux-x86_64", false
	}
	return "", false
}

// selfUpdatable tells whether this copy's files can be replaced, and if not why.
func selfUpdatable() (bool, string) {
	if !appVersionRe.MatchString(appVersion) {
		return false, "This is a test build of score2audio, which is not updated automatically."
	}
	if _, inst := updateKind(); inst {
		return true, ""
	}
	if !writable(exeDir()) {
		return false, "score2audio is in a folder it cannot change (" + exeDir() + "), so it cannot update itself. Download the new version from the releases page instead, or update it the way you installed it."
	}
	return true, ""
}

var updateClient = &http.Client{Timeout: 60 * time.Second}

func fetchSmall(ctx context.Context, url string, limit int64) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("User-Agent", "score2audio/"+appVersion)
	resp, err := updateClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode, fmt.Errorf("the server answered %s", resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err == nil && int64(len(b)) > limit {
		err = errors.New("the answer was too long")
	}
	return b, resp.StatusCode, err
}

// verifyManifest checks the signature and contents of a release's list of
// downloads. A missing signature gives errUnsigned.
var errUnsigned = errors.New("unsigned")

func verifyManifest(data, sig []byte, publicKey string) (*updateManifest, error) {
	var m updateManifest
	if err := json.Unmarshal(data, &m); err != nil || !versionRe.MatchString(m.Version) {
		return nil, errors.New("the release's list of files could not be read")
	}
	for k, f := range m.Files {
		if !updateNameRe.MatchString(f.Name) || f.Size <= 0 || f.Size > maxUpdateSize || len(f.SHA256) != 64 {
			return nil, fmt.Errorf("the release's list of files has a bad entry (%s)", k)
		}
		if _, err := hex.DecodeString(f.SHA256); err != nil {
			return nil, fmt.Errorf("the release's list of files has a bad entry (%s)", k)
		}
	}
	if sig == nil {
		return &m, errUnsigned
	}
	pub, err := base64.StdEncoding.DecodeString(publicKey)
	s, err2 := base64.StdEncoding.DecodeString(strings.TrimSpace(string(sig)))
	if err != nil || err2 != nil || len(pub) != ed25519.PublicKeySize || !ed25519.Verify(ed25519.PublicKey(pub), data, s) {
		return nil, errors.New("the release's signature is not right, so it was not trusted")
	}
	return &m, nil
}

// checkForUpdate looks for a newer release and records what it finds.
func checkForUpdate(ctx context.Context) {
	updateMu.Lock()
	if updateBusy {
		updateMu.Unlock()
		return
	}
	updateBusy = true
	updateState.Working, updateState.Error = "Checking for a new version", ""
	updateMu.Unlock()
	defer func() {
		updateMu.Lock()
		updateBusy = false
		updateState.Working = ""
		updateMu.Unlock()
	}()

	data, code, err := fetchSmall(ctx, updateBase+"/latest/download/latest.json", 64<<10)
	if code == http.StatusNotFound {
		// the newest release predates automatic updates: nothing to offer
		data, err = []byte(`{"version":"0.0.0","files":{}}`), nil
	}
	var sig []byte
	if err == nil && code != http.StatusNotFound {
		var code int
		sig, code, err = fetchSmall(ctx, updateBase+"/latest/download/latest.json.sig", 1<<10)
		if code == http.StatusNotFound {
			sig, err = nil, nil
		}
	}
	var m *updateManifest
	if err == nil {
		m, err = verifyManifest(data, sig, updatePublicKey)
		if code == http.StatusNotFound {
			err = nil
		}
	}
	s := loadSettings()
	s.LastUpdateCheck = time.Now()
	saveSettings(s)

	updateMu.Lock()
	defer updateMu.Unlock()
	updateState.Checked = time.Now().Format("2 January 2006, 15:04")
	updateState.Available, updateState.CanApply, updateState.Why = "", false, ""
	updateFound = nil
	if err != nil && !errors.Is(err, errUnsigned) {
		updateState.Error = "Could not check for a new version: " + err.Error()
		return
	}
	if !newerVersion(m.Version, appVersion) {
		return
	}
	updateState.Available = m.Version
	updateState.NotesURL = updateBase + "/tag/v" + m.Version
	key, _ := updateKind()
	ok, why := selfUpdatable()
	switch {
	case errors.Is(err, errUnsigned):
		updateState.Why = "This release cannot be installed automatically (it is not signed). Download it from the releases page."
	case m.Files[key].Name == "":
		updateState.Why = "This release has no download for this kind of computer that score2audio can install by itself."
	case !ok:
		updateState.Why = why
	default:
		updateState.CanApply = true
		updateFound = m
	}
}

// fetchUpdate downloads the update for this copy and checks it.
func fetchUpdate(ctx context.Context, m *updateManifest) (string, error) {
	key, _ := updateKind()
	f := m.Files[key]
	p := pinnedFile{what: "score2audio " + m.Version, url: updateBase + "/download/v" + m.Version + "/" + f.Name, size: f.Size, sha256: strings.ToLower(f.SHA256)}
	say := func(s string) {
		updateMu.Lock()
		updateState.Working = s
		updateMu.Unlock()
	}
	path, err := download(ctx, p, filepath.Join(dataDir, "updates"), say)
	if err != nil {
		return "", err
	}
	// remember it for the next start
	b, _ := json.Marshal(map[string]interface{}{"version": m.Version, "kind": key, "file": f})
	return path, os.WriteFile(filepath.Join(dataDir, "updates", "pending.json"), b, 0o600)
}

// pendingUpdate is a downloaded update waiting to be installed, checked again.
func pendingUpdate() (version, file string, ok bool) {
	b, err := os.ReadFile(filepath.Join(dataDir, "updates", "pending.json"))
	if err != nil {
		return "", "", false
	}
	var p struct {
		Version string     `json:"version"`
		Kind    string     `json:"kind"`
		File    updateFile `json:"file"`
	}
	key, _ := updateKind()
	if json.Unmarshal(b, &p) != nil || p.Kind != key || !newerVersion(p.Version, appVersion) || !updateNameRe.MatchString(p.File.Name) {
		os.RemoveAll(filepath.Join(dataDir, "updates"))
		return "", "", false
	}
	file = filepath.Join(dataDir, "updates", p.File.Name)
	if checkFile(file, pinnedFile{size: p.File.Size, sha256: strings.ToLower(p.File.SHA256)}) != nil {
		os.RemoveAll(filepath.Join(dataDir, "updates"))
		return "", "", false
	}
	return p.Version, file, true
}

// applyUpdate installs a checked download and starts the new version. It
// returns only if that fails.
func applyUpdate(version, file string) error {
	key, installer := updateKind()
	if installer {
		// the installer waits for this program to close, updates it
		// without asking anything, and starts the new version
		cmd := exec.Command(file, "/S", "/UPDATE")
		if err := cmd.Start(); err != nil {
			return err
		}
		os.Remove(filepath.Join(dataDir, "updates", "pending.json"))
		fmt.Println("Installing score2audio", version, "- it starts again by itself in a moment.")
		exitSoon()
		return nil
	}
	dir := exeDir()
	if err := replaceOwnFiles(dir, key, file); err != nil {
		return err
	}
	os.RemoveAll(filepath.Join(dataDir, "updates"))
	fmt.Println("score2audio was updated to", version+". Starting the new version.")
	exe := filepath.Join(dir, updateExe(key))
	var env []string
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "SCORE2AUDIO_UPDATED=") {
			env = append(env, kv)
		}
	}
	env = append(env, "SCORE2AUDIO_UPDATED="+version)
	go func() {
		time.Sleep(500 * time.Millisecond) // let the page have its answer first
		if err := restartAs(exe, os.Args[1:], env); err != nil {
			fmt.Println("The new version could not be started:", err, "- start score2audio again.")
			os.Exit(1)
		}
	}()
	return nil
}

var exitSoon = func() {
	go func() {
		time.Sleep(500 * time.Millisecond)
		os.Exit(0)
	}()
}

// updateExe is the program's file name in a release archive.
func updateExe(key string) string {
	if strings.HasPrefix(key, "windows") {
		return "score2audio.exe"
	}
	return "score2audio"
}

// updatePaths is where each file of a release archive goes, relative to the
// program's folder. Only these are taken from the archive.
func updatePaths(key, name string) (string, bool) {
	name = path.Clean(strings.TrimPrefix(name, "./"))
	if key != "windows-zip" {
		// the Mac and Linux archives have everything in a score2audio folder
		rest, ok := strings.CutPrefix(name, "score2audio/")
		if !ok {
			return "", false
		}
		name = rest
	}
	switch {
	case name == updateExe(key), name == "README.md", name == "README.txt", name == "LICENSE", name == "LICENSE.txt",
		name == "start-score2audio.command", name == "setup.sh", name == "Install helpers.command", name == "score2audio.desktop",
		name == "tessdata/eng.traineddata":
		return name, true
	case strings.HasPrefix(name, "verovio/") && !strings.Contains(name[len("verovio/"):], "/") && len(name) > len("verovio/"):
		return name, true
	}
	return "", false
}

// replaceOwnFiles unpacks a release archive over the copy of score2audio in dir.
func replaceOwnFiles(dir, key, archive string) error {
	type entry struct {
		name string
		mode os.FileMode
		open func() (io.ReadCloser, error)
		size int64
	}
	var entries []entry
	var closer io.Closer
	if strings.HasSuffix(archive, ".zip") {
		zr, err := zip.OpenReader(archive)
		if err != nil {
			return err
		}
		closer = zr
		for _, f := range zr.File {
			if f.FileInfo().IsDir() {
				continue
			}
			f := f
			entries = append(entries, entry{f.Name, f.Mode(), f.Open, int64(f.UncompressedSize64)})
		}
	} else {
		// .tar.gz: read it into memory once (it is checked and small)
		raw, err := os.ReadFile(archive)
		if err != nil {
			return err
		}
		gz, err := gzip.NewReader(bytes.NewReader(raw))
		if err != nil {
			return err
		}
		tr := tar.NewReader(io.LimitReader(gz, maxUpdateSize))
		for {
			h, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return err
			}
			if _, ok := updatePaths(key, h.Name); !ok || h.Typeflag != tar.TypeReg || h.Size > maxUpdateSize {
				continue
			}
			b, err := io.ReadAll(io.LimitReader(tr, h.Size))
			if err != nil {
				return err
			}
			entries = append(entries, entry{h.Name, os.FileMode(h.Mode) & 0o777, func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(b)), nil }, h.Size})
		}
	}
	if closer != nil {
		defer closer.Close()
	}
	exe := updateExe(key)
	// installed with "make install" (the program alone in a bin folder, its
	// data elsewhere): replace only the program, never other files there
	onlyExe := !isFile(filepath.Join(dir, "tessdata", "eng.traineddata"))
	var plan []entry
	hasExe := false
	for _, e := range entries {
		rel, ok := updatePaths(key, e.name)
		if !ok || e.size > maxUpdateSize || (onlyExe && rel != exe) {
			continue
		}
		if rel == exe {
			hasExe = true
		}
		e.name = rel
		plan = append(plan, e)
	}
	if !hasExe {
		return errors.New("the update does not contain score2audio itself")
	}
	// write everything next to where it goes first, then swap it all in
	var staged [][2]string
	cleanup := func() {
		for _, s := range staged {
			os.Remove(s[0])
		}
	}
	for _, e := range plan {
		dest := filepath.Join(dir, filepath.FromSlash(e.name))
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			cleanup()
			return err
		}
		rc, err := e.open()
		if err != nil {
			cleanup()
			return err
		}
		mode := os.FileMode(0o644)
		if e.name == exe || strings.HasSuffix(e.name, ".command") || strings.HasSuffix(e.name, ".sh") || e.mode&0o111 != 0 {
			mode = 0o755
		}
		tmp := dest + ".new"
		w, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
		if err == nil {
			_, err = io.Copy(w, io.LimitReader(rc, e.size))
			if cerr := w.Close(); err == nil {
				err = cerr
			}
		}
		rc.Close()
		if err != nil {
			os.Remove(tmp)
			cleanup()
			return err
		}
		os.Chmod(tmp, mode)
		staged = append(staged, [2]string{tmp, dest})
	}
	for _, s := range staged {
		if err := replaceFile(s[0], s[1]); err != nil {
			cleanup()
			return fmt.Errorf("could not replace %s: %v", s[1], err)
		}
	}
	return nil
}

// cleanOldFiles removes what the last update renamed out of the way: only
// score2audio's own files, as its folder may be shared (~/.local/bin).
func cleanOldFiles() {
	dir := exeDir()
	old := []string{filepath.Join(dir, "score2audio.exe.old"), filepath.Join(dir, "score2audio.old"), filepath.Join(dir, "ffmpeg", "ffmpeg.exe.old")}
	for _, sub := range []string{"verovio", "tessdata"} {
		more, _ := filepath.Glob(filepath.Join(dir, sub, "*.old"))
		old = append(old, more...)
	}
	for _, f := range old {
		os.Remove(f)
	}
}

// startupUpdate runs before the page is served: it installs an update
// downloaded earlier (in automatic mode), and starts the daily check.
func startupUpdate() {
	cleanOldFiles()
	s := loadSettings()
	mode := s.UpdateMode
	if mode == "" {
		mode = "auto"
	}
	updateMu.Lock()
	updateState.Mode = mode
	updateMu.Unlock()
	if mode == "off" {
		return
	}
	v, file, ok := pendingUpdate()
	if !ok {
		os.RemoveAll(filepath.Join(dataDir, "updates")) // an installer that has run, or an unfinished download
	}
	if ok {
		if mode == "auto" {
			fmt.Println("Installing score2audio", v, "(downloaded earlier and checked).")
			if err := applyUpdate(v, file); err != nil {
				fmt.Println("The update could not be installed:", err)
			} else {
				select {} // the new version is starting; wait to be closed
			}
		} else {
			updateMu.Lock()
			updateState.Available, updateState.Ready, updateState.CanApply = v, true, true
			updateState.NotesURL = updateBase + "/tag/v" + v
			updateMu.Unlock()
		}
	}
	if time.Since(s.LastUpdateCheck) < 20*time.Hour || !appVersionRe.MatchString(appVersion) {
		return
	}
	go func() {
		time.Sleep(5 * time.Second) // let the page open first
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		checkForUpdate(ctx)
		updateMu.Lock()
		m, mode := updateFound, updateState.Mode
		updateMu.Unlock()
		if m != nil && mode == "auto" {
			if _, err := fetchUpdate(ctx, m); err == nil {
				updateMu.Lock()
				updateState.Ready = true
				updateMu.Unlock()
			} else {
				updateMu.Lock()
				updateState.Error = "The new version could not be downloaded: " + err.Error()
				updateMu.Unlock()
			}
		}
	}()
}

func currentUpdateInfo() updateInfo {
	updateMu.Lock()
	defer updateMu.Unlock()
	return updateState
}

// handleUpdate is /api/update/...: check, install, mode.
func handleUpdate(w http.ResponseWriter, r *http.Request, action string) {
	switch action {
	case "check":
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
		defer cancel()
		checkForUpdate(ctx)
		writeJSON(w, currentUpdateInfo())
	case "install":
		if busyReading() {
			httpErr(w, 409, "A score is being read. Update when it is finished.")
			return
		}
		v, file, ok := pendingUpdate()
		if !ok {
			updateMu.Lock()
			m := updateFound
			updateMu.Unlock()
			if m == nil {
				httpErr(w, 400, "There is no new version to install. Check for updates first.")
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), 30*time.Minute)
			defer cancel()
			if _, err := fetchUpdate(ctx, m); err != nil {
				httpErr(w, 500, "The new version could not be downloaded: "+err.Error())
				return
			}
			if v, file, ok = pendingUpdate(); !ok {
				httpErr(w, 500, "The downloaded update did not check out, so it was not installed.")
				return
			}
		}
		if err := applyUpdate(v, file); err != nil {
			httpErr(w, 500, "The update could not be installed: "+err.Error())
			return
		}
		writeJSON(w, map[string]string{"version": v})
	case "mode":
		var body struct {
			Mode string `json:"mode"`
		}
		if json.NewDecoder(io.LimitReader(r.Body, 1<<10)).Decode(&body) != nil || (body.Mode != "auto" && body.Mode != "ask" && body.Mode != "off") {
			httpErr(w, 400, "Unknown update setting.")
			return
		}
		s := loadSettings()
		s.UpdateMode = body.Mode
		if err := saveSettings(s); err != nil {
			httpErr(w, 500, "Could not save the setting: "+err.Error())
			return
		}
		updateMu.Lock()
		updateState.Mode = body.Mode
		updateMu.Unlock()
		writeJSON(w, currentUpdateInfo())
	default:
		httpErr(w, 404, "Unknown request.")
	}
}

func busyReading() bool {
	jobsMu.Lock()
	defer jobsMu.Unlock()
	for _, j := range jobs {
		if j.State == "running" {
			return true
		}
	}
	return false
}
