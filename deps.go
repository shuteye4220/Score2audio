package main

// The helper programs score2audio uses, and installing them for the user.
//
// Audiveris (needed to read PDFs) and FFmpeg (optional: MP3 files, and AAC
// sound in videos made by browsers that only make Opus) are downloaded from
// fixed addresses and checked against the size and SHA-256 recorded here
// before anything is run or unpacked, so a changed or tampered download is
// never used. "score2audio --setup" does this from a terminal or from the
// Windows installer; the Setup section of the page does it on Windows and
// Mac. On Linux, the distribution's own packages are used where it has them.

import (
	"archive/zip"
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

type pinnedFile struct {
	what   string // for messages: "Audiveris 5.11.0"
	url    string
	size   int64
	sha256 string
}

func (p pinnedFile) name() string { return p.url[strings.LastIndex(p.url, "/")+1:] }

const audiverisRelease = "https://github.com/Audiveris/audiveris/releases/download/5.11.0/"

var (
	audiverisWindows = pinnedFile{"Audiveris 5.11.0", audiverisRelease + "Audiveris-5.11.0-windowsConsole-x86_64.msi", 85062430,
		"5f1b4e96a12c53c7da426814b76e599363c4181e291855996e0a6878dda95f71"}
	audiverisMacARM = pinnedFile{"Audiveris 5.11.0", audiverisRelease + "Audiveris-5.11.0-macosx-arm64.dmg", 84929243,
		"17491af8b6d40153b031dd1f0815e37213f0999ef23f10586af46706e59b2eb6"}
	audiverisMacIntel = pinnedFile{"Audiveris 5.11.0", audiverisRelease + "Audiveris-5.11.0-macosx-x86_64.dmg", 87065977,
		"11794424c6f1698617836a77d2c5818c46405a3fe7388281c623c4e383fa33cb"}
	audiverisUbuntu24 = pinnedFile{"Audiveris 5.11.0", audiverisRelease + "Audiveris-5.11.0-ubuntu24.04-x86_64.deb", 77716868,
		"f20113aaa33b3149ec8d6a09b2a7963360e65fafd92d69389987a85bbc3ec7a3"}
	audiverisUbuntu22 = pinnedFile{"Audiveris 5.11.0", audiverisRelease + "Audiveris-5.11.0-ubuntu22.04-x86_64.deb", 78048872,
		"ae714594f40e54b1a4951fc3f914f08ae38fe5d07b7f2283b1a904fdb6e0a318"}
	// FFmpeg for Windows, the "essentials" build from gyan.dev (GPL)
	ffmpegWindows = pinnedFile{"FFmpeg 8.1.2", "https://github.com/GyanD/codexffmpeg/releases/download/8.1.2/ffmpeg-8.1.2-essentials_build.zip", 109728040,
		"db580001caa24ac104c8cb856cd113a87b0a443f7bdf47d8c12b1d740584a2ec"}
)

// ---------- finding FFmpeg ----------

func exeName(n string) string {
	if runtime.GOOS == "windows" {
		return n + ".exe"
	}
	return n
}

func findFFmpeg() string {
	cands := []string{filepath.Join(exeDir(), "ffmpeg", exeName("ffmpeg")), filepath.Join(exeDir(), exeName("ffmpeg")),
		filepath.Join(dataDir, "ffmpeg", exeName("ffmpeg"))}
	for _, c := range cands {
		if isFile(c) {
			return c
		}
	}
	if p, err := exec.LookPath("ffmpeg"); err == nil {
		return p
	}
	for _, c := range []string{"/opt/homebrew/bin/ffmpeg", "/usr/local/bin/ffmpeg", "/usr/bin/ffmpeg"} {
		if runtime.GOOS != "windows" && isFile(c) {
			return c
		}
	}
	return ""
}

// ---------- downloading ----------

var downloadClient = &http.Client{Transport: &http.Transport{
	Proxy: http.ProxyFromEnvironment, TLSHandshakeTimeout: 30 * time.Second, ResponseHeaderTimeout: 60 * time.Second,
	IdleConnTimeout: 30 * time.Second,
}}

// download fetches a pinned file into dir and checks it. A file already
// there that checks out is used as it is.
func download(ctx context.Context, p pinnedFile, dir string, say func(string)) (string, error) {
	path := filepath.Join(dir, p.name())
	if checkFile(path, p) == nil {
		return path, nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "score2audio/"+appVersion)
	resp, err := downloadClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("could not download %s: %v", p.what, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("could not download %s: the server answered %s", p.what, resp.Status)
	}
	tmp := path + ".download"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	mb := (p.size + 1<<19) >> 20
	var done int64
	lastTenth := -1
	buf := make([]byte, 256<<10)
	body := io.LimitReader(resp.Body, p.size+1)
	for {
		n, rerr := body.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				f.Close()
				os.Remove(tmp)
				return "", werr
			}
			h.Write(buf[:n])
			done += int64(n)
			if t := int(done * 10 / p.size); t != lastTenth && t < 10 {
				lastTenth = t
				say(fmt.Sprintf("Downloading %s (%d MB): %d percent", p.what, mb, t*10))
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			f.Close()
			os.Remove(tmp)
			return "", fmt.Errorf("the download of %s stopped: %v", p.what, rerr)
		}
	}
	f.Close()
	if done != p.size || hex.EncodeToString(h.Sum(nil)) != p.sha256 {
		os.Remove(tmp)
		return "", fmt.Errorf("the download of %s was not the expected file, so it was not used", p.what)
	}
	if err := os.Rename(tmp, path); err != nil {
		return "", err
	}
	say(fmt.Sprintf("Downloaded %s and checked that it is the expected file.", p.what))
	return path, nil
}

