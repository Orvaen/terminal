package update

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// checkEvery : délai entre deux vérifications automatiques au démarrage du terminal.
const checkEvery = 24 * time.Hour

type checkState struct {
	Latest  string    `json:"latest"`
	Checked time.Time `json:"checked"`
}

// StatePath : fichier qui garde le résultat de la dernière vérification (remplacé dans les tests).
var StatePath = func() string {
	base, err := os.UserCacheDir() // %LOCALAPPDATA% sous Windows
	if err != nil {
		base = os.TempDir()
	}
	return filepath.Join(base, "OrvaenTerminal", "update.json")
}

func loadCheck() checkState {
	var st checkState
	if b, err := os.ReadFile(StatePath()); err == nil {
		_ = json.Unmarshal(b, &st)
	}
	return st
}

func saveCheck(latest string) {
	p := StatePath()
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	b, _ := json.Marshal(checkState{Latest: latest, Checked: time.Now()})
	_ = os.WriteFile(p, b, 0o644)
}

// Available renvoie la dernière version connue si elle diffère de current, d'après la dernière
// vérification enregistrée. Ne contacte pas le réseau : instantané au démarrage.
func Available(current string) (string, bool) {
	if IsDev(current) {
		return "", false
	}
	st := loadCheck()
	if st.Latest == "" || st.Latest == current {
		return "", false
	}
	return st.Latest, true
}

// RefreshInBackground vérifie la dernière release si la vérification précédente date de plus
// d'un jour. Le résultat sert à la prochaine ouverture du terminal ; rien n'est affiché.
func RefreshInBackground(current string) {
	if IsDev(current) || time.Since(loadCheck().Checked) < checkEvery {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if rel, err := Latest(ctx); err == nil {
			saveCheck(rel.Tag)
		}
	}()
}

// Check interroge GitHub tout de suite (pour apt update) et mémorise le résultat.
func Check(current string, timeout time.Duration) (string, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	rel, err := Latest(ctx)
	if err != nil {
		return "", false
	}
	saveCheck(rel.Tag)
	return rel.Tag, rel.Tag != current
}
