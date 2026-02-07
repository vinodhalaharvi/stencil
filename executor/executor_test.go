package executor

import (
	"strings"
	"testing"

	"github.com/vinodhalaharvi/stencil/grammar"
	"github.com/vinodhalaharvi/stencil/matcher"
)

func TestInsertPrepend(t *testing.T) {
	src := `package main

func Fetch(url string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}
`

	// Create matcher and find the function
	m, err := matcher.New(src)
	if err != nil {
		t.Fatalf("matcher error: %v", err)
	}

	// Parse lift rule
	parser, _ := grammar.NewParser()
	prog, _ := parser.ParseString("test.lift", `
lift "test" {
	from go {
		match FuncDecl {
			name: $FuncName
			body: $Body
		}
	}

	insert code {
		prepend $Body
		`+"`"+`ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()`+"`"+`
	}
}
`)

	matches, _ := m.MatchBlock(prog.Blocks[0])

	// Execute - use NewFromMatcher to share the same AST
	exec := NewFromMatcher(m)

	result, err := exec.Execute(prog.Blocks[0], matches)
	if err != nil {
		t.Fatalf("execute error: %v", err)
	}

	// Check the output
	if !strings.Contains(result.ModifiedSource, "context.WithTimeout") {
		t.Error("expected context.WithTimeout in output")
	}

	if !strings.Contains(result.ModifiedSource, "defer cancel()") {
		t.Error("expected defer cancel() in output")
	}

	// Check imports were added
	if !strings.Contains(result.ModifiedSource, `"context"`) {
		t.Error("expected context import")
	}

	if !strings.Contains(result.ModifiedSource, `"time"`) {
		t.Error("expected time import")
	}

	t.Logf("✓ Insert prepend works")
	if testing.Verbose() {
		t.Logf("Output:\n%s", result.ModifiedSource)
	}
}

func TestPatchRename(t *testing.T) {
	src := `package main

func OldName() {
	println("hello")
}
`

	m, err := matcher.New(src)
	if err != nil {
		t.Fatalf("matcher error: %v", err)
	}

	parser, _ := grammar.NewParser()
	prog, _ := parser.ParseString("test.lift", `
lift "test" {
	from go {
		match FuncDecl {
			name: $Name
		}
	}

	patch {
		rename $Name "NewName"
	}
}
`)

	matches, _ := m.MatchBlock(prog.Blocks[0])

	exec := NewFromMatcher(m)

	result, err := exec.Execute(prog.Blocks[0], matches)
	if err != nil {
		t.Fatalf("execute error: %v", err)
	}

	if !strings.Contains(result.ModifiedSource, "func NewName()") {
		t.Errorf("expected func NewName(), got:\n%s", result.ModifiedSource)
	}

	if strings.Contains(result.ModifiedSource, "OldName") {
		t.Error("OldName should have been renamed")
	}

	t.Logf("✓ Patch rename works")
}

func TestPatchSetParamsFirst(t *testing.T) {
	src := `package main

func Fetch(url string) error {
	return nil
}
`

	m, err := matcher.New(src)
	if err != nil {
		t.Fatalf("matcher error: %v", err)
	}

	parser, _ := grammar.NewParser()
	prog, _ := parser.ParseString("test.lift", `
lift "test" {
	from go {
		match FuncDecl {
			name: $Name
			type: FuncType {
				params: $Params...
			}
		}
	}

	patch {
		set $Params.first = "ctx context.Context"
	}
}
`)

	matches, _ := m.MatchBlock(prog.Blocks[0])

	exec := NewFromMatcher(m)

	result, err := exec.Execute(prog.Blocks[0], matches)
	if err != nil {
		t.Fatalf("execute error: %v", err)
	}

	if !strings.Contains(result.ModifiedSource, "ctx context.Context") {
		t.Errorf("expected ctx context.Context param, got:\n%s", result.ModifiedSource)
	}

	// ctx should come before url
	ctxIdx := strings.Index(result.ModifiedSource, "ctx context.Context")
	urlIdx := strings.Index(result.ModifiedSource, "url string")
	if ctxIdx > urlIdx {
		t.Error("ctx should come before url")
	}

	t.Logf("✓ Patch set params.first works")
}

func TestConditionalPatch(t *testing.T) {
	src := `package main

func WithCtx(ctx context.Context, url string) error {
	return nil
}

func NoCtx(url string) error {
	return nil
}
`

	m, err := matcher.New(src)
	if err != nil {
		t.Fatalf("matcher error: %v", err)
	}

	parser, _ := grammar.NewParser()
	prog, _ := parser.ParseString("test.lift", `
lift "test" {
	from go {
		match FuncDecl {
			name: $Name
			type: FuncType {
				params: $Params...
			}
		}
	}

	where {
		not contains($Params, Field {
			type: SelectorExpr {
				x: Ident { name: "context" }
				sel: Ident { name: "Context" }
			}
		})
	}

	patch {
		set $Params.first = "ctx context.Context"
	}
}
`)

	matches, _ := m.MatchBlock(prog.Blocks[0])
	matches = matcher.FilterMatches(matches, prog.Blocks[0].Where)

	// Should only match NoCtx
	if len(matches) != 1 {
		t.Fatalf("expected 1 match (NoCtx), got %d", len(matches))
	}

	exec := NewFromMatcher(m)

	result, err := exec.Execute(prog.Blocks[0], matches)
	if err != nil {
		t.Fatalf("execute error: %v", err)
	}

	// NoCtx should now have ctx param
	if !strings.Contains(result.ModifiedSource, "func NoCtx(ctx context.Context") {
		t.Errorf("NoCtx should have ctx param:\n%s", result.ModifiedSource)
	}

	t.Logf("✓ Conditional patch works (only patches functions without ctx)")
}

