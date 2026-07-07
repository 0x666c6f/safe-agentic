package main

import (
	"fmt"
	"os"
	"strings"

	profilepkg "github.com/0x666c6f/berth/pkg/profiles"
	"github.com/spf13/cobra"
)

var profileDirs []string
var profileRunDryRun bool

var profileCmd = &cobra.Command{
	Use:     "profile",
	Short:   "Run reusable agent profiles",
	GroupID: groupSpawn,
}

var profileListCmd = &cobra.Command{
	Use:   "list",
	Short: "List configured agent profiles",
	Args:  cobra.NoArgs,
	RunE:  runProfileList,
}

var profileShowCmd = &cobra.Command{
	Use:   "show <name>",
	Short: "Show one agent profile",
	Args:  cobra.ExactArgs(1),
	RunE:  runProfileShow,
}

var profileRunCmd = &cobra.Command{
	Use:   "run <name> [prompt]",
	Short: "Spawn an agent from a profile",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runProfileRun,
}

func init() {
	profileCmd.PersistentFlags().StringArrayVar(&profileDirs, "dir", nil, "Additional profile directory; repeatable")
	profileRunCmd.Flags().BoolVar(&profileRunDryRun, "dry-run", false, "Show what would run without executing")
	profileCmd.AddCommand(profileListCmd, profileShowCmd, profileRunCmd)
	rootCmd.AddCommand(profileCmd)
}

func loadProfileCatalog() (profilepkg.Catalog, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return profilepkg.Catalog{}, fmt.Errorf("get cwd: %w", err)
	}
	dirs := profilepkg.DefaultDirs(cwd)
	dirs = append(dirs, profileDirs...)
	return profilepkg.LoadDirs(dirs)
}

func runProfileList(cmd *cobra.Command, args []string) error {
	catalog, err := loadProfileCatalog()
	if err != nil {
		return err
	}
	if len(catalog.Profiles) == 0 {
		fmt.Println("No profiles configured.")
		fmt.Printf("Create %s/<name>.toml or .berth/agents/<name>.toml.\n", profilepkg.UserDir())
		return nil
	}
	for _, profile := range catalog.Profiles {
		agentType := profile.AgentType
		if agentType == "" {
			agentType = "claude"
		}
		summary := profile.Prompt
		if summary == "" {
			summary = profile.Template
		}
		fmt.Printf("%-20s %-8s %s\n", profile.Name, agentType, oneLine(summary))
	}
	return nil
}

func runProfileShow(cmd *cobra.Command, args []string) error {
	profile, err := getProfile(args[0])
	if err != nil {
		return err
	}
	fmt.Printf("name: %s\n", profile.Name)
	fmt.Printf("source: %s\n", profile.Source)
	if profile.AgentType != "" {
		fmt.Printf("agent_type: %s\n", profile.AgentType)
	}
	if len(profile.Repos) > 0 {
		fmt.Printf("repos: %s\n", strings.Join(profile.Repos, ","))
	}
	if profile.ContainerName != "" {
		fmt.Printf("container_name: %s\n", profile.ContainerName)
	}
	if profile.Template != "" {
		fmt.Printf("template: %s\n", profile.Template)
	}
	if profile.Prompt != "" {
		fmt.Printf("prompt: %s\n", profile.Prompt)
	}
	printProfileBool("ssh", profile.SSH)
	printProfileBool("reuse_auth", profile.ReuseAuth)
	printProfileBool("reuse_gh_auth", profile.ReuseGHAuth)
	printProfileBool("seed_auth", profile.SeedAuth)
	printProfileBool("docker", profile.Docker)
	printProfileBool("docker_socket", profile.DockerSocket)
	printProfileBool("background", profile.Background)
	return nil
}

func runProfileRun(cmd *cobra.Command, args []string) error {
	profile, err := getProfile(args[0])
	if err != nil {
		return err
	}
	prompt := ""
	if len(args) > 1 {
		prompt = strings.Join(args[1:], " ")
	}
	opts := profileToSpawnOpts(profile, prompt)
	opts.DryRun = profileRunDryRun
	return executeSpawn(opts)
}

func getProfile(name string) (profilepkg.Profile, error) {
	catalog, err := loadProfileCatalog()
	if err != nil {
		return profilepkg.Profile{}, err
	}
	profile, ok := catalog.Get(name)
	if !ok {
		return profilepkg.Profile{}, fmt.Errorf("profile %q not found", name)
	}
	return profile, nil
}

func profileToSpawnOpts(profile profilepkg.Profile, promptOverride string) SpawnOpts {
	agentType := profile.AgentType
	if agentType == "" {
		agentType = "claude"
	}
	prompt := profile.Prompt
	if promptOverride != "" {
		if prompt != "" {
			prompt += "\n\n" + promptOverride
		} else {
			prompt = promptOverride
		}
	}
	opts := SpawnOpts{
		AgentType:         agentType,
		Repos:             append([]string{}, profile.Repos...),
		Name:              profile.ContainerName,
		Prompt:            prompt,
		Template:          profile.Template,
		TemplateVars:      append([]string{}, profile.TemplateVars...),
		Instructions:      profile.Instructions,
		InstructionsFile:  profile.InstructionsFile,
		Network:           profile.Network,
		Memory:            profile.Memory,
		CPUs:              profile.CPUs,
		PIDsLimit:         profile.PIDsLimit,
		Identity:          profile.Identity,
		AWSProfile:        profile.AWSProfile,
		MaxCost:           profile.MaxCost,
		Notify:            profile.Notify,
		OnExit:            profile.OnExit,
		OnComplete:        profile.OnComplete,
		OnFail:            profile.OnFail,
		AllowSetupScripts: boolValue(profile.AllowSetupScripts),
		AutoTrust:         boolValue(profile.AutoTrust),
		Background:        boolValue(profile.Background),
		EphemeralAuth:     boolValue(profile.EphemeralAuth),
	}
	applyProfileBool(profile.SSH, &opts.SSH, &opts.NoSSH)
	applyProfileBool(profile.ReuseAuth, &opts.ReuseAuth, &opts.NoReuseAuth)
	applyProfileBool(profile.ReuseGHAuth, &opts.ReuseGHAuth, &opts.NoReuseGHAuth)
	applyProfileBool(profile.SeedAuth, &opts.SeedAuth, &opts.NoSeedAuth)
	applyProfileBool(profile.Docker, &opts.DockerAccess, &opts.NoDocker)
	applyProfileBool(profile.DockerSocket, &opts.DockerSocket, &opts.NoDockerSocket)
	return opts
}

func applyProfileBool(value *bool, enable *bool, disable *bool) {
	if value == nil {
		return
	}
	if *value {
		*enable = true
	} else {
		*disable = true
	}
}

func boolValue(value *bool) bool {
	return value != nil && *value
}

func printProfileBool(name string, value *bool) {
	if value != nil {
		fmt.Printf("%s: %v\n", name, *value)
	}
}
