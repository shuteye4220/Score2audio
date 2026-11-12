package main

// Converting files the page made with FFmpeg, when it is installed: audio
// files to MP3, and videos whose sound is Opus (from browsers that cannot
// make AAC) to AAC sound, which every phone plays. The page sends the file,
// FFmpeg converts it in a temporary folder, and the result goes back.

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

const maxConvertUpload = 4 << 30 // a long video at high quality is well under this

type conversion struct {
	in, out, contentType string
	args                 []string // between the input and the output
}

var conversions = map[string]conversion{
	// variable bit rate, about 190 kilobits a second: as good as the WAV to the ear
	"mp3": {"in.wav", "out.mp3", "audio/mpeg", []string{"-vn", "-codec:a", "libmp3lame", "-q:a", "2"}},
	// the picture is copied as it is; only the sound is made again
	"video-aac": {"in.mp4", "out.mp4", "video/mp4", []string{"-map", "0:v:0", "-map", "0:a:0", "-c:v", "copy", "-c:a", "aac", "-b:a", "192k", "-movflags", "+faststart"}},
}

func convertArgs(c conversion, dir string) []string {
	args := []string{"-hide_banner", "-loglevel", "error", "-nostdin", "-y", "-i", filepath.Join(dir, c.in)}
	args = append(args, c.args...)
	return append(args, filepath.Join(dir, c.out))
}

var convertSlots = make(chan struct{}, 2) // at most two conversions at once

func handleConvert(w http.ResponseWriter, r *http.Request) {
	c, ok := conversions[r.URL.Query().Get("to")]
	if !ok {
		httpErr(w, 400, "Unknown conversion.")
		return
	}
	ff := findFFmpeg()
	if ff == "" {
		httpErr(w, 400, "FFmpeg is not installed. See the Setup section of this page.")
		return
	}
	select {
	case convertSlots <- struct{}{}:
		defer func() { <-convertSlots }()
	default:
		httpErr(w, 503, "Two files are already being converted. Try again when they are done.")
		return
	}
	dir, err := os.MkdirTemp("", "score2audio-convert")
	if err != nil {
		httpErr(w, 500, err.Error())
		return
	}
	defer os.RemoveAll(dir)
	f, err := os.OpenFile(filepath.Join(dir, c.in), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		httpErr(w, 500, err.Error())
		return
	}
	n, err := io.Copy(f, io.LimitReader(r.Body, maxConvertUpload+1))
	f.Close()
	if err != nil || n == 0 || n > maxConvertUpload {
		httpErr(w, 400, "The file to convert could not be received.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, ff, convertArgs(c, dir)...)
	hideWindow(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := string(out)
		if len(msg) > 300 {
			msg = msg[:300]
		}
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			msg = "it took too long"
		}
		httpErr(w, 500, "FFmpeg could not convert the file: "+msg)
		return
	}
	res, err := os.Open(filepath.Join(dir, c.out))
	if err != nil {
		httpErr(w, 500, "FFmpeg did not make a file.")
		return
	}
	defer res.Close()
	st, _ := res.Stat()
	w.Header().Set("Content-Type", c.contentType)
	http.ServeContent(w, r, "", st.ModTime(), res)
}
