package main

// GenerateScreen is the TUI hub for the `zever generate <subcommand>`
// family: a kind picker form, then a kind-specific huh form, then preview,
// confirm, and an ExecModel run of the matching Generate core. Server,
// worker and seed entrypoint paths resolve from the project config inside
// the ExecFunc, never on the Update path.

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	huh "charm.land/huh/v2"

	"github.com/zenta-dev/zever/cmd/zever/tui"
)

// generateKinds is the full `zever generate` subcommand set in usage order.
var generateKinds = []string{
	"module", "entity", "job", "schedule", "server", "worker", "seed", "tinker", "adapter",
}

// GenerateScreen drives the generate family. kindForm picks the subcommand;
// once chosen, flow.form collects its arguments. Shared field slots are
// reused across kinds (only the active subform's slots matter).
type GenerateScreen struct {
	flow       scaffoldFlow
	kindForm   *huh.Form
	kindChosen bool
	kind       string

	module      string
	name        string
	queue       string
	cron        string
	dispatch    string
	fieldsText  string
	appPkg      string
	outDir      string
	battery     string
	adapterName string
	force       bool
}

// NewGenerateScreen builds the generate hub, defaulting to the entity kind
// and the `generate job` queue default.
func NewGenerateScreen() *GenerateScreen {
	m := &GenerateScreen{kind: "entity", queue: "default"}

	m.kindForm = huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("What to generate").
				Options(huh.NewOptions(generateKinds...)...).
				Value(&m.kind),
		),
	)
	m.flow = newScaffoldFlow("zever generate", nil)
	m.flow.onPrev = m.freezePreview
	m.flow.mkExec = m.buildExec

	return m
}

// buildSubform constructs the argument form for the chosen kind, bound to
// m's slots with the same inline validation the cores enforce.
func (m *GenerateScreen) buildSubform() *huh.Form {
	switch m.kind {
	case "module":
		return huh.NewForm(huh.NewGroup(
			huh.NewInput().Title("Module name").Validate(validateIdentField).Value(&m.name),
		))
	case "entity":
		return huh.NewForm(huh.NewGroup(
			huh.NewInput().Title("Module").Validate(validateIdentField).Value(&m.module),
			huh.NewInput().Title("Entity name").Validate(validateIdentField).Value(&m.name),
			huh.NewInput().Title("Fields (optional)").Description("Comma-separated name:type").
				Placeholder("title:string,total:int64").Validate(validateEntityFieldList).Value(&m.fieldsText),
		))
	case "job":
		return huh.NewForm(huh.NewGroup(
			huh.NewInput().Title("Module").Validate(validateIdentField).Value(&m.module),
			huh.NewInput().Title("Job name").Validate(validateIdentField).Value(&m.name),
			huh.NewInput().Title("Queue").Validate(validateOptionalIdentField).Value(&m.queue),
		))
	case "schedule":
		return huh.NewForm(huh.NewGroup(
			huh.NewInput().Title("Module").Validate(validateIdentField).Value(&m.module),
			huh.NewInput().Title("Schedule name").Validate(validateIdentField).Value(&m.name),
			huh.NewInput().Title("Cron spec").Placeholder("*/5 * * * *").
				Validate(validateCronField).Value(&m.cron),
			huh.NewInput().Title("Dispatch job").Description("Must name a job the module already declares").
				Validate(validateIdentField).Value(&m.dispatch),
		))
	case "server", "worker", "seed":
		return huh.NewForm(huh.NewGroup(
			huh.NewConfirm().Title("Overwrite the entrypoint if it already exists?").
				Affirmative("Force").Negative("No").Value(&m.force),
		))
	case "tinker":
		return huh.NewForm(huh.NewGroup(
			huh.NewInput().Title("App import path (optional)").Description("Defaults to <module>/internal/app").
				Validate(validateOptionalAppPath).Value(&m.appPkg),
			huh.NewInput().Title("Output directory (optional)").Description("Defaults to the project tinker_entry").
				Value(&m.outDir),
			huh.NewConfirm().Title("Overwrite the shim if it already exists?").
				Affirmative("Force").Negative("No").Value(&m.force),
		))
	case "adapter":
		return huh.NewForm(huh.NewGroup(
			huh.NewSelect[string]().Title("Battery").
				Options(huh.NewOptions(sortedKeys(batterySpecs)...)...).Value(&m.battery),
			huh.NewInput().Title("Adapter name").Description("Go package name").
				Validate(validatePackageNameField).Value(&m.adapterName),
			huh.NewInput().Title("Options fields (optional)").Description("Comma-separated key:type").
				Placeholder("api_key:string,timeout:duration").Validate(validateAdapterFieldList).Value(&m.fieldsText),
			huh.NewConfirm().Title("Overwrite the adapter files if they exist?").
				Affirmative("Force").Negative("No").Value(&m.force),
		))
	default:
		return huh.NewForm(huh.NewGroup(
			huh.NewInput().Title("Module name").Validate(validateIdentField).Value(&m.name),
		))
	}
}

