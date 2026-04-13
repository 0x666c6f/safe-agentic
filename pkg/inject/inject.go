package inject

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func EncodeB64(s string) string {
	return base64.StdEncoding.EncodeToString([]byte(s))
}

func DecodeB64(s string) (string, error) {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func EncodeFileB64(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

func ReadClaudeConfig(configDir string) (map[string]string, error) {
	envs := make(map[string]string)
	settingsPath := filepath.Join(configDir, "settings.json")
	data, err := os.ReadFile(settingsPath)
	if os.IsNotExist(err) {
		return envs, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read claude settings: %w", err)
	}
	envs["SAFE_AGENTIC_CLAUDE_CONFIG_B64"] = base64.StdEncoding.EncodeToString(data)
	return envs, nil
}

// ReadClaudeSupportFiles tars CLAUDE.md, hooks/, commands/, statusline-command.sh
// from configDir and returns them as SAFE_AGENTIC_CLAUDE_SUPPORT_B64.
// The entrypoint extracts this into ~/.claude/ inside the container.
func ReadClaudeSupportFiles(configDir string) (map[string]string, error) {
	envs := make(map[string]string)

	type entry struct {
		path  string
		isDir bool
	}
	var entries []entry

	for _, name := range []string{"CLAUDE.md", "statusline-command.sh"} {
		p := filepath.Join(configDir, name)
		if info, err := os.Lstat(p); err == nil && info.Mode().IsRegular() {
			entries = append(entries, entry{name, false})
		}
	}
	for _, name := range []string{"hooks", "commands"} {
		p := filepath.Join(configDir, name)
		if info, err := os.Lstat(p); err == nil && info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
			entries = append(entries, entry{name, true})
		}
	}

	if len(entries) == 0 {
		return envs, nil
	}

	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	for _, e := range entries {
		fullPath := filepath.Join(configDir, e.path)
		if e.isDir {
			if err := tarDir(tw, configDir, e.path); err != nil {
				tw.Close()
				gw.Close()
				return nil, fmt.Errorf("tar %s: %w", e.path, err)
			}
		} else {
			if err := tarFile(tw, fullPath, e.path); err != nil {
				tw.Close()
				gw.Close()
				return nil, fmt.Errorf("tar %s: %w", e.path, err)
			}
		}
	}

	tw.Close()
	gw.Close()
	envs["SAFE_AGENTIC_CLAUDE_SUPPORT_B64"] = base64.StdEncoding.EncodeToString(buf.Bytes())
	return envs, nil
}

func tarFile(tw *tar.Writer, fullPath, name string) error {
	info, err := os.Lstat(fullPath)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing to include symlink %s", name)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("refusing to include non-regular file %s", name)
	}
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return err
	}
	hdr := &tar.Header{
		Name: name,
		Mode: 0644,
		Size: int64(len(data)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	_, err = tw.Write(data)
	return err
}

func tarDir(tw *tar.Writer, baseDir, dirName string) error {
	root := filepath.Join(baseDir, dirName)
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		rel, _ := filepath.Rel(baseDir, path)
		if info.IsDir() {
			hdr := &tar.Header{
				Typeflag: tar.TypeDir,
				Name:     rel + "/",
				Mode:     0755,
			}
			return tw.WriteHeader(hdr)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		hdr := &tar.Header{
			Name: rel,
			Mode: 0644,
			Size: int64(len(data)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		_, err = io.Copy(tw, bytes.NewReader(data))
		return err
	})
}

func ReadCodexConfig(codexHome string) (map[string]string, error) {
	envs := make(map[string]string)
	configPath := filepath.Join(codexHome, "config.toml")
	data, err := os.ReadFile(configPath)
	if os.IsNotExist(err) {
		return envs, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read codex config: %w", err)
	}
	envs["SAFE_AGENTIC_CODEX_CONFIG_B64"] = base64.StdEncoding.EncodeToString(data)
	return envs, nil
}

func ReadAWSCredentials(credPath, profile string) (map[string]string, error) {
	data, err := os.ReadFile(credPath)
	if err != nil {
		return nil, fmt.Errorf("read AWS credentials: %w", err)
	}
	content, err := extractAWSProfile(string(data), profile)
	if err != nil {
		return nil, fmt.Errorf("AWS profile %q not found in %s", profile, credPath)
	}
	envs := map[string]string{
		"SAFE_AGENTIC_AWS_CREDS_B64": base64.StdEncoding.EncodeToString([]byte(content)),
		"AWS_PROFILE":                profile,
	}
	if r := os.Getenv("AWS_DEFAULT_REGION"); r != "" {
		envs["AWS_DEFAULT_REGION"] = r
	}
	if r := os.Getenv("AWS_REGION"); r != "" {
		envs["AWS_REGION"] = r
	}
	return envs, nil
}

func extractAWSProfile(content, profile string) (string, error) {
	lines := strings.Split(content, "\n")
	var current string
	var builder strings.Builder
	found := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			current = strings.TrimSpace(trimmed[1 : len(trimmed)-1])
		}
		if current != profile {
			continue
		}
		builder.WriteString(line)
		builder.WriteString("\n")
		found = true
	}

	if !found {
		return "", fmt.Errorf("profile not found")
	}
	return builder.String(), nil
}
