package apt

import (
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
)

// Sorties réelles de winget search (anglais et français), avec l'indicateur de progression.
const searchEN = "   - \r   \\ \r                                                                                                                        \r" +
	`Name                       Id                                  Version  Match            Source
-------------------------------------------------------------------------------------------------
Visual Studio Code         Microsoft.VisualStudioCode          1.105.1  Command: code    winget
Visual Studio Code Insiders Microsoft.VisualStudioCode.Insiders 1.106.0  Command: code    winget
Visual Studio Code         XP9KHM4BK9FZ7Q                      Unknown                   msstore
Un nom très long qui dépasse… VendorWithAVeryLongName.Applicat…  2.0                       winget
`

const searchFR = `Nom                ID                   Version
-----------------------------------------------
Éditeur de thèmes  Exemple.ÉditeurThèmes 3.1
fd                 sharkdp.fd            10.2.0

`

func TestParseSearch(t *testing.T) {
	got := parseSearch(searchEN)
	want := []Package{
		{Name: "Visual Studio Code", ID: "Microsoft.VisualStudioCode", Version: "1.105.1", Source: "winget"},
		{Name: "Visual Studio Code Insiders", ID: "Microsoft.VisualStudioCode.Insiders", Version: "1.106.0", Source: "winget"},
		{Name: "Visual Studio Code", ID: "XP9KHM4BK9FZ7Q", Version: "Unknown", Source: "msstore"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("obtenu %#v", got)
	}
	got = parseSearch(searchFR)
	want = []Package{
		{Name: "Éditeur de thèmes", ID: "Exemple.ÉditeurThèmes", Version: "3.1"},
		{Name: "fd", ID: "sharkdp.fd", Version: "10.2.0"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("en-tête français : obtenu %#v", got)
	}
	if got := parseSearch("Aucun package ne correspond aux critères d'entrée.\n"); got != nil {
		t.Fatalf("aucun résultat attendu, obtenu %v", got)
	}
}

// fakeSearch répond à winget search selon le premier argument (--moniker, --command ou la requête).
func fakeSearch(t *testing.T, answers map[string]string) *[][]string {
	t.Helper()
	var log [][]string
	wingetSearchOutput = func(args ...string) string {
		log = append(log, args)
		return answers[strings.Join(args[:2], " ")]
	}
	return &log
}

// table produit un tableau aligné comme celui de winget ; chaque ligne : "Nom|Id".
func table(rows ...string) string {
	line := func(a, b, c, d string) string { return fmt.Sprintf("%-20s%-30s%-10s%s\n", a, b, c, d) }
	out := line("Name", "Id", "Version", "Source") + strings.Repeat("-", 66) + "\n"
	for _, r := range rows {
		name, id, _ := strings.Cut(r, "|")
		out += line(name, id, "1.0", "winget")
	}
	return out
}

func TestInstallTableName(t *testing.T) {
	calls := fakeWinget(t, nil)
	search := fakeSearch(t, nil)
	Run([]string{"install", "fd-find"})
	want := [][]string{{"install", "--id", "sharkdp.fd", "--exact", acceptSource, acceptPackage}}
	if !reflect.DeepEqual(*calls, want) || len(*search) != 0 {
		t.Fatalf("appels %v, recherches %v", *calls, *search)
	}
}

func TestInstallByCommand(t *testing.T) {
	calls := fakeWinget(t, nil)
	search := fakeSearch(t, map[string]string{
		"--command mytool": table("My Tool|Vendor.MyTool"),
	})
	if code := Run([]string{"install", "mytool"}); code != 0 {
		t.Fatalf("code %d", code)
	}
	if (*search)[0][0] != "--moniker" || (*search)[1][0] != "--command" {
		t.Fatalf("ordre des recherches : %v", *search)
	}
	want := [][]string{{"install", "--id", "Vendor.MyTool", "--exact", acceptSource, acceptPackage}}
	if !reflect.DeepEqual(*calls, want) {
		t.Fatalf("appels %v", *calls)
	}
}

func TestInstallAmbiguousWithoutConsoleDoesNotGuess(t *testing.T) {
	calls := fakeWinget(t, nil)
	fakeSearch(t, map[string]string{
		"thing --count": table("Thing A|A.Thing", "Thing B|B.Thing"),
	})
	if code := Run([]string{"install", "thing"}); code != 100 {
		t.Fatalf("code 100 attendu, obtenu %d", code)
	}
	if len(*calls) != 0 {
		t.Fatalf("rien ne doit être installé : %v", *calls)
	}
}

func TestInstallAmbiguousChoiceIsRemembered(t *testing.T) {
	calls := fakeWinget(t, nil)
	search := fakeSearch(t, map[string]string{
		"thing --count": table("Thing A|A.Thing", "Thing B|B.Thing"),
	})
	interactive = func() bool { return true }
	oldAsk := askChoice
	askChoice = func(string, []Package) int { return 2 }
	t.Cleanup(func() { askChoice = oldAsk })

	Run([]string{"install", "thing"})
	Run([]string{"install", "thing"}) // la seconde fois, sans recherche
	want := [][]string{
		{"install", "--id", "B.Thing", "--exact", acceptSource, acceptPackage},
		{"install", "--id", "B.Thing", "--exact", acceptSource, acceptPackage},
	}
	if !reflect.DeepEqual(*calls, want) {
		t.Fatalf("appels %v", *calls)
	}
	if len(*search) != 3 {
		t.Fatalf("une seule série de recherches attendue, obtenu %v", *search)
	}
}

func TestStrongMatchWins(t *testing.T) {
	calls := fakeWinget(t, nil)
	fakeSearch(t, map[string]string{
		"neowidget --count": table("Neowidget|Neo.Neowidget", "Neowidget Pro|Neo.NeowidgetPro"),
	})
	Run([]string{"install", "neowidget"})
	if len(*calls) != 1 || (*calls)[0][2] != "Neo.Neowidget" {
		t.Fatalf("appels %v", *calls)
	}
}

func TestCuratedIDMissingFallsBackToSearch(t *testing.T) {
	notFound := int(int32(wingetNoPackageFound - 1<<32))
	calls := fakeWinget(t, map[string]int{"sharkdp.fd": notFound})
	fakeSearch(t, map[string]string{"--command fd-find": table("fd|sharkdp.fd2")})
	if code := Run([]string{"install", "fd-find"}); code != 0 {
		t.Fatalf("code %d, appels %v", code, *calls)
	}
	if len(*calls) != 2 || (*calls)[1][2] != "sharkdp.fd2" {
		t.Fatalf("appels %v", *calls)
	}
}

func TestWindowsNotesAndAliasCommand(t *testing.T) {
	calls := fakeWinget(t, nil)
	if code := Run([]string{"install", "tree"}); code != 0 || len(*calls) != 0 {
		t.Fatalf("rien à installer attendu : code %d, appels %v", code, *calls)
	}
	if Run([]string{"alias", "tree", "Some.Tree"}) != 0 {
		t.Fatal("alias")
	}
	Run([]string{"install", "tree"})
	if len(*calls) != 1 || (*calls)[0][2] != "Some.Tree" {
		t.Fatalf("ton nom passe avant la note : %v", *calls)
	}
	if Run([]string{"alias", "-r", "tree"}) != 0 {
		t.Fatal("alias -r")
	}
	if _, ok := userAlias("tree"); ok {
		t.Fatal("le nom aurait dû être oublié")
	}
	data, _ := os.ReadFile(userAliasPath())
	if strings.Contains(string(data), "tree") {
		t.Fatalf("fichier : %s", data)
	}
}

func TestAliasTableIsConsistent(t *testing.T) {
	for name, id := range aliases {
		if name != strings.ToLower(name) || strings.ContainsAny(name, " \t") {
			t.Errorf("nom %q : minuscules, sans espace", name)
		}
		if primaryName[id] == "" {
			t.Errorf("%s (%s) n'a pas de nom principal", id, name)
		}
	}
	for id, name := range primaryName {
		if aliases[name] != id {
			t.Errorf("nom principal %q → %q, mais la table dit %q", name, id, aliases[name])
		}
	}
}
