package docker

import (
	"fmt"
	"sort"
	"strings"
)

type DockerRunCmd struct {
	name     string
	image    string
	Detached bool
	flags    []string
	labels   map[string]string
	envs     []envEntry
	mounts   []string
	tmpfs    []string
}

type envEntry struct {
	key   string
	value string
}

func NewRunCmd(name, image string) *DockerRunCmd {
	return &DockerRunCmd{
		name:   name,
		image:  image,
		labels: make(map[string]string),
	}
}

func (d *DockerRunCmd) AddLabel(key, value string) { d.labels[key] = value }
func (d *DockerRunCmd) AddEnv(key, value string)   { d.envs = append(d.envs, envEntry{key, value}) }
func (d *DockerRunCmd) AddFlag(flags ...string)     { d.flags = append(d.flags, flags...) }
func (d *DockerRunCmd) AddNamedVolume(src, dst string) {
	d.mounts = append(d.mounts, fmt.Sprintf("--mount type=volume,src=%s,dst=%s", src, dst))
}
func (d *DockerRunCmd) AddEphemeralVolume(dst string) {
	d.mounts = append(d.mounts, fmt.Sprintf("--mount type=volume,dst=%s", dst))
}
func (d *DockerRunCmd) AddTmpfs(path, size string, noexec, nosuid bool) {
	opts := "rw"
	if noexec {
		opts += ",noexec"
	}
	if nosuid {
		opts += ",nosuid"
	}
	if size != "" {
		opts += ",size=" + size
	}
	d.tmpfs = append(d.tmpfs, fmt.Sprintf("%s:%s", path, opts))
}

// Build produces the final []string for exec.Command.
// Order: docker run [-it|-d] --name --hostname [flags] [labels] [envs] [mounts] [tmpfs] image
func (d *DockerRunCmd) Build() []string {
	args := []string{"docker", "run"}
	if d.Detached {
		args = append(args, "-d")
	} else {
		args = append(args, "-it")
	}
	args = append(args, "--name", d.name)
	args = append(args, "--hostname", d.name)
	args = append(args, "--pull", "never")
	args = append(args, d.flags...)
	// Labels sorted for deterministic output
	labelKeys := make([]string, 0, len(d.labels))
	for k := range d.labels {
		labelKeys = append(labelKeys, k)
	}
	sort.Strings(labelKeys)
	for _, k := range labelKeys {
		args = append(args, "--label", k+"="+d.labels[k])
	}
	for _, e := range d.envs {
		args = append(args, "-e", e.key+"="+e.value)
	}
	args = append(args, d.mounts...)
	for _, t := range d.tmpfs {
		args = append(args, "--tmpfs", t)
	}
	args = append(args, d.image)
	return args
}

func (d *DockerRunCmd) Render() string {
	args := d.Build()
	var quoted []string
	for _, a := range args {
		if strings.ContainsAny(a, " \t\"'$\\") {
			quoted = append(quoted, fmt.Sprintf("%q", a))
		} else {
			quoted = append(quoted, a)
		}
	}
	return strings.Join(quoted, " ")
}

type HardeningOpts struct {
	Network     string
	Memory      string
	CPUs        string
	PIDsLimit   int
	SeccompPath string
}

func AppendRuntimeHardening(cmd *DockerRunCmd, opts HardeningOpts) {
	seccomp := opts.SeccompPath
	if seccomp == "" {
		seccomp = "/etc/safe-agentic/seccomp.json"
	}
	cmd.AddFlag("--cap-drop=ALL")
	cmd.AddFlag("--security-opt=no-new-privileges:true")
	cmd.AddFlag("--security-opt=seccomp=" + seccomp)
	cmd.AddFlag("--read-only")
	if opts.Network != "" {
		cmd.AddFlag("--network", opts.Network)
	}
	if opts.Memory != "" {
		cmd.AddFlag("--memory", opts.Memory)
	}
	if opts.CPUs != "" {
		cmd.AddFlag("--cpus", opts.CPUs)
	}
	if opts.PIDsLimit > 0 {
		cmd.AddFlag("--pids-limit", fmt.Sprintf("%d", opts.PIDsLimit))
	}
	cmd.AddFlag("--ulimit", "nofile=65536:65536")
	cmd.AddTmpfs("/tmp", "512m", true, true)
	cmd.AddTmpfs("/var/tmp", "256m", true, true)
	cmd.AddTmpfs("/run", "16m", true, true)
	cmd.AddTmpfs("/dev/shm", "64m", true, true)
	cmd.AddTmpfs("/home/agent/.config", "32m", true, false)
	cmd.AddTmpfs("/home/agent/.ssh", "1m", true, false)
	cmd.AddEphemeralVolume("/workspace")
}

func AppendCacheMounts(cmd *DockerRunCmd) {
	caches := []string{
		"/home/agent/.npm",
		"/home/agent/.cache/pip",
		"/home/agent/go",
		"/home/agent/.terraform.d/plugin-cache",
	}
	for _, c := range caches {
		cmd.AddEphemeralVolume(c)
	}
}
