package apt

import (
	"io"
	"reflect"
	"testing"
)

// fakeWinget remplace winget et enregistre les appels.
func fakeWinget(t *testing.T, codes map[string]int) *[][]string {
	t.Helper()
	var calls [][]string
	oldW, oldA, oldOut := winget, wingetAvailable, stdout
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
	t.Cleanup(func() { winget, wingetAvailable, stdout = oldW, oldA, oldOut })
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
	calls := fakeWinget(t, map[string]int{"broken": 5})
	code := Run([]string{"install", "broken", "git"})
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