func checkFile(path string, p pinnedFile) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || st.Size() != p.size {
		return errors.New("wrong size")
	}
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	if hex.EncodeToString(h.Sum(nil)) != p.sha256 {
		return errors.New("wrong checksum")
	}
	return nil
}

// ---------- setting up ----------

type setupOptions struct {
	ask      func(question string) bool // nil: never run commands that need a password
	terminal bool                       // running in a terminal, so sudo can ask for a password
}

// runSetup installs whatever is missing. It reports each step through say
// and returns false if Audiveris is still missing at the end.
func runSetup(ctx context.Context, o setupOptions, say func(string)) bool {
	dl := filepath.Join(dataDir, "downloads")
	say("Checking the helper programs score2audio uses.")

	if p := findAudiveris(); p != "" {
		say("Audiveris, which reads the music in PDF files, is installed: " + p)
	} else {
		say("Audiveris, which reads the music in PDF files, is not installed yet.")
		if err := installAudiveris(ctx, o, dl, say); err != nil {
			say("Audiveris was not installed: " + err.Error())
		}
		if p := findAudiveris(); p != "" {
			say("Audiveris is now installed: " + p)
		}
	}

	if p := findFFmpeg(); p != "" {
		say("FFmpeg, for MP3 files and video sound, is installed: " + p)
	} else {
		say("FFmpeg, for MP3 files and video sound, is not installed yet. It is optional: everything else works without it.")
		if err := installFFmpeg(ctx, o, dl, say); err != nil {
			say("FFmpeg was not installed: " + err.Error())
		}
		if p := findFFmpeg(); p != "" {
			say("FFmpeg is now installed: " + p)
		}
	}
	// the installers are not needed any more
	os.RemoveAll(dl)

	ok := findAudiveris() != ""
	if ok {
		say("Setup is finished. score2audio is ready to read PDF files.")
	} else {
		say("Setup is finished, but Audiveris is still missing, so PDF files cannot be read yet. MusicXML and score2audio files can still be opened. The Setup section of score2audio's page explains how to install Audiveris yourself.")
	}
	return ok
}

func installAudiveris(ctx context.Context, o setupOptions, dl string, say func(string)) error {
	switch runtime.GOOS {
	case "windows":
		if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
			return errors.New("Audiveris is only made for 64-bit Windows")
		}
		path, err := download(ctx, audiverisWindows, dl, say)
		if err != nil {
			return err
		}
		say("Installing Audiveris. Windows may ask whether to allow the Audiveris installer to make changes; answer Yes.")
		root := os.Getenv("SystemRoot")
		if !filepath.IsAbs(root) {
			root = `C:\Windows`
		}
		msi := filepath.Join(root, "System32", "msiexec.exe")
		cmd := exec.CommandContext(ctx, msi, "/i", path, "/passive", "/norestart")
		err = cmd.Run()
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			switch ee.ExitCode() {
			case 3010, 1641:
				say("Audiveris is installed; Windows says it should be restarted at some point.")
				return nil
			case 1602, 1223:
				return errors.New("the installation was cancelled")
			}
			return fmt.Errorf("the Audiveris installer stopped with code %d", ee.ExitCode())
		}
		return err
	case "darwin":
		p := audiverisMacARM
		if runtime.GOARCH == "amd64" {
			p = audiverisMacIntel
		}
		path, err := download(ctx, p, dl, say)
		if err != nil {
			return err
		}
		return installMacApp(ctx, path, say)
	case "linux":
		return installLinux(ctx, o, dl, "Audiveris", say)
	}
	return errors.New("score2audio cannot install Audiveris on this system; see audiveris.org")
}

