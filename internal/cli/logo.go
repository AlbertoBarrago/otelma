package cli

import (
	"fmt"
	"os"
	"time"
)

// logoLines is a hand-drawn block-letter wordmark for "OTELMA", generated
// programmatically from a small pixel font and verified visually before
// being hardcoded here (see the project's dev notes), rather than typed
// character-by-character where a typo could go unnoticed.
var logoLines = []string{
	"█████  █████  █████  █      █   █   ███ ",
	"█   █    █    █      █      ██ ██  █   █",
	"█   █    █    ████   █      █ █ █  █████",
	"█   █    █    █      █      █   █  █   █",
	"█████    █    █████  █████  █   █  █   █",
	"",
	"  local LLM inference runtime",
}

// printLogo reveals logoLines one at a time when stdout is an interactive
// terminal, and prints them instantly otherwise (piped output, CI, tests)
// so scripting `otelma` output never has to wait on an animation.
func printLogo() {
	if !isTerminal(os.Stdout) {
		for _, l := range logoLines {
			fmt.Println(l)
		}
		return
	}
	for _, l := range logoLines {
		fmt.Println(l)
		time.Sleep(35 * time.Millisecond)
	}
}

func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
