package main

import (
	"fmt"
	"os"
	"os/exec"
	pathpkg "path"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/0x666c6f/safe-agentic/pkg/config"
	"github.com/0x666c6f/safe-agentic/pkg/repourl"
	"github.com/0x666c6f/safe-agentic/pkg/risk"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// ShowOverlay displays text in a scrollable overlay pane.
func ShowOverlay(app *App, name string, title string, content string) {
	ShowOverlayCapture(app, name, title, content)
}

// ShowOverlayCapture is like ShowOverlay but returns the TextView so the caller
// can attach extra key handlers (e.g. the diff overlay's side-by-side toggle).
func ShowOverlayCapture(app *App, name string, title string, content string) *tview.TextView {
	tv := tview.NewTextView().
		SetText(content).
		SetScrollable(true).
		SetDynamicColors(false)
	tv.SetBorder(true).
		SetTitle(" " + title + " (Esc to close) ").
		SetTitleColor(colorTitle).
		SetBorderColor(colorBorder).
		SetBackgroundColor(tcell.ColorDefault)

	app.pages.AddAndSwitchToPage(name, tv, true)
	app.tapp.SetFocus(tv)
	return tv
}

// ShowOverlayLive is like ShowOverlay but returns the TextView for live updates.
func ShowOverlayLive(app *App, name string, title string, content string) *tview.TextView {
	tv := tview.NewTextView().
		SetText(content).
		SetScrollable(true).
		SetDynamicColors(false)
	tv.SetBorder(true).
		SetTitle(" " + title + " ").
		SetTitleColor(colorTitle).
		SetBorderColor(colorBorder).
		SetBackgroundColor(tcell.ColorDefault)

	app.pages.AddAndSwitchToPage(name, tv, true)
	app.tapp.SetFocus(tv)
	tv.ScrollToEnd()
	return tv
}

// ShowCopyForm shows a modal form for transferring files between VM and container.
func ShowCopyForm(app *App, containerName string) {
	form := tview.NewForm().
		AddInputField("Agent path:", "/workspace/", 40, nil, nil).
		AddInputField("VM path (pull dest):", "/tmp/", 40, nil, nil).
		AddInputField("VM source (push):", "/tmp/", 40, nil, nil).
		AddInputField("Agent path (push dest):", "/workspace/", 40, nil, nil)

	form.AddButton("Pull", func() {
		agentPath, err := cleanAgentCopyPath(form.GetFormItemByLabel("Agent path:").(*tview.InputField).GetText())
		if err != nil {
			app.footer.ShowStatus(err.Error(), true)
			app.tapp.SetFocus(form)
			return
		}
		vmPath, err := cleanVMCopyPath(form.GetFormItemByLabel("VM path (pull dest):").(*tview.InputField).GetText(), "VM pull destination")
		if err != nil {
			app.footer.ShowStatus(err.Error(), true)
			app.tapp.SetFocus(form)
			return
		}

		closeCopyForm(app)
		app.footer.ShowStatus("Copying...", false)
		go func() {
			out, err := execVM("docker", "cp", containerName+":"+agentPath, vmPath)
			app.tapp.QueueUpdateDraw(func() {
				if err != nil {
					app.footer.ShowStatus("Copy failed: "+string(out), true)
				} else {
					app.footer.ShowStatus("Copied to "+vmPath, false)
				}
			})
		}()
	})
	form.AddButton("Push", func() {
		vmPath, err := cleanVMCopyPath(form.GetFormItemByLabel("VM source (push):").(*tview.InputField).GetText(), "VM push source")
		if err != nil {
			app.footer.ShowStatus(err.Error(), true)
			app.tapp.SetFocus(form)
			return
		}
		agentPath, err := cleanAgentCopyPath(form.GetFormItemByLabel("Agent path (push dest):").(*tview.InputField).GetText())
		if err != nil {
			app.footer.ShowStatus(err.Error(), true)
			app.tapp.SetFocus(form)
			return
		}

		closeCopyForm(app)
		app.footer.ShowStatus("Pushing...", false)
		go func() {
			out, err := execVM("docker", "cp", vmPath, containerName+":"+agentPath)
			app.tapp.QueueUpdateDraw(func() {
				if err != nil {
					app.footer.ShowStatus("Push failed: "+string(out), true)
				} else {
					app.footer.ShowStatus("Pushed to "+agentPath, false)
				}
			})
		}()
	})

	form.AddButton("Cancel", func() {
		closeCopyForm(app)
	})

	form.SetBorder(true).
		SetTitle(" File transfer: " + containerName + " (Esc to close) ").
		SetTitleColor(colorTitle).
		SetBorderColor(colorBorder).
		SetBackgroundColor(tcell.ColorDefault)

	// Center the form
	modal := tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(form, 10, 0, true).
			AddItem(nil, 0, 1, false),
			50, 0, true).
		AddItem(nil, 0, 1, false)

	app.pages.AddAndSwitchToPage("copy", modal, true)
	app.tapp.SetFocus(form)
}

