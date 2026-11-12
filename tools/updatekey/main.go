// Command updatekey makes the key pair that signs score2audio's update
// manifests, or signs a file with the private key.
//
//	go run ./tools/updatekey new PRIVATE-KEY-FILE   writes a new private key, prints its public key
//	go run ./tools/updatekey manifest VERSION DIR   writes DIR/latest.json, the list of a
//	                                                release's downloads with their SHA-256
//	go run ./tools/updatekey sign FILE              signs FILE with the key in $UPDATE_SIGNING_KEY,
//	                                                writing FILE.sig
//
// The private key goes into the repository's UPDATE_SIGNING_KEY secret; the
// public key into updatePublicKey in update.go.
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// downloads is which release file updates which kind of copy (see
// updateKind in update.go).
var downloads = map[string]string{
	"windows-installer": "score2audio-windows-setup.exe",
	"windows-zip":       "score2audio-windows.zip",
	"mac":               "score2audio-mac.zip",
	"linux-x86_64":      "score2audio-linux-x86_64.tar.gz",
	"linux-arm64":       "score2audio-linux-arm64.tar.gz",
}

type file struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

func manifest(version, dir string) error {
	m := struct {
		Version string          `json:"version"`
		Files   map[string]file `json:"files"`
	}{version, map[string]file{}}
	for key, name := range downloads {
		f, err := os.Open(filepath.Join(dir, name))
		if os.IsNotExist(err) {
			fmt.Fprintln(os.Stderr, "not in this release:", name)
			continue
		}
		if err != nil {
			return err
		}
		h := sha256.New()
		n, err := io.Copy(h, f)
		f.Close()
		if err != nil {
			return err
		}
		m.Files[key] = file{name, n, hex.EncodeToString(h.Sum(nil))}
	}
	b, _ := json.MarshalIndent(m, "", "  ")
	return os.WriteFile(filepath.Join(dir, "latest.json"), append(b, '\n'), 0o644)
}

func main() {
	if len(os.Args) == 4 && os.Args[1] == "manifest" {
		if err := manifest(os.Args[2], os.Args[3]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: updatekey new PRIVATE-KEY-FILE | updatekey manifest VERSION DIR | updatekey sign FILE")
		os.Exit(2)
	}
	switch os.Args[1] {
	case "new":
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			panic(err)
		}
		f, err := os.OpenFile(os.Args[2], os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Fprintln(f, base64.StdEncoding.EncodeToString(priv.Seed()))
		f.Close()
		fmt.Println(base64.StdEncoding.EncodeToString(pub))
	case "sign":
		seed, err := base64.StdEncoding.DecodeString(strings.TrimSpace(os.Getenv("UPDATE_SIGNING_KEY")))
		if err != nil || len(seed) != ed25519.SeedSize {
			fmt.Fprintln(os.Stderr, "UPDATE_SIGNING_KEY is not set to a private key made by 'updatekey new'.")
			os.Exit(1)
		}
		data, err := os.ReadFile(os.Args[2])
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		sig := ed25519.Sign(ed25519.NewKeyFromSeed(seed), data)
		if err := os.WriteFile(os.Args[2]+".sig", []byte(base64.StdEncoding.EncodeToString(sig)+"\n"), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	default:
		fmt.Fprintln(os.Stderr, "usage: updatekey new PRIVATE-KEY-FILE | updatekey sign FILE")
		os.Exit(2)
	}
}
