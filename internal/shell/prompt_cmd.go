package shell

import (
	"fmt"
	"strings"
)

// runPrompt : prompt | prompt <thème> | prompt perso "<format>" | prompt aide
func runPrompt(s *Shell, args []string) int {
	cfg := loadPromptConfig()
	current := cfg.Theme
	if current == "" {
		current = DefaultTheme
	}
	ctx := s.context()

	if len(args) == 0 {
		fmt.Fprintln(s.out, "Styles de prompt (aperçu avec ton dossier actuel) :")
		for _, t := range Themes {
			mark := "  "
			if strings.EqualFold(t.Name, current) {
				mark = s.paint("1;32", "● ")
			}
			fmt.Fprintf(s.out, "\n%s%s — %s\n", mark, s.paint("1", t.Name), t.Description)
			fmt.Fprint(s.out, indentPreview(renderPrompt(t.Format, ctx)))
		}
		if current == "perso" && cfg.Format != "" {
			fmt.Fprintf(s.out, "\n%s%s — ton format : %s\n", s.paint("1;32", "● "), s.paint("1", "perso"), cfg.Format)
			fmt.Fprint(s.out, indentPreview(renderPrompt(cfg.Format, ctx)))
		}
		fmt.Fprintln(s.out, "\nPour choisir : prompt <nom>      Pour créer le tien : prompt perso \"<format>\"   (prompt aide)")
		return 0
	}

	switch strings.ToLower(args[0]) {
	case "aide", "help", "--help", "-h":
		fmt.Fprint(s.out, promptHelp)
		return 0
	case "perso", "custom":
		if len(args) < 2 {
			s.errorf("usage : prompt perso \"{cyan}{dossier}{/} {symbole-fleche}\"   (voir prompt aide)")
			return 1
		}
		format := strings.Join(args[1:], " ")
		if err := savePromptConfig(promptConfig{Theme: "perso", Format: format}); err != nil {
			s.errorf("prompt: %v", err)
			return 1
		}
		s.promptFmt = format
		fmt.Fprintln(s.out, "Ton prompt est enregistré. Aperçu :")
		fmt.Fprint(s.out, indentPreview(renderPrompt(format, ctx)))
		return 0
	}

	t, ok := themeByName(args[0])
	if !ok {
		var names []string
		for _, t := range Themes {
			names = append(names, t.Name)
		}
		s.errorf("prompt: style inconnu : %s (styles : %s, ou perso)", args[0], strings.Join(names, ", "))
		return 1
	}
	if err := savePromptConfig(promptConfig{Theme: t.Name}); err != nil {
		s.errorf("prompt: %v", err)
		return 1
	}
	s.promptFmt = t.Format
	fmt.Fprintf(s.out, "Style « %s » choisi, il est gardé pour les prochaines fenêtres.\n", t.Name)
	return 0
}

// indentPreview décale l'aperçu et termine par un retour à la ligne.
func indentPreview(p string) string {
	p = strings.TrimPrefix(p, "\n")
	lines := strings.Split(p, "\n")
	for i, l := range lines {
		lines[i] = "    " + l
	}
	return strings.Join(lines, "\n") + "\n"
}

const promptHelp = `Créer ton propre prompt : prompt perso "<format>"

Éléments
  {user} {host}       ton nom d'utilisateur, le nom du PC
  {dir}               le dossier façon Linux : ~/Desktop/projet
  {dossier}           seulement le nom du dossier : projet
  {chemin}            le chemin Windows : C:\Users\toi\Desktop\projet
  {git}               la branche Git (vide hors d'un dépôt)
  {heure}             l'heure (14:05)
  {code}              le code de sortie de la dernière commande, si elle a échoué
  {admin}             ADMIN en administrateur, sinon WIN
  {symbole}           $ (ou # en administrateur), vert ou rouge selon la dernière commande
  {symbole-fleche}    pareil avec ❯
  {nl}                retour à la ligne : la dernière ligne est celle où tu tapes

Couleurs
  {rouge} {vert} {jaune} {bleu} {violet} {cyan} {blanc} {gris} {gras}
  {fond-bleu} {fond-violet} {fond-vert} {fond-rouge} {fond-gris}
  {/}                 revient à la couleur normale

Parties facultatives
  [[ … ]]             affiché seulement si ses éléments ont une valeur :
                      [[ ({git})]] n'affiche rien hors d'un dépôt Git

Exemple
  prompt perso "{nl}{cyan}{dossier}{/}[[ {violet}{git}{/}]] {symbole-fleche}"
`
