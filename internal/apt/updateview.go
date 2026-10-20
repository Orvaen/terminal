package apt

// Affichage d'apt update et apt list --upgradable : les sources avec un indicateur animé puis ✓,
// et un seul tableau des mises à jour (winget, nodejs.org, Orvaen Terminal), au lieu de la
// sortie brute de winget.

import (
	"bufio"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/Orvaen/terminal/internal/ui"
	"github.com/Orvaen/terminal/internal/update"
)

// wingetLines lance winget, appelle onLine pour chaque ligne (si non nil) et renvoie toute la sortie.
// Remplacée dans les tests.
var wingetLines = func(args []string, onLine func(string)) (string, int) {
	cmd := exec.Command("winget", args...)
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return "", 1
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		ui.Errorf("impossible de lancer winget : %v", err)
		return "", 1
	}
	var all strings.Builder
	sc := bufio.NewScanner(pipe)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	sc.Split(scanLinesCR)
	for sc.Scan() {
		line := sc.Text()
		all.WriteString(line + "\n")
		if onLine != nil {
			onLine(line)
		}
	}
	code := 0
	if err := cmd.Wait(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			code = 1
		}
	}
	return all.String(), code
}

// scanLinesCR coupe aussi sur \r : winget réécrit ses lignes de progression sur place.
func scanLinesCR(data []byte, atEOF bool) (int, []byte, error) {
	for i, b := range data {
		if b == '\n' || b == '\r' {
			return i + 1, data[:i], nil
		}
	}
	if atEOF && len(data) > 0 {
		return len(data), data, nil
	}
	return 0, nil, nil
}

// « Mise à jour de la source : winget... », « Updating source: winget... »
var reSourceLine = regexp.MustCompile(`[:：]\s*(\S[^:：]*?)\s*(?:\.\.\.|…)\s*$`)

// updateSources : apt update, première partie.
func updateSources() int {
	ui.Step("Mise à jour des sources")
	sp := ui.StartSpinner("Connexion aux sources de winget…")
	current, since := "", time.Now()
	finish := func() {
		if current != "" {
			sp.Println(fmt.Sprintf("  %s %-14s %s", ui.Color("32", "✓"), current, ui.Color("2", ui.FormatDuration(time.Since(since)))))
			current = ""
		}
	}
	out, code := wingetLines([]string{"source", "update"}, func(line string) {
		if m := reSourceLine.FindStringSubmatch(line); m != nil && !strings.Contains(m[1], " ") {
			finish()
			current, since = m[1], time.Now()
			sp.Update("Mise à jour de la source " + current + "…")
		}
	})
	if code != 0 {
		sp.Fail("La mise à jour des sources a échoué (code " + fmt.Sprint(code) + ") :")
		fmt.Fprint(stdout, out)
		return code
	}
	finish()
	sp.Clear()
	return 0
}

// upgradeItem : une ligne du tableau des mises à jour.
type upgradeItem struct {
	Name, From, To, Handle string
}

// officialItems : ce que nodejs.org et les releases d'Orvaen Terminal proposent, et les remarques.
func officialItems() (items []upgradeItem, notes []string, nodeTarget bool) {
	var wg sync.WaitGroup
	var self *upgradeItem
	wg.Add(1)
	go func() {
		defer wg.Done()
		if update.IsDev(Version) {
			return
		}
		if latest, ok := selfCheck(); ok {
			self = &upgradeItem{Name: "Orvaen Terminal", From: Version, To: latest, Handle: "terminal"}
		}
	}()
	plan, ok, err := checkNode()
	wg.Wait()
	var zero version
	switch {
	case !ok:
	case err != nil:
		notes = append(notes, "nodejs.org injoignable : version de Node non vérifiée")
	default:
		if plan.target != zero {
			items = append(items, upgradeItem{Name: "Node.js (nodejs.org)", From: plan.installed.String(), To: plan.target.String(), Handle: "node"})
			nodeTarget = true
		}
		if plan.newMajorLTS != zero {
			notes = append(notes, fmt.Sprintf("Node %s LTS disponible (nouvelle version majeure) : apt install node", plan.newMajorLTS))
		}
		if plan.newCurrent != zero && plan.newCurrent.newer(plan.newMajorLTS) {
			notes = append(notes, fmt.Sprintf("Node %s « Current » disponible (hors LTS) : apt install node-current", plan.newCurrent))
		}
	}
	if self != nil {
		items = append(items, *self)
	}
	return items, notes, nodeTarget
}

