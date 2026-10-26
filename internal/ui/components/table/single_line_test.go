package table

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"
)

func TestTableSingleLineRows(t *testing.T) {
	for name, tc := range map[string]struct {
		input string
		want  string
	}{
		"newline":         {input: "first\nsecond", want: "first second"},
		"CRLF":            {input: "first\r\nsecond", want: "first second"},
		"carriage return": {input: "first\rsecond", want: "first second"},
		"tab":             {input: "first\tsecond", want: "first second"},
		"vertical tab":    {input: "first\vsecond", want: "first second"},
		"form feed":       {input: "first\fsecond", want: "first second"},
		"spaces":          {input: "first  second", want: "first  second"},
		"ANSI":            {input: "\x1b[1mfirst\nsecond\x1b[0m", want: "first second"},
	} {
		t.Run(name, func(t *testing.T) {
			for _, fullRow := range []bool{false, true} {
				m := newTestTable(
					WithColumns([]Column{{Title: "Job", Width: 3}, {Title: "Message", Width: 7}}),
					WithRows([]Row{
						row("first", tc.input, tc.input),
						row("second", "next", "message"),
						row("last", "last", "message"),
					}),
					WithWidth(40), WithHeight(4),
				)
				if fullRow {
					m.SetFullRows(map[int]string{0: tc.input})
				}
				output := ansi.Strip(m.View())
				if !strings.Contains(output, tc.want) {
					t.Errorf("fullRow=%t: missing %q in\n%s", fullRow, tc.want, output)
				}
				if m.Rows()[0].Cells[0] != tc.input {
					t.Fatal("rendering changed the stored cell")
				}
				assertSingleLineTableSize(t, m)
				m.SetSize(12, 4)
				assertSingleLineTableSize(t, m)
				m.ScrollRight()
				assertSingleLineTableSize(t, m)
				m.GotoBottom()
				m.ScrollToStart()
				assertSingleLineTableSize(t, m)
				if !strings.Contains(ansi.Strip(m.View()), "last") {
					t.Fatal("selected final row is not visible")
				}
			}
		})
	}
}

func TestTableSingleLinePreservesCellStyles(t *testing.T) {
	const input = "\x1b[1mfirst\nsecond\x1b[0m"
	m := newTestTable(
		WithColumns([]Column{{Title: "Message", Width: 7}}),
		WithRows([]Row{row("selected", "other"), row("styled", input)}),
		WithWidth(30), WithHeight(4),
	)
	output := m.View()
	if !strings.Contains(output, "\x1b[1mfirst second\x1b[0m") {
		t.Fatalf("cell style was lost: %q", output)
	}
	assertANSIGrounded(t, output)
}

func TestGoldenSingleLineRows(t *testing.T) {
	for _, width := range []int{24, 40} {
		t.Run(fmt.Sprintf("width %d", width), func(t *testing.T) {
			m := newTestTable(
				WithColumns([]Column{{Title: "Job", Width: 4}, {Title: "Message", Width: 7}}),
				WithRows([]Row{
					row("database", "Database\nJob", "closed\n\tconnection"),
					row("process", "", ""),
					row("next", "NextJob", "short"),
					row("last", "LastJob", "tail"),
				}),
				WithWidth(width), WithHeight(5),
			)
			m.SetFullRows(map[int]string{1: "\x1b[1mProcess\tworker\r\nhealthy\x1b[0m"})
			golden.RequireEqual(t, []byte(ansi.Strip(m.View())))
		})
	}
}

func assertSingleLineTableSize(t *testing.T, m Model) {
	t.Helper()
	lines := strings.Split(ansi.Strip(m.View()), "\n")
	if len(lines) != m.Height() {
		t.Fatalf("table has %d lines, want %d", len(lines), m.Height())
	}
	for i, line := range lines {
		if width := ansi.StringWidth(line); width != m.Width() {
			t.Errorf("line %d width = %d, want %d: %q", i, width, m.Width(), line)
		}
		if strings.ContainsAny(line, "\r\t\v\f") {
			t.Errorf("line %d retains layout control characters: %q", i, line)
		}
	}
}