// installMacApp copies Audiveris.app from a disk image into ~/Applications.
func installMacApp(ctx context.Context, dmg string, say func(string)) error {
	mnt, err := os.MkdirTemp("", "score2audio-dmg")
	if err != nil {
		return err
	}
	defer os.Remove(mnt)
	if out, err := exec.CommandContext(ctx, "hdiutil", "attach", "-nobrowse", "-readonly", "-noautoopen", "-mountpoint", mnt, dmg).CombinedOutput(); err != nil {
		return fmt.Errorf("the Audiveris disk image could not be opened: %s", strings.TrimSpace(string(out)))
	}
	defer exec.Command("hdiutil", "detach", "-quiet", mnt).Run()
	apps, _ := filepath.Glob(filepath.Join(mnt, "*.app"))
	if len(apps) == 0 {
		return errors.New("the Audiveris disk image has no app in it")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dest := filepath.Join(home, "Applications", "Audiveris.app")
	os.MkdirAll(filepath.Dir(dest), 0o755)
	os.RemoveAll(dest)
	say("Copying Audiveris into the Applications folder in your home folder.")
	if out, err := exec.CommandContext(ctx, "ditto", apps[0], dest).CombinedOutput(); err != nil {
		return fmt.Errorf("Audiveris could not be copied: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

func installFFmpeg(ctx context.Context, o setupOptions, dl string, say func(string)) error {
	switch runtime.GOOS {
	case "windows":
		if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
			return errors.New("this FFmpeg build is for 64-bit Windows")
		}
		path, err := download(ctx, ffmpegWindows, dl, say)
		if err != nil {
			return err
		}
		return unpackFFmpeg(path, []string{filepath.Join(exeDir(), "ffmpeg"), filepath.Join(dataDir, "ffmpeg")}, say)
	case "darwin":
		for _, brew := range []string{"/opt/homebrew/bin/brew", "/usr/local/bin/brew"} {
			if isFile(brew) {
				say("Installing FFmpeg with Homebrew. This can take a few minutes.")
				cmd := exec.CommandContext(ctx, brew, "install", "ffmpeg")
				return runSaying(cmd, say)
			}
		}
		return errors.New("Homebrew (brew.sh) is not installed. FFmpeg is optional; to add it later, install Homebrew and then run: brew install ffmpeg")
	case "linux":
		return installLinux(ctx, o, dl, "FFmpeg", say)
	}
	return errors.New("score2audio cannot install FFmpeg on this system")
}

// unpackFFmpeg takes ffmpeg.exe and its licence out of the FFmpeg zip, into
// the first of dests that can be written to: an ffmpeg folder next to
// score2audio, or else in its data folder.
func unpackFFmpeg(zipPath string, dests []string, say func(string)) error {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer zr.Close()
	want := map[string]string{"bin/ffmpeg.exe": "ffmpeg.exe", "LICENSE": "LICENSE.txt", "README.txt": "README.txt"}
	dest := ""
	for _, d := range dests {
		if os.MkdirAll(d, 0o755) == nil && writable(d) {
			dest = d
			break
		}
	}
	if dest == "" {
		return errors.New("there is no folder FFmpeg can be put in")
	}
	found := 0
	for _, f := range zr.File {
		// entries are "ffmpeg-8.1.2-essentials_build/bin/ffmpeg.exe" and so on
		i := strings.IndexByte(f.Name, '/')
		if i < 0 {
			continue
		}
		out, ok := want[f.Name[i+1:]]
		if !ok || f.UncompressedSize64 > 300<<20 {
			continue
		}
		if err := extractTo(f, filepath.Join(dest, out), 0o755, 300<<20); err != nil {
			return err
		}
		found++
	}
	if found < len(want) {
		return errors.New("the FFmpeg download did not contain the expected files")
	}
	say("FFmpeg was put in " + dest)
	return nil
}

func extractTo(f *zip.File, path string, mode os.FileMode, limit int64) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	tmp := path + ".new"
	w, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	n, err := io.Copy(w, io.LimitReader(rc, limit+1))
	if cerr := w.Close(); err == nil {
		err = cerr
	}
	if err == nil && n > limit {
		err = errors.New("a file in the download is larger than expected")
	}
	if err != nil {
		os.Remove(tmp)
		return err
	}
	return replaceFile(tmp, path)
}

// replaceFile moves a new file into place. On Windows a running program
// cannot be replaced, but it can be renamed out of the way.
func replaceFile(tmp, path string) error {
	if err := os.Rename(tmp, path); err == nil {
		return nil
	}
	old := path + ".old"
	os.Remove(old)
	if err := os.Rename(path, old); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Rename(old, path)
		return err
	}
	return nil
}

func writable(dir string) bool {
	f, err := os.CreateTemp(dir, ".write-test-*")
	if err != nil {
		return false
	}
	name := f.Name()
	f.Close()
	os.Remove(name)
	return true
}

// runSaying runs a command and passes on what it prints, line by line.
func runSaying(cmd *exec.Cmd, say func(string)) error {
	pr, pw := io.Pipe()
	cmd.Stdout, cmd.Stderr = pw, pw
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		sc := bufio.NewScanner(pr)
		sc.Buffer(make([]byte, 64<<10), 1<<20)
		for sc.Scan() {
			if t := strings.TrimSpace(sc.Text()); t != "" {
				say(t)
			}
		}
		io.Copy(io.Discard, pr)
	}()
	err := cmd.Run()
	pw.Close()
	wg.Wait()
	return err
}