// listUpgrades : apt update (seconde partie) et apt list --upgradable.
func listUpgrades() int {
	ui.Step("Mises à jour disponibles")
	sp := ui.StartSpinner("Recherche des mises à jour…")
	var (
		out  string
		code int
		wg   sync.WaitGroup
	)
	wg.Add(1)
	go func() {
		defer wg.Done()
		out, code = wingetLines([]string{"upgrade", acceptSource}, nil)
	}()
	official, notes, nodeTarget := officialItems()
	wg.Wait()
	sp.Clear()

	items, unknown := parseUpgrades(out)
	if nodeTarget { // nodejs.org a déjà la version suivante : la ligne de winget ferait doublon
		var kept []upgradeItem
		for _, it := range items {
			if !isNode(it.Handle) && it.Handle != "OpenJS.NodeJS" {
				kept = append(kept, it)
			}
		}
		items = kept
	}
	items = append(items, official...)
	if len(items) == 0 && code != 0 && !isNotApplicable(code) && strings.TrimSpace(out) != "" {
		ui.Errorf("winget upgrade a échoué (code %d) :", code)
		fmt.Fprint(stdout, out)
		return code
	}

	if len(items) == 0 {
		fmt.Fprintf(stdout, "  %s Tout est à jour.\n", ui.Color("32", "✓"))
	} else {
		printUpgrades(items)
	}
	if unknown {
		notes = append(notes, "Les logiciels dont winget ne connaît pas la version ne sont pas listés.")
	}
	for _, n := range notes {
		fmt.Fprintln(stdout, "  "+ui.Color("2", n))
	}
	if len(items) > 0 {
		fmt.Fprintf(stdout, "\nTout mettre à jour : %s    Un seul : %s\n", ui.Color("1", "apt upgrade"), ui.Color("1", "apt upgrade <nom>"))
	}
	return 0
}

func printUpgrades(items []upgradeItem) {
	nameW, fromW := 10, 6
	for _, it := range items {
		nameW = max(nameW, min(38, utf8.RuneCountInString(it.Name)))
		fromW = max(fromW, utf8.RuneCountInString(it.From))
	}
	fmt.Fprintf(stdout, "  %d logiciel(s) à mettre à jour\n\n", len(items))
	for _, it := range items {
		fmt.Fprintf(stdout, "  %s  %s %s %s  %s\n",
			pad(truncate(it.Name, nameW), nameW),
			ui.Color("2", pad(it.From, fromW)), ui.Color("2", "→"), ui.Color("1;32", pad(it.To, 10)),
			ui.Color("2", "apt upgrade "+it.Handle))
	}
}

func pad(s string, w int) string {
	if n := utf8.RuneCountInString(s); n < w {
		return s + strings.Repeat(" ", w-n)
	}
	return s
}

func truncate(s string, w int) string {
	if utf8.RuneCountInString(s) <= w {
		return s
	}
	return string([]rune(s)[:w-1]) + "…"
}

var reWingetID = regexp.MustCompile(`^([A-Za-z0-9][\w.+-]*\.[\w.+-]+|[0-9A-Z]{12})$`)

// parseUpgrades lit le tableau de « winget upgrade » (toutes langues) : Nom, ID, Version, Disponible, Source.
// unknown : winget signale des logiciels de version inconnue (--include-unknown).
func parseUpgrades(out string) (items []upgradeItem, unknown bool) {
	lines := strings.Split(strings.ReplaceAll(out, "\r\n", "\n"), "\n")
	for i := range lines {
		if j := strings.LastIndex(lines[i], "\r"); j >= 0 {
			lines[i] = lines[i][j+1:]
		}
		lines[i] = strings.TrimRight(lines[i], " \t")
		if strings.Contains(lines[i], "--include-unknown") {
			unknown = true
		}
	}
	for i := 1; i < len(lines); i++ {
		t := strings.TrimSpace(lines[i])
		if len(t) < 10 || strings.Trim(t, "-─") != "" {
			continue
		}
		header := []rune(lines[i-1])
		var starts []int
		for k, r := range header {
			if r != ' ' && (k == 0 || header[k-1] == ' ') {
				starts = append(starts, k)
			}
		}
		if len(starts) < 4 {
			continue
		}
		for _, l := range lines[i+1:] {
			row := []rune(l)
			if strings.TrimSpace(l) == "" || len(row) <= starts[3] {
				break
			}
			cell := func(c int) string {
				end := len(row)
				if c+1 < len(starts) && starts[c+1] < end {
					end = starts[c+1]
				}
				if starts[c] >= end {
					return ""
				}
				return strings.TrimSpace(string(row[starts[c]:end]))
			}
			id := cell(1)
			if !reWingetID.MatchString(id) {
				break // fin du tableau (phrase de résumé)
			}
			handle := id
			if name, ok := primaryName[id]; ok {
				handle = name
			}
			items = append(items, upgradeItem{Name: cell(0), From: cell(2), To: cell(3), Handle: handle})
		}
	}
	return items, unknown
}
