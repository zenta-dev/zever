package main

import (
	"go.lsp.dev/protocol"

	"github.com/zenta-dev/zever/dsl/ast"
	"github.com/zenta-dev/zever/dsl/diag"
)

// walkErrorCases calls fn once for every item in every rpc's errors: {...}
// set across the whole file, in declaration order. Each item is either an
// *ast.IdentValue (a bare error code) or an *ast.CallValue (a code with an
// optional positional string message) -- both already carry Name/Pos, per
// ast_service.go's RPCDecl.Errors doc comment -- so this only needs to
// destructure the two shapes, not interpret them.
func walkErrorCases(file *ast.File, fn func(name string, pos diag.Position)) {
	for _, decl := range flattenDecls(file) {
		svc, ok := decl.(*ast.ServiceDecl)
		if !ok {
			continue
		}

		for _, rpc := range svc.RPCs {
			for _, v := range rpc.Errors {
				switch val := v.(type) {
				case *ast.IdentValue:
					fn(val.Name, val.Pos)
				case *ast.CallValue:
					fn(val.Name, val.Pos)
				default:
				}
			}
		}
	}
}

// errorCaseAt returns the error-case identifier under the cursor, if any.
func errorCaseAt(file *ast.File, cursor protocol.Position) (name string, pos diag.Position, ok bool) {
	walkErrorCases(file, func(n string, p diag.Position) {
		if ok || !coversIdent(p, n, cursor) {
			return
		}

		name, pos, ok = n, p, true
	})

	return name, pos, ok
}
