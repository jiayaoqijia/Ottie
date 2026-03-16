package channels

import (
	"math/rand"
)

// thinkingPhrases provides varied placeholder text for the "thinking" state,
// inspired by Claude Code's whimsical gerund spinner verbs.
var thinkingPhrases = []string{
	"Thinking...",
	"Pondering...",
	"Reasoning...",
	"Analyzing...",
	"Computing...",
	"Synthesizing...",
	"Orchestrating...",
	"Architecting...",
	"Crafting...",
	"Deciphering...",
	"Envisioning...",
	"Crystallizing...",
	"Calculating...",
	"Brainstorming...",
	"Contemplating...",
	"Investigating...",
	"Assembling...",
	"Conjuring...",
	"Distilling...",
	"Formulating...",
	"Percolating...",
	"Simmering...",
	"Untangling...",
	"Calibrating...",
	"Illuminating...",
	"Navigating...",
	"Bootstrapping...",
	"Spelunking...",
	"Moonwalking...",
	"Shenaniganing...",
}

// RandomThinkingPhrase returns a random thinking placeholder string.
func RandomThinkingPhrase() string {
	return thinkingPhrases[rand.Intn(len(thinkingPhrases))]
}
