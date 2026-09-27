package gogen

import (
	"fmt"
	"strings"
)

// renderRegister renders "<module>/register.go": ModuleImpls, a struct with
// one field per service in the module (field name = service name, field
// type = that service's own generated interface from service.go), and
// RegisterModule, which wires every service's HTTP routes (router.go's
// Register<Service>Routes) and gRPC server (grpc.go's New<Service>GRPCServer
// plus the real pb.Register<Service>Server protoc-gen-go-grpc generates) in
// one call. This is the one call a project's main.go needs per module,
// instead of one Register<Service>Routes/New<Service>GRPCServer/
// pb.Register<Service>Server per service. This file contains no business
// logic and is always regenerated.
//
// GRPCPolicies() (grpc.go) is deliberately NOT merged in here: the
// authz.UnaryServerInterceptor it feeds is attached once, at grpc.Server
// construction time, which necessarily happens before RegisterModule (this
// function needs an already-constructed *grpc.Server to register services
// on) -- so merging every module's GRPCPolicies() into one map is still the
// caller's job, done once, before any module's RegisterModule is called.
func renderRegister(pkg string, data moduleModel) string {
	withAuthz := needsAuthzImports(data)

	var body strings.Builder

	renderModuleImpls(&body, data)
	renderRegisterModuleFunc(&body, data, withAuthz)

	var b strings.Builder

	b.WriteString(generatedHeader)
	fmt.Fprintf(&b, "// Package %s holds the generated per-module registration aggregator for\n", pkg)
	fmt.Fprintf(&b, "// the %s module.\n", pkg)
	fmt.Fprintf(&b, "package %s\n\n", pkg)
	b.WriteString("import (\n")
	fmt.Fprintf(&b, "\t%q\n\n", pkgGRPC)

	if withAuthz {
		fmt.Fprintf(&b, "\t%q\n", pkgAuth)
		fmt.Fprintf(&b, "\t%q\n\n", pkgPermission)
	}

	fmt.Fprintf(&b, "\t%q\n\n", pkgRouter)
	fmt.Fprintf(&b, "\t%s %q\n", data.PBAlias, data.PBImportPath)
	b.WriteString(")\n\n")
	b.WriteString(body.String())

	return b.String()
}

// renderModuleImpls emits the ModuleImpls struct: one field per service,
// named exactly after the service (matching how a developer would naturally
// refer to it in a struct literal, e.g. "ModuleImpls{UserService: impl}").
func renderModuleImpls(b *strings.Builder, data moduleModel) {
	b.WriteString("// ModuleImpls names one business-logic implementation per service this\n")
	b.WriteString("// module declares, for RegisterModule to wire onto both transports at once.\n")
	b.WriteString("type ModuleImpls struct {\n")

	for _, svc := range data.Services {
		fmt.Fprintf(b, "\t%s %s\n", svc.Name, svc.Name)
	}

	b.WriteString("}\n\n")
}

// renderRegisterModuleFunc emits RegisterModule, calling each service's
// Register<Service>Routes and wiring its gRPC adapter, in schema declaration
// order. withAuthz mirrors router.go's own gate exactly (see
// renderRegisterFunc's doc comment): every service in one module shares one
// uniform Register<Service>Routes signature, so RegisterModule's own
// signature only needs auth.Auth/permission.Checker params when the module
// needs them at all.
func renderRegisterModuleFunc(b *strings.Builder, data moduleModel, withAuthz bool) {
	b.WriteString("// RegisterModule wires every service's HTTP routes and gRPC server onto r\n")
	b.WriteString("// and grpcServer respectively, from impls. Call this once per module against\n")
	b.WriteString("// the project's one shared router.Router and *grpc.Server -- merge every\n")
	b.WriteString("// module's own GRPCPolicies() into the interceptor passed to grpcServer's\n")
	b.WriteString("// construction first (see grpc.go's GRPCPolicies doc comment); RegisterModule\n")
	b.WriteString("// only registers services, it does not touch the interceptor.\n")

	if withAuthz {
		b.WriteString("func RegisterModule(r router.Router, grpcServer *grpc.Server, a auth.Auth, p permission.Checker, impls ModuleImpls) {\n")
	} else {
		b.WriteString("func RegisterModule(r router.Router, grpcServer *grpc.Server, impls ModuleImpls) {\n")
	}

	for _, svc := range data.Services {
		if withAuthz {
			fmt.Fprintf(b, "\tRegister%sRoutes(r, impls.%s, a, p)\n", svc.Name, svc.Name)
		} else {
			fmt.Fprintf(b, "\tRegister%sRoutes(r, impls.%s)\n", svc.Name, svc.Name)
		}

		fmt.Fprintf(b, "\tpb.Register%sServer(grpcServer, New%sGRPCServer(impls.%s))\n", svc.Name, svc.Name, svc.Name)
	}

	b.WriteString("}\n")
}