func TestEmitTemplate(t *testing.T) {
	src := `package main

type User struct {
	ID   int
	Name string
}
`

	m, err := matcher.New(src)
	if err != nil {
		t.Fatalf("matcher error: %v", err)
	}

	parser, _ := grammar.NewParser()
	prog, _ := parser.ParseString("test.lift", `
lift "test" {
	from go {
		match TypeSpec {
			name: $Name
			type: StructType {
				fields: $Fields...
			}
		}
	}

	emit proto {
		file "model.proto"
		template {`+"`"+`syntax = "proto3";

message ${Name} {
  // fields go here
}`+"`"+`}
	}
}
`)

	matches, _ := m.MatchBlock(prog.Blocks[0])

	exec := NewFromMatcher(m)

	result, err := exec.Execute(prog.Blocks[0], matches)
	if err != nil {
		t.Fatalf("execute error: %v", err)
	}

	protoContent, ok := result.EmittedFiles["model.proto"]
	if !ok {
		t.Fatal("expected model.proto in emitted files")
	}

	if !strings.Contains(protoContent, "message User") {
		t.Errorf("expected 'message User', got:\n%s", protoContent)
	}

	t.Logf("✓ Emit template works")
}

func TestEmitCodeWithTransform(t *testing.T) {
	src := `package main

type UserAccount struct {
	ID int
}
`

	m, err := matcher.New(src)
	if err != nil {
		t.Fatalf("matcher error: %v", err)
	}

	parser, _ := grammar.NewParser()
	prog, _ := parser.ParseString("test.lift", `
lift "test" {
	from go {
		match TypeSpec {
			name: $Name
		}
	}

	emit sql {
		file "migration.sql"
		template {`+"`"+`CREATE TABLE ${Name | snake_case} (
  id SERIAL PRIMARY KEY
);`+"`"+`}
	}
}
`)

	matches, _ := m.MatchBlock(prog.Blocks[0])

	exec := NewFromMatcher(m)

	result, err := exec.Execute(prog.Blocks[0], matches)
	if err != nil {
		t.Fatalf("execute error: %v", err)
	}

	sqlContent, ok := result.EmittedFiles["migration.sql"]
	if !ok {
		t.Fatal("expected migration.sql in emitted files")
	}

	if !strings.Contains(sqlContent, "user_account") {
		t.Errorf("expected 'user_account' (snake_case), got:\n%s", sqlContent)
	}

	t.Logf("✓ Emit with transform works")
}

func TestFullEnforceContextTimeout(t *testing.T) {
	src := `package client

import (
	"net/http"
)

func Fetch(url string) (*http.Response, error) {
	return http.Get(url)
}
`

	m, err := matcher.New(src)
	if err != nil {
		t.Fatalf("matcher error: %v", err)
	}

	parser, _ := grammar.NewParser()
	prog, _ := parser.ParseString("test.lift", `
lift "enforce-ctx-timeout" {
	from go {
		match FuncDecl {
			name: $FuncName
			type: FuncType {
				params: $Params...
			}
			body: $Body
		}

		match CallExpr in $Body {
			fun: SelectorExpr {
				sel: $CallName
			}
		}
	}

	where {
		$CallName in ["Get", "Post", "Do"]
		not contains($Body, CallExpr {
			fun: SelectorExpr {
				x: Ident { name: "context" }
				sel: Ident { name: "WithTimeout" }
			}
		})
	}

	patch {
		if not contains($Params, Field {
			type: SelectorExpr {
				x: Ident { name: "context" }
				sel: Ident { name: "Context" }
			}
		}) {
			set $Params.first = "ctx context.Context"
		}
	}

	insert code {
		prepend $Body
		`+"`"+`ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()`+"`"+`
	}
}
`)

	matches, _ := m.MatchBlock(prog.Blocks[0])
	matches = matcher.FilterMatches(matches, prog.Blocks[0].Where)

	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}

	exec := NewFromMatcher(m)

	result, err := exec.Execute(prog.Blocks[0], matches)
	if err != nil {
		t.Fatalf("execute error: %v", err)
	}

	// Verify all transformations applied
	out := result.ModifiedSource

	// 1. ctx param added
	if !strings.Contains(out, "ctx context.Context") {
		t.Error("expected ctx context.Context param")
	}

	// 2. timeout boilerplate added (may be reformatted across lines)
	if !strings.Contains(out, "WithTimeout") {
		t.Error("expected WithTimeout")
	}

	if !strings.Contains(out, "defer cancel()") {
		t.Error("expected defer cancel()")
	}

	// 3. imports added
	if !strings.Contains(out, `"context"`) {
		t.Error("expected context import")
	}

	if !strings.Contains(out, `"time"`) {
		t.Error("expected time import")
	}

	t.Logf("✓ Full enforce-ctx-timeout transformation works")
	t.Logf("Output:\n%s", out)
}
