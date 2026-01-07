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

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/fang"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

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

// Execute initializes and runs the lazykiq terminal application.
func Execute(version, commit, date, builtBy string) error {
	var enableDangerousActions bool
	rootCmd := &cobra.Command{
		Use:   "lazykiq",
		Short: "A terminal UI for Sidekiq.",
		Long:  "A terminal UI for Sidekiq.",
		Args:  cobra.NoArgs,
	}

	rootCmd.Version = buildVersion(version, commit, date, builtBy)
	rootCmd.SetVersionTemplate(`lazykiq {{printf "version %s\n" .Version}}`)

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
	rootCmd.Flags().SetNormalizeFunc(func(_ *pflag.FlagSet, name string) pflag.NormalizedName {
		switch name {
		case "yolo":
			name = "danger"
		}
		return pflag.NormalizedName(name)
	})

	rootCmd.RunE = func(cmd *cobra.Command, _ []string) error {
		cpuprofile, err := cmd.Flags().GetString("cpuprofile")
		if err != nil {
			return fmt.Errorf("parse cpuprofile flag: %w", err)
		}

		redisURL, err := cmd.Flags().GetString("redis")
		if err != nil {
			return fmt.Errorf("parse redis flag: %w", err)
		}

		client, err := sidekiq.NewClient(redisURL)
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

		app := ui.New(client, version, enableDangerousActions)
		p := tea.NewProgram(app)
		if _, err := p.Run(); err != nil {
			return fmt.Errorf("run lazykiq: %w", err)
		}

		return nil
	}

	return fang.Execute(
		context.Background(),
		rootCmd,
		fang.WithVersion(rootCmd.Version),
		fang.WithoutCompletions(),
		fang.WithoutManpage(),
	)
}