// validateOptionalAppPath accepts empty (resolved at run) and otherwise
// requires a non-traversal path value.
func validateOptionalAppPath(v string) error {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	if isTraversalName(v) && (v == ".." || strings.HasPrefix(v, "/")) {
		return fmt.Errorf("%w: %q", ErrPathTraversal, v)
	}

	return nil
}

// generateCLI renders the exact equivalent CLI for the chosen kind. Pure.
func (m *GenerateScreen) generateCLI() string {
	queue := strings.TrimSpace(m.queue)
	if queue == "" {
		queue = "default"
	}

	switch m.kind {
	case "module":
		return "zever generate module " + cliQuote(strings.TrimSpace(m.name))
	case "entity":
		var b strings.Builder
		b.WriteString("zever generate entity " + cliQuote(strings.TrimSpace(m.module)) + " " + cliQuote(strings.TrimSpace(m.name)))
		for _, f := range splitFieldList(m.fieldsText) {
			b.WriteString(" --field " + cliQuote(f))
		}

		return b.String()
	case "job":
		return "zever generate job " + cliQuote(strings.TrimSpace(m.module)) + " " +
			cliQuote(strings.TrimSpace(m.name)) + " --queue " + cliQuote(queue)
	case "schedule":
		return "zever generate schedule " + cliQuote(strings.TrimSpace(m.module)) + " " +
			cliQuote(strings.TrimSpace(m.name)) + " --cron " + cliQuote(strings.TrimSpace(m.cron)) +
			" --dispatch " + cliQuote(strings.TrimSpace(m.dispatch))
	case "server":
		return "zever generate server" + forceFlag(m.force)
	case "worker":
		return "zever generate worker" + forceFlag(m.force)
	case "seed":
		return "zever generate seed" + forceFlag(m.force)
	case "tinker":
		return "zever generate tinker" + cliFlag("app", m.appPkg) + cliFlag("dir", m.outDir) + forceFlag(m.force)
	case "adapter":
		var b strings.Builder
		b.WriteString("zever generate adapter " + cliQuote(strings.TrimSpace(m.battery)) + " " + cliQuote(strings.TrimSpace(m.adapterName)))
		for _, f := range splitFieldList(m.fieldsText) {
			b.WriteString(" --field " + cliQuote(f))
		}
		b.WriteString(forceFlag(m.force))

		return b.String()
	default:
		return "zever generate"
	}
}

// forceFlag renders " --force" when set, else "".
func forceFlag(force bool) string {
	if force {
		return " --force"
	}

	return ""
}

// splitFieldList splits a comma field list without validating, for CLI echo.
func splitFieldList(s string) []string {
	var out []string

	for _, part := range strings.Split(s, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}

	return out
}

