// Package grammar defines the typed AST for .lift files used by Stencil.
//
// "Parse, not validate" — if a .lift file parses, it's structurally valid.
// Arbitrary nesting comes for free from the grammar's recursive structure,
// which is exactly where YAML-based tools fall apart.
//
// The grammar maps 1:1 to Go AST node types for matching, ensuring formal
// correctness when describing structural patterns in Go source code.
package grammar

import (
	"github.com/alecthomas/participle/v2"
	"github.com/alecthomas/participle/v2/lexer"
)

// ---------------------------------------------------------------------------
// Custom lexer — handles "...", multi-char operators, raw strings, comments
// ---------------------------------------------------------------------------

var liftLexer = lexer.MustSimple([]lexer.SimpleRule{
	{Name: "Comment", Pattern: `//[^\n]*`},
	{Name: "RawString", Pattern: "`[^`]*`"},
	{Name: "String", Pattern: `"[^"]*"`},
	{Name: "Spread", Pattern: `\.\.\.`},
	{Name: "Int", Pattern: `[0-9]+`},
	{Name: "OpMulti", Pattern: `>=|<=|!=|==`},
	{Name: "Punct", Pattern: `[{}\[\]():=.,<>|*$@!]`},
	{Name: "Ident", Pattern: `[a-zA-Z_][a-zA-Z0-9_]*`},
	{Name: "Whitespace", Pattern: `[\s]+`},
})

// ---------------------------------------------------------------------------
// Top-level
// ---------------------------------------------------------------------------

// Program is the root of a .lift file.
type Program struct {
	Pos    lexer.Position
	Blocks []*LiftBlock `@@*`
}

// LiftBlock is a named transformation unit.
type LiftBlock struct {
	Pos     lexer.Position
	Name    string         `"lift" @String "{"`
	From    *FromClause    `@@`
	Where   []*WhereClause `@@*`
	Actions []*Action      `@@* "}"`
}

// ---------------------------------------------------------------------------
// MATCH — formal Go AST patterns
// ---------------------------------------------------------------------------

// FromClause: from go { ... }
type FromClause struct {
	Pos      lexer.Position
	Matchers []*MatchStmt `"from" "go" "{" @@* "}"`
}

// MatchStmt: match TypeSpec { ... } or match CallExpr in $Body { ... }
type MatchStmt struct {
	Pos      lexer.Position
	NodeType string        `"match" @Ident`
	In       *string       `( "in" "$" @Ident )?`
	Fields   []*FieldMatch `"{" @@* "}"`
}

// FieldMatch: name: $Name
type FieldMatch struct {
	Pos   lexer.Position
	Name  string      `@Ident ":"`
	Value *MatchValue `@@`
}

// MatchValue — the recursive heart. This is where arbitrary nesting lives.
//
// MatchValue → ASTPattern → FieldMatch → MatchValue → ...
type MatchValue struct {
	Pos     lexer.Position
	Spread  *SpreadBinding `  @@`
	Binding *SimpleBinding `| @@`
	Pattern *ASTPattern    `| @@`
	List    []*MatchValue  `| "[" ( @@ ( "," @@ )* )? "]"`
	Exact   *string        `| @String`
	Wild    bool           `| @"_"`
}

// SpreadBinding: $Fields...
type SpreadBinding struct {
	Pos  lexer.Position
	Name string `"$" @Ident Spread`
}

// SimpleBinding: $Name (no spread)
type SimpleBinding struct {
	Pos  lexer.Position
	Name string `"$" @Ident`
}

// ASTPattern: StructType { fields: $Fields... }
// Recurses via FieldMatch → MatchValue → ASTPattern
type ASTPattern struct {
	Pos      lexer.Position
	NodeType string        `@Ident "{"`
	Fields   []*FieldMatch `@@* "}"`
}

// ---------------------------------------------------------------------------
// FILTER
// ---------------------------------------------------------------------------

