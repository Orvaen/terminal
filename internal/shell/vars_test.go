package shell

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func run(t *testing.T, s *Shell, line string) {
	t.Helper()
	s.RunLine(line)
}

func TestVariablesEchoAndExport(t *testing.T) {
	s, out, errOut, _ := testShell(t)
	t.Setenv("ORVAEN_TEST_ENV", "")
	os.Unsetenv("ORVAEN_TEST_ENV")

	run(t, s, `X=bonjour`)
	run(t, s, `echo $X ${X}! "$X" '$X' \$X`)
	if got := out.String(); got != "bonjour bonjour! bonjour $X $X\n" {
		t.Fatalf("echo : %q (erreurs %q)", got, errOut.String())
	}
	if _, ok := os.LookupEnv("X"); ok {
		t.Fatal("X=… seul ne doit pas aller dans l'environnement")
	}
	run(t, s, `export X`)
	if os.Getenv("X") != "bonjour" {
		t.Fatal("export X doit transmettre la variable")
	}
	run(t, s, `export -n X`)
	if _, ok := os.LookupEnv("X"); ok {
		t.Fatal("export -n doit la retirer de l'environnement")
	}
	if v, _ := s.getVar("X"); v != "bonjour" {
		t.Fatal("export -n doit garder la variable dans le terminal")
	}
	run(t, s, `unset X`)
	if _, ok := s.getVar("X"); ok {
		t.Fatal("unset")
	}
	t.Cleanup(func() { os.Unsetenv("X") })

	// NOM=valeur commande : seulement pour la commande
	out.Reset()
	run(t, s, `ORVAEN_TEST_ENV=un printenv ORVAEN_TEST_ENV`)
	if out.String() != "un\n" {
		t.Fatalf("NOM=valeur commande : %q", out.String())
	}
	if _, ok := os.LookupEnv("ORVAEN_TEST_ENV"); ok {
		t.Fatal("la variable doit disparaître après la commande")
	}

	// export NOM=valeur avec espaces : pas de découpage
	run(t, s, `Y="a  b"; export ORVAEN_TEST_ENV=$Y`)
	if os.Getenv("ORVAEN_TEST_ENV") != "a  b" {
		t.Fatalf("export NOM=$Y : %q", os.Getenv("ORVAEN_TEST_ENV"))
	}
}

func TestExpandSplittingAndQuotes(t *testing.T) {
	s, _, _, work := testShell(t)
	s.setVar("X", "a  b")
	s.setVar("VIDE", "")
	_, args, err := s.expandCommand(`cmd $X "$X" '$X' $VIDE "$VIDE" pre$X`)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"cmd", "a", "b", "a  b", "$X", "", "prea", "b"}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("obtenu %q, attendu %q", args, want)
	}

	// les jokers d'une valeur hors guillemets sont actifs, pas ses accolades
	for _, f := range []string{"a.txt", "b.txt"} {
		os.WriteFile(filepath.Join(work, f), nil, 0o644)
	}
	s.setVar("MOTIF", "*.txt")
	s.setVar("ACC", "{x,y}")
	_, args, _ = s.expandCommand(`ls $MOTIF "$MOTIF" $ACC`)
	if !reflect.DeepEqual(args, []string{"ls", "a.txt", "b.txt", "*.txt", "{x,y}"}) {
		t.Fatalf("jokers d'une valeur : %q", args)
	}

	// affectations : pas de découpage, ~ après = et :
	home := os.Getenv("HOME")
	as, args, _ := s.expandCommand(`A=$X B=~/bin:~/outils commande`)
	if len(as) != 2 || as[0].value != "a  b" || as[1].value != home+"/bin:"+home+"/outils" || !reflect.DeepEqual(args, []string{"commande"}) {
		t.Fatalf("affectations : %+v %q", as, args)
	}
	// un = plus loin n'est pas une affectation
	as, args, _ = s.expandCommand(`git config user.name=moi`)
	if len(as) != 0 || len(args) != 3 {
		t.Fatalf("%+v %q", as, args)
	}
}

func TestParameterOperators(t *testing.T) {
	s, _, _, _ := testShell(t)
	s.setVar("F", "src/app/main.test.ts")
	s.setVar("V", "")
	s.setVar("N", "Bonjour")
	cases := map[string]string{
		`${F##*/}`:         "main.test.ts",
		`${F#*/}`:          "app/main.test.ts",
		`${F%.*}`:          "src/app/main.test",
		`${F%%.*}`:         "src/app/main",
		`${F/t/T}`:         "src/app/main.Test.ts",
		`${F//t/T}`:        "src/app/main.TesT.Ts",
		`${F/#src/lib}`:    "lib/app/main.test.ts",
		`${F/%ts/js}`:      "src/app/main.test.js",
		`${#N}`:            "7",
		`${N:0:3}`:         "Bon",
		`${N:3}`:           "jour",
		`${N: -4}`:         "jour",
		`${N^^}`:           "BONJOUR",
		`${N,,}`:           "bonjour",
		`${V:-défaut}`:     "défaut",
		`${V-défaut}`:      "",
		`${ABSENT-x}`:      "x",
		`${N:+oui}`:        "oui",
		`${V:+oui}`:        "",
		`${ABSENT:-$N!}`:   "Bonjour!",
		`${NEW:=créée}`:    "créée",
		`$((2 + 3 * 4))`:   "14",
		`$(( (2+3)*4 ))`:   "20",
		`$((2**10 % 7))`:   "2",
		`$((-7 / 2))`:      "-3",
		`$((10 > 3 && 1))`: "1",
		`$((0x10 + 010))`:  "24",
	}
	s.setVar("I", "5")
	cases[`$((I * 2))`] = "10"
	cases[`$((I > 3 ? 100 : 200))`] = "100"
	for in, want := range cases {
		got, err := s.expandText(in)
		if err != nil || got != want {
			t.Errorf("%s : obtenu %q (%v), attendu %q", in, got, err, want)
		}
	}
	if v, _ := s.getVar("NEW"); v != "créée" {
		t.Error("${NEW:=…} doit enregistrer la valeur")
	}
	for _, bad := range []string{`${V:?vide !}`, `$((1/0))`, `${}`, `$((I += 1))`} {
		if _, err := s.expandText(bad); err == nil {
			t.Errorf("%s : erreur attendue", bad)
		}
	}
}

