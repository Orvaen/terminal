package setup

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

const exe = `C:\Users\Rayan\AppData\Local\Programs\apt-windows\terminal.exe`

func TestAddProfileKeepsSettingsAndOrder(t *testing.T) {
	in := []byte(`{
    "editor.fontSize": 14,
    "terminal.integrated.profiles.windows": {
        "Git Bash": { "path": "C:\\Program Files\\Git\\bin\\bash.exe" }
    },
    "workbench.colorTheme": "Default Dark Modern"
}`)
	out, changed, err := AddProfile(in, exe)
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("JSON invalide : %v\n%s", err, out)
	}
	profiles := got[profilesKey].(map[string]any)
	if profiles["Git Bash"] == nil {
		t.Error("le profil existant doit être conservé")
	}
	if p := profiles[ProfileName].(map[string]any); p["path"] != exe {
		t.Errorf("chemin : %v", p["path"])
	}
	if v, ok := profiles[dockerProfile]; !ok || v != nil {
		t.Error("la machine Docker doit être masquée (null)")
	}
	s := string(out)
	if !(strings.Index(s, "editor.fontSize") < strings.Index(s, profilesKey) && strings.Index(s, profilesKey) < strings.Index(s, "workbench.colorTheme")) {
		t.Errorf("l'ordre des réglages doit être conservé :\n%s", s)
	}

	// deuxième passage : rien ne change
	if _, changed, _ := AddProfile(out, exe); changed {
		t.Error("le second passage ne doit rien modifier")
	}
}

func TestAddProfileEmptyAndComments(t *testing.T) {
	out, changed, err := AddProfile(nil, exe)
	if err != nil || !changed || !json.Valid(out) {
		t.Fatalf("fichier vide : %v %v\n%s", changed, err, out)
	}
	_, _, err = AddProfile([]byte("{\n  // mon réglage\n  \"editor.fontSize\": 14,\n}"), exe)
	if !errors.Is(err, ErrNotPlainJSON) {
		t.Fatalf("un fichier avec commentaires ne doit pas être réécrit : %v", err)
	}
	if !json.Valid([]byte("{" + Snippet(exe) + "}")) {
		t.Fatal("l'extrait à copier doit être du JSON valide")
	}
}
