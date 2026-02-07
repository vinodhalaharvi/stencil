// Package executor applies actions from .lift files to Go source code.
//
// It takes matches (with captured bindings) from the matcher package and
// executes the actions defined in the lift block: patch, insert, delete, emit.
//
// The executor works by:
// 1. Modifying the AST in place (for patch/insert/delete)
// 2. Rendering the modified AST back to source code
// 3. Writing new files (for emit)
package executor

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"regexp"
	"strings"

	"github.com/vinodhalaharvi/stencil/grammar"
	"github.com/vinodhalaharvi/stencil/matcher"
)

// Result holds the output of executing actions.
type Result struct {
	// ModifiedSource is the transformed Go source code (for patch/insert/delete)
	ModifiedSource string

	// EmittedFiles maps filename to content (for emit actions)
	EmittedFiles map[string]string

	// Applied tracks which actions were applied
	Applied []string
}

// Executor applies lift block actions to Go source.
type Executor struct {
	fset    *token.FileSet
	file    *ast.File
	src     string
	imports map[string]bool // track imports to add
}

// New creates an Executor from Go source code.
func New(src string) (*Executor, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "src.go", src, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse error: %w", err)
	}
	return &Executor{
		fset:    fset,
		file:    file,
		src:     src,
		imports: make(map[string]bool),
	}, nil
}

// NewFromMatcher creates an Executor that shares the AST with a matcher.
// This is important: the matcher's bindings point to nodes in its AST,
// so the executor must modify the same AST.
func NewFromMatcher(m *matcher.Matcher) *Executor {
	return &Executor{
		fset:    m.FileSet(),
		file:    m.File(),
		imports: make(map[string]bool),
	}
}

// Execute applies all actions in a lift block using the provided matches.
func (e *Executor) Execute(block *grammar.LiftBlock, matches []matcher.Match) (*Result, error) {
	result := &Result{
		EmittedFiles: make(map[string]string),
	}

	for _, action := range block.Actions {
		for _, match := range matches {
			if action.Insert != nil {
				if err := e.executeInsert(action.Insert, match.Bindings); err != nil {
					return nil, fmt.Errorf("insert failed: %w", err)
				}
				result.Applied = append(result.Applied, "insert")
			}

			if action.Patch != nil {
				if err := e.executePatch(action.Patch, match.Bindings); err != nil {
					return nil, fmt.Errorf("patch failed: %w", err)
				}
				result.Applied = append(result.Applied, "patch")
			}

			if action.Delete != nil {
				if err := e.executeDelete(action.Delete, match.Bindings); err != nil {
					return nil, fmt.Errorf("delete failed: %w", err)
				}
				result.Applied = append(result.Applied, "delete")
			}

			if action.Emit != nil {
				content, err := e.executeEmit(action.Emit, match.Bindings)
				if err != nil {
					return nil, fmt.Errorf("emit failed: %w", err)
				}
				filename := strings.Trim(action.Emit.File, `"`)
				result.EmittedFiles[filename] = content
				result.Applied = append(result.Applied, "emit:"+filename)
			}
		}
	}

	// Add any required imports
	e.addImports()

	// Render modified AST back to source
	var buf bytes.Buffer
	if err := format.Node(&buf, e.fset, e.file); err != nil {
		return nil, fmt.Errorf("format error: %w", err)
	}
	result.ModifiedSource = buf.String()

	return result, nil
}

// executeInsert handles insert actions (prepend/append code to blocks).
func (e *Executor) executeInsert(ins *grammar.InsertClause, bindings matcher.Bindings) error {
	if ins.Mode != "code" {
		// AST mode insert not yet implemented
		return fmt.Errorf("insert mode %q not yet supported", ins.Mode)
	}

	if ins.Code == nil {
		return fmt.Errorf("insert code requires code block")
	}

	// Get the target binding
	if ins.Position.Binding == nil {
		return fmt.Errorf("insert requires target binding")
	}

	targetName := *ins.Position.Binding
	target, ok := bindings[targetName]
	if !ok {
		return fmt.Errorf("binding $%s not found", targetName)
	}

	blockStmt, ok := target.(*ast.BlockStmt)
	if !ok {
		return fmt.Errorf("$%s is not a BlockStmt", targetName)
	}

	// Parse the code to insert
	codeText := strings.Trim(ins.Code.Text, "`")
	codeText = e.interpolate(codeText, bindings)

	// Track imports needed
	if strings.Contains(codeText, "context.") {
		e.imports["context"] = true
	}
	if strings.Contains(codeText, "time.") {
		e.imports["time"] = true
	}

	// Parse as statements
	stmts, err := parseStatements(codeText)
	if err != nil {
		return fmt.Errorf("parse insert code: %w", err)
	}

	// Apply based on position
	switch ins.Position.Kind {
	case "prepend":
		blockStmt.List = append(stmts, blockStmt.List...)
	case "append":
		blockStmt.List = append(blockStmt.List, stmts...)
	default:
		return fmt.Errorf("insert position %q not yet supported", ins.Position.Kind)
	}

	return nil
}