// WhereClause: where { ... }
type WhereClause struct {
	Pos        lexer.Position
	Predicates []*Predicate `"where" "{" @@* "}"`
}

// Predicate — supports negation, contains, len, membership, property check.
// Ordered carefully for Participle's PEG-style parsing.
type Predicate struct {
	Pos         lexer.Position
	Not         *Predicate    `  "not" @@`
	Contains    *ContainsPred `| "contains" @@`
	LenCheck    *LenPred      `| "len" @@`
	MemberCheck *MemberPred   `| @@`
	PropCheck   *PropertyPred `| @@`
}

// ContainsPred: contains($Body, CallExpr { ... })
type ContainsPred struct {
	Pos     lexer.Position
	Binding string      `"(" "$" @Ident ","`
	Pattern *ASTPattern `@@ ")"`
}

// LenPred: len($Methods) > 0
type LenPred struct {
	Pos     lexer.Position
	Binding string `"(" "$" @Ident ")"`
	Op      string `@( ">=" | "<=" | "!=" | "==" | ">" | "<" )`
	Value   int    `@Int`
}

// MemberPred: $CallName in ["Get", "Post"]
type MemberPred struct {
	Pos     lexer.Position
	Binding string   `"$" @Ident "in"`
	Values  []string `"[" @String ( "," @String )* "]"`
}

// PropertyPred: $Name.exported
type PropertyPred struct {
	Pos      lexer.Position
	Binding  string `"$" @Ident`
	Property string `"." @( "exported" | "pointer" | "slice" | "map" | "builtin" | "error" )`
}

// ---------------------------------------------------------------------------
// ACTIONS
// ---------------------------------------------------------------------------

// Action is a sum type: exactly one of patch/delete/insert/emit.
type Action struct {
	Pos    lexer.Position
	Patch  *PatchClause  `  @@`
	Delete *DeleteClause `| @@`
	Insert *InsertClause `| @@`
	Emit   *EmitClause   `| @@`
}

// --- PATCH ---

// PatchClause: patch { ... }
type PatchClause struct {
	Pos   lexer.Position
	Stmts []*PatchStmt `"patch" "{" @@* "}"`
}

// PatchStmt: one of if/set/rename/retype.
type PatchStmt struct {
	Pos    lexer.Position
	If     *ConditionalPatch `  @@`
	Set    *SetStmt          `| @@`
	Rename *RenameStmt       `| @@`
	Retype *RetypeStmt       `| @@`
}

// ConditionalPatch: if not contains(...) { set ... }
type ConditionalPatch struct {
	Pos       lexer.Position
	Condition *Predicate   `"if" @@`
	Stmts     []*PatchStmt `"{" @@* "}"`
}

// SetStmt: set $Field.type = "string"
type SetStmt struct {
	Pos   lexer.Position
	Path  *FieldPath `"set" @@`
	Value *Expr      `"=" @@`
}

// RenameStmt: rename $Name "NewName"
type RenameStmt struct {
	Pos     lexer.Position
	Binding string `"rename" "$" @Ident`
	NewName string `@String`
}

// RetypeStmt: retype $Field "string"
type RetypeStmt struct {
	Pos     lexer.Position
	Binding string `"retype" "$" @Ident`
	NewType string `@String`
}

// FieldPath: $Field.type.name
type FieldPath struct {
	Pos      lexer.Position
	Binding  string   `"$" @Ident`
	Segments []string `( "." @Ident )*`
}

// --- DELETE ---

// DeleteClause: delete { ... }
type DeleteClause struct {
	Pos   lexer.Position
	Stmts []*DeleteStmt `"delete" "{" @@* "}"`
}

// DeleteStmt: remove $X or remove $X.field
type DeleteStmt struct {
	Pos  lexer.Position
	Path *FieldPath `"remove" @@`
}

// --- INSERT ---

// InsertClause: insert ast { ... } or insert code { ... }
type InsertClause struct {
	Pos      lexer.Position
	Mode     string     `"insert" @( "ast" | "code" )`
	Position *InsertPos `"{" @@`
	ASTNode  *ASTBuild  `( @@`
	Code     *CodeBlock `| @@ )? "}"`
}

