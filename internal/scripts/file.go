// Package scripts implémente « run » : des scripts de projet décrits dans terminal.json,
// comme les « scripts » de package.json, avec en plus des commandes lancées en parallèle
// (à la place de concurrently).
//
//	{
//	  "scripts": {
//	    "up":    "make up",
//	    "check": ["make up", "make test f=00"],
//	    "api":   { "run": "npm run dev", "cwd": "api", "env": { "PORT": "3000" } },
//	    "dev":   { "parallel": ["run api", "make logs"] }
//	  }
//	}
package scripts

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// FileName : nom du fichier des scripts, cherché dans le dossier courant puis dans ses parents.
const FileName = "terminal.json"

// Script : une entrée de « scripts ».
type Script struct {
	Name        string
	Description string
	Cwd         string            // relatif au dossier de terminal.json
	Env         map[string]string // variables ajoutées
	Steps       []string          // commandes lancées l'une après l'autre
	Parallel    []string          // commandes lancées en même temps
}

// File : un terminal.json chargé, scripts dans l'ordre du fichier.
type File struct {
	Path    string
	Dir     string
	Scripts []Script
}

// Get renvoie le script nommé name.
func (f *File) Get(name string) (Script, bool) {
	for _, s := range f.Scripts {
		if s.Name == name {
			return s, true
		}
	}
	return Script{}, false
}

// Names renvoie les noms des scripts, dans l'ordre du fichier.
func (f *File) Names() []string {
	names := make([]string, len(f.Scripts))
	for i, s := range f.Scripts {
		names[i] = s.Name
	}
	return names
}

// ErrNotFound : aucun terminal.json dans le dossier ni dans ses parents.
var ErrNotFound = errors.New(FileName + " introuvable dans ce dossier ni dans ses parents")

// Find cherche terminal.json à partir de dir, en remontant jusqu'à la racine du disque.
func Find(dir string) (string, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for {
		p := filepath.Join(dir, FileName)
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", ErrNotFound
		}
		dir = parent
	}
}

// Load lit et vérifie un terminal.json.
func Load(path string) (*File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	f, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("%s : %w", path, err)
	}
	f.Path, f.Dir = path, filepath.Dir(path)
	return f, nil
}

// Parse analyse le contenu d'un terminal.json.
func Parse(data []byte) (*File, error) {
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf")) // BOM laissé par certains éditeurs Windows
	var top struct {
		Scripts json.RawMessage `json:"scripts"`
	}
	if err := json.Unmarshal(data, &top); err != nil {
		return nil, fmt.Errorf("JSON invalide : %w", err)
	}
	if len(top.Scripts) == 0 {
		return nil, errors.New(`pas de section "scripts"`)
	}
	f := &File{}
	err := orderedObject(top.Scripts, func(name string, raw json.RawMessage) error {
		if strings.TrimSpace(name) == "" || strings.ContainsAny(name, " \t") {
			return fmt.Errorf("nom de script invalide : %q (pas d'espace)", name)
		}
		if _, dup := f.Get(name); dup {
			return fmt.Errorf("le script %q est défini deux fois", name)
		}
		s, err := parseScript(name, raw)
		if err != nil {
			return err
		}
		f.Scripts = append(f.Scripts, s)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return f, nil
}

func parseScript(name string, raw json.RawMessage) (Script, error) {
	s := Script{Name: name}
	bad := func(msg string) (Script, error) {
		return Script{}, fmt.Errorf("script %q : %s", name, msg)
	}
	switch firstByte(raw) {
	case '"', '[':
		steps, err := commands(raw)
		if err != nil {
			return bad(err.Error())
		}
		s.Steps = steps
		return s, nil
	case '{':
		var obj struct {
			Run         json.RawMessage   `json:"run"`
			Parallel    json.RawMessage   `json:"parallel"`
			Cwd         string            `json:"cwd"`
			Env         map[string]string `json:"env"`
			Description string            `json:"description"`
		}
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&obj); err != nil {
			return bad(strings.TrimPrefix(err.Error(), "json: "))
		}
		s.Cwd, s.Env, s.Description = obj.Cwd, obj.Env, obj.Description
		switch {
		case obj.Run != nil && obj.Parallel != nil:
			return bad(`"run" et "parallel" ne vont pas ensemble : choisis l'un des deux`)
		case obj.Run != nil:
			steps, err := commands(obj.Run)
			if err != nil {
				return bad(`"run" : ` + err.Error())
			}
			s.Steps = steps
		case obj.Parallel != nil:
			cmds, err := commands(obj.Parallel)
			if err != nil {
				return bad(`"parallel" : ` + err.Error())
			}
			s.Parallel = cmds
		default:
			return bad(`il faut "run" (commandes l'une après l'autre) ou "parallel" (en même temps)`)
		}
		return s, nil
	}
	return bad("attendu une commande, une liste de commandes ou un objet { \"run\": … }")
}

// commands accepte "commande" ou ["commande", ...].
func commands(raw json.RawMessage) ([]string, error) {
	var one string
	if err := json.Unmarshal(raw, &one); err == nil {
		if strings.TrimSpace(one) == "" {
			return nil, errors.New("commande vide")
		}
		return []string{one}, nil
	}
	var list []string
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, errors.New("attendu une commande ou une liste de commandes (texte)")
	}
	if len(list) == 0 {
		return nil, errors.New("liste vide")
	}
	for _, c := range list {
		if strings.TrimSpace(c) == "" {
			return nil, errors.New("commande vide dans la liste")
		}
	}
	return list, nil
}

func firstByte(raw json.RawMessage) byte {
	t := bytes.TrimSpace(raw)
	if len(t) == 0 {
		return 0
	}
	return t[0]
}

// orderedObject parcourt un objet JSON en gardant l'ordre des clés.
func orderedObject(raw json.RawMessage, fn func(key string, value json.RawMessage) error) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return errors.New(`"scripts" doit être un objet { "nom": "commande" }`)
	}
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		key, _ := tok.(string)
		var val json.RawMessage
		if err := dec.Decode(&val); err != nil {
			return err
		}
		if err := fn(key, val); err != nil {
			return err
		}
	}
	_, err = dec.Token()
	if err == io.EOF {
		err = nil
	}
	return err
}
