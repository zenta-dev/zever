package main

import "os"

// shouldShowHint reports whether hint footer should be emitted.
func shouldShowHint() bool {
	return os.Getenv("ZEVER_NO_HINT") == ""
}

// generalTips rotates through contextual tips. Keep deterministic for tests.
var generalTips = []string{
	"run 'zever doctor' to verify batteries",
	"run 'zever dev' for watch-mode (recompile + restart on change)",
	"run 'zever generate --help' for scaffolding commands",
	"run 'zever compile --help' to see available backends",
}

// hintFor returns a contextual hint for a given successful command.
func hintFor(cmd string) string {
	switch cmd {
	case "new":
		return "next: cd <app> && zever compile schema/app.zen && zever serve"
	case "generate module":
		return "next: zever generate entity <module> <Name> --field title:string"
	case "generate entity":
		return "next: zever compile to validate schema"
	case "generate job":
		return "next: zever generate schedule <module> <Name> --cron \"*/5 * * * *\" --dispatch <Job>"
	case "generate schedule":
		return "next: verify with zever compile"
	case "generate server":
		return "next: zever serve  •  or zever dev for watch-mode"
	case "generate worker":
		return "next: zever queue:work  •  or zever dev"
	case "generate seed":
		return "next: zever db seed"
	case "generate tinker":
		return "next: zever tinker"
	case "generate adapter":
		return "next: implement TODOs in the new adapter package"
	case "extract":
		return "next: cd <service> && go mod tidy && zever compile"
	case "compile":
		return "next: zever db migrate --adapter=sqlite --dsn=data/app.db schema/*.zen"
	case "doctor":
		return "next: zever compile schema/app.zen"
	case "routes":
		return "tip: add http binding to an RPC to see more routes"
	case "check":
		return "next: zever compile schema/*.zen --backend=proto,zenorm"
	case "fmt":
		return "next: zever check schema/*.zen"
	case "check-boundaries":
		return "tip: keep modules isolated — move shared entities to their own module"
	case "db migrate":
		return "next: zever db seed  •  or zever serve"
	case "db seed":
		return "next: zever serve"
	default:
		return ""
	}
}