// InsertPos: after $X / before $X / prepend $X / append $X
type InsertPos struct {
	Pos     lexer.Position
	Kind    string  `@( "after" | "before" | "prepend" | "append" | "into" )`
	Binding *string `( "$" @Ident )?`
}

// --- EMIT ---

// EmitClause: emit go { file "x.go" ast { ... } }
type EmitClause struct {
	Pos      lexer.Position
	Target   string         `"emit" @( "go" | "proto" | "sql" | "graphql" | "json" | "yaml" | "toml" )`
	File     string         `"{" "file" @String`
	Package  *string        `( "package" @Ident )?`
	ASTBody  *ASTEmitBlock  `( @@`
	CodeBody *CodeEmitBlock `| @@`
	Template *TplEmitBlock  `| @@ )? "}"`
}

// ASTEmitBlock: ast { GenDecl { ... } }
type ASTEmitBlock struct {
	Pos  lexer.Position
	Body *ASTBuild `"ast" "{" @@ "}"`
}

// CodeEmitBlock: code { `...` }
type CodeEmitBlock struct {
	Pos  lexer.Position
	Text string `"code" "{" @RawString "}"`
}

// TplEmitBlock: template { `...` }
type TplEmitBlock struct {
	Pos  lexer.Position
	Text string `"template" "{" @RawString "}"`
}

// CodeBlock for insert code — just raw string
type CodeBlock struct {
	Pos  lexer.Position
	Text string `@RawString`
}

// ---------------------------------------------------------------------------
// AST CONSTRUCTION — for type-safe Go output
// ---------------------------------------------------------------------------

// ASTBuild constructs a new Go AST node.
// Recurses: ASTBuild → ASTBuildField → ASTBuildValue → ASTBuild
type ASTBuild struct {
	Pos      lexer.Position
	NodeType string           `@Ident "{"`
	Fields   []*ASTBuildField `@@* "}"`
}

// ASTBuildField: tok: "TYPE"
type ASTBuildField struct {
	Pos   lexer.Position
	Name  string         `@Ident ":"`
	Value *ASTBuildValue `@@`
}

// ASTBuildValue — the recursive value type for AST construction.
type ASTBuildValue struct {
	Pos       lexer.Position
	ForLoop   *ForASTLoop      `  @@`
	Binding   *BindingRef      `| @@`
	Construct *ASTBuild        `| @@`
	List      []*ASTBuildValue `| "[" ( @@ ( "," @@ )* )? "]"`
	String    *string          `| @String`
	Number    *int             `| @Int`
}

// ForASTLoop: for $m in $Methods { Field { ... } }
type ForASTLoop struct {
	Pos     lexer.Position
	Binding string      `"for" "$" @Ident "in"`
	Source  *BindingRef `@@`
	Body    *ASTBuild   `"{" @@ "}"`
}

// BindingRef: $Name or $m.MethodType or $f.Type | proto_type
type BindingRef struct {
	Pos        lexer.Position
	Name       string   `"$" @Ident`
	Field      *string  `( "." @Ident )?`
	Transforms []string `( "|" @Ident )*`
}

// ---------------------------------------------------------------------------
// SHARED
// ---------------------------------------------------------------------------

// Expr: general expression used in set, comparisons, etc.
type Expr struct {
	Pos     lexer.Position
	Binding *BindingRef `  @@`
	String  *string     `| @String`
	Number  *int        `| @Int`
}

// ---------------------------------------------------------------------------
// Parser constructor
// ---------------------------------------------------------------------------

// NewParser builds a Participle parser for .lift files.
func NewParser() (*participle.Parser[Program], error) {
	return participle.Build[Program](
		participle.Lexer(liftLexer),
		participle.UseLookahead(5),
		participle.Elide("Comment", "Whitespace"),
	)
}
