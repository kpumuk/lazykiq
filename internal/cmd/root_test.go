package cmd

import (
	"io"
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
