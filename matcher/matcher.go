// Package matcher implements structural pattern matching against Go AST.
//
// It takes a parsed .lift file (from the grammar package) and Go source code,
// walks the AST, matches patterns from the "from go { ... }" clause, and
// returns captured bindings.
//
// The matcher handles:
//   - Direct node type matching (match FuncDecl { ... })
//   - Deep matching (match CallExpr in $Body { ... })
//   - Field matching with bindings ($Name), spreads ($Fields...), wildcards (_)
//   - Nested AST patterns (recursive structural matching)
//   - Exact string matching for identifiers
package matcher

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"strings"

	"github.com/vinodhalaharvi/stencil/grammar"
)

// Bindings holds captured values from pattern matching.
// Keys are binding names (without $), values are the captured AST nodes.
type Bindings map[string]any

// Copy creates a shallow copy of the bindings.
func (b Bindings) Copy() Bindings {
	c := make(Bindings, len(b))
	for k, v := range b {
		c[k] = v
	}
	return c
}

// Match represents a successful pattern match with its captured bindings.
type Match struct {
	Node     ast.Node // The matched AST node
	Bindings Bindings // Captured bindings from the match
}

// Matcher performs pattern matching against Go AST.
type Matcher struct {
	fset *token.FileSet
	file *ast.File
}

// New creates a Matcher from Go source code.
func New(src string) (*Matcher, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "src.go", src, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse error: %w", err)
	}
	return &Matcher{fset: fset, file: file}, nil
}

// NewFromFile creates a Matcher from a Go source file path.
func NewFromFile(path string) (*Matcher, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse error: %w", err)
	}
	return &Matcher{fset: fset, file: file}, nil
}

// FileSet returns the token.FileSet for position information.
func (m *Matcher) FileSet() *token.FileSet {
	return m.fset
}

// File returns the parsed AST file.
func (m *Matcher) File() *ast.File {
	return m.file
}

// MatchBlock executes all matchers in a lift block's from clause.
// Returns all matches with their bindings.
func (m *Matcher) MatchBlock(block *grammar.LiftBlock) ([]Match, error) {
	if block.From == nil || len(block.From.Matchers) == 0 {
		return nil, nil
	}

	// Start with the first matcher against the whole file
	firstMatcher := block.From.Matchers[0]
	matches := m.matchStmt(firstMatcher, m.file, nil)

	// For subsequent matchers with "in $Binding", match within captured bindings
	for i := 1; i < len(block.From.Matchers); i++ {
		stmt := block.From.Matchers[i]
		if stmt.In == nil {
			// No "in" clause — match against whole file, merge bindings
			newMatches := m.matchStmt(stmt, m.file, nil)
			matches = crossJoin(matches, newMatches)
		} else {
			// "in $Binding" — match within the captured binding
			var newMatches []Match
			for _, match := range matches {
				bindingName := *stmt.In
				if scope, ok := match.Bindings[bindingName]; ok {
					if scopeNode, ok := scope.(ast.Node); ok {
						subMatches := m.matchStmt(stmt, scopeNode, match.Bindings)
						newMatches = append(newMatches, subMatches...)
					}
				}
			}
			matches = newMatches
		}
	}

	return matches, nil
}

// crossJoin combines matches from two matchers, merging their bindings.
func crossJoin(a, b []Match) []Match {
	if len(a) == 0 {
		return b
	}
	if len(b) == 0 {
		return a
	}
	var result []Match
	for _, ma := range a {
		for _, mb := range b {
			merged := ma.Bindings.Copy()
			for k, v := range mb.Bindings {
				merged[k] = v
			}
			result = append(result, Match{
				Node:     ma.Node,
				Bindings: merged,
			})
		}
	}
	return result
}

// matchStmt finds all nodes matching a MatchStmt, optionally within a scope.
func (m *Matcher) matchStmt(stmt *grammar.MatchStmt, scope ast.Node, inherited Bindings) []Match {
	var matches []Match

	ast.Inspect(scope, func(n ast.Node) bool {
		if n == nil {
			return false
		}

		// Check if node type matches
		if !nodeTypeMatches(n, stmt.NodeType) {
			return true // continue traversing
		}

		// Try to match fields
		bindings := make(Bindings)
		if inherited != nil {
			for k, v := range inherited {
				bindings[k] = v
			}
		}

		if matchFields(n, stmt.Fields, bindings) {
			matches = append(matches, Match{
				Node:     n,
				Bindings: bindings,
			})
		}

		return true // continue to find more matches
	})

	return matches
}

// nodeTypeMatches checks if a node's type matches the expected type name.
func nodeTypeMatches(n ast.Node, typeName string) bool {
	// Get the actual type name without package prefix
	t := reflect.TypeOf(n)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t.Name() == typeName
}

// matchFields attempts to match all field constraints against a node.
// Returns true if all fields match, populating bindings along the way.
func matchFields(n ast.Node, fields []*grammar.FieldMatch, bindings Bindings) bool {
	for _, field := range fields {
		if !matchField(n, field, bindings) {
			return false
		}
	}
	return true
}

