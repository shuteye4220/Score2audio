package main

// score2audio: reads a PDF of sheet music (through Audiveris) and plays the
// lines of any score back in an accessible web page served on this computer only.

import (
	"bufio"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var appVersion = "1.0.0"

//go:embed web
var webFiles embed.FS

var (
	dataDir   string
	token     string
	jobsMu    sync.Mutex
	jobs      = map[string]*Job{}
	settingMu sync.Mutex
)

type Settings struct {
	AudiverisPath string `json:"audiverisPath"`
}

type Job struct {
	ID       string    `json:"id"`
	ScoreID  string    `json:"scoreId"`
	State    string    `json:"state"` // running | done | error
	Page     int       `json:"page"`
	Pages    int       `json:"pages"`
	Message  string    `json:"message"`
	Started  time.Time `json:"started"`
	Elapsed  int       `json:"elapsed"`
	LogLines []string  `json:"-"`
	Details  []string  `json:"details,omitempty"`
	LogPath  string    `json:"logPath,omitempty"`
	failed   []int
}

type ScoreMeta struct {
	ID       string    `json:"id"`
	Title    string    `json:"title"`
	FileName string    `json:"fileName"`
	Kind     string    `json:"kind"` // pdf | musicxml
	Created  time.Time `json:"created"`
	Ready    bool      `json:"ready"`
	Measures int       `json:"measures"`
	// pages Audiveris could not read music from
	SkippedPages []int `json:"skippedPages,omitempty"`
}

func main() {
	if len(os.Args) > 2 && os.Args[1] == "--dump" {
		m, err := LoadModel(os.Args[2:], "test")
		if err != nil {
			fmt.Println("error:", err)
			os.Exit(1)
		}
		b, _ := json.MarshalIndent(m, "", " ")
		os.Stdout.Write(b)
		return
	}

	dataDir = defaultDataDir()
	if v := os.Getenv("SCORE2AUDIO_DATA"); v != "" {
		dataDir = v
	}
	os.MkdirAll(filepath.Join(dataDir, "scores"), 0o755)

	b := make([]byte, 16)
	rand.Read(b)
	token = hex.EncodeToString(b)

	port := 0
	if v := os.Getenv("SCORE2AUDIO_PORT"); v != "" {
		port, _ = strconv.Atoi(v)
	}
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		fmt.Println("score2audio could not start:", err)
		waitForEnter()
		return
	}
	addr := ln.Addr().(*net.TCPAddr)
	host := fmt.Sprintf("127.0.0.1:%d", addr.Port)
	url := fmt.Sprintf("http://%s/?t=%s", host, token)

	mux := http.NewServeMux()
	sub, _ := fs.Sub(webFiles, "web")
	static := http.FileServer(http.FS(sub))
	mux.Handle("/", noCache(static))
	mux.HandleFunc("/api/", apiHandler)

	srv := &http.Server{Handler: hostGuard(host, mux)}
	fmt.Println("score2audio", appVersion, "is running.")
	fmt.Println("Your web browser should open by itself. If it does not, open this address:")
	fmt.Println(url)
	fmt.Println()
	fmt.Println("To quit, use the Quit button at the bottom of the page, or close this window.")
	if os.Getenv("SCORE2AUDIO_NO_BROWSER") == "" {
		go func() {
			time.Sleep(300 * time.Millisecond)
			openBrowser(url)
		}()
	}
	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		fmt.Println("Server stopped:", err)
	}
}

func waitForEnter() {
	fmt.Println("Press Enter to close.")
	bufio.NewReader(os.Stdin).ReadString('\n')
}

func noCache(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		h.ServeHTTP(w, r)
	})
}

// hostGuard blocks requests that did not come to our own address (protects
// against DNS-rebinding) and requires the session token for the API.
func hostGuard(host string, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != host && r.Host != strings.Replace(host, "127.0.0.1", "localhost", 1) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") && r.Header.Get("X-Token") != token {
			http.Error(w, "This page is out of date. Please close it and start score2audio again.", http.StatusForbidden)
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		h.ServeHTTP(w, r)
	})
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	cmd.Start()
}

