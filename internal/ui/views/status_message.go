package views

import "github.com/kpumuk/lazykiq/internal/ui/components/messagebox"

func renderStatusMessage(title, msg string, styles Styles, width, height int) string {
	return messagebox.New(
		messagebox.WithStyles(messagebox.Styles{
			Title:  styles.Title,
			Muted:  styles.Muted,
			Border: styles.FocusBorder,
		}),
		messagebox.WithTitle(title),
		messagebox.WithMessage(msg),
		messagebox.WithSize(width, height),
	).View()
}