// ---------- Linux ----------

type osRelease struct{ id, like, version string }

func readOSRelease() osRelease {
	var r osRelease
	b, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return r
	}
	for _, line := range strings.Split(string(b), "\n") {
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		v = strings.Trim(strings.TrimSpace(v), `"'`)
		switch k {
		case "ID":
			r.id = v
		case "ID_LIKE":
			r.like = v
		case "VERSION_ID":
			r.version = v
		}
	}
	return r
}

func (r osRelease) is(names ...string) bool {
	all := " " + r.id + " " + r.like + " "
	for _, n := range names {
		if strings.Contains(all, " "+n+" ") {
			return true
		}
	}
	return false
}

// linuxPlan is how to install a program on this distribution: a command to
// run (which may need sudo), or a file to download first.
type linuxPlan struct {
	download *pinnedFile
	cmd      []string // {download} stands for the downloaded file
	sudo     bool
	manual   string // when there is no command: what to tell the user
}

func linuxPlanFor(r osRelease, what string, has func(string) bool) linuxPlan {
	if what == "FFmpeg" {
		switch {
		case has("pacman"):
			return linuxPlan{cmd: []string{"pacman", "-S", "--needed", "ffmpeg"}, sudo: true}
		case has("apt-get"):
			return linuxPlan{cmd: []string{"apt-get", "install", "-y", "ffmpeg"}, sudo: true}
		case has("dnf"):
			return linuxPlan{cmd: []string{"dnf", "install", "-y", "ffmpeg-free"}, sudo: true}
		case has("zypper"):
			return linuxPlan{cmd: []string{"zypper", "install", "-y", "ffmpeg"}, sudo: true}
		}
		return linuxPlan{manual: "Install the ffmpeg package with your distribution's package manager."}
	}
	switch {
	case r.is("arch", "manjaro", "endeavouros", "cachyos", "garuda"):
		for _, h := range []string{"paru", "yay"} {
			if has(h) {
				return linuxPlan{cmd: []string{h, "-S", "--needed", "audiveris"}}
			}
		}
		return linuxPlan{manual: "Install the audiveris package from the Arch User Repository (AUR), for example with an AUR helper such as yay or paru: yay -S audiveris"}
	case r.is("ubuntu", "debian") && runtime.GOARCH == "amd64" && has("apt-get"):
		p := audiverisUbuntu22
		if newerDistro(r) {
			p = audiverisUbuntu24
		}
		return linuxPlan{download: &p, cmd: []string{"apt-get", "install", "-y", "{download}"}, sudo: true}
	case has("flatpak"):
		return linuxPlan{cmd: []string{"flatpak", "install", "--user", "-y", "flathub", "org.audiveris.audiveris"}}
	}
	return linuxPlan{manual: "Install Audiveris from audiveris.org, or install Flatpak and run: flatpak install flathub org.audiveris.audiveris"}
}

