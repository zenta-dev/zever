package testdata

// Fixture: zever paths inside a code-generator template, a string
// constant, and comments must not count as imports.

const serverTemplate = `package app

import "github.com/zenta-dev/zever/core/cache"

func run() {}
`

const pathConstant = "github.com/zenta-dev/zever/core/queue"

// import "github.com/zenta-dev/zever/core/log"

/* import "github.com/zenta-dev/zever/core/db" */

var _ = serverTemplate + pathConstant
