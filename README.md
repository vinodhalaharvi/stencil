# Stencil

**Structural code matching and generation for Go.**

Stencil reads `.lift` files that describe patterns to match in Go source code and actions to perform — patching, deleting, inserting, or emitting new code in Go, protobuf, SQL, GraphQL, and other targets.

## Why Stencil?

Tools like semgrep and ast-grep use YAML for configuration. YAML breaks down when you need arbitrary nesting, type safety, multi-target generation, or composability. Stencil uses a **typed EBNF grammar** parsed by [Participle](https://github.com/alecthomas/participle), giving you all of the above. The `.lift` files live outside your source code and can be applied to any Go codebase.

## Quick Start

```bash
make build
./stencil parse examples/entity-service.lift
./stencil inspect examples/enforce-ctx-timeout.lift
make test
```

## Example: Enforce Context Timeout

```
lift "enforce-ctx-timeout" {
    from go {
        match FuncDecl {
            name: $FuncName
            type: FuncType { params: $Params... results: $Results... }
            body: $Body
        }
        match CallExpr in $Body {
            fun: SelectorExpr { sel: $CallName }
            args: $CallArgs...
        }
    }

    where {
        $CallName in ["Get", "Post", "Do", "Dial", "NewRequest"]
        not contains($Body, CallExpr {
            fun: SelectorExpr {
                x: Ident { name: "context" }
                sel: Ident { name: "WithTimeout" }
            }
        })
    }

    insert code {
        prepend $Body
        `ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
        defer cancel()`
    }
}
```

## Project Structure

```
stencil/
├── main.go                     # CLI entry point
├── grammar/
│   ├── grammar.go              # Participle AST types
│   ├── grammar_test.go         # Unit tests
│   └── examples_test.go        # Integration tests
├── examples/
│   ├── enforce-ctx-timeout.lift
│   └── entity-service.lift
├── Makefile
└── README.md
```

## Roadmap

- [x] Phase 1: `.lift` grammar + Participle parser
- [ ] Phase 2: Go AST matcher
- [ ] Phase 3: Action executor
- [ ] Phase 4: CLI `stencil apply`
- [ ] Phase 5: LSP server for `.lift` files

## License

MIT
