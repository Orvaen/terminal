package req

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"os"
	"regexp"
	"strings"
	"time"
)

var placeholder = regexp.MustCompile(`\{\{\s*([^{}]+?)\s*\}\}`)

// VarError : variable inconnue dans un {{...}}.
type VarError struct{ Name string }

func (e *VarError) Error() string { return "variable inconnue {{" + e.Name + "}}" }

// expand remplace les {{variables}} ; les valeurs peuvent elles-mêmes contenir des variables.
func expand(s string, vars map[string]string) (string, error) {
	var firstErr error
	for depth := 0; depth < 10 && strings.Contains(s, "{{"); depth++ {
		changed := false
		s = placeholder.ReplaceAllStringFunc(s, func(m string) string {
			name := placeholder.FindStringSubmatch(m)[1]
			v, ok := lookup(name, vars)
			if !ok {
				if firstErr == nil {
					firstErr = &VarError{name}
				}
				return m
			}
			changed = true
			return v
		})
		if firstErr != nil || !changed {
			break
		}
	}
	return s, firstErr
}

func lookup(name string, vars map[string]string) (string, bool) {
	if v, ok := vars[name]; ok {
		return v, true
	}
	switch {
	case name == "$uuid" || name == "$guid":
		return uuid(), true
	case name == "$timestamp":
		return fmt.Sprint(time.Now().Unix()), true
	case name == "$isoTimestamp" || name == "$datetime":
		return time.Now().UTC().Format(time.RFC3339), true
	case name == "$randomInt":
		n, _ := rand.Int(rand.Reader, big.NewInt(1000))
		return n.String(), true
	case strings.HasPrefix(name, "$env."):
		return os.LookupEnv(strings.TrimPrefix(name, "$env."))
	case strings.HasPrefix(name, "$processEnv "): // syntaxe de REST Client
		return os.LookupEnv(strings.TrimSpace(strings.TrimPrefix(name, "$processEnv ")))
	}
	return "", false
}

func uuid() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}