func closeCopyForm(app *App) {
	app.pages.SwitchToPage("main")
	app.pages.RemovePage("copy")
	app.tapp.SetFocus(app.table.Table())
}

func cleanAgentCopyPath(value string) (string, error) {
	raw := strings.TrimSpace(value)
	if raw == "" {
		return "", fmt.Errorf("agent path required")
	}
	if strings.ContainsRune(raw, 0) {
		return "", fmt.Errorf("agent path contains NUL byte")
	}
	if strings.Contains(raw, ":") {
		return "", fmt.Errorf("agent path must not contain ':'")
	}
	if !strings.HasPrefix(raw, "/") {
		return "", fmt.Errorf("agent path must be absolute under /workspace")
	}
	clean := pathpkg.Clean(raw)
	if clean != "/workspace" && !strings.HasPrefix(clean, "/workspace/") {
		return "", fmt.Errorf("agent path must stay under /workspace")
	}
	return clean, nil
}

func cleanVMCopyPath(value string, field string) (string, error) {
	raw := strings.TrimSpace(value)
	if raw == "" {
		return "", fmt.Errorf("%s required", field)
	}
	if strings.ContainsRune(raw, 0) {
		return "", fmt.Errorf("%s contains NUL byte", field)
	}
	if strings.Contains(raw, ":") {
		return "", fmt.Errorf("%s must be a VM path, not Docker container:path syntax", field)
	}
	if !filepath.IsAbs(raw) {
		return "", fmt.Errorf("%s must be an absolute VM path", field)
	}
	return filepath.Clean(raw), nil
}

// ShowSteerForm shows a small input overlay to send a follow-up message to an
// agent's tmux session (via `safe-ag steer <name> <message>`).
func ShowSteerForm(app *App, containerName string) {
	form := tview.NewForm().
		AddInputField("Message:", "", 60, nil, nil)

	send := func() {
		text := strings.TrimSpace(form.GetFormItemByLabel("Message:").(*tview.InputField).GetText())
		if text == "" {
			app.footer.ShowStatus("Message required", true)
			app.tapp.SetFocus(form)
			return
		}
		closeSteerForm(app)
		app.footer.ShowStatus("Sending to "+containerName+"...", false)
		go func() {
			out, err := newCLICmd("steer", containerName, text).CombinedOutput()
			app.tapp.QueueUpdateDraw(func() {
				if err != nil {
					msg := strings.TrimSpace(string(out))
					if msg == "" {
						msg = "Steer failed"
					}
					app.footer.ShowStatus(msg, true)
					return
				}
				app.footer.ShowStatus("Sent to "+containerName, false)
			})
		}()
	}

	form.AddButton("Send", send)
	form.AddButton("Cancel", func() { closeSteerForm(app) })

	form.SetBorder(true).
		SetTitle(" Steer: " + containerName + " (Esc to close) ").
		SetTitleColor(colorTitle).
		SetBorderColor(colorBorder).
		SetBackgroundColor(tcell.ColorDefault)

	modal := tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(form, 7, 0, true).
			AddItem(nil, 0, 1, false),
			70, 0, true).
		AddItem(nil, 0, 1, false)

	app.pages.AddAndSwitchToPage("steer", modal, true)
	app.tapp.SetFocus(form)
}

