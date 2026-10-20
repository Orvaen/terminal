package ui

// Indicateur animé et barre de progression, façon pip :
//
//	⠹ Mise à jour de la source winget…
//	✓ winget                                  1,2 s
//	node-v24.22.0-x64.msi  ━━━━━━━━━━━━━━━━━━━━━━━━╺━━━━━━━━━━━━━━━  32,1/45,6 Mo  8,4 Mo/s  0:00:02
//
// Hors d'une console (sortie redirigée, script), rien n'est animé : une seule ligne à la fin.

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// Out : où s'affichent les indicateurs (remplacé dans les tests).
var Out io.Writer = os.Stdout

// Interactive : la sortie est une console capable d'afficher les animations (remplacée dans les tests).
var Interactive = func() bool {
	if !color {
		return false
	}
	info, err := os.Stdout.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// Spinner : un indicateur animé sur une ligne, remplacé par un résultat à la fin.
type Spinner struct {
	mu      sync.Mutex
	msg     string
	done    chan struct{}
	stopped chan struct{}
	live    bool
	start   time.Time
}

// StartSpinner affiche msg avec une animation jusqu'à Done ou Fail.
func StartSpinner(msg string) *Spinner {
	s := &Spinner{msg: msg, done: make(chan struct{}), stopped: make(chan struct{}), live: Interactive(), start: time.Now()}
	if !s.live {
		close(s.stopped)
		return s
	}
	go func() {
		defer close(s.stopped)
		t := time.NewTicker(80 * time.Millisecond)
		defer t.Stop()
		for i := 0; ; i++ {
			s.mu.Lock()
			fmt.Fprintf(Out, "\r\x1b[2K%s %s", paint("36", spinnerFrames[i%len(spinnerFrames)]), s.msg)
			s.mu.Unlock()
			select {
			case <-s.done:
				return
			case <-t.C:
			}
		}
	}()
	return s
}

// Update change le texte affiché.
func (s *Spinner) Update(msg string) {
	s.mu.Lock()
	s.msg = msg
	s.mu.Unlock()
}

// Println affiche une ligne au-dessus de l'animation, qui continue en dessous.
func (s *Spinner) Println(line string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.live {
		fmt.Fprint(Out, "\r\x1b[2K")
	}
	fmt.Fprintln(Out, line)
}

// Elapsed : le temps écoulé depuis le début.
func (s *Spinner) Elapsed() time.Duration { return time.Since(s.start) }

func (s *Spinner) stop(line string) {
	if s.live {
		close(s.done)
		<-s.stopped
		s.mu.Lock()
		fmt.Fprint(Out, "\r\x1b[2K")
		s.mu.Unlock()
	}
	if line != "" {
		fmt.Fprintln(Out, line)
	}
}

// Done remplace l'animation par « ✓ texte ».
func (s *Spinner) Done(text string) { s.stop(paint("32", "✓") + " " + text) }

// Fail remplace l'animation par « ✗ texte ».
func (s *Spinner) Fail(text string) { s.stop(paint("31", "✗") + " " + text) }

// Clear retire l'animation sans rien afficher.
func (s *Spinner) Clear() { s.stop("") }

// ---------------------------------------------------------------------------
// Barre de progression

const barWidth = 36

// Bar suit un téléchargement : à utiliser comme io.Writer (avec io.MultiWriter) ou via Reader.
type Bar struct {
	label     string
	total     int64
	done      int64
	start     time.Time
	lastDraw  time.Time
	live      bool
	finished  bool
	mu        sync.Mutex
	lastWidth int
}

// NewBar crée une barre pour total octets (total ≤ 0 : taille inconnue).
func NewBar(label string, total int64) *Bar {
	b := &Bar{label: label, total: total, start: time.Now(), live: Interactive()}
	if b.live {
		b.draw()
	}
	return b
}

// Write compte les octets reçus.
func (b *Bar) Write(p []byte) (int, error) {
	b.mu.Lock()
	b.done += int64(len(p))
	if b.live && time.Since(b.lastDraw) >= 80*time.Millisecond {
		b.draw()
	}
	b.mu.Unlock()
	return len(p), nil
}

// Reader enveloppe r pour faire avancer la barre à chaque lecture.
func (b *Bar) Reader(r io.Reader) io.Reader { return io.TeeReader(r, b) }

func (b *Bar) draw() {
	b.lastDraw = time.Now()
	fmt.Fprint(Out, "\r\x1b[2K"+b.line(false))
}

// line : « label  ━━━━━╺━━━━  12,3/45,6 Mo  3,4 Mo/s  0:00:12 ».
func (b *Bar) line(final bool) string {
	elapsed := time.Since(b.start).Seconds()
	speed := 0.0
	if elapsed > 0 {
		speed = float64(b.done) / elapsed
	}
	label := fitLabel(b.label, 28)
	var bar string
	switch {
	case final:
		bar = paint("32", strings.Repeat("━", barWidth))
	case b.total > 0:
		frac := float64(b.done) / float64(b.total)
		if frac > 1 {
			frac = 1
		}
		full := int(frac * barWidth)
		bar = paint("35", strings.Repeat("━", full))
		if full < barWidth {
			bar += paint("35", "╸") + paint("90", strings.Repeat("━", barWidth-full-1))
		}
	default:
		// taille inconnue : un segment qui se déplace
		pos := int(elapsed*12) % barWidth
		bar = paint("90", strings.Repeat("━", pos)) + paint("35", "━━━") + paint("90", strings.Repeat("━", max(0, barWidth-pos-3)))
	}
	size := HumanBytes(b.done)
	if b.total > 0 {
		size = HumanBytes(b.done) + "/" + HumanBytes(b.total)
	}
	tail := fmt.Sprintf("%s  %s/s", size, HumanBytes(int64(speed)))
	if final {
		tail = fmt.Sprintf("%s  %s", HumanBytes(b.done), paint("2", formatDuration(time.Since(b.start))))
	} else if b.total > 0 && speed > 0 {
		eta := time.Duration(float64(b.total-b.done)/speed) * time.Second
		tail += "  " + paint("36", formatETA(eta))
	}
	return label + "  " + bar + "  " + tail
}

// Finish affiche la barre complète (ou rien d'animé hors console : une ligne de résultat).
func (b *Bar) Finish() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.finished {
		return
	}
	b.finished = true
	if b.live {
		fmt.Fprint(Out, "\r\x1b[2K")
	}
	fmt.Fprintln(Out, b.line(true))
}

