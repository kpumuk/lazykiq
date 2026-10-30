// Package cmd provides the entrypoint and CLI command configuration for the
// lazykiq application.
package cmd

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"runtime/debug"
	"runtime/pprof"
	"slices"
	"strings"

	"charm.land/fang/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/kpumuk/lazykiq/internal/devtools"
	"github.com/kpumuk/lazykiq/internal/sidekiq"
	"github.com/kpumuk/lazykiq/internal/ui"
)

func buildVersion(version, commit, date, builtBy string) string {
	result := version
	if commit != "" {
		result = fmt.Sprintf("%s\ncommit: %s", result, commit)
	}
	if date != "" {
		result = fmt.Sprintf("%s\nbuilt at: %s", result, date)
	}
	if builtBy != "" {
		result = fmt.Sprintf("%s\nbuilt by: %s", result, builtBy)
	}
	result = fmt.Sprintf("%s\ngoos: %s\ngoarch: %s", result, runtime.GOOS, runtime.GOARCH)
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Sum != "" {
		result = fmt.Sprintf("%s\nmodule version: %s, checksum: %s", result, info.Main.Version, info.Main.Sum)
	}

	return result
}

// newRootCommand builds the CLI without starting Redis or the terminal UI.
func newRootCommand(version, commit, date, builtBy string) *cobra.Command {
	var enableDangerousActions bool
	var development bool
	var enabledFeatures []string
	rootCmd := &cobra.Command{
		Use:               "lazykiq",
		Short:             "A terminal UI for Sidekiq.",
		Long:              "A terminal UI for Sidekiq.",
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		SilenceUsage:      true,
	}

	rootCmd.Version = buildVersion(version, commit, date, builtBy)
	rootCmd.SetVersionTemplate(`lazykiq {{printf "version %s\n" .Version}}`)

	rootCmd.Flags().StringSliceVar(&enabledFeatures, "enable", nil, "enable optional features (pro, ent)")
	rootCmd.Flags().String("periodic-timezone", "UTC", "server timezone for periodic jobs without an explicit zone")

	rootCmd.Flags().String(
		"cpuprofile",
		"",
		"write cpu profile to file",
	)

	rootCmd.Flags().BoolP(
		"help",
		"h",
		false,
		"help for lazykiq",
	)

	rootCmd.Flags().String(
		"redis",
		"redis://localhost:6379/0",
		"redis URL",
	)
	rootCmd.Flags().BoolVar(
		&enableDangerousActions,
		"danger",
		false,
		"enable dangerous operations",
	)
	rootCmd.Flags().BoolVar(
		&development,
		"development",
		false,
		"enable development diagnostics",
	)
	rootCmd.Flags().SetNormalizeFunc(func(_ *pflag.FlagSet, name string) pflag.NormalizedName {
		switch name {
		case "yolo":
			name = "danger"
		}
		return pflag.NormalizedName(name)
	})
	for flag, complete := range map[string]cobra.CompletionFunc{
		"enable":            completeFeatures,
		"redis":             cobra.NoFileCompletions,
		"periodic-timezone": cobra.NoFileCompletions,
	} {
		if err := rootCmd.RegisterFlagCompletionFunc(flag, complete); err != nil {
			panic(err)
		}
	}

	rootCmd.RunE = func(cmd *cobra.Command, _ []string) error {
		features, err := sidekiq.ResolveFeatures(enabledFeatures)
		if err != nil {
			return err
		}
		sidekiq.DisableRedisLogging()

		cpuprofile, err := cmd.Flags().GetString("cpuprofile")
		if err != nil {
			return fmt.Errorf("parse cpuprofile flag: %w", err)
		}

		redisURL, err := cmd.Flags().GetString("redis")
		if err != nil {
			return fmt.Errorf("parse redis flag: %w", err)
		}

		periodicTimezone, err := cmd.Flags().GetString("periodic-timezone")
		if err != nil {
			return err
		}
		client, err := sidekiq.NewClient(redisURL, sidekiq.WithFeatures(features), sidekiq.WithPeriodicTimezone(periodicTimezone))
		if err != nil {
			return fmt.Errorf("create redis client: %w", err)
		}
		defer func() {
			_ = client.Close()
		}()

		var profileFile *os.File
		if cpuprofile != "" {
			file, err := os.Create(cpuprofile)
			if err != nil {
				return fmt.Errorf("create cpuprofile file: %w", err)
			}
			profileFile = file
			if err := pprof.StartCPUProfile(profileFile); err != nil {
				_ = profileFile.Close()
				return fmt.Errorf("start cpu profile: %w", err)
			}
			defer func() {
				pprof.StopCPUProfile()
				_ = profileFile.Close()
			}()
		}

		var tracker *devtools.Tracker
		if development {
			tracker = devtools.NewTracker()
			client.AddHook(tracker.Hook())
		}

		app := ui.NewWithContext(cmd.Context(), client, version, enableDangerousActions, tracker)
		// Bubble Tea does not manage the custom appearance-reporting mode.
		// Disable it after terminal ownership returns, including on errors.
		defer func() { _, _ = fmt.Fprint(os.Stdout, ansi.ResetModeLightDark) }()
		if _, err := app.Run(); err != nil {
			return fmt.Errorf("run lazykiq: %w", err)
		}

		return nil
	}

	return rootCmd
}

func completeFeatures(cmd *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	enabled, err := cmd.Flags().GetStringSlice("enable")
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}

	parts := strings.Split(toComplete, ",")
	partial := parts[len(parts)-1]
	prefix := strings.TrimSuffix(toComplete, partial)
	selected := parts[:len(parts)-1]
	var completions []string
	for _, name := range sidekiq.FeatureNames() {
		if strings.HasPrefix(name, partial) && !slices.Contains(enabled, name) && !slices.Contains(selected, name) {
			completions = append(completions, prefix+name)
		}
	}
	return completions, cobra.ShellCompDirectiveNoFileComp
}

// Execute initializes and runs the lazykiq terminal application.
func Execute(version, commit, date, builtBy string) error {
	rootCmd := newRootCommand(version, commit, date, builtBy)

	return fang.Execute(
		context.Background(),
		rootCmd,
		fang.WithoutVersion(),
		fang.WithoutManpage(),
	)
}