func TestCommandSubstitution(t *testing.T) {
	s, out, errOut, work := testShell(t)
	other := t.TempDir()
	run(t, s, `echo "[$(echo a; echo b)]" `+"`echo c`"+` && echo fin`)
	if got := out.String(); got != "[a\nb] c\nfin\n" {
		t.Fatalf("obtenu %q (erreurs %q)", got, errOut.String())
	}
	// ce que fait la sous-commande ne touche pas la session
	out.Reset()
	run(t, s, `R=$(cd "`+other+`"; export ORVAEN_SOUS=1; Z=2; echo ok)`)
	if v, _ := s.getVar("R"); v != "ok" {
		t.Fatalf("R=%q", v)
	}
	cwd, _ := os.Getwd()
	if !strings.EqualFold(filepath.Clean(cwd), filepath.Clean(work)) {
		t.Fatalf("le dossier a changé : %s", cwd)
	}
	if _, ok := os.LookupEnv("ORVAEN_SOUS"); ok {
		t.Fatal("l'export de la sous-commande a fui")
	}
	if _, ok := s.getVar("Z"); ok {
		t.Fatal("la variable de la sous-commande a fui")
	}
	// imbriquées
	out.Reset()
	run(t, s, `echo $(echo $(echo profond))`)
	if out.String() != "profond\n" {
		t.Fatalf("imbriquées : %q", out.String())
	}
}

func TestSpecialParameters(t *testing.T) {
	s, out, _, _ := testShell(t)
	run(t, s, `ls ./n-existe-pas`)
	out.Reset()
	run(t, s, `echo $?`)
	if out.String() == "0\n" {
		t.Fatal("$? doit valoir le code de la commande ratée")
	}
	run(t, s, `echo un deux trois`)
	out.Reset()
	run(t, s, `echo $_`)
	if out.String() != "trois\n" {
		t.Fatalf("$_ : %q", out.String())
	}
	if v, _ := s.getVar("$"); v != itoa(os.Getpid()) {
		t.Fatal("$$")
	}
	if err := s.setVar("UID", "0"); err == nil {
		t.Fatal("UID est en lecture seule")
	}
	for _, name := range []string{"RANDOM", "SECONDS", "HOSTNAME", "HOSTTYPE", "OSTYPE", "MACHTYPE", "PPID", "UID", "EUID", "COLUMNS", "LINES", "EPOCHSECONDS", "PS1"} {
		if v, ok := s.getVar(name); !ok || v == "" {
			t.Errorf("$%s devrait être défini", name)
		}
	}
	t.Setenv("ORVAEN_PS", "ps")
	if got, _ := s.expandText(`$env:ORVAEN_PS/${ORVAEN_PS}`); got != "ps/ps" {
		t.Fatalf("$env:NOM : %q", got)
	}
}

func TestChainKeepsSubstitutionsWhole(t *testing.T) {
	segs, err := splitChain(`echo "$(a; b && c)" $(d || e) && f`)
	if err != nil || len(segs) != 2 || segs[0].text != `echo "$(a; b && c)" $(d || e)` || segs[0].special {
		t.Fatalf("%+v %v", segs, err)
	}
	st, err := parsePipeline(`echo $(a | b) | wc > "$OUT"`, expandLine)
	if err != nil || len(st) != 2 || st[0].text != "echo $(a | b)" {
		t.Fatalf("%+v %v", st, err)
	}
	if _, err := splitChain(`echo $(oups`); err == nil {
		t.Fatal("$( non fermé : erreur attendue")
	}
}

func TestPS1BashSequences(t *testing.T) {
	got := ps1ToFormat(`\[\e[32m\]\u@\h\[\e[0m\]:\w\$ `)
	want := "\x1b[32m{user}@{host}\x1b[0m:{dir}{signe} "
	if got != want {
		t.Fatalf("obtenu %q, attendu %q", got, want)
	}
	s, _, _, _ := testShell(t)
	s.setVar("PS1", `\W> `)
	if p := s.prompt(); !strings.HasSuffix(p, "work> ") {
		t.Fatalf("prompt : %q", p)
	}
	s.unsetVar("PS1")
	if s.ps1 != "" {
		t.Fatal("unset PS1")
	}
}

func TestJoinArgsProtectsExpansions(t *testing.T) {
	s, _, _, _ := testShell(t)
	args := []string{"echo", "$HOME", "*.log", "{a,b}", "l'eau", "a b"}
	_, got, err := s.expandCommand(joinArgs(args))
	if err != nil || !reflect.DeepEqual(got, args) {
		t.Fatalf("obtenu %q (%v)", got, err)
	}
}
