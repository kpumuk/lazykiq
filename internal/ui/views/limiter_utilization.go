package views

import (
	"fmt"
	"math"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/kpumuk/lazykiq/internal/mathutil"
	"github.com/kpumuk/lazykiq/internal/sidekiq"
)

func (m *RateLimits) utilizationHeight() int {
	if !m.detail || m.limiter == nil || m.limiter.Missing || m.limiter.Type != "Concurrent" || m.width < 24 || m.height < 8 {
		return 0
	}
	return 3
}

func (m *RateLimits) concurrentUtilization() string {
	width := max(m.width-4, 0)
	used, free, total := m.limiter.Values["used"], m.limiter.Values["free"], m.limiter.Values["size"]
	for _, value := range []sidekiq.LimiterValue{used, free, total} {
		if !value.Valid || value.Number < 0 || math.IsNaN(value.Number) || math.IsInf(value.Number, 0) {
			return m.styles.TableHeader.Render("Utilization") + "\n" + m.styles.Muted.Render("Unavailable")
		}
	}
	if used.Number+free.Number != total.Number {
		return m.styles.TableHeader.Render("Utilization") + "\n" + m.styles.Muted.Render("Unavailable")
	}
	if total.Number == 0 {
		return m.styles.TableHeader.Render("Utilization") + "\n" + m.styles.Muted.Render("0 slots")
	}

	fraction := used.Number / total.Number
	percentage := fmt.Sprintf("%.0f%%", fraction*100)
	if fraction > 0 && fraction < 0.01 {
		percentage = "<1%"
	} else if fraction > 0.99 && fraction < 1 {
		percentage = ">99%"
	}
	label := "Utilization " + percentage
	summary := fmt.Sprintf("%s used · %s free · %s total", used.Raw, free.Raw, total.Raw)
	if width < 60 {
		label = percentage + " used"
		summary = fmt.Sprintf("%s/%s · %s free", used.Raw, total.Raw, free.Raw)
	}
	heading := m.styles.TableHeader.Render(label)
	if gap := width - ansi.StringWidth(label) - ansi.StringWidth(summary); gap > 0 {
		heading += strings.Repeat(" ", gap) + m.styles.Text.Render(summary)
	}
	filled := mathutil.Clamp(int(math.Round(fraction*float64(width))), 0, width)
	if used.Number > 0 && free.Number > 0 {
		filled = mathutil.Clamp(filled, 1, width-1)
	}
	usedStyle := m.styles.ChartHistogram
	if free.Number == 0 {
		usedStyle = m.styles.ChartFailure
	}
	bar := usedStyle.Render(strings.Repeat("█", filled)) + m.styles.ChartSuccess.Render(strings.Repeat("░", width-filled))
	return heading + "\n" + bar
}
