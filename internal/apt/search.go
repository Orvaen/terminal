package apt

// Lecture des résultats de « winget search », pour retrouver un logiciel à partir d'un nom Linux.

import (
	"os/exec"
	"strings"
	"unicode/utf8"
)

// Package : une ligne de résultat de winget search.
type Package struct {
	Name, ID, Version, Source string
}

// wingetSearchOutput lance winget search et renvoie sa sortie (injectable dans les tests).
var wingetSearchOutput = func(args ...string) string {
	cmd := exec.Command("winget", append(append([]string{"search"}, args...), acceptSource)...)
	out, _ := cmd.Output() // aucun résultat : winget renvoie un code d'erreur, la sortie suffit
	return string(out)
}

// searchPackages cherche avec winget et renvoie les résultats lisibles.
func searchPackages(args ...string) []Package {
	return parseSearch(wingetSearchOutput(args...))
}

// parseSearch lit le tableau de winget search. L'en-tête dépend de la langue de Windows
// (Name/Nom, Id/ID…) : on ne lit pas les titres, seulement la position des colonnes, repérée
// grâce à la ligne de tirets sous l'en-tête. La colonne 1 est le nom, la 2 l'identifiant,
// la 3 la version ; la source (winget, msstore) est en dernier quand plusieurs sources répondent.
func parseSearch(out string) []Package {
	var lines []string
	for _, l := range strings.Split(strings.ReplaceAll(out, "\r\n", "\n"), "\n") {
		if i := strings.LastIndex(l, "\r"); i >= 0 { // indicateur de progression réécrit sur place
			l = l[i+1:]
		}
		lines = append(lines, strings.TrimRight(l, " \t"))
	}

	sep := -1
	for i, l := range lines {
		t := strings.TrimSpace(l)
		if len(t) >= 10 && strings.Trim(t, "-─") == "" && i > 0 {
			sep = i
			break
		}
	}
	if sep < 0 {
		return nil
	}
	header := []rune(lines[sep-1])
	var starts []int
	for i, r := range header {
		if r != ' ' && (i == 0 || header[i-1] == ' ') {
			starts = append(starts, i)
		}
	}
	if len(starts) < 3 {
		return nil
	}

	var pkgs []Package
	for _, l := range lines[sep+1:] {
		if strings.TrimSpace(l) == "" {
			break
		}
		row := []rune(l)
		if len(row) <= starts[1] {
			continue
		}
		name := strings.TrimSpace(string(row[:starts[1]]))
		fields := strings.Fields(string(row[starts[1]:]))
		if len(fields) == 0 {
			continue
		}
		p := Package{Name: name, ID: fields[0]}
		if strings.HasSuffix(p.ID, "…") || !utf8.ValidString(p.ID) {
			continue // identifiant coupé par winget : inutilisable
		}
		if len(fields) > 1 {
			p.Version = fields[1]
		}
		if last := fields[len(fields)-1]; len(fields) > 2 && (last == "winget" || last == "msstore") {
			p.Source = last
		}
		pkgs = append(pkgs, p)
	}
	return pkgs
}
