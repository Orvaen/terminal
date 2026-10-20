package apt

import (
	"io"
	"path/filepath"
	"reflect"
	"testing"
)

// fakeWinget remplace winget et enregistre les appels.
func fakeWinget(t *testing.T, codes map[string]int) *[][]string {
	t.Helper()
	var calls [][]string
	oldW, oldA, oldOut := winget, wingetAvailable, stdout
	oldSU, oldSC := selfUpdate, selfCheck
	oldSearch, oldAliasPath, oldInter := wingetSearchOutput, userAliasPath, interactive
	aliasFile := filepath.Join(t.TempDir(), "paquets.json")
	wingetSearchOutput = func(...string) string { return "" }
	userAliasPath = func() string { return aliasFile }
	interactive = func() bool { return false }
	selfUpdate = func(bool) int { calls = append(calls, []string{"self-update"}); return 0 }
	selfCheck = func() (string, bool) { return "", false }
	winget = func(args ...string) int {
		calls = append(calls, args)
		for _, a := range args {
			if c, ok := codes[a]; ok {
				return c
			}
		}
		return 0
	}
	wingetAvailable = func() bool { return true }
	stdout = io.Discard
	t.Cleanup(func() {
		winget, wingetAvailable, stdout = oldW, oldA, oldOut
		selfUpdate, selfCheck = oldSU, oldSC
		wingetSearchOutput, userAliasPath, interactive = oldSearch, oldAliasPath, oldInter
	})
	return &calls
}

func TestParse(t *testing.T) {
	inv := Parse([]string{"-y", "INSTALL", "git", "--upgradable", "vlc"})
	if inv.Command != "install" || !inv.Yes || !inv.Upgradable {
		t.Fatalf("analyse incorrecte : %+v", inv)
	}
	if !reflect.DeepEqual(inv.Packages, []string{"git", "vlc"}) {
		t.Fatalf("paquets incorrects : %v", inv.Packages)
	}
}

func TestInstallAliasUsesExactID(t *testing.T) {
	calls := fakeWinget(t, nil)
	if code := Run([]string{"install", "node", "-y"}); code != 0 {
		t.Fatalf("code %d", code)
	}
	want := [][]string{{"install", "--id", "OpenJS.NodeJS.LTS", "--exact", acceptSource, acceptPackage}}
	if !reflect.DeepEqual(*calls, want) {
		t.Fatalf("appels %v, attendu %v", *calls, want)
	}
}

func TestInstallUnknownNameIsPassedThrough(t *testing.T) {
	calls := fakeWinget(t, nil)
	Run([]string{"install", "Spotify.Spotify"})
	want := [][]string{{"install", "Spotify.Spotify", acceptSource, acceptPackage}}
	if !reflect.DeepEqual(*calls, want) {
		t.Fatalf("appels %v, attendu %v", *calls, want)
	}
}

func TestInstallContinuesAfterFailure(t *testing.T) {
	calls := fakeWinget(t, map[string]int{"Broken.Pkg": 5})
	code := Run([]string{"install", "Broken.Pkg", "git"})
	if code != 5 {
		t.Fatalf("code %d, attendu 5", code)
	}
	if len(*calls) != 2 {
		t.Fatalf("le second paquet aurait dû être installé : %v", *calls)
	}
}

func TestUpgradeAllTreatsNotApplicableAsSuccess(t *testing.T) {
	notApplicable := int(int32(-1978335189)) // 0x8A15002B vu comme entier signé
	fakeWinget(t, map[string]int{"--all": notApplicable})
	if code := Run([]string{"upgrade"}); code != 0 {
		t.Fatalf("code %d, attendu 0", code)
	}
}

func TestRemoveAndListUpgradable(t *testing.T) {
	calls := fakeWinget(t, nil)
	Run([]string{"purge", "vlc"})
	Run([]string{"list", "--upgradable"})
	want := [][]string{
		{"uninstall", "--id", "VideoLAN.VLC", "--exact", acceptSource},
		{"upgrade", acceptSource},
	}
	if !reflect.DeepEqual(*calls, want) {
		t.Fatalf("appels %v, attendu %v", *calls, want)
	}
}

