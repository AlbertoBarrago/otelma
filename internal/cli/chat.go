package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func runChatCommand(c *client, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: otelma chat <name>")
	}
	return runChat(c, args[0])
}

// runChat is an interactive, multi-turn REPL: it keeps the full transcript
// in memory and sends it whole on every turn, so the model sees prior
// context instead of each message being a fresh, unrelated request.
func runChat(c *client, name string) error {
	fmt.Printf("chatting with %s (Ctrl+D or /exit to quit, /clear to reset context)\n", name)

	var history []chatMessage
	scanner := bufio.NewScanner(os.Stdin)

	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			fmt.Println()
			return scanner.Err()
		}

		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		switch line {
		case "/exit", "/quit":
			return nil
		case "/clear":
			history = nil
			fmt.Println("(context cleared)")
			continue
		}

		history = append(history, chatMessage{Role: "user", Content: line})

		var out struct {
			Output string `json:"output"`
		}
		if err := c.post("/api/run", map[string]any{"name": name, "messages": history}, &out); err != nil {
			// Drop the user turn we couldn't get a reply to, so a retry
			// doesn't duplicate it in the transcript sent next time.
			history = history[:len(history)-1]
			fmt.Fprintln(os.Stderr, "otelma:", err)
			continue
		}

		fmt.Println(out.Output)
		history = append(history, chatMessage{Role: "assistant", Content: out.Output})
	}
}
