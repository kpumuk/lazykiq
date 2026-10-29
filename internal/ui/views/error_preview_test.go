package views

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/kpumuk/lazykiq/internal/sidekiq"
	"github.com/kpumuk/lazykiq/internal/ui/components/table"
)

func errorPreviewEntry(class, message string) *sidekiq.SortedEntry {
	return &sidekiq.SortedEntry{JobRecord: sidekiq.NewJobRecord(fmt.Sprintf(
		`{"jid":"preview","class":"PreviewJob","queue":"default","args":[],"error_class":%q,"error_message":%q}`,
		class, message,
	), "default")}
}

func TestErrorDisplay(t *testing.T) {
	for name, tc := range map[string]struct {
		class, message, want string
	}{
		"keycap emoji":               {"E", strings.Repeat("1️⃣", 49), "E: " + strings.Repeat("1️⃣", 48) + "…"},
		"short ASCII":                {"Error", "failed", "Error: failed"},
		"99 cells":                   {"E", strings.Repeat("x", 96), "E: " + strings.Repeat("x", 96)},
		"100 cells":                  {"E", strings.Repeat("x", 97), "E: " + strings.Repeat("x", 97)},
		"101 cells":                  {"E", strings.Repeat("x", 98), "E: " + strings.Repeat("x", 96) + "…"},
		"CJK fits despite bytes":     {"EE", strings.Repeat("界", 40), "EE: " + strings.Repeat("界", 40)},
		"CJK exact boundary":         {"EE", strings.Repeat("界", 48), "EE: " + strings.Repeat("界", 48)},
		"CJK clipped":                {"EE", strings.Repeat("界", 49), "EE: " + strings.Repeat("界", 47) + "…"},
		"emoji":                      {"E", strings.Repeat("😀", 49), "E: " + strings.Repeat("😀", 48) + "…"},
		"joined emoji":               {"E", strings.Repeat("👩🏽‍💻", 49), "E: " + strings.Repeat("👩🏽‍💻", 48) + "…"},
		"combining fits":             {"E", strings.Repeat("e\u0301", 97), "E: " + strings.Repeat("e\u0301", 97)},
		"combining clipped":          {"E", strings.Repeat("e\u0301", 98), "E: " + strings.Repeat("e\u0301", 96) + "…"},
		"wide cluster straddles cut": {"E", strings.Repeat("x", 95) + "👩🏽‍💻tail", "E: " + strings.Repeat("x", 95) + "…"},
		"long class":                 {strings.Repeat("界", 51), "failed", strings.Repeat("界", 49) + "…"},
		"empty error":                {"", "", ": "},
	} {
		t.Run(name, func(t *testing.T) {
			got := errorDisplay(errorPreviewEntry(tc.class, tc.message))
			if !utf8.ValidString(got) {
				t.Fatalf("preview contains invalid UTF-8: %q", got)
			}
			if width := ansi.StringWidth(got); width > 100 {
				t.Errorf("preview width = %d, exceeds 100 cells", width)
			}
			if got != tc.want {
				t.Errorf("preview = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestErrorDisplayWithoutError(t *testing.T) {
	for name, entry := range map[string]*sidekiq.SortedEntry{
		"nil entry":             nil,
		"nil record":            {},
		"no error":              {JobRecord: sidekiq.NewJobRecord(`{"jid":"preview"}`, "default")},
		"message without class": {JobRecord: sidekiq.NewJobRecord(`{"error_message":"failed"}`, "default")},
	} {
		t.Run(name, func(t *testing.T) {
			if got := errorDisplay(entry); got != "" {
				t.Errorf("preview = %q, want empty", got)
			}
		})
	}
}

func TestGoldenErrorsDetailsUnicodePreview(t *testing.T) {
	view := NewErrorsDetails(nil)
	rows := view.buildRows([]sidekiq.ErrorGroupEntry{
		{Source: "retry", Entry: errorPreviewEntry("EE", strings.Repeat("界", 40))},
		{Source: "dead", Entry: errorPreviewEntry("EE", strings.Repeat("界", 60))},
		{Source: "retry", Entry: errorPreviewEntry("E", strings.Repeat("e\u0301", 98))},
	})
	for i := range rows {
		rows[i].Cells[1] = "1h"
	}
	for _, width := range []int{80, 240} {
		t.Run(fmt.Sprintf("width %d", width), func(t *testing.T) {
			model := table.New(table.WithColumns(errorDetailsColumns), table.WithRows(rows))
			model.SetSize(width, 5)
			model.ScrollToEnd()
			golden.RequireEqual(t, []byte(ansi.Strip(model.View())))
		})
	}
}
