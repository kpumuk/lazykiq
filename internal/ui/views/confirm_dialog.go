package views

import (
	"charm.land/lipgloss/v2"

	confirmdialog "github.com/kpumuk/lazykiq/internal/ui/dialogs/confirm"
)

func newConfirmDialog(styles Styles, title, message, target string, yesStyle lipgloss.Style) *confirmdialog.Model {
	return confirmdialog.New(
		confirmdialog.WithStyles(confirmdialog.Styles{
			Title:           styles.Title,
			Border:          styles.FocusBorder,
			Text:            styles.Text,
			Muted:           styles.Muted,
			Button:          styles.Muted.Padding(0, 1),
			ButtonYesActive: yesStyle,
			ButtonNoActive:  styles.NeutralAction,
		}),
		confirmdialog.WithTitle(title),
		confirmdialog.WithMessage(message),
		confirmdialog.WithTarget(target),
	)
}