// Abort efface la barre après une erreur.
func (b *Bar) Abort() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.finished {
		return
	}
	b.finished = true
	if b.live {
		fmt.Fprint(Out, "\r\x1b[2K")
	}
}

func fitLabel(s string, width int) string {
	n := utf8.RuneCountInString(s)
	if n > width {
		r := []rune(s)
		return string(r[:width-1]) + "…"
	}
	return s + strings.Repeat(" ", width-n)
}

// HumanBytes : 1536 → « 1,5 Ko », 45 600 000 → « 43,5 Mo ».
func HumanBytes(n int64) string {
	units := []string{"o", "Ko", "Mo", "Go", "To"}
	f := float64(n)
	i := 0
	for f >= 1024 && i < len(units)-1 {
		f /= 1024
		i++
	}
	if i == 0 {
		return fmt.Sprintf("%d %s", n, units[0])
	}
	return strings.Replace(fmt.Sprintf("%.1f %s", f, units[i]), ".", ",", 1)
}

func formatETA(d time.Duration) string {
	s := int(d.Seconds())
	return fmt.Sprintf("%d:%02d:%02d", s/3600, s/60%60, s%60)
}

func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%d ms", d.Milliseconds())
	}
	return strings.Replace(fmt.Sprintf("%.1f s", d.Seconds()), ".", ",", 1)
}

// FormatDuration : « 340 ms », « 1,2 s ».
func FormatDuration(d time.Duration) string { return formatDuration(d) }