// generatePreview freezes the resolved plan into preview lines. Pure.
func (m *GenerateScreen) generatePreview() []string {
	queue := strings.TrimSpace(m.queue)
	if queue == "" {
		queue = "default"
	}

	switch m.kind {
	case "module":
		return []string{previewRow("module", strings.TrimSpace(m.name))}
	case "entity":
		return []string{
			previewRow("module", strings.TrimSpace(m.module)),
			previewRow("entity", strings.TrimSpace(m.name)),
			previewRow("fields", strings.Join(splitFieldList(m.fieldsText), ", ")),
		}
	case "job":
		return []string{
			previewRow("module", strings.TrimSpace(m.module)),
			previewRow("job", strings.TrimSpace(m.name)),
			previewRow("queue", queue),
		}
	case "schedule":
		return []string{
			previewRow("module", strings.TrimSpace(m.module)),
			previewRow("schedule", strings.TrimSpace(m.name)),
			previewRow("cron", strings.TrimSpace(m.cron)),
			previewRow("dispatch", strings.TrimSpace(m.dispatch)),
		}
	case "server", "worker", "seed":
		return []string{
			previewRow("entrypoint", m.kind+" (paths resolve from project config at run)"),
			previewRow("force", strconv.FormatBool(m.force)),
		}
	case "tinker":
		return []string{
			previewRow("app", strings.TrimSpace(m.appPkg)),
			previewRow("dir", strings.TrimSpace(m.outDir)),
			previewRow("force", strconv.FormatBool(m.force)),
		}
	case "adapter":
		return []string{
			previewRow("battery", strings.TrimSpace(m.battery)),
			previewRow("adapter", strings.TrimSpace(m.adapterName)),
			previewRow("fields", strings.Join(splitFieldList(m.fieldsText), ", ")),
			previewRow("force", strconv.FormatBool(m.force)),
		}
	default:
		return []string{previewRow("kind", m.kind)}
	}
}

// freezePreview resolves the completed subform into preview lines and CLI.
func (m *GenerateScreen) freezePreview() {
	m.flow.preview = m.generatePreview()
	m.flow.cli = m.generateCLI()
}

// buildExec wires the frozen values to the matching Generate core.
func (m *GenerateScreen) buildExec() tui.ExecModel {
	cli := m.generateCLI()
	title := "zever generate " + m.kind

	switch m.kind {
	case "module":
		name := strings.TrimSpace(m.name)
		return tui.NewExec(title, cli, func(context.Context) (string, error) {
			return runGenerateModuleScreen(name)
		})
	case "entity":
		module, name, fieldsText := strings.TrimSpace(m.module), strings.TrimSpace(m.name), m.fieldsText
		return tui.NewExec(title, cli, func(context.Context) (string, error) {
			return runGenerateEntityScreen(module, name, fieldsText)
		})
	case "job":
		module, name, queue := strings.TrimSpace(m.module), strings.TrimSpace(m.name), strings.TrimSpace(m.queue)
		return tui.NewExec(title, cli, func(context.Context) (string, error) {
			return runGenerateJobScreen(module, name, queue)
		})
	case "schedule":
		module, name, cron, dispatch := strings.TrimSpace(m.module), strings.TrimSpace(m.name), strings.TrimSpace(m.cron), strings.TrimSpace(m.dispatch)
		return tui.NewExec(title, cli, func(context.Context) (string, error) {
			return runGenerateScheduleScreen(module, name, cron, dispatch)
		})
	case "server":
		force := m.force
		return tui.NewExec(title, cli, func(context.Context) (string, error) {
			return runGenerateServerScreen(force) //nolint:contextcheck // CLI entrypoint helpers take no ctx by convention; threading ctx through the CLI layer is out of scope
		})
	case "worker":
		force := m.force
		return tui.NewExec(title, cli, func(context.Context) (string, error) {
			return runGenerateWorkerScreen(force) //nolint:contextcheck // CLI entrypoint helpers take no ctx by convention; threading ctx through the CLI layer is out of scope
		})
	case "seed":
		force := m.force
		return tui.NewExec(title, cli, func(context.Context) (string, error) {
			return runGenerateSeedScreen(force)
		})
	case "tinker":
		appPkg, outDir, force := strings.TrimSpace(m.appPkg), strings.TrimSpace(m.outDir), m.force
		return tui.NewExec(title, cli, func(context.Context) (string, error) {
			return runGenerateTinkerScreen(appPkg, outDir, force)
		})
	case "adapter":
		battery, name, fieldsText, force := strings.TrimSpace(m.battery), strings.TrimSpace(m.adapterName), m.fieldsText, m.force
		return tui.NewExec(title, cli, func(context.Context) (string, error) {
			return runGenerateAdapterScreen(battery, name, fieldsText, force)
		})
	default:
		return tui.NewExec(title, cli, func(context.Context) (string, error) {
			return "", fmt.Errorf("zever generate: unknown kind %q", m.kind)
		})
	}
}

