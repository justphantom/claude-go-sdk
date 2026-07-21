// Command basic runs a single Claude turn and prints the event stream.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	claude "github.com/justphantom/claude-go-sdk"
)

func main() {
	prompt := strings.Join(os.Args[1:], " ")
	if prompt == "" {
		fmt.Fprintln(os.Stderr, "usage: basic <prompt>")
		os.Exit(2)
	}

	c := claude.New(claude.Options{})
	ch, err := c.Run(context.Background(), claude.RunOptions{Prompt: prompt})
	if err != nil {
		fmt.Fprintln(os.Stderr, "run:", err)
		os.Exit(1)
	}
	for ev := range ch {
		switch ev.Type {
		case claude.EventText:
			fmt.Print(ev.Text)
		case claude.EventResult:
			fmt.Printf("\n[result] %s\n", ev.Result)
		case claude.EventError:
			fmt.Fprintln(os.Stderr, "error:", ev.Text)
			os.Exit(1)
		default:
			fmt.Printf("[%s] %s\n", ev.Type, ev.Text)
		}
	}
}
