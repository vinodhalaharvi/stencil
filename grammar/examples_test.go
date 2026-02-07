package grammar

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseExampleFiles(t *testing.T) {
	parser, err := NewParser()
	if err != nil {
		t.Fatalf("failed to build parser: %v", err)
	}

	examples, err := filepath.Glob("../examples/*.lift")
	if err != nil {
		t.Fatalf("failed to glob: %v", err)
	}
	if len(examples) == 0 {
		t.Fatal("no example .lift files found")
	}

	for _, path := range examples {
		t.Run(filepath.Base(path), func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read: %v", err)
			}

			prog, err := parser.ParseString(filepath.Base(path), string(data))
			if err != nil {
				t.Fatalf("parse %s:\n%v", path, err)
			}

			t.Logf("✓ %s: %d lift block(s)", filepath.Base(path), len(prog.Blocks))

			for _, block := range prog.Blocks {
				matchers := 0
				if block.From != nil {
					matchers = len(block.From.Matchers)
				}
				t.Logf("  block %s: %d matcher(s), %d where(s), %d action(s)",
					block.Name, matchers, len(block.Where), len(block.Actions))
			}
		})
	}
}
