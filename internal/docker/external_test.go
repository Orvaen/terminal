package docker

import (
	"strings"
	"testing"
)

func envOf(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestExternalEngine(t *testing.T) {
	cases := []struct {
		env  map[string]string
		want string
	}{
		{map[string]string{}, ""},
		{map[string]string{"DOCKER_HOST": "  "}, ""},
		{map[string]string{"DOCKER_CONTEXT": "default"}, ""},
		{map[string]string{"DOCKER_CONTEXT": "Default"}, ""},
		{map[string]string{"DOCKER_HOST": "npipe:////./pipe/docker_engine"}, "npipe:////./pipe/docker_engine"},
		{map[string]string{"DOCKER_CONTEXT": "desktop-linux"}, "contexte desktop-linux"},
		{map[string]string{"DOCKER_HOST": "tcp://srv:2376", "DOCKER_CONTEXT": "x"}, "tcp://srv:2376"},
	}
	for _, c := range cases {
		if got := externalEngine(envOf(c.env)); got != c.want {
			t.Errorf("externalEngine(%v) = %q, attendu %q", c.env, got, c.want)
		}
	}
}

func TestExternalEnv(t *testing.T) {
	base := []string{"PATH=x", "DOCKER_HOST=tcp://srv:2376"}

	// DOCKER_HOST seul : config d'Orvaen pour compose et buildx, le reste intact.
	env := externalEnv(base, envOf(map[string]string{"DOCKER_HOST": "tcp://srv:2376"}))
	if !has(env, "DOCKER_HOST=tcp://srv:2376") || !hasPrefix(env, "DOCKER_CONFIG=") {
		t.Errorf("DOCKER_HOST seul : %v", env)
	}

	// Contexte : la config de l'utilisateur (qui contient le contexte) n'est pas remplacée.
	env = externalEnv([]string{"DOCKER_CONTEXT=desktop-linux"}, envOf(map[string]string{"DOCKER_CONTEXT": "desktop-linux"}))
	if hasPrefix(env, "DOCKER_CONFIG=") {
		t.Errorf("contexte : DOCKER_CONFIG ne doit pas être ajouté : %v", env)
	}

	// DOCKER_CONFIG fourni : gardé tel quel.
	env = externalEnv([]string{"DOCKER_CONFIG=C:\\cfg"}, envOf(map[string]string{"DOCKER_HOST": "h", "DOCKER_CONFIG": "C:\\cfg"}))
	if n := countPrefix(env, "DOCKER_CONFIG="); n != 1 {
		t.Errorf("DOCKER_CONFIG en double : %v", env)
	}
}

func has(env []string, kv string) bool {
	for _, e := range env {
		if e == kv {
			return true
		}
	}
	return false
}

func hasPrefix(env []string, p string) bool { return countPrefix(env, p) > 0 }

func countPrefix(env []string, p string) int {
	n := 0
	for _, e := range env {
		if strings.HasPrefix(e, p) {
			n++
		}
	}
	return n
}
