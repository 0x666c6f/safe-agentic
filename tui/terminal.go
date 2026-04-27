package main

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

var (
	terminalGOOS       = runtime.GOOS
	lookPath           = exec.LookPath
	runTerminalCommand = func(name string, args ...string) error {
		return exec.Command(name, args...).Run()
	}
)

func openTerminal(args []string) error {
	if len(args) == 0 {
		return nil
	}
	if terminalGOOS != "darwin" {
		return fmt.Errorf("external terminal launch unsupported on %s", terminalGOOS)
	}

	command := "exec " + shellQuoteArgs(args)
	if _, err := lookPath("osascript"); err != nil {
		return err
	}

	for _, appName := range []string{"iTerm", "iTerm2"} {
		if hasApplication(appName) {
			if err := runTerminalCommand("osascript", iTermScript(appName, command)...); err == nil {
				return nil
			}
		}
	}
	return runTerminalCommand("osascript", terminalScript(command)...)
}

func hasApplication(name string) bool {
	return runTerminalCommand("osascript", "-e", `id of application "`+appleScriptString(name)+`"`) == nil
}

func iTermScript(appName, command string) []string {
	return []string{
		"-e", `tell application "` + appleScriptString(appName) + `"`,
		"-e", "activate",
		"-e", `create window with default profile command "` + appleScriptString(command) + `"`,
		"-e", "end tell",
	}
}

func terminalScript(command string) []string {
	return []string{
		"-e", `tell application "Terminal"`,
		"-e", "activate",
		"-e", `do script "` + appleScriptString(command) + `"`,
		"-e", "end tell",
	}
}

func appleScriptString(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	return strings.ReplaceAll(s, `"`, `\"`)
}
