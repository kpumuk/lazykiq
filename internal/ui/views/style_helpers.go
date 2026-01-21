package views

import (
	"github.com/kpumuk/lazykiq/internal/ui/components/frame"
	"github.com/kpumuk/lazykiq/internal/ui/components/table"
	filterdialog "github.com/kpumuk/lazykiq/internal/ui/dialogs/filter"
)

func frameStylesFromTheme(styles Styles) frame.Styles {
	return frame.Styles{
		Focused: frame.StyleState{
			Title:  styles.Title,
			Muted:  styles.Muted,
			Filter: styles.FilterFocused,
			Border: styles.FocusBorder,
		},
		Blurred: frame.StyleState{
			Title:  styles.Title,
			Muted:  styles.Muted,
			Filter: styles.FilterBlurred,
			Border: styles.BorderStyle,
		},
	}
}

func filterDialogStylesFromTheme(styles Styles) filterdialog.Styles {
	return filterdialog.Styles{
		Title:       styles.Title,
		Border:      styles.FocusBorder,
		Text:        styles.Text,
		Placeholder: styles.Muted,
		Cursor:      styles.Text,
	}
}

func filterDialogStylesWithPrompt(styles Styles) filterdialog.Styles {
	dialogStyles := filterDialogStylesFromTheme(styles)
	dialogStyles.Prompt = styles.Text
	return dialogStyles
}

func tableStylesFromTheme(styles Styles) table.Styles {
	return table.Styles{
		Text:           styles.Text,
		Muted:          styles.Muted,
		Header:         styles.TableHeader,
		Selected:       styles.TableSelected,
		Separator:      styles.TableSeparator,
		ScrollbarTrack: styles.ScrollbarTrack,
		ScrollbarThumb: styles.ScrollbarThumb,
	}
}