func closeSteerForm(app *App) {
	app.pages.SwitchToPage("main")
	app.pages.RemovePage("steer")
	app.tapp.SetFocus(app.table.Table())
}

// ShowSpawnForm shows a modal form for spawning a new agent.
func ShowSpawnForm(app *App) {
	agentType := "claude"
	defaults := loadSpawnFormDefaults()
	form := tview.NewForm().
		AddDropDown("Type:", []string{"claude", "codex", "shell"}, 0, func(option string, index int) {
			agentType = option
		}).
		AddInputField("Repo URL (optional):", "", 50, nil, nil).
		AddInputField("Name (optional):", "", 30, nil, nil).
		AddInputField("Prompt (optional):", "", 50, nil, nil).
		AddCheckbox("SSH:", defaults.ssh, nil).
		AddCheckbox("Reuse auth:", defaults.reuseAuth, nil).
		AddCheckbox("Reuse GH auth:", defaults.reuseGHAuth, nil).
		AddCheckbox("Seed host auth:", defaults.seedAuth, nil).
		AddInputField("AWS profile (optional):", "", 30, nil, nil).
		AddCheckbox("Docker:", defaults.docker, nil).
		AddCheckbox("Docker socket:", defaults.dockerSocket, nil).
		AddInputField("Identity (optional):", "", 40, nil, nil)

	form.AddButton("Spawn", func() {
		spec := readSpawnForm(form, agentType, defaults)
		if promptSpawnRiskConfirm(app, form, spec) {
			return
		}
		closeSpawnForm(app)
		runSpawnFormSubmit(app, spec)
	})

	form.AddButton("Cancel", func() {
		app.pages.SwitchToPage("main")
		app.pages.RemovePage("spawn")
		app.tapp.SetFocus(app.table.Table())
	})

	form.SetBorder(true).
		SetTitle(" Spawn New Agent (Esc to close) ").
		SetTitleColor(colorTitle).
		SetBorderColor(colorBorder).
		SetBackgroundColor(tcell.ColorDefault)

	modal := tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(form, 30, 0, true).
			AddItem(nil, 0, 1, false),
			70, 0, true).
		AddItem(nil, 0, 1, false)

	app.pages.AddAndSwitchToPage("spawn", modal, true)
	app.tapp.SetFocus(form)
}

type spawnFormSpec struct {
	agentType    string
	repoURL      string
	name         string
	prompt       string
	ssh          bool
	reuseAuth    bool
	reuseGHAuth  bool
	seedAuth     bool
	awsProfile   string
	docker       bool
	dockerSocket bool
	identity     string
	defaults     spawnFormDefaults
}

type spawnFormDefaults struct {
	ssh          bool
	reuseAuth    bool
	reuseGHAuth  bool
	seedAuth     bool
	docker       bool
	dockerSocket bool
}

func loadSpawnFormDefaults() spawnFormDefaults {
	cfg, err := config.LoadDefaults(config.DefaultsPath())
	if err != nil {
		return spawnFormDefaults{}
	}
	return spawnFormDefaults{
		ssh:          cfg.Defaults.SSH,
		reuseAuth:    cfg.Defaults.ReuseAuth,
		reuseGHAuth:  cfg.Defaults.ReuseGHAuth,
		seedAuth:     cfg.Defaults.SeedAuth,
		docker:       cfg.Defaults.Docker,
		dockerSocket: cfg.Defaults.DockerSocket,
	}
}

func readSpawnForm(form *tview.Form, agentType string, defaults spawnFormDefaults) spawnFormSpec {
	return spawnFormSpec{
		agentType:    agentType,
		repoURL:      strings.TrimSpace(formInputText(form, "Repo URL (optional):")),
		name:         strings.TrimSpace(formInputText(form, "Name (optional):")),
		prompt:       formInputText(form, "Prompt (optional):"),
		ssh:          formCheckboxValue(form, "SSH:"),
		reuseAuth:    formCheckboxValue(form, "Reuse auth:"),
		reuseGHAuth:  formCheckboxValue(form, "Reuse GH auth:"),
		seedAuth:     formCheckboxValue(form, "Seed host auth:"),
		awsProfile:   strings.TrimSpace(formInputText(form, "AWS profile (optional):")),
		docker:       formCheckboxValue(form, "Docker:"),
		dockerSocket: formCheckboxValue(form, "Docker socket:"),
		identity:     strings.TrimSpace(formInputText(form, "Identity (optional):")),
		defaults:     defaults,
	}
}

