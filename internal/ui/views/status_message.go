package views

import "github.com/kpumuk/lazykiq/internal/ui/components/messagebox"

// frameFooter holds the label the app asks a view to inset into the bottom
// border of its bottom-most frame. Views embed it to implement FooterSetter.
type frameFooter struct {
	footer string
}

// SetFooter implements FooterSetter.
func (f *frameFooter) SetFooter(footer string) {
	f.footer = footer
}

func renderStatusMessage(title, msg, footer string, styles Styles, width, height int) string {
	return messagebox.New(
		messagebox.WithStyles(messagebox.Styles{
			Title:  styles.Title,
			Muted:  styles.Muted,
			Border: styles.FocusBorder,
		}),
		messagebox.WithTitle(title),
		messagebox.WithMessage(msg),
		messagebox.WithFooter(footer),
		messagebox.WithSize(width, height),
	).View()
}