// executePatch handles patch actions (rename, retype, set).
func (e *Executor) executePatch(patch *grammar.PatchClause, bindings matcher.Bindings) error {
	for _, stmt := range patch.Stmts {
		if stmt.If != nil {
			// Evaluate condition
			if matcher.EvalPredicate(stmt.If.Condition, bindings) {
				// Apply nested statements
				for _, nested := range stmt.If.Stmts {
					if err := e.executePatchStmt(nested, bindings); err != nil {
						return err
					}
				}
			}
			continue
		}

		if err := e.executePatchStmt(stmt, bindings); err != nil {
			return err
		}
	}
	return nil
}

func (e *Executor) executePatchStmt(stmt *grammar.PatchStmt, bindings matcher.Bindings) error {
	if stmt.Rename != nil {
		target, ok := bindings[stmt.Rename.Binding]
		if !ok {
			return fmt.Errorf("binding $%s not found", stmt.Rename.Binding)
		}

		ident, ok := target.(*ast.Ident)
		if !ok {
			return fmt.Errorf("$%s is not an identifier", stmt.Rename.Binding)
		}

		newName := strings.Trim(stmt.Rename.NewName, `"`)
		ident.Name = newName
		return nil
	}

	if stmt.Set != nil {
		// Set field value - more complex, handle common cases
		return e.executeSet(stmt.Set, bindings)
	}

	if stmt.Retype != nil {
		// Retype - change type of a node
		return fmt.Errorf("retype not yet implemented")
	}

	return nil
}

func (e *Executor) executeSet(set *grammar.SetStmt, bindings matcher.Bindings) error {
	// Handle special case: $Params.first = "ctx context.Context"
	target, ok := bindings[set.Path.Binding]
	if !ok {
		return fmt.Errorf("binding $%s not found", set.Path.Binding)
	}

	if len(set.Path.Segments) == 1 && set.Path.Segments[0] == "first" {
		// Prepend to a FieldList
		fl, ok := target.(*ast.FieldList)
		if !ok {
			return fmt.Errorf("$%s is not a FieldList", set.Path.Binding)
		}

		if set.Value.String == nil {
			return fmt.Errorf("set value must be a string")
		}

		// Parse the field spec
		fieldSpec := strings.Trim(*set.Value.String, `"`)
		parts := strings.SplitN(fieldSpec, " ", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid field spec: %s", fieldSpec)
		}

		// Create new field
		newField := &ast.Field{
			Names: []*ast.Ident{{Name: parts[0]}},
			Type:  parseTypeExpr(parts[1]),
		}

		// Prepend
		if fl.List == nil {
			fl.List = []*ast.Field{newField}
		} else {
			fl.List = append([]*ast.Field{newField}, fl.List...)
		}

		// Track import
		if strings.Contains(parts[1], "context.") {
			e.imports["context"] = true
		}

		return nil
	}

	return fmt.Errorf("set path %s.%v not yet supported", set.Path.Binding, set.Path.Segments)
}

// executeDelete handles delete actions.
func (e *Executor) executeDelete(del *grammar.DeleteClause, bindings matcher.Bindings) error {
	// Delete implementation would remove nodes from the AST
	// For now, just log what would be deleted
	for _, stmt := range del.Stmts {
		_ = stmt // TODO: implement deletion
	}
	return fmt.Errorf("delete not yet implemented")
}

// executeEmit handles emit actions (generate new files).
func (e *Executor) executeEmit(emit *grammar.EmitClause, bindings matcher.Bindings) (string, error) {
	var content string

	if emit.Template != nil {
		// Template mode - just interpolate
		content = strings.Trim(emit.Template.Text, "`")
		content = e.interpolate(content, bindings)
	} else if emit.CodeBody != nil {
		// Code mode - interpolate Go code
		content = strings.Trim(emit.CodeBody.Text, "`")
		content = e.interpolate(content, bindings)

		// Add package declaration if specified
		if emit.Package != nil {
			content = fmt.Sprintf("package %s\n\n%s", *emit.Package, content)
		}
	} else if emit.ASTBody != nil {
		// AST mode - build AST and render
		return "", fmt.Errorf("emit ast mode not yet implemented")
	}

	return content, nil
}

