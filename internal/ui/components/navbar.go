package components

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/kpumuk/lazykiq/internal/ui/theme"
	"github.com/kpumuk/lazykiq/internal/ui/views"
)

// Navbar displays the navigation bar at the bottom of the screen
type Navbar struct {
	width  int
	views  []views.View
	styles *theme.Styles
}

// NewNavbar creates a new Navbar component
func NewNavbar(viewList []views.View, styles *theme.Styles) Navbar {
	return Navbar{
		views:  viewList,
		styles: styles,
	}
}

// Init returns an initial command
func (n Navbar) Init() tea.Cmd {
	return nil
}

// Update handles messages
func (n Navbar) Update(msg tea.Msg) (Navbar, tea.Cmd) {
	return n, nil
}

// View renders the navbar
func (n Navbar) View() string {
	s := n.styles

	barStyle := s.NavBar.Width(n.width)

	items := ""
	for i, v := range n.views {
		key := s.NavKey.Render(fmt.Sprintf("%d", i+1))
		name := s.NavItem.Render(v.Name())
		items += key + name
	}

	// Add quit hint
	items += s.NavKey.Render("q") + s.NavQuit.Render("quit")

	// Add theme toggle hint
	items += s.NavKey.Render("t") + s.NavQuit.Render("theme")

	return barStyle.Render(items)
}

// SetWidth updates the width of the navbar
func (n Navbar) SetWidth(width int) Navbar {
	n.width = width
	return n
}

// SetStyles updates the styles
func (n Navbar) SetStyles(styles *theme.Styles) Navbar {
	n.styles = styles
	return n
}

// Height returns the height of the navbar
func (n Navbar) Height() int {
	return 1
}