func formInputText(form *tview.Form, label string) string {
	return form.GetFormItemByLabel(label).(*tview.InputField).GetText()
}

func formCheckboxValue(form *tview.Form, label string) bool {
	return form.GetFormItemByLabel(label).(*tview.Checkbox).IsChecked()
}

func closeSpawnForm(app *App) {
	app.pages.SwitchToPage("main")
	app.pages.RemovePage("spawn")
	app.tapp.SetFocus(app.table.Table())
}

func promptSpawnRiskConfirm(app *App, form *tview.Form, spec spawnFormSpec) bool {
	notices := spawnFormRiskNotices(spec)
	if len(notices) == 0 {
		return false
	}
	app.footer.ShowConfirm(spawnRiskConfirmMessage(notices), func(yes bool) {
		if !yes {
			app.tapp.SetFocus(form)
			return
		}
		app.tapp.QueueUpdateDraw(func() {
			closeSpawnForm(app)
			runSpawnFormSubmit(app, spec)
		})
	})
	return true
}

func spawnFormRiskNotices(spec spawnFormSpec) []risk.Notice {
	sshEnabled := spec.ssh || repourl.UsesSSH(spec.repoURL)
	return risk.SpawnNotices(risk.SpawnInput{
		SSH:          sshEnabled,
		ReuseAuth:    spec.reuseAuth,
		ReuseGHAuth:  spec.reuseGHAuth,
		SeedAuth:     spec.seedAuth,
		AWSProfile:   spec.awsProfile,
		Docker:       spec.docker,
		DockerSocket: spec.dockerSocket,
	})
}

func spawnRiskConfirmMessage(notices []risk.Notice) string {
	var flags []string
	for _, notice := range notices {
		flags = append(flags, notice.Flag)
	}
	const maxFlags = 3
	if len(flags) > maxFlags {
		flags = append(flags[:maxFlags], fmt.Sprintf("+%d more", len(flags)-maxFlags))
	}
	return fmt.Sprintf("Spawn widens sandbox (%s). Continue?", strings.Join(flags, ", "))
}

func runSpawnFormSubmit(app *App, spec spawnFormSpec) {
	app.footer.ShowStatus("Spawning "+spec.agentType+" agent...", false)
	go func() {
		outStr, err := executeSpawnForm(spec)
		if err != nil {
			app.tapp.QueueUpdateDraw(func() {
				msg := "Spawn failed"
				if outStr != "" {
					msg += ": " + outStr
				} else if err != nil {
					msg += ": " + err.Error()
				}
				app.footer.ShowStatus(msg, true)
			})
			return
		}
		containerName := spawnedContainerName(outStr)
		app.poller.ForceRefresh()
		app.tapp.QueueUpdateDraw(func() {
			if containerName != "" {
				app.footer.ShowStatus("Spawned "+containerName+". Press 'r' to connect.", false)
			} else {
				app.footer.ShowStatus("Agent spawned. Press 'r' to connect.", false)
			}
		})
	}()
}

func executeSpawnForm(spec spawnFormSpec) (string, error) {
	if spec.docker && spec.dockerSocket {
		return "", fmt.Errorf("--docker and --docker-socket are mutually exclusive")
	}
	// --yes: the spawn form itself is the user's confirmation; the child CLI
	// has non-TTY stdin, so the risk gate would otherwise reject the spawn.
	spawnArgs := append(buildSpawnFormArgs(spec), "--background", "--yes")
	spawnOut, spawnErr := newAgentCmd(spawnArgs...).CombinedOutput()
	return strings.TrimSpace(string(spawnOut)), spawnErr
}

