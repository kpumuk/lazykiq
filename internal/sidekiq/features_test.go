package sidekiq

import "testing"

func TestPeriodicJobsRequireEnterprise(t *testing.T) {
	for name, test := range map[string]struct {
		names []string
		want  bool
	}{
		"default":    {},
		"pro":        {names: []string{"pro"}},
		"enterprise": {names: []string{"ent"}, want: true},
		"combined":   {names: []string{"pro", "ent"}, want: true},
		"repeated":   {names: []string{"ent", "pro", "ent"}, want: true},
	} {
		t.Run(name, func(t *testing.T) {
			mr, client := setupTestRedis(t)
			_ = mr.Set("periodic-version", "fixture")
			capabilities, err := ResolveFeatures(test.names)
			if err != nil {
				t.Fatal(err)
			}
			client.capabilities = capabilities
			if got := client.GetCapabilities().SupportsPeriodicJobs(); got != test.want {
				t.Fatalf("SupportsPeriodicJobs = %v, want %v", got, test.want)
			}
			if test.want && !client.GetCapabilities().SupportsQueuePause() {
				t.Fatal("Enterprise must retain Pro capabilities")
			}
		})
	}
}

func TestInvalidFeatureClearsPeriodicJobs(t *testing.T) {
	capabilities, err := ResolveFeatures([]string{"ent", "unknown"})
	if err == nil || capabilities.SupportsPeriodicJobs() || capabilities.SupportsQueuePause() {
		t.Fatalf("ResolveFeatures = %+v, %v; want no capabilities and an error", capabilities, err)
	}
}
