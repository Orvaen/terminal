package apt

import (
	"reflect"
	"strings"
	"testing"
)

func TestRemovePipNeverRemovesPython(t *testing.T) {
	calls := fakeWinget(t, nil)
	exportInstalled = func() ([]string, error) { return []string{"Python.Python.3.14"}, nil }
	if code := Run([]string{"remove", "pip"}); code != 1 {
		t.Fatalf("code 1 attendu, obtenu %d", code)
	}
	if len(*calls) != 0 {
		t.Fatalf("rien ne doit être désinstallé : %v", *calls)
	}
}

func TestInstallComponentInstallsProvider(t *testing.T) {
	calls := fakeWinget(t, nil)
	Run([]string{"install", "pip"})
	Run([]string{"install", "npm"})
	want := [][]string{
		{"install", "--id", "Python.Python.3.14", "--exact", acceptSource, acceptPackage},
		{"install", "--id", "OpenJS.NodeJS.LTS", "--exact", acceptSource, acceptPackage},
	}
	if !reflect.DeepEqual(*calls, want) {
		t.Fatalf("appels %v", *calls)
	}
}

func TestUpgradeComponentUsesItsOwnCommand(t *testing.T) {
	calls := fakeWinget(t, nil)
	Run([]string{"upgrade", "pip", "npm", "cargo", "g++"})
	got := []string{}
	for _, c := range *calls {
		got = append(got, strings.Join(c, " "))
	}
	want := []string{
		"tool: python -m pip install --upgrade pip",
		"tool: npm install --global npm@latest",
		"tool: rustup update",
		"upgrade --id BrechtSanders.WinLibs.POSIX.UCRT --exact " + acceptSource + " " + acceptPackage,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("obtenu\n%v\nattendu\n%v", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestRemoveAndUpgradeTargetInstalledVersion(t *testing.T) {
	calls := fakeWinget(t, nil)
	exportInstalled = func() ([]string, error) { return []string{"Git.Git", "Python.Python.3.12"}, nil }
	Run([]string{"remove", "python"})
	if len(*calls) != 1 || (*calls)[0][2] != "Python.Python.3.12" {
		t.Fatalf("remove : %v", *calls)
	}

	*calls = nil
	exportInstalled = func() ([]string, error) {
		return []string{"Python.Python.3.12", "Python.Python.3.13", "EclipseAdoptium.Temurin.17.JDK"}, nil
	}
	if code := Run([]string{"remove", "python"}); code != 1 || len(*calls) != 0 {
		t.Fatalf("deux versions : il faut préciser, code %d, appels %v", code, *calls)
	}
	Run([]string{"upgrade", "python", "java"})
	ids := []string{}
	for _, c := range *calls {
		ids = append(ids, c[2])
	}
	if strings.Join(ids, ",") != "Python.Python.3.12,Python.Python.3.13,EclipseAdoptium.Temurin.17.JDK" {
		t.Fatalf("upgrade : %v", ids)
	}
}

func TestFamilyPattern(t *testing.T) {
	for id, cases := range map[string]map[string]bool{
		"Python.Python.3.14":             {"Python.Python.3.12": true, "Python.Launcher": false, "python.python.3.9": true},
		"OpenJS.NodeJS.LTS":              {"OpenJS.NodeJS.22": true, "OpenJS.NodeJS": true, "OpenJS.NodeJS.LTS": true, "OpenJS.Other": false},
		"EclipseAdoptium.Temurin.25.JDK": {"EclipseAdoptium.Temurin.21.JDK": true, "EclipseAdoptium.Temurin.21.JRE": false},
		"Microsoft.DotNet.SDK.10":        {"Microsoft.DotNet.SDK.8": true, "Microsoft.DotNet.Runtime.8": false},
	} {
		re := familyPattern(id)
		for other, want := range cases {
			if re.MatchString(other) != want {
				t.Errorf("%s / %s : %v attendu", id, other, want)
			}
		}
	}
	if familyPattern("Git.Git") != nil {
		t.Error("un identifiant sans version n'a pas de famille")
	}
}
