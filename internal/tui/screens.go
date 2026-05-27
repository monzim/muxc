package tui

// screenID identifies which screen the App is currently rendering.
type screenID int

const (
	screenSessions screenID = iota
	screenMenu
	screenNewForm
	screenKillPicker
	screenInfo
	screenDoctor
)

// String is used by error/debug messages — not required by Bubble Tea itself.
func (s screenID) String() string {
	switch s {
	case screenSessions:
		return "sessions"
	case screenMenu:
		return "menu"
	case screenNewForm:
		return "new"
	case screenKillPicker:
		return "kill"
	case screenInfo:
		return "info"
	case screenDoctor:
		return "doctor"
	default:
		return "?"
	}
}
