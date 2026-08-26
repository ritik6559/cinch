package tui

import (
	"fmt"
	"math/rand/v2"
	"time"
)

var gerunds = []string{
	"Pondering", "Noodling", "Cerebrating", "Percolating", "Ruminating",
	"Whirring", "Tinkering", "Marinating", "Puzzling", "Simmering",
	"Mulling", "Cogitating", "Untangling", "Rummaging", "Deliberating",
	"Brewing", "Scheming", "Wrangling", "Fossicking", "Contemplating",
	"Chewing", "Spelunking", "Divining", "Reticulating", "Distilling",
	"Assembling", "Considering", "Excavating", "Weighing", "Sifting",
	"Conjuring", "Parsing", "Threading", "Wondering", "Reckoning",
	"Grokking", "Circling", "Hatching", "Sketching", "Winnowing",
}

var glyphs = []string{"✻", "✽", "✢", "·", "✢", "✽"}

const (
	tickEvery = 400 * time.Millisecond
	wordEvery = 4 * time.Second
)

type working struct {
	started time.Time
	word    string
	changed time.Time
	frame   int
}

func newWorking() working {
	now := time.Now()
	return working{
		started: now,
		word:    gerunds[rand.IntN(len(gerunds))],
		changed: now,
	}
}

func (w *working) tick() {
	w.frame++

	if time.Since(w.changed) < wordEvery {
		return
	}
	for {
		next := gerunds[rand.IntN(len(gerunds))]
		if next != w.word {
			w.word = next
			break
		}
	}
	w.changed = time.Now()
}

func (w working) line(t Theme, tokens int) string {
	glyph := glyphs[w.frame%len(glyphs)]
	elapsed := int(time.Since(w.started).Seconds())

	detail := fmt.Sprintf("(%ds", elapsed)
	if tokens > 0 {
		detail += fmt.Sprintf(" · ↑ %s tokens", compactTokens(tokens))
	}
	detail += " · esc to interrupt)"

	return t.Working.Render(glyph+" "+w.word+"… ") + t.Hint.Render(detail)
}
