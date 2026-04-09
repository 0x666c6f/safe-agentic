package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"

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

// ShowCopyForm shows a modal form for copying files from a container.
func ShowCopyForm(app *App, containerName string) {
	form := tview.NewForm().
		AddInputField("Container path:", "/workspace/", 40, nil, nil).
		AddInputField("VM path (not macOS host):", "./", 40, nil, nil)

	form.AddButton("Copy", func() {
		containerPath := form.GetFormItemByLabel("Container path:").(*tview.InputField).GetText()
		hostPath := form.GetFormItemByLabel("VM path (not macOS host):").(*tview.InputField).GetText()

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

	form.AddButton("Cancel", func() {
		app.pages.SwitchToPage("main")
		app.pages.RemovePage("copy")
		app.tapp.SetFocus(app.table.Table())
	})

	form.SetBorder(true).
		SetTitle(" Copy from " + containerName + " (Esc to close) ").
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
	form := tview.NewForm().
		AddDropDown("Type:", []string{"claude", "codex"}, 0, func(option string, index int) {
			agentType = option
		}).
		AddInputField("Repo URL (optional):", "", 50, nil, nil).
		AddInputField("Name (optional):", "", 30, nil, nil).
		AddInputField("Prompt (optional):", "", 50, nil, nil).
		AddCheckbox("SSH:", true, nil).
		AddCheckbox("Reuse auth:", true, nil).
		AddCheckbox("Reuse GH auth:", false, nil).
		AddInputField("AWS profile (optional):", "", 30, nil, nil).
		AddCheckbox("Docker:", false, nil).
		AddInputField("Identity (optional):", "", 40, nil, nil)

	form.AddButton("Spawn", func() {
		repoURL := form.GetFormItemByLabel("Repo URL (optional):").(*tview.InputField).GetText()
		name := form.GetFormItemByLabel("Name (optional):").(*tview.InputField).GetText()
		prompt := form.GetFormItemByLabel("Prompt (optional):").(*tview.InputField).GetText()
		ssh := form.GetFormItemByLabel("SSH:").(*tview.Checkbox).IsChecked()
		reuseAuth := form.GetFormItemByLabel("Reuse auth:").(*tview.Checkbox).IsChecked()
		reuseGHAuth := form.GetFormItemByLabel("Reuse GH auth:").(*tview.Checkbox).IsChecked()
		awsProfile := form.GetFormItemByLabel("AWS profile (optional):").(*tview.InputField).GetText()
		docker := form.GetFormItemByLabel("Docker:").(*tview.Checkbox).IsChecked()
		identity := form.GetFormItemByLabel("Identity (optional):").(*tview.InputField).GetText()

		app.pages.SwitchToPage("main")
		app.pages.RemovePage("spawn")
		app.tapp.SetFocus(app.table.Table())

		// Auto-convert HTTPS GitHub URLs to SSH when SSH is enabled
		if ssh && strings.HasPrefix(repoURL, "https://github.com/") {
			path := strings.TrimPrefix(repoURL, "https://github.com/")
			path = strings.TrimSuffix(path, ".git")
			repoURL = "git@github.com:" + path + ".git"
		}

		args := []string{"spawn", agentType}
		if repoURL != "" {
			args = append(args, "--repo", repoURL)
		}
		if name != "" {
			args = append(args, "--name", name)
		}
		if prompt != "" {
			args = append(args, "--prompt", prompt)
		}
		if ssh {
			args = append(args, "--ssh")
		}
		if reuseAuth {
			args = append(args, "--reuse-auth")
		}
		if reuseGHAuth {
			args = append(args, "--reuse-gh-auth")
		}
		if awsProfile != "" {
			args = append(args, "--aws", awsProfile)
		}
		if docker {
			args = append(args, "--docker")
		}
		if identity != "" {
			args = append(args, "--identity", identity)
		}

		// Spawn detached using --background flag: the agent CLI handles
		// creating networks and running the container in detached mode.
		app.footer.ShowStatus("Spawning "+agentType+" agent...", false)
		go func() {
			spawnArgs := append(args, "--background")
			spawnCmd := newAgentCmd(spawnArgs...)
			spawnOut, spawnErr := spawnCmd.CombinedOutput()
			outStr := strings.TrimSpace(string(spawnOut))

			if spawnErr != nil {
				app.tapp.QueueUpdateDraw(func() {
					msg := "Spawn failed"
					if outStr != "" {
						msg += ": " + outStr
					}
					app.footer.ShowStatus(msg, true)
				})
				return
			}

			// Extract container name from output (last non-empty line)
			containerName := ""
			for _, line := range strings.Split(outStr, "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "agent-") {
					containerName = line
				}
			}

			app.poller.ForceRefresh()
			app.tapp.QueueUpdateDraw(func() {
				if containerName != "" {
					app.footer.ShowStatus("Spawned "+containerName+". Press 'r' to connect.", false)
				} else {
					app.footer.ShowStatus("Agent spawned. Press 'r' to connect.", false)
				}
			})
		}()
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
			AddItem(form, 28, 0, true).
			AddItem(nil, 0, 1, false),
			70, 0, true).
		AddItem(nil, 0, 1, false)

	app.pages.AddAndSwitchToPage("spawn", modal, true)
	app.tapp.SetFocus(form)
}

// newAgentCmd creates an exec.Cmd for the agent CLI.
func newAgentCmd(args ...string) *exec.Cmd {
	return exec.Command("agent", args...)
}

// execAgent replaces the current process with `agent <args>`.
// Used for spawn which needs a real TTY chain for nested TUI apps (claude/codex).
func execAgent(args []string) {
	agentPath, err := exec.LookPath("agent")
	if err != nil {
		fmt.Fprintf(os.Stderr, "agent not found: %v\n", err)
		os.Exit(1)
	}
	fullArgs := append([]string{"agent"}, args...)
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
