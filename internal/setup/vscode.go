// Package setup intègre le terminal aux autres outils (profil de terminal de VS Code).
package setup

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const (
	profilesKey   = "terminal.integrated.profiles.windows"
	ProfileName   = "Orvaen Terminal"
	dockerProfile = "orvaen-docker (WSL)" // machine interne du Docker léger, détectée par VS Code
)

// ErrNotPlainJSON : le fichier contient des commentaires ou des virgules finales,
// on ne le réécrit pas pour ne rien perdre.
var ErrNotPlainJSON = errors.New("settings.json contient des commentaires")

type member struct {
	key string
	val json.RawMessage
}

// readObject lit un objet JSON en gardant l'ordre de ses clés.
func readObject(data []byte) ([]member, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return nil, errors.New("objet JSON attendu")
	}
	var out []member
	for dec.More() {
		kt, err := dec.Token()
		if err != nil {
			return nil, err
		}
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return nil, err
		}
		out = append(out, member{kt.(string), raw})
	}
	if _, err := dec.Token(); err != nil {
		return nil, err
	}
	return out, nil
}

func set(obj []member, key string, val json.RawMessage) []member {
	for i := range obj {
		if obj[i].key == key {
			obj[i].val = val
			return obj
		}
	}
	return append(obj, member{key, val})
}

func writeObject(obj []member, indent string) []byte {
	var b bytes.Buffer
	b.WriteString("{")
	for i, m := range obj {
		if i > 0 {
			b.WriteString(",")
		}
		k, _ := json.Marshal(m.key)
		var v bytes.Buffer
		if err := json.Indent(&v, m.val, indent+"\t", "\t"); err != nil {
			v.Write(m.val)
		}
		b.WriteString("\n" + indent + "\t" + string(k) + ": " + v.String())
	}
	if len(obj) > 0 {
		b.WriteString("\n" + indent)
	}
	b.WriteString("}")
	return b.Bytes()
}

// AddProfile ajoute le profil « Orvaen Terminal » et masque la machine Docker dans un settings.json.
func AddProfile(settings []byte, exe string) ([]byte, bool, error) {
	if len(bytes.TrimSpace(settings)) == 0 {
		settings = []byte("{}")
	}
	if !json.Valid(settings) {
		return nil, false, ErrNotPlainJSON
	}
	root, err := readObject(settings)
	if err != nil {
		return nil, false, err
	}
	var profiles []member
	for _, m := range root {
		if m.key == profilesKey && string(bytes.TrimSpace(m.val)) != "null" {
			if profiles, err = readObject(m.val); err != nil {
				return nil, false, fmt.Errorf("%s : %w", profilesKey, err)
			}
		}
	}
	entry, _ := json.Marshal(map[string]any{"path": exe, "icon": "terminal"})
	profiles = set(profiles, ProfileName, entry)
	profiles = set(profiles, dockerProfile, json.RawMessage("null"))

	root = set(root, profilesKey, writeObject(profiles, ""))
	out := append(writeObject(root, ""), '\n')

	var a, b bytes.Buffer
	_ = json.Compact(&a, settings)
	_ = json.Compact(&b, out)
	return out, a.String() != b.String(), nil
}

// Snippet : ce qu'il faut ajouter à la main quand le fichier contient des commentaires.
func Snippet(exe string) string {
	p, _ := json.Marshal(exe)
	return fmt.Sprintf(`"%s": {
    "%s": { "path": %s, "icon": "terminal" },
    "%s": null
}`, profilesKey, ProfileName, p, dockerProfile)
}

// settingsFiles : les settings.json des éditeurs de la famille VS Code installés.
func settingsFiles() []string {
	appdata := os.Getenv("APPDATA")
	if appdata == "" {
		return nil
	}
	var out []string
	for _, app := range []string{"Code", "Code - Insiders", "VSCodium", "Cursor"} {
		user := filepath.Join(appdata, app, "User")
		if info, err := os.Stat(user); err == nil && info.IsDir() {
			out = append(out, filepath.Join(user, "settings.json"))
		}
	}
	return out
}

// VSCode ajoute le profil à chaque éditeur trouvé ; renvoie le code de sortie.
func VSCode(exe string, out io.Writer) int {
	files := settingsFiles()
	if len(files) == 0 {
		fmt.Fprintln(out, "VS Code n'est pas installé : rien à faire.")
		return 0
	}
	code := 0
	for _, path := range files {
		app := filepath.Base(filepath.Dir(filepath.Dir(path)))
		data, err := os.ReadFile(path)
		if err != nil && !os.IsNotExist(err) {
			fmt.Fprintf(out, "%s : lecture impossible (%v)\n", app, err)
			code = 1
			continue
		}
		updated, changed, err := AddProfile(data, exe)
		switch {
		case errors.Is(err, ErrNotPlainJSON):
			fmt.Fprintf(out, "%s : ton settings.json contient des commentaires, je ne le modifie pas.\n", app)
			fmt.Fprintf(out, "Ajoute ceci dans %s :\n\n%s\n\n", path, Snippet(exe))
			continue
		case err != nil:
			fmt.Fprintf(out, "%s : %v\n", app, err)
			code = 1
			continue
		case !changed:
			fmt.Fprintf(out, "%s : profil « %s » déjà présent.\n", app, ProfileName)
			continue
		}
		if len(data) > 0 {
			_ = os.WriteFile(path+".bak", data, 0o644)
		}
		if err := os.WriteFile(path, updated, 0o644); err != nil {
			fmt.Fprintf(out, "%s : écriture impossible (%v)\n", app, err)
			code = 1
			continue
		}
		fmt.Fprintf(out, "%s : profil « %s » ajouté au terminal intégré.\n", app, ProfileName)
	}
	return code
}
