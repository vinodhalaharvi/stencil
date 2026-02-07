package matcher

import (
	"testing"

	"github.com/vinodhalaharvi/stencil/grammar"
)

func TestMatchFuncDecl(t *testing.T) {
	src := `
package main

func Hello(name string) string {
	return "Hello, " + name
}

func Goodbye() {
	println("bye")
}
`
	m, err := New(src)
	if err != nil {
		t.Fatalf("failed to create matcher: %v", err)
	}

	// Parse a simple .lift pattern
	parser, _ := grammar.NewParser()
	prog, err := parser.ParseString("test.lift", `
lift "test" {
	from go {
		match FuncDecl {
			name: $Name
			type: $Type
			body: $Body
		}
	}
}
`)
	if err != nil {
		t.Fatalf("failed to parse lift: %v", err)
	}

	matches, err := m.MatchBlock(prog.Blocks[0])
	if err != nil {
		t.Fatalf("match error: %v", err)
	}

	if len(matches) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(matches))
	}

	// Check first match
	if matches[0].Bindings["Name"] == nil {
		t.Error("expected $Name binding")
	}

	t.Logf("✓ Matched %d FuncDecls", len(matches))
}

func TestMatchTypeSpec(t *testing.T) {
	src := `
package main

type User struct {
	ID   int
	Name string
}

type Config struct {
	Debug bool
}
`
	m, err := New(src)
	if err != nil {
		t.Fatalf("failed to create matcher: %v", err)
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
}
`)

	matches, _ := m.MatchBlock(prog.Blocks[0])

	if len(matches) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(matches))
	}

	t.Logf("✓ Matched %d TypeSpecs with StructType", len(matches))
}

func TestDeepMatch(t *testing.T) {
	src := `
package main

import "net/http"

func Fetch(url string) error {
	_, err := http.Get(url)
	return err
}

func Other() {
	println("no http")
}
`
	m, err := New(src)
	if err != nil {
		t.Fatalf("failed to create matcher: %v", err)
	}

	parser, _ := grammar.NewParser()
	prog, _ := parser.ParseString("test.lift", `
lift "test" {
	from go {
		match FuncDecl {
			name: $FuncName
			body: $Body
		}

		match CallExpr in $Body {
			fun: $Fun
			args: $Args...
		}
	}
}
`)

	matches, _ := m.MatchBlock(prog.Blocks[0])

	// Should match Fetch function with its http.Get call
	// Other function has no CallExpr that matches
	if len(matches) < 1 {
		t.Fatalf("expected at least 1 match, got %d", len(matches))
	}

	t.Logf("✓ Deep match found %d matches", len(matches))
}

func TestExactMatch(t *testing.T) {
	src := `
package main

import "net/http"

func Fetch() {
	http.Get("url")
	http.Post("url", "", nil)
}
`
	m, err := New(src)
	if err != nil {
		t.Fatalf("failed to create matcher: %v", err)
	}

	parser, _ := grammar.NewParser()
	prog, _ := parser.ParseString("test.lift", `
lift "test" {
	from go {
		match FuncDecl {
			body: $Body
		}

		match CallExpr in $Body {
			fun: SelectorExpr {
				x: Ident { name: "http" }
				sel: Ident { name: "Get" }
			}
		}
	}
}
`)

	matches, _ := m.MatchBlock(prog.Blocks[0])

	if len(matches) != 1 {
		t.Fatalf("expected 1 match (http.Get only), got %d", len(matches))
	}

	t.Logf("✓ Exact match for http.Get found")
}

func TestPredicateMemberCheck(t *testing.T) {
	src := `
package main

import "net/http"

func Fetch() {
	http.Get("url")
	http.Post("url", "", nil)
	http.Head("url")
}
`
	m, err := New(src)
	if err != nil {
		t.Fatalf("failed to create matcher: %v", err)
	}

	parser, _ := grammar.NewParser()
	prog, _ := parser.ParseString("test.lift", `
lift "test" {
	from go {
		match FuncDecl {
			body: $Body
		}

		match CallExpr in $Body {
			fun: SelectorExpr {
				sel: $Method
			}
		}
	}

	where {
		$Method in ["Get", "Post"]
	}
}
`)

	matches, _ := m.MatchBlock(prog.Blocks[0])
	matches = FilterMatches(matches, prog.Blocks[0].Where)

	if len(matches) != 2 {
		t.Fatalf("expected 2 matches (Get, Post), got %d", len(matches))
	}

	t.Logf("✓ Member predicate filtered to %d matches", len(matches))
}

func TestPredicateContains(t *testing.T) {
	src := `
package main

import "context"

func WithTimeout() {
	ctx, _ := context.WithTimeout(nil, 0)
	_ = ctx
}

func NoTimeout() {
	println("no timeout")
}
`
	m, err := New(src)
	if err != nil {
		t.Fatalf("failed to create matcher: %v", err)
	}

	parser, _ := grammar.NewParser()
	prog, _ := parser.ParseString("test.lift", `
lift "test" {
	from go {
		match FuncDecl {
			name: $Name
			body: $Body
		}
	}

	where {
		not contains($Body, CallExpr {
			fun: SelectorExpr {
				x: Ident { name: "context" }
				sel: Ident { name: "WithTimeout" }
			}
		})
	}
}
`)

	matches, _ := m.MatchBlock(prog.Blocks[0])
	matches = FilterMatches(matches, prog.Blocks[0].Where)

	if len(matches) != 1 {
		t.Fatalf("expected 1 match (NoTimeout only), got %d", len(matches))
	}

	t.Logf("✓ Contains predicate filtered correctly")
}

func TestPredicateExported(t *testing.T) {
	src := `
package main

func PublicFunc() {}
func privateFunc() {}
`
	m, err := New(src)
	if err != nil {
		t.Fatalf("failed to create matcher: %v", err)
	}

	parser, _ := grammar.NewParser()
	prog, _ := parser.ParseString("test.lift", `
lift "test" {
	from go {
		match FuncDecl {
			name: $Name
		}
	}

	where {
		$Name.exported
	}
}
`)

	matches, _ := m.MatchBlock(prog.Blocks[0])
	matches = FilterMatches(matches, prog.Blocks[0].Where)

	if len(matches) != 1 {
		t.Fatalf("expected 1 match (PublicFunc only), got %d", len(matches))
	}

	t.Logf("✓ Exported predicate works")
}

func TestPredicateLenCheck(t *testing.T) {
	src := `
package main

func NoParams() {}
func OneParam(a int) {}
func TwoParams(a, b int) {}
`
	m, err := New(src)
	if err != nil {
		t.Fatalf("failed to create matcher: %v", err)
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
		len($Params) > 0
	}
}
`)

	matches, _ := m.MatchBlock(prog.Blocks[0])
	matches = FilterMatches(matches, prog.Blocks[0].Where)

	if len(matches) != 2 {
		t.Fatalf("expected 2 matches (OneParam, TwoParams), got %d", len(matches))
	}

	t.Logf("✓ Len predicate works")
}

