package grammar

import (
	"encoding/json"
	"fmt"
	"testing"
)

func TestBasicStructMatch(t *testing.T) {
	input := `
lift "interface-from-struct" {
	from go {
		match TypeSpec {
			name: $Name
			type: StructType {
				fields: $Fields...
			}
		}

		match FuncDecl {
			recv: StarExpr { x: $Name }
			name: $MethodName
			type: $MethodType
		}
	}

	where {
		$Name.exported
		len($Methods) > 0
	}

	emit go {
		file "service.go"
		package main

		ast {
			GenDecl {
				tok: "TYPE"
				specs: TypeSpec {
					name: "Service"
					type: InterfaceType {
						methods: for $m in $Methods {
							Field {
								names: [$m.Name]
								type: $m.MethodType
							}
						}
					}
				}
			}
		}
	}
}
`
	parser, err := NewParser()
	if err != nil {
		t.Fatalf("failed to build parser: %v", err)
	}

	prog, err := parser.ParseString("test.lift", input)
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	if len(prog.Blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(prog.Blocks))
	}

	block := prog.Blocks[0]
	if block.From == nil {
		t.Fatal("expected from clause")
	}
	if len(block.From.Matchers) != 2 {
		t.Fatalf("expected 2 matchers, got %d", len(block.From.Matchers))
	}
	if block.From.Matchers[0].NodeType != "TypeSpec" {
		t.Errorf("expected TypeSpec, got %s", block.From.Matchers[0].NodeType)
	}
	if block.From.Matchers[1].NodeType != "FuncDecl" {
		t.Errorf("expected FuncDecl, got %s", block.From.Matchers[1].NodeType)
	}
	if len(block.Where) != 1 || len(block.Where[0].Predicates) != 2 {
		t.Error("expected 1 where with 2 predicates")
	}
	if len(block.Actions) != 1 || block.Actions[0].Emit == nil {
		t.Fatal("expected 1 emit action")
	}
	if block.Actions[0].Emit.Target != "go" {
		t.Errorf("expected target go, got %s", block.Actions[0].Emit.Target)
	}

	t.Log("✓ Basic struct match parsed")
}

func TestDeepMatchWithContains(t *testing.T) {
	input := `
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

	insert code {
		prepend $Body
	}
}
`
	parser, err := NewParser()
	if err != nil {
		t.Fatalf("failed to build parser: %v", err)
	}

	prog, err := parser.ParseString("timeout.lift", input)
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	block := prog.Blocks[0]
	deep := block.From.Matchers[1]
	if deep.In == nil || *deep.In != "Body" {
		t.Errorf("expected deep match in $Body")
	}

	preds := block.Where[0].Predicates
	if preds[0].MemberCheck == nil || len(preds[0].MemberCheck.Values) != 4 {
		t.Error("expected MemberCheck with 4 values")
	}
	if preds[1].Not == nil || preds[1].Not.Contains == nil {
		t.Error("expected not contains()")
	}

	action := block.Actions[0]
	if action.Insert == nil || action.Insert.Mode != "code" {
		t.Fatal("expected insert code action")
	}
	if action.Insert.Position.Kind != "prepend" {
		t.Errorf("expected prepend, got %s", action.Insert.Position.Kind)
	}

	t.Log("✓ Deep match with contains parsed")
}

func TestPatchWithConditional(t *testing.T) {
	input := `
lift "add-ctx-param" {
	from go {
		match FuncDecl {
			name: $FuncName
			type: FuncType {
				params: $Params...
			}
		}
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
}
`
	parser, err := NewParser()
	if err != nil {
		t.Fatalf("failed to build parser: %v", err)
	}

	prog, err := parser.ParseString("patch.lift", input)
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	stmts := prog.Blocks[0].Actions[0].Patch.Stmts
	if len(stmts) != 1 || stmts[0].If == nil {
		t.Fatal("expected 1 conditional patch")
	}

	t.Log("✓ Conditional patch parsed")
}

func TestDeleteAction(t *testing.T) {
	input := `
lift "remove-tags" {
	from go {
		match TypeSpec {
			name: $Name
			type: StructType {
				fields: $Fields...
			}
		}
	}

	delete {
		remove $Fields.tags
	}
}
`
	parser, err := NewParser()
	if err != nil {
		t.Fatalf("failed to build parser: %v", err)
	}

	prog, err := parser.ParseString("delete.lift", input)
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	if prog.Blocks[0].Actions[0].Delete == nil {
		t.Fatal("expected delete action")
	}
	if len(prog.Blocks[0].Actions[0].Delete.Stmts) != 1 {
		t.Fatal("expected 1 delete stmt")
	}

	t.Log("✓ Delete action parsed")
}

func TestEmitProtoTemplate(t *testing.T) {
	input := `
lift "proto-from-struct" {
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
		template {` + " `" + `syntax = "proto3"; message ${Name} { ${Fields} }` + "` }" + `
	}
}
`
	parser, err := NewParser()
	if err != nil {
		t.Fatalf("failed to build parser: %v", err)
	}

	prog, err := parser.ParseString("proto.lift", input)
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	emit := prog.Blocks[0].Actions[0].Emit
	if emit == nil || emit.Target != "proto" || emit.Template == nil {
		t.Fatal("expected proto template emit")
	}

	t.Log("✓ Proto template emission parsed")
}