// runGenerateModuleScreen resolves the schema dir and scaffolds the module.
// It mirrors runGenerateModule's resolution minus flag parsing and prompts.
func runGenerateModuleScreen(name string) (string, error) {
	schemaDir := defaultSchemaDir
	if pc, err := loadProjectConfig(); err == nil && pc.SchemaDir != "" {
		schemaDir = pc.SchemaDir
	}

	return GenerateModule(GenerateModuleConfig{Name: name, Lang: "go", SchemaDir: schemaDir})
}

// runGenerateEntityScreen parses the field list and appends the entity.
func runGenerateEntityScreen(module, name, fieldsText string) (string, error) {
	fields, err := parseEntityFieldList(fieldsText)
	if err != nil {
		return "", err
	}

	return GenerateEntity(GenerateEntityConfig{Module: module, Name: name, Fields: fields})
}

// runGenerateJobScreen appends the job, defaulting an empty queue.
func runGenerateJobScreen(module, name, queue string) (string, error) {
	if strings.TrimSpace(queue) == "" {
		queue = "default"
	}

	return GenerateJob(GenerateJobConfig{Module: module, Name: name, Queue: queue})
}

// runGenerateScheduleScreen appends the schedule.
func runGenerateScheduleScreen(module, name, cron, dispatch string) (string, error) {
	return GenerateSchedule(GenerateScheduleConfig{Module: module, Name: name, Cron: cron, Dispatch: dispatch})
}

// runGenerateServerScreen resolves project paths and renders the server
// entrypoint. It mirrors runGenerateServer minus flags and prompts.
func runGenerateServerScreen(force bool) (string, error) {
	project, err := loadProjectConfig()
	if err != nil {
		return "", err
	}

	modulePath, err := goModulePath()
	if err != nil {
		return "", err
	}

	// "./generated" mirrors runGenerateServer's --out flag default exactly:
	// the flag path never consults project.GeneratedDir either.
	res, err := GenerateServer(GenerateServerConfig{
		ModulePath:       modulePath,
		OutDir:           "./generated",
		SchemaDir:        project.SchemaDir,
		ServerEntry:      project.ServerEntry,
		Force:            force,
		RateLimitEnabled: rateLimitConfigured(),
	})
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("server entrypoint at %s (%d stub(s))", res.Entrypoint, len(res.Stubs)), nil
}

// runGenerateWorkerScreen resolves project paths and renders the worker
// entrypoint. It mirrors runGenerateWorker minus flags and prompts.
func runGenerateWorkerScreen(force bool) (string, error) {
	project, err := loadProjectConfig()
	if err != nil {
		return "", err
	}

	modulePath, err := goModulePath()
	if err != nil {
		return "", err
	}

	res, err := GenerateWorker(GenerateWorkerConfig{
		ModulePath:  modulePath,
		SchemaDir:   project.SchemaDir,
		WorkerEntry: project.WorkerEntry,
		Force:       force,
	})
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("worker entrypoint at %s (%d stub(s))", res.Entrypoint, len(res.Stubs)), nil
}

// runGenerateSeedScreen resolves project paths and renders the seed
// entrypoint. It mirrors runGenerateSeed minus flags and prompts.
func runGenerateSeedScreen(force bool) (string, error) {
	project, err := loadProjectConfig()
	if err != nil {
		return "", err
	}

	modulePath, err := goModulePath()
	if err != nil {
		return "", err
	}

	res, err := GenerateSeed(GenerateSeedConfig{
		ModulePath:   modulePath,
		SeedEntry:    project.SeedEntry,
		GeneratedDir: project.GeneratedDir,
		Force:        force,
	})
	if err != nil {
		return "", err
	}

	return "seed entrypoint at " + res.Entrypoint, nil
}