// ---------- settings & Audiveris discovery ----------

func loadSettings() Settings {
	settingMu.Lock()
	defer settingMu.Unlock()
	var s Settings
	b, err := os.ReadFile(filepath.Join(dataDir, "settings.json"))
	if err == nil {
		json.Unmarshal(b, &s)
	}
	return s
}

func saveSettings(s Settings) error {
	settingMu.Lock()
	defer settingMu.Unlock()
	b, _ := json.MarshalIndent(s, "", "  ")
	return os.WriteFile(filepath.Join(dataDir, "settings.json"), b, 0o644)
}

func isFile(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

// resolveAudiveris accepts either the program itself or the folder it is in.
func resolveAudiveris(p string) string {
	p = strings.Trim(strings.TrimSpace(p), `"`)
	if p == "" {
		return ""
	}
	if isFile(p) {
		return p
	}
	for _, c := range []string{"Audiveris.exe", "audiveris.exe", filepath.Join("bin", "Audiveris"), "Audiveris", "audiveris", filepath.Join("bin", "audiveris"),
		filepath.Join("Contents", "MacOS", "Audiveris"), filepath.Join("Audiveris.app", "Contents", "MacOS", "Audiveris")} {
		if q := filepath.Join(p, c); isFile(q) {
			return q
		}
	}
	return ""
}

func findAudiveris() string {
	if p := resolveAudiveris(loadSettings().AudiverisPath); p != "" {
		return p
	}
	if p := resolveAudiveris(os.Getenv("AUDIVERIS_EXE")); p != "" {
		return p
	}
	for _, n := range []string{"Audiveris", "audiveris", "Audiveris.exe"} {
		if p, err := exec.LookPath(n); err == nil {
			return p
		}
	}
	var candidates []string
	if runtime.GOOS == "windows" {
		for _, env := range []string{"ProgramFiles", "ProgramW6432", "ProgramFiles(x86)", "LOCALAPPDATA"} {
			root := os.Getenv(env)
			if root == "" {
				continue
			}
			if env == "LOCALAPPDATA" {
				root = filepath.Join(root, "Programs")
			}
			matches, _ := filepath.Glob(filepath.Join(root, "Audiveris*"))
			sort.Sort(sort.Reverse(sort.StringSlice(matches)))
			candidates = append(candidates, matches...)
		}
	} else {
		home, _ := os.UserHomeDir()
		candidates = []string{"/opt/audiveris", "/usr/share/audiveris", "/usr/lib/audiveris", "/opt/Audiveris",
			"/Applications/Audiveris.app", filepath.Join(home, "Applications", "Audiveris.app"),
			"/var/lib/flatpak/exports/bin/org.audiveris.audiveris", filepath.Join(home, ".local/share/flatpak/exports/bin/org.audiveris.audiveris")}
	}
	for _, c := range candidates {
		if p := resolveAudiveris(c); p != "" {
			return p
		}
	}
	return ""
}

func exeDir() string {
	p, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(p)
}

func tessdataDir() string {
	dirs := []string{filepath.Join(exeDir(), "tessdata"), filepath.Join(exeDir(), "..", "Resources", "tessdata"),
		filepath.Join(exeDir(), "..", "share", "score2audio", "tessdata"), filepath.Join(dataDir, "tessdata")}
	if runtime.GOOS != "windows" {
		dirs = append(dirs, "/usr/share/score2audio/tessdata", "/usr/share/tessdata", "/usr/share/tesseract-ocr/5/tessdata",
			"/usr/share/tesseract-ocr/4.00/tessdata", "/usr/local/share/tessdata", "/opt/homebrew/share/tessdata")
	}
	for _, d := range dirs {
		if isFile(filepath.Join(d, "eng.traineddata")) {
			return d
		}
	}
	return ""
}

// ---------- API ----------

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(v)
}

