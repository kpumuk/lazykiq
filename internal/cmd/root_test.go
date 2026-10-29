package cmd

import (
	"io"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/kpumuk/lazykiq/internal/sidekiq"
)

func TestEnableFeatures(t *testing.T) {
	for name, test := range map[string]struct {
		args    []string
		want    bool
		invalid bool
	}{
		"none":          {},
		"pro":           {args: []string{"--enable=pro"}, want: true},
		"enterprise":    {args: []string{"--enable=ent"}, want: true},
		"comma":         {args: []string{"--enable=pro,ent"}, want: true},
		"repeated":      {args: []string{"--enable=pro", "--enable=ent", "--enable=pro"}, want: true},
		"unknown":       {args: []string{"--enable=smth-else"}, invalid: true},
		"mixed unknown": {args: []string{"--enable=pro,smth-else"}, invalid: true},
	} {
		t.Run(name, func(t *testing.T) {
			cmd := newRootCommand("test", "", "", "")
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)
			cmd.SetArgs(test.args)
			cmd.RunE = func(cmd *cobra.Command, _ []string) error {
				names, err := cmd.Flags().GetStringSlice("enable")
				if err != nil {
					return err
				}
				features, err := sidekiq.ResolveFeatures(names)
				if err == nil && features.SupportsQueuePause() != test.want {
					t.Fatalf("queue pause = %v", features.SupportsQueuePause())
				}
				return err
			}
			err := cmd.Execute()
			if (err != nil) != test.invalid {
				t.Fatalf("Execute = %v", err)
			}
		})
	}
}

func TestUnknownFeatureRejectedBeforeStartup(t *testing.T) {
	cmd := newRootCommand("test", "", "", "")
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"--enable=unknown", "--redis=invalid"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), `unknown feature "unknown"`) {
		t.Fatalf("Execute = %v", err)
	}
}

func TestInformationalFlagsSkipStartup(t *testing.T) {
	for _, flag := range []string{"--help", "--version"} {
		t.Run(flag, func(t *testing.T) {
			cmd := newRootCommand("test-version", "abc", "today", "builder")
			var out strings.Builder
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			cmd.SetArgs([]string{flag})
			cmd.RunE = func(*cobra.Command, []string) error { t.Fatal("informational flag started TUI"); return nil }
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}
			want := "--redis"
			if flag == "--version" {
				want = "test-version\ncommit: abc\nbuilt at: today\nbuilt by: builder"
			}
			if !strings.Contains(out.String(), want) {
				t.Fatalf("missing %q in %q", want, out.String())
			}
		})
	}
}

func TestShellCompletion(t *testing.T) {
	for name, test := range map[string]struct {
		args      []string
		want      []string
		directive cobra.ShellCompDirective
	}{
		"flag name":       {args: []string{"--re"}, want: []string{"--redis"}, directive: cobra.ShellCompDirectiveNoFileComp},
		"features":        {args: []string{"--enable", ""}, want: []string{"ent", "pro"}, directive: cobra.ShellCompDirectiveNoFileComp},
		"feature prefix":  {args: []string{"--enable=pr"}, want: []string{"pro"}, directive: cobra.ShellCompDirectiveNoFileComp},
		"comma separated": {args: []string{"--enable", "pro,e"}, want: []string{"pro,ent"}, directive: cobra.ShellCompDirectiveNoFileComp},
		"after comma":     {args: []string{"--enable", "ent,"}, want: []string{"ent,pro"}, directive: cobra.ShellCompDirectiveNoFileComp},
		"repeated flag":   {args: []string{"--enable=pro", "--enable", ""}, want: []string{"ent"}, directive: cobra.ShellCompDirectiveNoFileComp},
		"all selected":    {args: []string{"--enable=pro,ent", "--enable", ""}, directive: cobra.ShellCompDirectiveNoFileComp},
		"unknown feature": {args: []string{"--enable", "unknown"}, directive: cobra.ShellCompDirectiveNoFileComp},
		"redis URL":       {args: []string{"--redis", ""}, directive: cobra.ShellCompDirectiveNoFileComp},
		"no positional":   {args: []string{""}, directive: cobra.ShellCompDirectiveNoFileComp},
		"profile file":    {args: []string{"--cpuprofile", ""}, directive: cobra.ShellCompDirectiveDefault},
	} {
		t.Run(name, func(t *testing.T) {
			cmd := newRootCommand("test", "", "", "")
			var out strings.Builder
			cmd.SetOut(&out)
			cmd.SetErr(io.Discard)
			cmd.SetArgs(append([]string{cobra.ShellCompNoDescRequestCmd}, test.args...))
			cmd.RunE = func(*cobra.Command, []string) error { t.Fatal("completion started TUI"); return nil }
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}
			lines := strings.Split(strings.TrimSpace(out.String()), "\n")
			wantDirective := ":" + strconv.Itoa(int(test.directive))
			if got := lines[len(lines)-1]; got != wantDirective {
				t.Fatalf("directive = %q, want %q", got, wantDirective)
			}
			if got := lines[:len(lines)-1]; !slices.Equal(got, test.want) {
				t.Fatalf("completions = %q, want %q", got, test.want)
			}
		})
	}
}

func TestCompletionScriptsSkipStartup(t *testing.T) {
	for _, shell := range []string{"bash", "zsh", "fish", "powershell"} {
		t.Run(shell, func(t *testing.T) {
			cmd := newRootCommand("test", "", "", "")
			var out strings.Builder
			cmd.SetOut(&out)
			cmd.SetErr(io.Discard)
			cmd.SetArgs([]string{"completion", shell})
			cmd.RunE = func(*cobra.Command, []string) error { t.Fatal("script generation started TUI"); return nil }
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out.String(), "lazykiq") || strings.Contains(out.String(), "\x1b") {
				t.Fatalf("invalid %s completion script", shell)
			}
		})
	}
}
