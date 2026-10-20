package docker

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Orvaen/terminal/internal/ui"
)

const alpineReleases = "https://dl-cdn.alpinelinux.org/alpine/latest-stable/releases/%s/"

// alpineArch traduit l'architecture Go en architecture Alpine.
func alpineArch() (string, error) {
	switch runtime.GOARCH {
	case "amd64":
		return "x86_64", nil
	case "arm64":
		return "aarch64", nil
	}
	return "", fmt.Errorf("architecture non prise en charge : %s", runtime.GOARCH)
}

type release struct {
	File   string
	SHA256 string
}

// parseLatestReleases lit latest-releases.yaml et renvoie l'entrée « mini root filesystem ».
// Analyse volontairement simple : on ne lit que les clés flavor, file et sha256.
func parseLatestReleases(r io.Reader) (release, error) {
	var (
		cur    = map[string]string{}
		found  release
		ok     bool
		finish = func() {
			if !ok && cur["flavor"] == "alpine-minirootfs" && cur["file"] != "" && cur["sha256"] != "" {
				found, ok = release{File: cur["file"], SHA256: strings.ToLower(cur["sha256"])}, true
			}
			cur = map[string]string{}
		}
	)
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := sc.Text()
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "-") && !strings.HasPrefix(trimmed, "---") {
			finish()
			trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, "-"))
		}
		key, val, hasColon := strings.Cut(trimmed, ":")
		if !hasColon {
			continue
		}
		key = strings.TrimSpace(key)
		switch key {
		case "flavor", "file", "sha256":
			cur[key] = strings.Trim(strings.TrimSpace(val), `"'`)
		}
	}
	finish()
	if err := sc.Err(); err != nil {
		return release{}, err
	}
	if !ok {
		return release{}, fmt.Errorf("aucune image « alpine-minirootfs » dans la liste des versions d'Alpine")
	}
	return found, nil
}

// downloadAlpine télécharge la dernière image Alpine minimale et vérifie sa somme SHA-256.
func downloadAlpine(dir string) (string, error) {
	arch, err := alpineArch()
	if err != nil {
		return "", err
	}
	base := fmt.Sprintf(alpineReleases, arch)
	client := &http.Client{Timeout: 5 * time.Minute}

	resp, err := client.Get(base + "latest-releases.yaml")
	if err != nil {
		return "", fmt.Errorf("liste des versions d'Alpine : %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("liste des versions d'Alpine : HTTP %d", resp.StatusCode)
	}
	rel, err := parseLatestReleases(resp.Body)
	if err != nil {
		return "", err
	}
	if strings.ContainsAny(rel.File, `/\`) {
		return "", fmt.Errorf("nom de fichier inattendu : %q", rel.File)
	}

	img, err := client.Get(base + rel.File)
	if err != nil {
		return "", fmt.Errorf("téléchargement d'Alpine : %w", err)
	}
	defer img.Body.Close()
	if img.StatusCode != http.StatusOK {
		return "", fmt.Errorf("téléchargement d'Alpine : HTTP %d", img.StatusCode)
	}

	path := filepath.Join(dir, rel.File)
	out, err := os.Create(path)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	bar := ui.NewBar(rel.File, img.ContentLength)
	if _, err := io.Copy(io.MultiWriter(out, h, bar), img.Body); err != nil {
		bar.Abort()
		out.Close()
		return "", fmt.Errorf("téléchargement d'Alpine : %w", err)
	}
	bar.Finish()
	if err := out.Close(); err != nil {
		return "", err
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != rel.SHA256 {
		os.Remove(path)
		return "", fmt.Errorf("somme de contrôle invalide pour %s (attendu %s, obtenu %s)", rel.File, rel.SHA256, got)
	}
	return path, nil
}