func buildSpawnFormArgs(spec spawnFormSpec) []string {
	args := []string{"spawn", spec.agentType}
	sshEnabled := spec.ssh || repourl.UsesSSH(spec.repoURL)
	repoURL := normalizeSpawnRepoURL(spec.repoURL, sshEnabled)
	args = appendStringArg(args, "--repo", repoURL)
	args = appendStringArg(args, "--name", spec.name)
	args = appendStringArg(args, "--prompt", spec.prompt)
	args = appendBoolOverrideArg(args, sshEnabled, spec.defaults.ssh, "--ssh", "--no-ssh")
	args = appendBoolOverrideArg(args, spec.reuseAuth, spec.defaults.reuseAuth, "--reuse-auth", "--no-reuse-auth")
	args = appendBoolOverrideArg(args, spec.reuseGHAuth, spec.defaults.reuseGHAuth, "--reuse-gh-auth", "--no-reuse-gh-auth")
	args = appendBoolOverrideArg(args, spec.seedAuth, spec.defaults.seedAuth, "--seed-auth", "--no-seed-auth")
	args = appendStringArg(args, "--aws", spec.awsProfile)
	args = appendBoolOverrideArg(args, spec.docker, spec.defaults.docker, "--docker", "--no-docker")
	args = appendBoolOverrideArg(args, spec.dockerSocket, spec.defaults.dockerSocket, "--docker-socket", "--no-docker-socket")
	args = appendStringArg(args, "--identity", spec.identity)
	return args
}

func normalizeSpawnRepoURL(repoURL string, ssh bool) string {
	if !ssh || !strings.HasPrefix(repoURL, "https://github.com/") {
		return repoURL
	}
	path := strings.TrimPrefix(repoURL, "https://github.com/")
	path = strings.TrimSuffix(path, ".git")
	return "git@github.com:" + path + ".git"
}

func appendStringArg(args []string, flag, value string) []string {
	if value == "" {
		return args
	}
	return append(args, flag, value)
}

func appendBoolOverrideArg(args []string, value, defaultValue bool, enabledFlag, disabledFlag string) []string {
	if value == defaultValue {
		return args
	}
	if value {
		return append(args, enabledFlag)
	}
	return append(args, disabledFlag)
}

func spawnedContainerName(outStr string) string {
	containerName := ""
	for _, line := range strings.Split(outStr, "\n") {
		line = strings.TrimSpace(line)
		if after, ok := strings.CutPrefix(line, "Agent "); ok {
			if _, name, found := strings.Cut(after, " started: "); found && strings.HasPrefix(name, "agent-") {
				containerName = strings.TrimSpace(name)
			}
		}
		if strings.HasPrefix(line, "agent-") {
			containerName = line
		}
	}
	return containerName
}

// newAgentCmd creates an exec.Cmd for the safe-ag CLI.
func newAgentCmd(args ...string) *exec.Cmd {
	return exec.Command("safe-ag", args...)
}

// execAgent replaces the current process with `safe-ag <args>`.
// Used for spawn which needs a real TTY chain for nested TUI apps (claude/codex).
func execAgent(args []string) {
	agentPath, err := exec.LookPath("safe-ag")
	if err != nil {
		fmt.Fprintf(os.Stderr, "safe-ag not found: %v\n", err)
		os.Exit(1)
	}
	fullArgs := append([]string{"safe-ag"}, args...)
	// Replace process — never returns on success
	if err := syscall.Exec(agentPath, fullArgs, os.Environ()); err != nil {
		fmt.Fprintf(os.Stderr, "exec failed: %v\n", err)
		os.Exit(1)
	}
}

// shellQuoteArgs joins args with proper quoting for shell execution.
func shellQuoteArgs(args []string) string {
	var b strings.Builder
	for i, a := range args {
		if i > 0 {
			b.WriteByte(' ')
		}
		// Simple quoting: wrap in single quotes, escape existing single quotes
		if strings.ContainsAny(a, " \t'\"\\$`!&|;(){}[]<>?*#~") {
			b.WriteByte('\'')
			b.WriteString(strings.ReplaceAll(a, "'", "'\\''"))
			b.WriteByte('\'')
		} else {
			b.WriteString(a)
		}
	}
	return b.String()
}
