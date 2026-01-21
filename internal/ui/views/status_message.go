package views

import "github.com/kpumuk/lazykiq/internal/ui/components/messagebox"

func renderStatusMessage(title, msg string, styles Styles, width, height int) string {
	return messagebox.Render(messagebox.Styles{
		Title:  styles.Title,
		Muted:  styles.Muted,
		Border: styles.FocusBorder,
	}, title, msg, width, height)
}
