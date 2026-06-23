package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/0x666c6f/safe-agentic/pkg/config"
	"github.com/0x666c6f/safe-agentic/pkg/repourl"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// ShowOverlay displays text in a scrollable overlay pane.
func ShowOverlay(app *App, name string, title string, content string) {
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
		AddInputField("VM path (pull dest):", "./", 40, nil, nil).
		AddInputField("VM source (push):", "./", 40, nil, nil).
		AddInputField("Agent path (push dest):", "/workspace/", 40, nil, nil)

	form.AddButton("Pull", func() {
		containerPath := form.GetFormItemByLabel("Agent path:").(*tview.InputField).GetText()
		hostPath := form.GetFormItemByLabel("VM path (pull dest):").(*tview.InputField).GetText()

		app.pages.SwitchToPage("main")
		app.pages.RemovePage("copy")
		app.tapp.SetFocus(app.table.Table())

		if containerPath == "" || hostPath == "" {
			app.footer.ShowStatus("Both paths required", true)
			return
		}

		app.footer.ShowStatus("Copying...", false)
		go func() {
			out, err := execOrb("docker", "cp", containerName+":"+containerPath, hostPath)
			app.tapp.QueueUpdateDraw(func() {
				if err != nil {
					app.footer.ShowStatus("Copy failed: "+string(out), true)
				} else {
					app.footer.ShowStatus("Copied to "+hostPath, false)
				}
			})
		}()
	})
	form.AddButton("Push", func() {
		hostPath := form.GetFormItemByLabel("VM source (push):").(*tview.InputField).GetText()
		containerPath := form.GetFormItemByLabel("Agent path (push dest):").(*tview.InputField).GetText()

		app.pages.SwitchToPage("main")
		app.pages.RemovePage("copy")
		app.tapp.SetFocus(app.table.Table())

		if hostPath == "" || containerPath == "" {
			app.footer.ShowStatus("Both paths required", true)
			return
		}

		app.footer.ShowStatus("Pushing...", false)
		go func() {
			out, err := execOrb("docker", "cp", hostPath, containerName+":"+containerPath)
			app.tapp.QueueUpdateDraw(func() {
				if err != nil {
					app.footer.ShowStatus("Push failed: "+string(out), true)
				} else {
					app.footer.ShowStatus("Pushed to "+containerPath, false)
				}
			})
		}()
	})

	form.AddButton("Cancel", func() {
		app.pages.SwitchToPage("main")
		app.pages.RemovePage("copy")
		app.tapp.SetFocus(app.table.Table())
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
	spawnArgs := append(buildSpawnFormArgs(spec), "--background")
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
