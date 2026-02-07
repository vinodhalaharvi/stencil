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
	"go/ast"
	"os"

	"github.com/vinodhalaharvi/stencil/executor"
	"github.com/vinodhalaharvi/stencil/grammar"
	"github.com/vinodhalaharvi/stencil/matcher"
)

const version = "0.3.0"

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
	case "match":
		cmdMatch(os.Args[2:])
	case "apply":
		cmdApply(os.Args[2:])
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
  stencil parse   <file.lift>                     Validate a .lift file
  stencil inspect <file.lift>                     Parse and display structure
  stencil match   <file.lift> --source <file.go>  Find matches in Go source
  stencil apply   <file.lift> --source <file.go>  Apply transformations
  stencil version                                 Show version
  stencil help                                    Show this message

Examples:
  stencil parse examples/entity-service.lift
  stencil inspect examples/enforce-ctx-timeout.lift
  stencil match examples/enforce-ctx-timeout.lift --source testdata/bad_http_client.go
  stencil apply examples/enforce-ctx-timeout.lift --source testdata/bad_http_client.go`)
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

// cmdMatch runs pattern matching against Go source files.
func cmdMatch(args []string) {
	if len(args) < 3 {
		fmt.Fprintln(os.Stderr, "error: match requires <file.lift> --source <file.go>")
		os.Exit(1)
	}

	liftPath := args[0]
	var sourcePath string

	// Parse --source flag
	for i := 1; i < len(args); i++ {
		if args[i] == "--source" && i+1 < len(args) {
			sourcePath = args[i+1]
			break
		}
	}

	if sourcePath == "" {
		fmt.Fprintln(os.Stderr, "error: --source flag required")
		os.Exit(1)
	}

	// Parse .lift file
	parser, err := grammar.NewParser()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to build parser: %v\n", err)
		os.Exit(1)
	}

	liftData, err := os.ReadFile(liftPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	prog, err := parser.ParseString(liftPath, string(liftData))
	if err != nil {
		fmt.Fprintf(os.Stderr, "✗ %s\n  %v\n", liftPath, err)
		os.Exit(1)
	}

	// Create matcher from Go source
	m, err := matcher.NewFromFile(sourcePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	// Run matching for each lift block
	totalMatches := 0
	for _, block := range prog.Blocks {
		matches, err := m.MatchBlock(block)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error matching block %s: %v\n", block.Name, err)
			continue
		}

		// Apply where filters
		matches = matcher.FilterMatches(matches, block.Where)

		if len(matches) == 0 {
			continue
		}

		fmt.Printf("Block %s: %d match(es)\n", block.Name, len(matches))
		for i, match := range matches {
			pos := m.FileSet().Position(match.Node.Pos())
			fmt.Printf("  [%d] %s:%d\n", i+1, pos.Filename, pos.Line)

			// Print key bindings
			for name, val := range match.Bindings {
				valStr := formatBinding(val)
				if valStr != "" {
					fmt.Printf("      $%s = %s\n", name, valStr)
				}
			}
		}
		totalMatches += len(matches)
	}

	if totalMatches == 0 {
		fmt.Println("No matches found.")
	} else {
		fmt.Printf("\nTotal: %d match(es)\n", totalMatches)
	}
}

// formatBinding formats a binding value for display.
func formatBinding(v any) string {
	if v == nil {
		return "<nil>"
	}

	switch val := v.(type) {
	case *ast.Ident:
		return val.Name
	case *ast.FuncType:
		return "<FuncType>"
	case *ast.BlockStmt:
		return "<BlockStmt>"
	case *ast.FieldList:
		if val == nil || val.List == nil {
			return "<FieldList(0)>"
		}
		return fmt.Sprintf("<FieldList(%d)>", len(val.List))
	default:
		return fmt.Sprintf("<%T>", v)
	}
}

// cmdApply applies transformations from a .lift file to Go source.
func cmdApply(args []string) {
	if len(args) < 3 {
		fmt.Fprintln(os.Stderr, "error: apply requires <file.lift> --source <file.go>")
		os.Exit(1)
	}

	liftPath := args[0]
	var sourcePath string
	var outputPath string
	writeInPlace := false

	// Parse flags
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--source":
			if i+1 < len(args) {
				sourcePath = args[i+1]
				i++
			}
		case "--output", "-o":
			if i+1 < len(args) {
				outputPath = args[i+1]
				i++
			}
		case "--write", "-w":
			writeInPlace = true
		}
	}

	if sourcePath == "" {
		fmt.Fprintln(os.Stderr, "error: --source flag required")
		os.Exit(1)
	}

	// Parse .lift file
	parser, err := grammar.NewParser()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to build parser: %v\n", err)
		os.Exit(1)
	}

	liftData, err := os.ReadFile(liftPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	prog, err := parser.ParseString(liftPath, string(liftData))
	if err != nil {
		fmt.Fprintf(os.Stderr, "✗ %s\n  %v\n", liftPath, err)
		os.Exit(1)
	}

	// Create matcher from Go source
	m, err := matcher.NewFromFile(sourcePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	// Create executor sharing the same AST
	exec := executor.NewFromMatcher(m)

	// Process each lift block
	var lastResult *executor.Result
	totalMatches := 0

	for _, block := range prog.Blocks {
		matches, err := m.MatchBlock(block)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error matching block %s: %v\n", block.Name, err)
			continue
		}

		// Apply where filters
		matches = matcher.FilterMatches(matches, block.Where)

		if len(matches) == 0 {
			continue
		}

		fmt.Printf("Block %s: applying to %d match(es)\n", block.Name, len(matches))
		totalMatches += len(matches)

		// Execute actions
		result, err := exec.Execute(block, matches)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error executing block %s: %v\n", block.Name, err)
			continue
		}
		lastResult = result

		// Report applied actions
		for _, action := range result.Applied {
			fmt.Printf("  ✓ %s\n", action)
		}

		// Write emitted files
		for filename, content := range result.EmittedFiles {
			if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
				fmt.Fprintf(os.Stderr, "error writing %s: %v\n", filename, err)
			} else {
				fmt.Printf("  → wrote %s\n", filename)
			}
		}
	}

	if totalMatches == 0 {
		fmt.Println("No matches found.")
		return
	}

	// Handle output
	if lastResult != nil && lastResult.ModifiedSource != "" {
		if writeInPlace {
			if err := os.WriteFile(sourcePath, []byte(lastResult.ModifiedSource), 0644); err != nil {
				fmt.Fprintf(os.Stderr, "error writing %s: %v\n", sourcePath, err)
				os.Exit(1)
			}
			fmt.Printf("\n→ wrote %s\n", sourcePath)
		} else if outputPath != "" {
			if err := os.WriteFile(outputPath, []byte(lastResult.ModifiedSource), 0644); err != nil {
				fmt.Fprintf(os.Stderr, "error writing %s: %v\n", outputPath, err)
				os.Exit(1)
			}
			fmt.Printf("\n→ wrote %s\n", outputPath)
		} else {
			// Print to stdout
			fmt.Println("\n--- Modified source ---")
			fmt.Println(lastResult.ModifiedSource)
		}
	}
}
