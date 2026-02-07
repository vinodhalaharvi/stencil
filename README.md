# Stencil

**Structural code matching and generation for Go.**

Stencil reads `.lift` files that describe patterns to match in Go source code and actions to perform — patching, deleting, inserting, or emitting new code in Go, protobuf, SQL, GraphQL, and other targets.

## Why Stencil?

Tools like semgrep and ast-grep use YAML for configuration. YAML breaks down when you need arbitrary nesting, type safety, multi-target generation, or composability. Stencil uses a **typed EBNF grammar** parsed by [Participle](https://github.com/alecthomas/participle), giving you all of the above. The `.lift` files live outside your source code and can be applied to any Go codebase.

## Quick Start

```bash
make build
./stencil parse examples/entity-service.lift
./stencil match examples/enforce-ctx-timeout.lift --source testdata/bad_http_client.go
make test
```

## Example: Find Functions Missing Context Timeout

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

Run matching:

```bash
$ ./stencil match examples/enforce-ctx-timeout.lift --source testdata/bad_http_client.go

Block "enforce-ctx-timeout": 4 match(es)
  [1] testdata/bad_http_client.go:17
      $FuncName = GetUser
      $CallName = Get
  [2] testdata/bad_http_client.go:32
      $FuncName = CreateUser
      $CallName = Post
  [3] testdata/bad_http_client.go:46
      $FuncName = FetchAll
      $CallName = Get
  [4] testdata/bad_http_client.go:58
      $FuncName = DialBackend
      $CallName = Dial

Total: 4 match(es)
```

## Project Structure

```
stencil/
├── main.go                     # CLI entry point
├── grammar/
│   ├── grammar.go              # Participle AST types
│   ├── grammar_test.go         # Unit tests
│   └── examples_test.go        # Integration tests
├── matcher/
│   ├── matcher.go              # Go AST pattern matcher
│   └── matcher_test.go         # Matcher tests
├── examples/
│   ├── enforce-ctx-timeout.lift
│   └── entity-service.lift
├── testdata/
│   ├── bad_http_client.go      # Example: missing timeouts
│   └── good_http_client.go     # Example: proper timeouts
├── Makefile
└── README.md
```

## Roadmap

- [x] Phase 1: `.lift` grammar + Participle parser
- [x] Phase 2: Go AST matcher (walk `go/ast`, match patterns, return bindings)
- [ ] Phase 3: Action executor (patch/delete/insert/emit using bindings)
- [ ] Phase 4: CLI `stencil apply` command
- [ ] Phase 5: LSP server for `.lift` files

## License

MIT