func httpErr(w http.ResponseWriter, code int, msg string) {
	http.Error(w, msg, code)
}

var idRe = regexp.MustCompile(`^[a-z0-9]{8,40}$`)

func newID() string {
	b := make([]byte, 6)
	rand.Read(b)
	return time.Now().Format("20060102150405") + hex.EncodeToString(b)
}

func apiHandler(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/"), "/"), "/")
	switch {
	case parts[0] == "status" && r.Method == http.MethodGet:
		aud := findAudiveris()
		writeJSON(w, map[string]interface{}{
			"version":        appVersion,
			"audiveris":      aud,
			"tessdata":       tessdataDir() != "",
			"os":             runtime.GOOS,
			"savedAudiveris": loadSettings().AudiverisPath,
		})
	case parts[0] == "settings" && r.Method == http.MethodPost:
		var s Settings
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&s); err != nil {
			httpErr(w, 400, "Could not read the settings.")
			return
		}
		if strings.TrimSpace(s.AudiverisPath) != "" {
			p := resolveAudiveris(s.AudiverisPath)
			if p == "" {
				httpErr(w, 400, "No Audiveris program was found at that location. Enter the full path of Audiveris.exe or of the folder that contains it.")
				return
			}
			s.AudiverisPath = p
		}
		if err := saveSettings(s); err != nil {
			httpErr(w, 500, "Could not save the settings: "+err.Error())
			return
		}
		writeJSON(w, map[string]string{"audiveris": findAudiveris()})
	case parts[0] == "library" && r.Method == http.MethodGet:
		writeJSON(w, listScores())
	case parts[0] == "upload" && r.Method == http.MethodPost:
		handleUpload(w, r)
	case parts[0] == "jobs" && len(parts) == 2:
		jobsMu.Lock()
		j, ok := jobs[parts[1]]
		var copyJob Job
		if ok {
			copyJob = *j
			copyJob.Elapsed = int(time.Since(j.Started).Seconds())
		}
		jobsMu.Unlock()
		if !ok {
			httpErr(w, 404, "That reading job was not found.")
			return
		}
		writeJSON(w, copyJob)
	case parts[0] == "scores" && len(parts) >= 2 && idRe.MatchString(parts[1]):
		handleScore(w, r, parts[1], parts[2:])
	case parts[0] == "quit" && r.Method == http.MethodPost:
		writeJSON(w, map[string]bool{"ok": true})
		go func() {
			time.Sleep(300 * time.Millisecond)
			fmt.Println("score2audio was closed from the web page.")
			os.Exit(0)
		}()
	default:
		httpErr(w, 404, "Unknown request.")
	}
}

func scoreDir(id string) string { return filepath.Join(dataDir, "scores", id) }

func readMeta(id string) (*ScoreMeta, error) {
	b, err := os.ReadFile(filepath.Join(scoreDir(id), "meta.json"))
	if err != nil {
		return nil, err
	}
	var m ScoreMeta
	err = json.Unmarshal(b, &m)
	return &m, err
}

func writeMeta(m *ScoreMeta) error {
	b, _ := json.MarshalIndent(m, "", "  ")
	return os.WriteFile(filepath.Join(scoreDir(m.ID), "meta.json"), b, 0o644)
}

func listScores() []ScoreMeta {
	entries, _ := os.ReadDir(filepath.Join(dataDir, "scores"))
	out := []ScoreMeta{}
	for _, e := range entries {
		if m, err := readMeta(e.Name()); err == nil && m.Ready {
			out = append(out, *m)
		}
	}
	sort.Slice(out, func(a, b int) bool { return out[a].Created.After(out[b].Created) })
	return out
}