// runGenerateTinkerScreen resolves the app package and output dir defaults
// and renders the tinker shim. It mirrors runGenerateTinker minus flags and
// prompts.
func runGenerateTinkerScreen(appPkg, outDir string, force bool) (string, error) {
	if appPkg == "" {
		mod, err := currentModulePath()
		if err != nil {
			return "", err
		}

		appPkg = mod + "/internal/app"
	}
	if outDir == "" {
		pc, err := loadProjectConfig()
		if err != nil {
			return "", err
		}

		outDir = pc.TinkerEntry
	}

	return GenerateTinker(GenerateTinkerConfig{AppPackage: appPkg, OutDir: outDir, Force: force})
}

// runGenerateAdapterScreen parses the options field list and scaffolds the
// adapter package.
func runGenerateAdapterScreen(battery, name, fieldsText string, force bool) (string, error) {
	fields, err := parseAdapterFieldList(fieldsText)
	if err != nil {
		return "", err
	}

	res, err := GenerateAdapter(GenerateAdapterConfig{Battery: battery, Name: name, Fields: fields, Force: force})
	if err != nil {
		return "", err
	}

	return "adapter at " + res.Dir, nil
}

// chooseKind locks in the picked subcommand and swaps in its argument form.
// It runs whenever the kind picker completes, whether completion lands on a
// key press or on a pumped follow-up message.
func (m *GenerateScreen) chooseKind() tea.Cmd {
	m.kindChosen = true
	m.flow.form = m.buildSubform()
	m.flow.title = "zever generate " + m.kind

	return m.flow.form.Init()
}

// backCmd emits BackMsg for the parent to pop this screen.
func backCmd() tea.Msg { return tui.BackMsg{} }

// forwardKind routes msg through the kind picker form.
func (m *GenerateScreen) forwardKind(msg tea.Msg) tea.Cmd {
	updated, cmd := m.kindForm.Update(msg)
	if nf, ok := updated.(*huh.Form); ok {
		m.kindForm = nf
	}

	return cmd
}

// checkKindCompleted swaps in the subform if the picker just completed.
func (m *GenerateScreen) checkKindCompleted() tea.Cmd {
	if m.kindForm.State == huh.StateCompleted {
		return m.chooseKind()
	}

	return nil
}

// Init implements tea.Model.
func (m *GenerateScreen) Init() tea.Cmd { return m.kindForm.Init() }

// Update implements tea.Model. The kind picker runs first; once chosen, the
// shared flow owns form → preview → exec. No I/O; work starts as Cmds.
func (m *GenerateScreen) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if !m.kindChosen {
		switch msg := msg.(type) {
		case tea.WindowSizeMsg:
			m.flow.setSize(msg.Width, msg.Height)

			return m, m.forwardKind(msg)
		case tea.KeyPressMsg:
			if m.flow.keys.Back.Matches(msg.String()) {
				return m, backCmd
			}

			// Completion is always noticed on a follow-up message via
			// checkKindCompleted (the commit handshake lands there), so
			// this branch only forwards.
			return m, m.forwardKind(msg)
		default:
			cmd := m.forwardKind(msg)
			if completed := m.checkKindCompleted(); completed != nil {
				return m, tea.Batch(cmd, completed)
			}

			return m, cmd
		}
	}

	return m, m.flow.update(msg)
}

// View implements tea.Model. Pure.
func (m *GenerateScreen) View() tea.View {
	if !m.kindChosen {
		var b strings.Builder

		b.WriteString(m.flow.theme.Title.Render("zever generate"))
		b.WriteString("\n\n")
		b.WriteString(stripScreenANSI(m.kindForm.View()))
		b.WriteString("\n")
		b.WriteString(m.flow.theme.Hint.Render("enter next · esc back"))

		return tea.NewView(b.String())
	}

	return tea.NewView(m.flow.view())
}
