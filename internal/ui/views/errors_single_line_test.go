package views

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/kpumuk/lazykiq/internal/sidekiq"
)

const multilineErrorMessage = "PQconsumeInput() server closed the connection unexpectedly\n\tThis probably means the server terminated abnormally\n\tbefore or while processing the request."

func multilineErrorsSummary(message string, width int) *ErrorsSummary {
	v := NewErrorsSummary(nil)
	v.SetStyles(Styles{})
	v.SetSize(width, 8)
	rows := make([]sidekiq.ErrorSummaryRow, 9)
	rows[0] = sidekiq.ErrorSummaryRow{
		DisplayClass: "DatabaseJob", ErrorClass: "ActiveRecord::ConnectionFailed",
		Queue: "periodic", Count: 1, ErrorMessage: message,
	}
	for i := range 8 {
		rows[i+1] = sidekiq.ErrorSummaryRow{
			DisplayClass: fmt.Sprintf("AfterJob%d", i), ErrorClass: "RuntimeError",
			Queue: "default", Count: 1, ErrorMessage: "after",
		}
	}
	v.Update(errorsSummaryDataMsg{rows: rows})
	return v
}

func TestErrorsSummarySingleLineMessages(t *testing.T) {
	for name, message := range map[string]string{
		"LF":   multilineErrorMessage,
		"CRLF": strings.ReplaceAll(multilineErrorMessage, "\n", "\r\n"),
		"tabs": "first\tsecond\ttail",
	} {
		t.Run(name, func(t *testing.T) {
			v := multilineErrorsSummary(message, 240)
			assertErrorsSummarySize(t, v)
			v.table.ScrollToEnd()
			assertErrorsSummarySize(t, v)
			tail := "processing the request."
			if name == "tabs" {
				tail = "first second tail"
			}
			if !strings.Contains(ansi.Strip(v.View()), tail) {
				t.Fatalf("message tail is not accessible by horizontal scrolling:\n%s", ansi.Strip(v.View()))
			}
			v.table.GotoBottom()
			v.table.ScrollToStart()
			assertErrorsSummarySize(t, v)
			if !strings.Contains(v.View(), "AfterJob7") {
				t.Fatal("selected final row is missing from the viewport")
			}
			if v.rows[0].ErrorMessage != message || v.table.Rows()[0].Cells[4] != message {
				t.Fatal("rendering changed the stored error message")
			}
		})
	}
}

func TestGoldenErrorsSummaryMultilineMessage(t *testing.T) {
	for _, width := range []int{160, 240} {
		t.Run(fmt.Sprintf("width %d", width), func(t *testing.T) {
			v := multilineErrorsSummary(multilineErrorMessage, width)
			golden.RequireEqual(t, []byte(ansi.Strip(v.View())))
		})
	}
}

func assertErrorsSummarySize(t *testing.T, v *ErrorsSummary) {
	t.Helper()
	lines := strings.Split(ansi.Strip(v.View()), "\n")
	if len(lines) != v.height {
		t.Fatalf("view has %d lines, want %d", len(lines), v.height)
	}
	for i, line := range lines {
		if width := ansi.StringWidth(line); width != v.width {
			t.Errorf("line %d width = %d, want %d", i, width, v.width)
		}
	}
}
