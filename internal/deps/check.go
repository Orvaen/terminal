package deps

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// State : où en est un outil sur ce PC.
type State int

const (
	Missing   State = iota // absent
	Outdated               // présent, mais plus ancien que la version demandée
	OK                     // présent et assez récent
	NoPackage              // absent, et apt ne sait pas l'installer (voir Note)
)

// Status : un besoin du projet et ce qui est installé.
type Status struct {
	Need
	State   State
	Have    string // version installée ("" si inconnue)
	Command string // programme trouvé
}

// Hooks remplaçables dans les tests.
var (
	// LookPath trouve un programme.
	LookPath = exec.LookPath
	// VersionOf lance « cmd --version » (ou équivalent) et renvoie la sortie.
	VersionOf = func(path string, args ...string) string {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		out, _ := exec.CommandContext(ctx, path, args...).CombinedOutput()
		return string(out)
	}
	// DockerInstalled : le Docker léger du terminal (branché par apt).
	DockerInstalled = func() bool { return false }
)

// versionArgs : comment demander sa version à chaque programme.
var versionArgs = map[string][]string{
	"go": {"version"}, "java": {"-version"}, "terraform": {"version"}, "erl": {"-noshell", "-eval",
		"io:fwrite(\"~s\", [erlang:system_info(otp_release)]), halt()."}, "dart": {"--version"},
	"elixir": {"--version"}, "helm": {"version", "--short"},
}

// isStoreStub : le faux python.exe de Windows, qui ouvre le Microsoft Store.
func isStoreStub(path string) bool {
	return strings.Contains(strings.ToLower(strings.ReplaceAll(path, `\`, "/")), "/microsoft/windowsapps/")
}

// Check vérifie chaque besoin : installé ? en quelle version ? assez récent ?
func Check(needs []Need) []Status {
	out := make([]Status, len(needs))
	for i, n := range needs {
		st := Status{Need: n, State: Missing}
		if n.Key == "docker" && DockerInstalled() {
			st.State, st.Have, st.Command = OK, "Docker léger", "docker"
			out[i] = st
			continue
		}
		for _, c := range n.Commands {
			p, err := LookPath(c)
			if err != nil || isStoreStub(p) {
				continue
			}
			st.Command, st.State = c, OK
			args, ok := versionArgs[c]
			if !ok {
				args = []string{"--version"}
			}
			st.Have = extractVersion(VersionOf(p, args...))
			break
		}
		switch {
		case st.State == Missing && n.Package == "":
			st.State = NoPackage
		case st.State == OK && n.Version != "" && st.Have != "" && compareVersions(st.Have, n.Version) < 0:
			st.State = Outdated
		}
		out[i] = st
	}
	return out
}

// extractVersion trouve la version dans la sortie d'un programme : « v24.1.0 »,
// « go version go1.23.1 windows/amd64 », « openjdk version "21.0.2" », « Python 3.12.1 ».
func extractVersion(out string) string {
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(strings.ToLower(line), "warning") {
			continue
		}
		if v := reVersionToken.FindString(line); v != "" {
			return v
		}
	}
	return ""
}

// compareVersions compare deux versions « 1.22.3 » partie par partie ; une partie absente
// ne compte pas (« 20 » est satisfait par 20.11.0).
func compareVersions(a, b string) int {
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	n := min(len(pa), len(pb))
	for i := 0; i < n; i++ {
		x, _ := strconv.Atoi(pa[i])
		y, _ := strconv.Atoi(pb[i])
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}

// PackageFor : le paquet apt qui fournit la version demandée (JDK 17 → openjdk-17-jdk…).
func PackageFor(n Need) string {
	major := n.Version
	if i := strings.Index(major, "."); i >= 0 {
		major = major[:i]
	}
	switch n.Key {
	case "java":
		switch major {
		case "17":
			return "openjdk-17-jdk"
		case "21":
			return "openjdk-21-jdk"
		}
	case "dotnet":
		switch major {
		case "8", "9", "10":
			return "dotnet-sdk-" + major + ".0"
		}
	case "php":
		if strings.HasPrefix(n.Version, "8.4") {
			return "php8.4"
		}
	}
	return n.Package
}
