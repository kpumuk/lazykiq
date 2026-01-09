package jsonview

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"
)

type samplePayload struct {
	Name   string  `json:"name"`
	Count  int     `json:"count"`
	Active bool    `json:"active"`
	Score  float64 `json:"score"`
}

func renderAll(m Model, offset, width int) string {
	if m.LineCount() == 0 {
		return ""
	}
	lines := make([]string, m.LineCount())
	for i := range lines {
		lines[i] = m.RenderLine(i, offset, width)
	}
	return strings.Join(lines, "\n")
}

func TestSetValueNil(t *testing.T) {
	m := New()
	m.SetValue(nil)

	if m.LineCount() != 0 {
		t.Fatalf("expected 0 lines, got %d", m.LineCount())
	}
	if m.MaxWidth() != 0 {
		t.Fatalf("expected max width 0, got %d", m.MaxWidth())
	}
	if got := m.RenderLine(0, 0, 10); got != "" {
		t.Fatalf("expected empty render, got %q", got)
	}
}

func TestRenderLineDimensions(t *testing.T) {
	payload := samplePayload{
		Name:   "job",
		Count:  12,
		Active: true,
		Score:  7.5,
	}

	m := New()
	m.SetValue(payload)

	tests := map[string]struct {
		index  int
		offset int
		width  int
	}{
		"first line":      {index: 0, offset: 0, width: 20},
		"scrolled line":   {index: 1, offset: 4, width: 16},
		"out of range":    {index: 100, offset: 0, width: 10},
		"zero width":      {index: 0, offset: 0, width: 0},
		"negative index":  {index: -1, offset: 0, width: 10},
		"negative offset": {index: 0, offset: -5, width: 12},
		"narrow viewport": {index: 2, offset: 0, width: 8},
		"wider viewport":  {index: 2, offset: 2, width: 24},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := m.RenderLine(tc.index, tc.offset, tc.width)
			if tc.width <= 0 || tc.index < 0 || tc.index >= m.LineCount() {
				if got != "" {
					t.Fatalf("expected empty render, got %q", got)
				}
				return
			}
			if w := ansi.StringWidth(got); w != tc.width {
				t.Fatalf("expected width %d, got %d", tc.width, w)
			}
		})
	}
}

func TestGoldenJSONView(t *testing.T) {
	payload := samplePayload{
		Name:   "job",
		Count:  12,
		Active: true,
		Score:  7.5,
	}

	m := New()
	m.SetValue(payload)

	output := ansi.Strip(renderAll(m, 0, 30))
	golden.RequireEqual(t, []byte(output))
}

func TestGoldenJSONViewHorizontalScroll(t *testing.T) {
	payload := samplePayload{
		Name:   "long-job-name",
		Count:  123456,
		Active: true,
		Score:  7.5,
	}

	m := New()
	m.SetValue(payload)

	output := ansi.Strip(renderAll(m, 6, 24))
	golden.RequireEqual(t, []byte(output))
}
