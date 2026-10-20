package apt

import (
	"bytes"
	"regexp"
	"strings"
	"testing"

	"github.com/Orvaen/terminal/internal/ui"
)

// Sortie réelle de winget upgrade (Windows en français), avec les phrases de résumé collées au tableau.
const upgradeFR = `Nom                                        ID                              Version  Disponible Source
-----------------------------------------------------------------------------------------------------
Epic Online Services                       EpicGames.EpicOnlineServices    4.2.0    4.3.1      winget
Microsoft OLE DB Driver 19 pour SQL Server Microsoft.SQLServer.OLEDBDriver 19.4.1.0 19.4.3     winget
2 mises à niveau disponibles.
1 package(s) a(ont) des numéros de version qui ne peuvent pas être déterminés. Utilisez --include-unknown pour afficher tous les résultats.
`

const upgradeEN = "   - \r   \\ \r" + `Name               Id                         Version Available Source
-----------------------------------------------------------------------
Visual Studio Code Microsoft.VisualStudioCode 1.104.0 1.105.1   winget
Git                Git.Git                    2.50.0  2.51.0    winget
Node.js LTS        OpenJS.NodeJS.LTS          24.1.0  24.2.0    winget
3 upgrades available.
`

func TestParseUpgradesFrench(t *testing.T) {
	items, unknown := parseUpgrades(upgradeFR)
	if len(items) != 2 || !unknown {
		t.Fatalf("obtenu %+v, inconnu %v", items, unknown)
	}
	want := upgradeItem{Name: "Microsoft OLE DB Driver 19 pour SQL Server", From: "19.4.1.0", To: "19.4.3", Handle: "Microsoft.SQLServer.OLEDBDriver"}
	if items[1] != want {
		t.Fatalf("obtenu %+v", items[1])
	}
}

func TestParseUpgradesUsesShortNames(t *testing.T) {
	items, _ := parseUpgrades(upgradeEN)
	handles := []string{}
	for _, it := range items {
		handles = append(handles, it.Handle)
	}
	if strings.Join(handles, ",") != "code,git,node" {
		t.Fatalf("noms courts : %v", handles)
	}
}

func TestUpdateView(t *testing.T) {
	fakeWinget(t, nil)
	var out bytes.Buffer
	oldOut, oldUI, oldInter := stdout, ui.Out, ui.Interactive
	stdout, ui.Out, ui.Interactive = &out, &out, func() bool { return false }
	t.Cleanup(func() { stdout, ui.Out, ui.Interactive = oldOut, oldUI, oldInter })

	oldVersion := Version
	Version = "r24"
	t.Cleanup(func() { Version = oldVersion })
	selfCheck = func() (string, bool) { return "r25", true }

	wingetLines = func(args []string, onLine func(string)) (string, int) {
		if args[0] == "source" {
			for _, l := range []string{"Mise à jour en cours de toutes les sources... Merci de patienter.",
				"Mise à jour de la source : msstore...", "Terminé", "Mise à jour de la source : winget...", "Terminé"} {
				onLine(l)
			}
			return "", 0
		}
		return upgradeFR, 0
	}
	if code := Run([]string{"update"}); code != 0 {
		t.Fatalf("code %d", code)
	}
	got := regexp.MustCompile("\x1b\\[[0-9;]*m").ReplaceAllString(out.String(), "")
	for _, want := range []string{"✓ msstore", "✓ winget", "3 logiciel(s) à mettre à jour",
		"Epic Online Services", "4.2.0", "→", "4.3.1", "Orvaen Terminal", "apt upgrade terminal",
		"version", "Tout mettre à jour"} {
		if !strings.Contains(got, want) {
			t.Errorf("%q manque :\n%s", want, got)
		}
	}
	if strings.Contains(got, "Merci de patienter") || strings.Contains(got, "------") {
		t.Errorf("la sortie brute de winget ne doit plus apparaître :\n%s", got)
	}
}

func TestUpdateViewAllUpToDate(t *testing.T) {
	fakeWinget(t, nil)
	var out bytes.Buffer
	oldOut, oldUI := stdout, ui.Out
	stdout, ui.Out = &out, &out
	t.Cleanup(func() { stdout, ui.Out = oldOut, oldUI })
	wingetLines = func(args []string, onLine func(string)) (string, int) {
		return "Aucune mise à niveau disponible.\n", 0
	}
	if code := Run([]string{"list", "--upgradable"}); code != 0 || !strings.Contains(out.String(), "Tout est à jour") {
		t.Fatalf("code %d : %q", code, out.String())
	}
}