func handleUpload(w http.ResponseWriter, r *http.Request) {
	name := filepath.Base(strings.ReplaceAll(r.URL.Query().Get("name"), `\`, "/"))
	ext := strings.ToLower(filepath.Ext(name))
	kind := ""
	switch ext {
	case ".pdf":
		kind = "pdf"
	case ".mxl", ".musicxml", ".xml":
		kind = "musicxml"
	default:
		httpErr(w, 400, "Please choose a PDF file, or a MusicXML file ending in .mxl, .musicxml or .xml.")
		return
	}
	id := newID()
	dir := scoreDir(id)
	os.MkdirAll(dir, 0o755)
	src := filepath.Join(dir, "source"+ext)
	f, err := os.Create(src)
	if err != nil {
		httpErr(w, 500, "Could not save the file: "+err.Error())
		return
	}
	n, err := io.Copy(f, io.LimitReader(r.Body, 300<<20))
	f.Close()
	if err != nil || n == 0 {
		os.RemoveAll(dir)
		httpErr(w, 400, "The file could not be received, or it was empty.")
		return
	}
	title := strings.TrimSuffix(name, filepath.Ext(name))
	title = strings.TrimSpace(regexp.MustCompile(`[_]+`).ReplaceAllString(title, " "))
	meta := &ScoreMeta{ID: id, Title: title, FileName: name, Kind: kind, Created: time.Now()}
	writeMeta(meta)

	if kind == "musicxml" {
		if _, err := buildAndStore(meta, []string{src}); err != nil {
			os.RemoveAll(dir)
			httpErr(w, 400, "That MusicXML file could not be read: "+err.Error())
			return
		}
		writeJSON(w, map[string]string{"scoreId": id})
		return
	}
	job, err := startOMR(meta)
	if err != nil {
		httpErr(w, 400, err.Error())
		return
	}
	writeJSON(w, map[string]string{"scoreId": id, "jobId": job.ID})
}

func buildAndStore(meta *ScoreMeta, files []string) (*Model, error) {
	m, err := LoadModel(files, meta.Title)
	if err != nil {
		return nil, err
	}
	if len(meta.SkippedPages) > 0 {
		var ps []string
		for _, p := range meta.SkippedPages {
			ps = append(ps, strconv.Itoa(p))
		}
		word := "Page"
		if len(ps) > 1 {
			word = "Pages"
		}
		m.Warnings = append(m.Warnings, fmt.Sprintf("%s %s of the PDF had no music that could be read and %s left out. This is normal for title pages and text pages; if a page of music is missing, try a cleaner copy of the PDF.",
			word, strings.Join(ps, ", "), map[bool]string{true: "were", false: "was"}[len(ps) > 1]))
	}
	// Audiveris rarely finds a real title; prefer the file name unless the score has one.
	if m.Title == "" || strings.EqualFold(m.Title, "source") {
		m.Title = meta.Title
	}
	meta.Title = m.Title
	meta.Ready = true
	meta.Measures = len(m.Measures)
	b, _ := json.Marshal(m)
	if err := os.WriteFile(filepath.Join(scoreDir(meta.ID), "model.json"), b, 0o644); err != nil {
		return nil, err
	}
	return m, writeMeta(meta)
}

func startOMR(meta *ScoreMeta) (*Job, error) {
	aud := findAudiveris()
	if aud == "" {
		return nil, errors.New("The music reading program, Audiveris, was not found. See the Setup section on this page.")
	}
	job := &Job{ID: newID(), ScoreID: meta.ID, State: "running", Message: "Starting the music reader", Started: time.Now()}
	jobsMu.Lock()
	jobs[job.ID] = job
	jobsMu.Unlock()
	go runOMR(job, meta, aud)
	return job, nil
}

var (
	sheetsRe = regexp.MustCompile(`(\d+) sheets? in`)
	sheetRe  = regexp.MustCompile(`\[[^\]#]*#(\d+)\]`)
)

var failedSheetRe = regexp.MustCompile(`\[[^\]#]*#(\d+)\].*(Error processing stub|No system found|StepException)`)

// runAudiveris runs Audiveris once, turning its log into progress messages.
func runAudiveris(job *Job, aud, dir string, args []string, set func(func(j *Job))) error {
	cmd := exec.Command(aud, args...)
	cmd.Dir = dir
	cmd.Env = audiverisEnv(os.Environ(), tessdataDir())
	hideWindow(cmd)
	pr, pw := io.Pipe()
	cmd.Stdout = pw
	cmd.Stderr = pw
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("Audiveris could not be started: %v", err)
	}
	scanned := make(chan struct{})
	go func() {
		defer close(scanned)
		sc := bufio.NewScanner(pr)
		sc.Buffer(make([]byte, 64*1024), 1024*1024)
		for sc.Scan() {
			line := sc.Text()
			set(func(j *Job) {
				if len(j.LogLines) > 2000 {
					j.LogLines = j.LogLines[1000:]
				}
				j.LogLines = append(j.LogLines, line)
				if m := sheetsRe.FindStringSubmatch(line); m != nil {
					j.Pages, _ = strconv.Atoi(m[1])
				}
				if m := sheetRe.FindStringSubmatch(line); m != nil {
					if p, _ := strconv.Atoi(m[1]); p > j.Page {
						j.Page = p
					}
				}
				if m := failedSheetRe.FindStringSubmatch(line); m != nil {
					p, _ := strconv.Atoi(m[1])
					j.failed = appendUnique(j.failed, p)
				}
				if j.Pages > 0 && j.Page > 0 {
					j.Message = fmt.Sprintf("Reading page %d of %d", j.Page, j.Pages)
				} else {
					j.Message = "Reading the music"
				}
			})
		}
		io.Copy(io.Discard, pr)
	}()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	var err error
	select {
	case err = <-done:
	case <-time.After(45 * time.Minute):
		cmd.Process.Kill()
		err = errors.New("it took longer than 45 minutes")
	}
	pw.Close()
	<-scanned
	return err
}

// audiverisEnv prepares the environment Audiveris runs in.
//
// Java is told to run without a screen ("headless"). Audiveris needs no
// window when score2audio drives it, and with a screen Java tries to load
// any assistive technology named in the user's .accessibility.properties.
// Screen reader users on Windows usually have the Java Access Bridge
// switched on there, which the Java bundled with Audiveris cannot load, so
// Audiveris would stop with "Assistive Technology not found".
func audiverisEnv(base []string, tessdata string) []string {
	var env []string
	toolOpts := ""
	for _, kv := range base {
		k := kv
		if i := strings.IndexByte(kv, '='); i >= 0 {
			k = kv[:i]
		}
		switch strings.ToUpper(k) {
		case "JAVA_TOOL_OPTIONS":
			toolOpts = kv[len(k)+1:]
			continue
		case "TESSDATA_PREFIX":
			if tessdata != "" {
				continue
			}
		}
		env = append(env, kv)
	}
	env = append(env, "JAVA_TOOL_OPTIONS="+strings.TrimSpace(toolOpts+" -Djava.awt.headless=true"))
	if tessdata != "" {
		env = append(env, "TESSDATA_PREFIX="+tessdata)
	}
	return env
}

func appendUnique(list []int, v int) []int {
	for _, x := range list {
		if x == v {
			return list
		}
	}
	return append(list, v)
}

func runOMR(job *Job, meta *ScoreMeta, aud string) {
	dir := scoreDir(meta.ID)
	out := filepath.Join(dir, "omr")
	os.RemoveAll(out)
	os.MkdirAll(out, 0o755)
	src := filepath.Join(dir, "source.pdf")
	set := func(fn func(j *Job)) {
		jobsMu.Lock()
		fn(job)
		jobsMu.Unlock()
	}
	err := runAudiveris(job, aud, dir, []string{"-batch", "-export", "-output", out, "--", src}, set)
	files := mxlFiles(out)

	jobsMu.Lock()
	failed := append([]int(nil), job.failed...)
	pages := job.Pages
	jobsMu.Unlock()
	sort.Ints(failed)

	// A page without readable music (a title page, notes, a blank page)
	// stops Audiveris from exporting anything. Export again from its saved
	// work without those pages.
	if len(files) == 0 && len(failed) > 0 && pages > len(failed) {
		books, _ := filepath.Glob(filepath.Join(out, "*.omr"))
		if len(books) > 0 {
			var good []string
			for p := 1; p <= pages; p++ {
				if !containsInt(failed, p) {
					good = append(good, strconv.Itoa(p))
				}
			}
			set(func(j *Job) { j.Message = "Some pages had no music that could be read. Finishing the other pages" })
			args := append([]string{"-batch", "-export", "-sheets"}, good...)
			args = append(args, "-output", out, "--", books[0])
			err = runAudiveris(job, aud, dir, args, set)
			files = mxlFiles(out)
		}
	}
	logPath := filepath.Join(dir, "audiveris-log.txt")
	jobsMu.Lock()
	logText := fmt.Sprintf("score2audio %s on %s/%s\nAudiveris: %s\nLyrics data: %s\n\n%s\n", appVersion, runtime.GOOS, runtime.GOARCH, aud, tessdataDir(), strings.Join(job.LogLines, "\n"))
	lineCount := len(job.LogLines)
	details := importantLines(job.LogLines)
	jobsMu.Unlock()
	os.WriteFile(logPath, []byte(logText), 0o644)
	if len(files) == 0 {
		msg := "Audiveris could not read music from this PDF."
		if err != nil {
			msg += " (" + err.Error() + ")"
		}
		if lineCount == 0 {
			msg += " Audiveris did not report anything. This usually means the version of Audiveris installed is the one without a console; on Windows, install the file ending in windowsConsole-x86_64.msi instead."
		}
		set(func(j *Job) { j.State, j.Message, j.Details, j.LogPath = "error", msg, details, logPath })
		return
	}
	meta.SkippedPages = failed
	set(func(j *Job) { j.Message = "Preparing the lines" })
	if _, perr := buildAndStore(meta, files); perr != nil {
		set(func(j *Job) { j.State, j.Message = "error", "The recognised music could not be read: "+perr.Error() })
		return
	}
	set(func(j *Job) { j.State, j.Message = "done", "Finished reading the score" })
}

var importantRe = regexp.MustCompile(`(?i)\b(WARN|ERROR|SEVERE|FATAL)\b|exception|error:|could not|cannot|unable|out of memory|denied|not found`)

// importantLines picks the lines of an Audiveris log that explain a failure.
func importantLines(lines []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, l := range lines {
		t := strings.TrimSpace(l)
		if t == "" || strings.HasPrefix(t, "at ") || !importantRe.MatchString(t) {
			continue
		}
		if len(t) > 300 {
			t = t[:300] + "..."
		}
		if !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	if len(out) == 0 && len(lines) > 0 {
		start := len(lines) - 15
		if start < 0 {
			start = 0
		}
		out = append(out, lines[start:]...)
	}
	if len(out) > 25 {
		out = append(out[:5], out[len(out)-20:]...)
	}
	return out
}

func containsInt(list []int, v int) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func handleScore(w http.ResponseWriter, r *http.Request, id string, rest []string) {
	meta, err := readMeta(id)
	if err != nil {
		httpErr(w, 404, "That score was not found.")
		return
	}
	dir := scoreDir(id)
	action := ""
	if len(rest) > 0 {
		action = rest[0]
	}
	switch {
	case action == "" && r.Method == http.MethodGet:
		b, err := os.ReadFile(filepath.Join(dir, "model.json"))
		var m Model
		if err == nil {
			err = json.Unmarshal(b, &m)
		}
		if err != nil || m.Version != modelVersion {
			// rebuild from the stored MusicXML (newer program version)
			files := mxlFiles(filepath.Join(dir, "omr"))
			for _, ext := range []string{".mxl", ".musicxml", ".xml"} {
				if isFile(filepath.Join(dir, "source"+ext)) {
					files = []string{filepath.Join(dir, "source"+ext)}
				}
			}
			mm, err := buildAndStore(meta, files)
			if err != nil {
				httpErr(w, 500, "Could not open the score: "+err.Error())
				return
			}
			m = *mm
		}
		prefs := json.RawMessage("null")
		if p, err := os.ReadFile(filepath.Join(dir, "prefs.json")); err == nil && json.Valid(p) {
			prefs = p
		}
		writeJSON(w, map[string]interface{}{"id": id, "meta": meta, "model": m, "prefs": prefs})
	case action == "prefs" && r.Method == http.MethodPut:
		b, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil || !json.Valid(b) {
			httpErr(w, 400, "Could not save your settings for this score.")
			return
		}
		os.WriteFile(filepath.Join(dir, "prefs.json"), b, 0o644)
		writeJSON(w, map[string]bool{"ok": true})
	case action == "rename" && r.Method == http.MethodPost:
		var body struct {
			Title string `json:"title"`
		}
		json.NewDecoder(io.LimitReader(r.Body, 1<<14)).Decode(&body)
		t := strings.TrimSpace(body.Title)
		if t == "" {
			httpErr(w, 400, "The title cannot be empty.")
			return
		}
		meta.Title = t
		writeMeta(meta)
		writeJSON(w, meta)
	case action == "reread" && r.Method == http.MethodPost:
		if meta.Kind != "pdf" {
			httpErr(w, 400, "Only PDF scores can be read again.")
			return
		}
		job, err := startOMR(meta)
		if err != nil {
			httpErr(w, 400, err.Error())
			return
		}
		writeJSON(w, map[string]string{"scoreId": id, "jobId": job.ID})
	case action == "musicxml" && r.Method == http.MethodGet:
		files := mxlFiles(filepath.Join(dir, "omr"))
		for _, ext := range []string{".mxl", ".musicxml", ".xml"} {
			if isFile(filepath.Join(dir, "source"+ext)) {
				files = []string{filepath.Join(dir, "source"+ext)}
			}
		}
		if len(files) == 0 {
			httpErr(w, 404, "No MusicXML is stored for this score.")
			return
		}
		safe := regexp.MustCompile(`[^\w\- ]+`).ReplaceAllString(meta.Title, "")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s%s"`, safe, filepath.Ext(files[0])))
		http.ServeFile(w, r, files[0])
	case action == "" && r.Method == http.MethodDelete:
		os.RemoveAll(dir)
		writeJSON(w, map[string]bool{"ok": true})
	default:
		httpErr(w, 404, "Unknown request.")
	}
}

// defaultDataDir is where the score library is kept: the local application
// data folder on Windows, Application Support on a Mac, and
// ~/.local/share on Linux.
func defaultDataDir() string {
	home, _ := os.UserHomeDir()
	var dir, old string
	switch runtime.GOOS {
	case "windows":
		base, err := os.UserCacheDir() // %LOCALAPPDATA%
		if err != nil {
			base = home
		}
		dir, old = filepath.Join(base, "score2audio"), filepath.Join(base, "ScoreReader")
	case "darwin":
		dir = filepath.Join(home, "Library", "Application Support", "score2audio")
		old = filepath.Join(home, "Library", "Caches", "ScoreReader")
	default:
		base := os.Getenv("XDG_DATA_HOME")
		if base == "" {
			base = filepath.Join(home, ".local", "share")
		}
		dir = filepath.Join(base, "score2audio")
		old = filepath.Join(home, ".cache", "ScoreReader")
	}
	// keep scores opened with the earlier test version (called Score Reader)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if st, err := os.Stat(old); err == nil && st.IsDir() {
			os.MkdirAll(filepath.Dir(dir), 0o755)
			os.Rename(old, dir)
		}
	}
	return dir
}