// newerDistro: Ubuntu 24.04 or later, Debian 13 or later (and their
// relatives without a version number of their own), which need the
// Audiveris package built for Ubuntu 24.04.
func newerDistro(r osRelease) bool {
	var major, minor int
	fmt.Sscanf(r.version, "%d.%d", &major, &minor)
	switch {
	case r.id == "ubuntu":
		return major >= 24
	case r.id == "debian":
		return major >= 13 || r.version == ""
	case r.id == "linuxmint":
		return major >= 22
	case r.id == "pop":
		return major >= 24
	}
	return true // most other relatives follow recent Ubuntu
}

func installLinux(ctx context.Context, o setupOptions, dl, what string, say func(string)) error {
	has := func(n string) bool { _, err := exec.LookPath(n); return err == nil }
	plan := linuxPlanFor(readOSRelease(), what, has)
	if plan.manual != "" {
		return errors.New(plan.manual)
	}
	args := append([]string(nil), plan.cmd...)
	if plan.download != nil {
		path, err := download(ctx, *plan.download, dl, say)
		if err != nil {
			return err
		}
		os.Chmod(dl, 0o755) // apt reads the package as its own user
		os.Chmod(path, 0o644)
		for i, a := range args {
			if a == "{download}" {
				args[i] = path
			}
		}
	}
	if plan.sudo && os.Geteuid() != 0 {
		if !o.terminal || !has("sudo") {
			return fmt.Errorf("it needs to be installed as the administrator. In a terminal, run: sudo %s", strings.Join(args, " "))
		}
		args = append([]string{"sudo"}, args...)
	}
	if o.ask == nil || !o.ask(fmt.Sprintf("To install %s, score2audio will run: %s", what, strings.Join(args, " "))) {
		return fmt.Errorf("not installed. To install it yourself, run: %s", strings.Join(args, " "))
	}
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Stdin = os.Stdin
	if o.terminal {
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		return cmd.Run()
	}
	return runSaying(cmd, say)
}

// setupCommand is "score2audio --setup": installs what is missing, asking
// before running any command that changes the system (or not, with --yes).
func setupCommand(yes bool) int {
	in := bufio.NewReader(os.Stdin)
	o := setupOptions{terminal: true, ask: func(q string) bool {
		fmt.Println(q)
		if yes {
			return true
		}
		fmt.Print("Go ahead? Type y and press Enter for yes, or just press Enter for no: ")
		line, _ := in.ReadString('\n')
		return strings.HasPrefix(strings.ToLower(strings.TrimSpace(line)), "y")
	}}
	if runtime.GOOS != "linux" {
		o.ask = func(string) bool { return true } // the downloads are checked; nothing runs as root without Windows or macOS asking
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancel()
	ok := runSetup(ctx, o, func(s string) { fmt.Println(s) })
	if !ok {
		return 1
	}
	return 0
}

// ---------- setup from the page ----------

type setupState struct {
	Running bool     `json:"running"`
	Done    bool     `json:"done"`
	OK      bool     `json:"ok"`
	Lines   []string `json:"lines"`
}

var (
	setupMu  sync.Mutex
	setupNow setupState
)

func startSetupFromPage() error {
	if runtime.GOOS == "linux" {
		return errors.New("On Linux, the helper programs are installed with your distribution's package manager, which needs your password: in a terminal, run score2audio --setup")
	}
	setupMu.Lock()
	defer setupMu.Unlock()
	if setupNow.Running {
		return nil
	}
	setupNow = setupState{Running: true}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
		defer cancel()
		ok := runSetup(ctx, setupOptions{ask: func(string) bool { return true }}, func(s string) {
			setupMu.Lock()
			if len(setupNow.Lines) < 500 {
				setupNow.Lines = append(setupNow.Lines, s)
			}
			setupMu.Unlock()
		})
		setupMu.Lock()
		setupNow.Running, setupNow.Done, setupNow.OK = false, true, ok
		setupMu.Unlock()
	}()
	return nil
}

func setupProgress() setupState {
	setupMu.Lock()
	defer setupMu.Unlock()
	s := setupNow
	s.Lines = append([]string(nil), setupNow.Lines...)
	return s
}