// interpolate replaces ${Var} and ${Var | transform} in text.
func (e *Executor) interpolate(text string, bindings matcher.Bindings) string {
	// Match ${Name} or ${Name | transform}
	re := regexp.MustCompile(`\$\{(\w+)(?:\s*\|\s*(\w+))?\}`)

	return re.ReplaceAllStringFunc(text, func(match string) string {
		parts := re.FindStringSubmatch(match)
		name := parts[1]
		transform := parts[2]

		val, ok := bindings[name]
		if !ok {
			return match // leave unchanged if not found
		}

		str := bindingToString(val)

		if transform != "" {
			str = applyTransform(str, transform)
		}

		return str
	})
}

// addImports adds any required imports to the file.
func (e *Executor) addImports() {
	if len(e.imports) == 0 {
		return
	}

	// Find or create import declaration
	var importDecl *ast.GenDecl
	for _, decl := range e.file.Decls {
		if gd, ok := decl.(*ast.GenDecl); ok && gd.Tok == token.IMPORT {
			importDecl = gd
			break
		}
	}

	if importDecl == nil {
		// Create new import declaration
		importDecl = &ast.GenDecl{
			Tok:    token.IMPORT,
			Lparen: 1, // force parenthesized
			Rparen: 1,
		}
		// Insert after package clause
		e.file.Decls = append([]ast.Decl{importDecl}, e.file.Decls...)
	}

	// Check existing imports
	existing := make(map[string]bool)
	for _, spec := range importDecl.Specs {
		if is, ok := spec.(*ast.ImportSpec); ok {
			path := strings.Trim(is.Path.Value, `"`)
			existing[path] = true
		}
	}

	// Add missing imports
	for imp := range e.imports {
		if !existing[imp] {
			importDecl.Specs = append(importDecl.Specs, &ast.ImportSpec{
				Path: &ast.BasicLit{
					Kind:  token.STRING,
					Value: fmt.Sprintf(`"%s"`, imp),
				},
			})
		}
	}

	// Ensure parentheses if multiple imports
	if len(importDecl.Specs) > 1 {
		importDecl.Lparen = 1
		importDecl.Rparen = 1
	}
}

// parseStatements parses a string as Go statements.
func parseStatements(code string) ([]ast.Stmt, error) {
	// Wrap in a function to parse as statements
	wrapped := fmt.Sprintf("package p\nfunc f() {\n%s\n}", code)
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "", wrapped, 0)
	if err != nil {
		return nil, err
	}

	// Extract statements from the function body
	for _, decl := range f.Decls {
		if fd, ok := decl.(*ast.FuncDecl); ok {
			return fd.Body.List, nil
		}
	}
	return nil, fmt.Errorf("no statements found")
}

// parseTypeExpr parses a type expression string.
func parseTypeExpr(typeStr string) ast.Expr {
	// Handle common cases
	if strings.Contains(typeStr, ".") {
		parts := strings.SplitN(typeStr, ".", 2)
		return &ast.SelectorExpr{
			X:   &ast.Ident{Name: parts[0]},
			Sel: &ast.Ident{Name: parts[1]},
		}
	}
	return &ast.Ident{Name: typeStr}
}

// bindingToString converts a binding value to string.
func bindingToString(v any) string {
	switch val := v.(type) {
	case *ast.Ident:
		return val.Name
	case string:
		return val
	case *ast.BasicLit:
		return val.Value
	default:
		return fmt.Sprintf("%v", v)
	}
}

// applyTransform applies a named transform to a string.
func applyTransform(s string, transform string) string {
	switch transform {
	case "snake_case":
		return toSnakeCase(s)
	case "lower":
		return strings.ToLower(s)
	case "upper":
		return strings.ToUpper(s)
	case "camel_case":
		return toCamelCase(s)
	default:
		return s
	}
}

// toSnakeCase converts PascalCase to snake_case.
func toSnakeCase(s string) string {
	var result bytes.Buffer
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			result.WriteByte('_')
		}
		result.WriteRune(r)
	}
	return strings.ToLower(result.String())
}

// toCamelCase converts snake_case to camelCase.
func toCamelCase(s string) string {
	parts := strings.Split(s, "_")
	for i := 1; i < len(parts); i++ {
		if len(parts[i]) > 0 {
			parts[i] = strings.ToUpper(parts[i][:1]) + parts[i][1:]
		}
	}
	return strings.Join(parts, "")
}