// matchField matches a single field constraint.
func matchField(n ast.Node, field *grammar.FieldMatch, bindings Bindings) bool {
	// Get the field value from the node using reflection
	fieldValue := getField(n, field.Name)
	if fieldValue == nil && !field.Value.Wild {
		// Field doesn't exist and we're not using wildcard
		// Check if it's an optional field that can be nil
		if field.Value.Binding != nil || field.Value.Spread != nil {
			// Bind nil
			if field.Value.Binding != nil {
				bindings[field.Value.Binding.Name] = nil
			}
			return true
		}
		return false
	}

	return matchValue(fieldValue, field.Value, bindings)
}

// matchValue matches a value against a MatchValue pattern.
func matchValue(value any, pattern *grammar.MatchValue, bindings Bindings) bool {
	// Wildcard matches anything
	if pattern.Wild {
		return true
	}

	// Simple binding — capture the value
	if pattern.Binding != nil {
		bindings[pattern.Binding.Name] = value
		return true
	}

	// Spread binding — capture as slice
	if pattern.Spread != nil {
		bindings[pattern.Spread.Name] = value
		return true
	}

	// Exact string match
	if pattern.Exact != nil {
		expected := strings.Trim(*pattern.Exact, `"`)
		return matchExact(value, expected)
	}

	// Nested AST pattern
	if pattern.Pattern != nil {
		return matchASTPattern(value, pattern.Pattern, bindings)
	}

	// List pattern
	if pattern.List != nil {
		return matchList(value, pattern.List, bindings)
	}

	return false
}

// matchExact checks if a value matches an exact string.
func matchExact(value any, expected string) bool {
	switch v := value.(type) {
	case *ast.Ident:
		return v != nil && v.Name == expected
	case string:
		return v == expected
	default:
		return false
	}
}

// matchASTPattern matches a value against a nested AST pattern.
func matchASTPattern(value any, pattern *grammar.ASTPattern, bindings Bindings) bool {
	// Handle the value being a node or needing unwrapping
	node := toNode(value)
	if node == nil {
		return false
	}

	// Check type matches
	if !nodeTypeMatches(node, pattern.NodeType) {
		return false
	}

	// Match all fields
	return matchFields(node, pattern.Fields, bindings)
}

// matchList matches a list value against a list pattern.
func matchList(value any, patterns []*grammar.MatchValue, bindings Bindings) bool {
	// Convert value to a slice of items
	items := toSlice(value)
	if items == nil {
		return false
	}

	// For now, simple positional matching
	// TODO: Handle spreads in list patterns more sophisticatedly
	if len(patterns) != len(items) {
		return false
	}

	for i, p := range patterns {
		if !matchValue(items[i], p, bindings) {
			return false
		}
	}
	return true
}

// getField retrieves a field from an AST node by name.
func getField(n ast.Node, name string) any {
	v := reflect.ValueOf(n)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return nil
	}

	// Map common .lift field names to actual Go AST field names
	goFieldName := mapFieldName(name)

	f := v.FieldByName(goFieldName)
	if !f.IsValid() {
		return nil
	}

	if f.Kind() == reflect.Ptr && f.IsNil() {
		return nil
	}

	return f.Interface()
}

// mapFieldName maps .lift field names to Go AST struct field names.
// The .lift grammar uses lowercase names, but Go AST uses PascalCase.
func mapFieldName(name string) string {
	// Map lowercase .lift names to Go AST field names
	fieldMap := map[string]string{
		"name":    "Name",
		"type":    "Type",
		"recv":    "Recv",
		"body":    "Body",
		"params":  "Params",
		"results": "Results",
		"fields":  "Fields",
		"list":    "List",
		"fun":     "Fun",
		"args":    "Args",
		"x":       "X",
		"sel":     "Sel",
		"tok":     "Tok",
		"specs":   "Specs",
		"decls":   "Decls",
		"names":   "Names",
		"tag":     "Tag",
		"value":   "Value",
		"values":  "Values",
		"elts":    "Elts",
		"elt":     "Elt",
		"key":     "Key",
		"len":     "Len",
		"lhs":     "Lhs",
		"rhs":     "Rhs",
		"cond":    "Cond",
		"init":    "Init",
		"post":    "Post",
		"op":      "Op",
	}

	if mapped, ok := fieldMap[name]; ok {
		return mapped
	}

	// Default: capitalize first letter
	if len(name) > 0 {
		return strings.ToUpper(name[:1]) + name[1:]
	}
	return name
}

// toNode converts various types to ast.Node.
func toNode(v any) ast.Node {
	if v == nil {
		return nil
	}

	// Direct node
	if n, ok := v.(ast.Node); ok {
		return n
	}

	// Handle *ast.FieldList specially — it's a container, not directly a node pattern target
	// but its List field contains []*ast.Field
	if fl, ok := v.(*ast.FieldList); ok && fl != nil {
		// Return the FieldList itself as a node
		return fl
	}

	return nil
}