func TestMultipleActions(t *testing.T) {
	input := `
lift "full-entity" {
	from go {
		match TypeSpec {
			name: $Name
			type: StructType { fields: $Fields... }
		}
	}

	emit go {
		file "interface.go"
		package main
		ast { GenDecl { tok: "TYPE" } }
	}

	emit proto {
		file "model.proto"
		template {` + " `" + `syntax = "proto3";` + "` }" + `
	}

	emit sql {
		file "migration.sql"
		template {` + " `" + `CREATE TABLE test;` + "` }" + `
	}
}
`
	parser, err := NewParser()
	if err != nil {
		t.Fatalf("failed to build parser: %v", err)
	}

	prog, err := parser.ParseString("multi.lift", input)
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	block := prog.Blocks[0]
	if len(block.Actions) != 3 {
		t.Fatalf("expected 3 actions, got %d", len(block.Actions))
	}
	for i, target := range []string{"go", "proto", "sql"} {
		if block.Actions[i].Emit == nil || block.Actions[i].Emit.Target != target {
			t.Errorf("action[%d]: expected target %s", i, target)
		}
	}

	t.Log("✓ Multiple actions parsed")
}

func TestPrettyPrint(t *testing.T) {
	input := `
lift "demo" {
	from go {
		match FuncDecl {
			recv: StarExpr { x: $RecvType }
			name: $FuncName
			type: FuncType {
				params: $Params...
				results: $Results...
			}
			body: $Body
		}

		match CallExpr in $Body {
			fun: SelectorExpr {
				x: Ident { name: "http" }
				sel: $Method
			}
		}
	}

	where {
		$Method in ["Get", "Post"]
		$FuncName.exported
	}

	patch {
		rename $FuncName "Patched"
	}

	emit go {
		file "generated.go"
		package main
		ast { GenDecl { tok: "TYPE" } }
	}
}
`
	parser, err := NewParser()
	if err != nil {
		t.Fatalf("failed to build parser: %v", err)
	}

	prog, err := parser.ParseString("demo.lift", input)
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	data, _ := json.MarshalIndent(prog, "", "  ")
	if testing.Verbose() {
		fmt.Println(string(data))
	}

	block := prog.Blocks[0]
	if len(block.From.Matchers) != 2 {
		t.Fatalf("expected 2 matchers")
	}
	if len(block.Actions) != 2 {
		t.Fatalf("expected 2 actions")
	}

	t.Log("✓ Pretty print / full structure verified")
}

func TestWildcardAndListMatch(t *testing.T) {
	input := `
lift "wildcard-test" {
	from go {
		match FuncDecl {
			name: _
			type: FuncType {
				params: [
					Field { type: Ident { name: "int" } },
					$Second
				]
			}
		}
	}

	delete {
		remove $Second
	}
}
`
	parser, err := NewParser()
	if err != nil {
		t.Fatalf("failed to build parser: %v", err)
	}

	prog, err := parser.ParseString("wildcard.lift", input)
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	matcher := prog.Blocks[0].From.Matchers[0]
	if !matcher.Fields[0].Value.Wild {
		t.Error("expected wildcard for name")
	}

	paramsField := matcher.Fields[1].Value.Pattern.Fields[0]
	if len(paramsField.Value.List) != 2 {
		t.Fatalf("expected list of 2, got %d", len(paramsField.Value.List))
	}

	t.Log("✓ Wildcard and list match parsed")
}

func TestEmitCodeMode(t *testing.T) {
	input := `
lift "repo-gen" {
	from go {
		match TypeSpec {
			name: $Name
			type: StructType { fields: $Fields... }
		}
	}

	emit go {
		file "repo.go"
		package main
		code {` + " `" + `type ${Name}Repository struct { db *sql.DB }` + "` }" + `
	}
}
`
	parser, err := NewParser()
	if err != nil {
		t.Fatalf("failed to build parser: %v", err)
	}

	prog, err := parser.ParseString("code.lift", input)
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	if prog.Blocks[0].Actions[0].Emit.CodeBody == nil {
		t.Fatal("expected code emit body")
	}

	t.Log("✓ Code mode emit parsed")
}

func TestRenameAndRetype(t *testing.T) {
	input := `
lift "transform" {
	from go {
		match TypeSpec {
			name: $Name
			type: StructType { fields: $Fields... }
		}
	}

	patch {
		rename $Name "Renamed"
		retype $Fields "string"
	}
}
`
	parser, err := NewParser()
	if err != nil {
		t.Fatalf("failed to build parser: %v", err)
	}

	prog, err := parser.ParseString("transform.lift", input)
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	stmts := prog.Blocks[0].Actions[0].Patch.Stmts
	if len(stmts) != 2 {
		t.Fatalf("expected 2 patch stmts, got %d", len(stmts))
	}
	if stmts[0].Rename == nil || stmts[1].Retype == nil {
		t.Error("expected rename + retype")
	}

	t.Log("✓ Rename and retype parsed")
}