func TestErrors(t *testing.T) {
	calls := fakeWinget(t, nil)
	for _, args := range [][]string{{"install"}, {"remove"}, {"search"}, {"frobnicate"}} {
		if code := Run(args); code != 1 {
			t.Errorf("%v : code %d, attendu 1", args, code)
		}
	}
	if len(*calls) != 0 {
		t.Fatalf("winget n'aurait pas dû être appelé : %v", *calls)
	}
}

func TestMissingWinget(t *testing.T) {
	fakeWinget(t, nil)
	wingetAvailable = func() bool { return false }
	if code := Run([]string{"update"}); code != 1 {
		t.Fatalf("code %d, attendu 1", code)
	}
}

func TestDockerBypassesWinget(t *testing.T) {
	calls := fakeWinget(t, nil)
	var log []string
	oldI, oldU, oldG, oldD := dockerInstall, dockerUninstall, dockerUpgrade, dockerInstalled
	dockerInstall = func(func(...string) int) int { log = append(log, "install"); return 0 }
	dockerUninstall = func(yes bool) int {
		if yes {
			log = append(log, "remove -y")
		} else {
			log = append(log, "remove")
		}
		return 0
	}
	dockerUpgrade = func() int { log = append(log, "upgrade"); return 0 }
	dockerInstalled = func() bool { return true }
	t.Cleanup(func() { dockerInstall, dockerUninstall, dockerUpgrade, dockerInstalled = oldI, oldU, oldG, oldD })

	Run([]string{"install", "docker", "git"})
	Run([]string{"remove", "docker", "-y"})
	Run([]string{"upgrade", "docker"})
	Run([]string{"upgrade"})

	if want := []string{"install", "remove -y", "upgrade", "upgrade"}; !reflect.DeepEqual(log, want) {
		t.Fatalf("appels docker %v, attendu %v", log, want)
	}
	wantWinget := [][]string{
		{"install", "--id", "Git.Git", "--exact", acceptSource, acceptPackage},
		{"upgrade", "--all", acceptSource, acceptPackage},
	}
	if !reflect.DeepEqual(*calls, wantWinget) {
		t.Fatalf("appels winget %v, attendu %v", *calls, wantWinget)
	}
}

func TestInstalledNamesFromWingetExport(t *testing.T) {
	ids, err := parseExport([]byte(`{"$schema":"https://aka.ms/winget-packages.schema.2.0.json",
 "Sources":[{"Packages":[{"PackageIdentifier":"Git.Git"},{"PackageIdentifier":"Spotify.Spotify"},
 {"PackageIdentifier":"OpenJS.NodeJS.LTS"}],"SourceDetails":{"Name":"winget"}}],"WinGetVersion":"1.9"}`))
	if err != nil {
		t.Fatal(err)
	}
	got := namesFromIDs(ids, true)
	want := []string{"git", "node", "spotify", "docker", "Git.Git", "OpenJS.NodeJS.LTS", "Spotify.Spotify"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("obtenu %q\nattendu %q", got, want)
	}
}

func TestUpgradeTerminalUsesSelfUpdate(t *testing.T) {
	calls := fakeWinget(t, nil)
	if code := Run([]string{"upgrade", "terminal"}); code != 0 {
		t.Fatalf("code %d", code)
	}
	if want := [][]string{{"self-update"}}; !reflect.DeepEqual(*calls, want) {
		t.Fatalf("appels %v, attendu %v (winget ne doit pas être appelé)", *calls, want)
	}
}

func TestUpgradeAllUpdatesReleaseBuildsOnly(t *testing.T) {
	old := Version
	t.Cleanup(func() { Version = old })

	Version = "dev"
	calls := fakeWinget(t, nil)
	Run([]string{"upgrade"})
	for _, c := range *calls {
		if c[0] == "self-update" {
			t.Fatal("une compilation locale ne doit pas être remplacée par apt upgrade")
		}
	}

	Version = "r5"
	calls = fakeWinget(t, nil)
	Run([]string{"upgrade"})
	last := (*calls)[len(*calls)-1]
	if last[0] != "self-update" {
		t.Fatalf("apt upgrade doit finir par la mise à jour du terminal, appels %v", *calls)
	}
}
