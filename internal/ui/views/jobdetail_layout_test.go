package views

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/kpumuk/lazykiq/internal/sidekiq"
)

func TestGoldenJobDetailProperties(t *testing.T) {
	for name, tc := range map[string]struct {
		width, height int
		bottom        bool
		message       string
	}{
		"narrow top":    {80, 12, false, "Request failed"},
		"narrow bottom": {80, 12, true, "Request failed"},
		"wide top":      {160, 16, false, "Request failed"},
		"wide bottom":   {160, 16, true, "Request failed"},
		"multiline":     {100, 18, false, "first line\n\nlast line\n"},
	} {
		t.Run(name, func(t *testing.T) {
			view := NewJobDetail()
			view.SetSize(80, 12)
			view.SetJob(sidekiq.NewJobRecord(fmt.Sprintf(
				`{"jid":"job1","queue":"default","class":"ReportJob","error_class":"RuntimeError","error_message":%q,"error_backtrace":["app/jobs/report.rb:42","app/reports/daily.rb:8"],"args":[%q]}`,
				tc.message, strings.Repeat("0123456789", 9)+"TAIL",
			), "default"))
			view.SetSize(tc.width, tc.height)
			if tc.bottom {
				view.Update(tea.KeyPressMsg{Code: 'G', Text: "G"})
			}
			golden.RequireEqual(t, []byte(ansi.Strip(view.View())))
		})
	}
	t.Run("empty", func(t *testing.T) {
		view := NewJobDetail()
		view.SetSize(80, 10)
		golden.RequireEqual(t, []byte(ansi.Strip(view.View())))
	})
}