func TestBadHTTPClient(t *testing.T) {
	// This is the actual testdata file content
	src := `
package client

import (
	"encoding/json"
	"fmt"
	"net/http"
)

type UserService struct {
	baseURL string
	client  *http.Client
}

func (s *UserService) GetUser(id string) (*User, error) {
	resp, err := s.client.Get(s.baseURL + "/users/" + id)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var user User
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, err
	}
	return &user, nil
}

func (s *UserService) CreateUser(user *User) error {
	resp, err := s.client.Post(s.baseURL+"/users", "application/json", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}
	return nil
}

type User struct {
	ID   string
	Name string
}
`
	m, err := New(src)
	if err != nil {
		t.Fatalf("failed to create matcher: %v", err)
	}

	parser, _ := grammar.NewParser()
	prog, _ := parser.ParseString("test.lift", `
lift "enforce-ctx-timeout" {
	from go {
		match FuncDecl {
			name: $FuncName
			type: FuncType {
				params: $Params...
				results: $Results...
			}
			body: $Body
		}

		match CallExpr in $Body {
			fun: SelectorExpr {
				sel: $CallName
			}
			args: $CallArgs...
		}
	}

	where {
		$CallName in ["Get", "Post", "Do", "Dial"]
		not contains($Body, CallExpr {
			fun: SelectorExpr {
				x: Ident { name: "context" }
				sel: Ident { name: "WithTimeout" }
			}
		})
	}
}
`)

	matches, _ := m.MatchBlock(prog.Blocks[0])
	matches = FilterMatches(matches, prog.Blocks[0].Where)

	// Should match GetUser (has Get call) and CreateUser (has Post call)
	// Both lack context.WithTimeout
	if len(matches) != 2 {
		t.Fatalf("expected 2 matches (GetUser, CreateUser), got %d", len(matches))
	}

	t.Logf("✓ Bad HTTP client pattern matched %d functions", len(matches))
	for _, match := range matches {
		if name, ok := match.Bindings["FuncName"]; ok {
			t.Logf("  - %v", name)
		}
	}
}

func TestWildcard(t *testing.T) {
	src := `
package main

func Test(a int, b string) {}
`
	m, err := New(src)
	if err != nil {
		t.Fatalf("failed to create matcher: %v", err)
	}

	parser, _ := grammar.NewParser()
	prog, _ := parser.ParseString("test.lift", `
lift "test" {
	from go {
		match FuncDecl {
			name: _
			type: $Type
		}
	}
}
`)

	matches, _ := m.MatchBlock(prog.Blocks[0])

	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}

	// Wildcard should NOT create a binding
	if _, ok := matches[0].Bindings["_"]; ok {
		t.Error("wildcard should not create binding")
	}

	t.Logf("✓ Wildcard matching works")
}