// toSlice converts various AST slice types to []any.
func toSlice(v any) []any {
	if v == nil {
		return nil
	}

	rv := reflect.ValueOf(v)

	// Handle *ast.FieldList specially
	if fl, ok := v.(*ast.FieldList); ok {
		if fl == nil || fl.List == nil {
			return nil
		}
		result := make([]any, len(fl.List))
		for i, f := range fl.List {
			result[i] = f
		}
		return result
	}

	// Handle slices
	if rv.Kind() == reflect.Slice {
		result := make([]any, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			result[i] = rv.Index(i).Interface()
		}
		return result
	}

	return nil
}

// --- Predicate evaluation ---

// EvalPredicate evaluates a predicate against bindings.
func EvalPredicate(pred *grammar.Predicate, bindings Bindings) bool {
	if pred.Not != nil {
		return !EvalPredicate(pred.Not, bindings)
	}

	if pred.Contains != nil {
		return evalContains(pred.Contains, bindings)
	}

	if pred.LenCheck != nil {
		return evalLenCheck(pred.LenCheck, bindings)
	}

	if pred.MemberCheck != nil {
		return evalMemberCheck(pred.MemberCheck, bindings)
	}

	if pred.PropCheck != nil {
		return evalPropCheck(pred.PropCheck, bindings)
	}

	return false
}

// evalContains checks if a binding contains a pattern.
func evalContains(pred *grammar.ContainsPred, bindings Bindings) bool {
	scope, ok := bindings[pred.Binding]
	if !ok {
		return false
	}

	scopeNode, ok := scope.(ast.Node)
	if !ok {
		return false
	}

	found := false
	ast.Inspect(scopeNode, func(n ast.Node) bool {
		if n == nil || found {
			return false
		}

		if nodeTypeMatches(n, pred.Pattern.NodeType) {
			subBindings := make(Bindings)
			if matchFields(n, pred.Pattern.Fields, subBindings) {
				found = true
				return false
			}
		}
		return true
	})

	return found
}

// evalLenCheck evaluates a length predicate.
func evalLenCheck(pred *grammar.LenPred, bindings Bindings) bool {
	val, ok := bindings[pred.Binding]
	if !ok {
		return false
	}

	length := getLength(val)

	switch pred.Op {
	case ">":
		return length > pred.Value
	case ">=":
		return length >= pred.Value
	case "<":
		return length < pred.Value
	case "<=":
		return length <= pred.Value
	case "==":
		return length == pred.Value
	case "!=":
		return length != pred.Value
	}
	return false
}

// getLength returns the length of a value (slice, FieldList, etc.)
func getLength(v any) int {
	if v == nil {
		return 0
	}

	if fl, ok := v.(*ast.FieldList); ok {
		if fl == nil || fl.List == nil {
			return 0
		}
		return len(fl.List)
	}

	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Slice {
		return rv.Len()
	}

	return 0
}

// evalMemberCheck checks if a binding's value is in a set.
func evalMemberCheck(pred *grammar.MemberPred, bindings Bindings) bool {
	val, ok := bindings[pred.Binding]
	if !ok {
		return false
	}

	// Get string value
	var strVal string
	switch v := val.(type) {
	case *ast.Ident:
		if v != nil {
			strVal = v.Name
		}
	case string:
		strVal = v
	default:
		return false
	}

	// Check membership
	for _, member := range pred.Values {
		if strings.Trim(member, `"`) == strVal {
			return true
		}
	}
	return false
}

// evalPropCheck evaluates a property predicate.
func evalPropCheck(pred *grammar.PropertyPred, bindings Bindings) bool {
	val, ok := bindings[pred.Binding]
	if !ok {
		return false
	}

	switch pred.Property {
	case "exported":
		return isExported(val)
	case "pointer":
		_, ok := val.(*ast.StarExpr)
		return ok
	case "slice":
		_, ok := val.(*ast.ArrayType)
		return ok
	case "map":
		_, ok := val.(*ast.MapType)
		return ok
	case "error":
		if ident, ok := val.(*ast.Ident); ok {
			return ident.Name == "error"
		}
		return false
	}
	return false
}

// isExported checks if a value represents an exported identifier.
func isExported(v any) bool {
	switch val := v.(type) {
	case *ast.Ident:
		return val != nil && len(val.Name) > 0 && val.Name[0] >= 'A' && val.Name[0] <= 'Z'
	case string:
		return len(val) > 0 && val[0] >= 'A' && val[0] <= 'Z'
	}
	return false
}

// FilterMatches filters matches using where clause predicates.
func FilterMatches(matches []Match, whereClauses []*grammar.WhereClause) []Match {
	if len(whereClauses) == 0 {
		return matches
	}

	var filtered []Match
	for _, m := range matches {
		pass := true
		for _, where := range whereClauses {
			for _, pred := range where.Predicates {
				if !EvalPredicate(pred, m.Bindings) {
					pass = false
					break
				}
			}
			if !pass {
				break
			}
		}
		if pass {
			filtered = append(filtered, m)
		}
	}
	return filtered
}
