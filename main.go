// Stencil — structural code matching and generation for Go.
//
// Usage:
//
//	stencil parse   <file.lift>    Validate a .lift file
//	stencil inspect <file.lift>    Parse and display structure as JSON
//	stencil version                Show version
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/vinodhalaharvi/stencil/grammar"
)

const version = "0.1.0"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "parse":
		cmdParse(os.Args[2:])
	case "inspect":
		cmdInspect(os.Args[2:])
	case "version":
		fmt.Printf("stencil v%s\n", version)
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`stencil — structural code matching and generation for Go

Usage:
  stencil parse   <file.lift>    Validate a .lift file
  stencil inspect <file.lift>    Parse and display structure
  stencil version                Show version
  stencil help                   Show this message

Examples:
  stencil parse examples/entity-service.lift
  stencil inspect examples/enforce-ctx-timeout.lift`)
}

func cmdParse(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "error: parse requires a .lift file path")
		os.Exit(1)
	}

	parser, err := grammar.NewParser()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to build parser: %v\n", err)
		os.Exit(1)
	}

	for _, path := range args {
		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}

		prog, err := parser.ParseString(path, string(data))
		if err != nil {
			fmt.Fprintf(os.Stderr, "✗ %s\n  %v\n", path, err)
			os.Exit(1)
		}

		fmt.Printf("✓ %s — %d lift block(s)\n", path, len(prog.Blocks))
		for _, b := range prog.Blocks {
			matchers := 0
			if b.From != nil {
				matchers = len(b.From.Matchers)
			}
			fmt.Printf("  %s: %d matcher(s), %d where(s), %d action(s)\n",
				b.Name, matchers, len(b.Where), len(b.Actions))
		}
	}
}

func cmdInspect(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "error: inspect requires a .lift file path")
		os.Exit(1)
	}

	parser, err := grammar.NewParser()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to build parser: %v\n", err)
		os.Exit(1)
	}

	path := args[0]
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	prog, err := parser.ParseString(path, string(data))
	if err != nil {
		fmt.Fprintf(os.Stderr, "✗ %s\n  %v\n", path, err)
		os.Exit(1)
	}

	out, _ := json.MarshalIndent(prog, "", "  ")
	fmt.Println(string(out))
}
