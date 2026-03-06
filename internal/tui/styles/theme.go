package styles

import "github.com/charmbracelet/lipgloss"

// Color palette
var (
	// Primary colors
	Primary   = lipgloss.Color("#7D56F4") // Purple
	Secondary = lipgloss.Color("#3C3C3C") // Dark gray

	// Status colors
	Success = lipgloss.Color("#04B575") // Green
	Error   = lipgloss.Color("#FF6B6B") // Red
	Warning = lipgloss.Color("#FFCC00") // Yellow
	Info    = lipgloss.Color("#5384FF") // Blue

	// UI colors
	Foreground = lipgloss.Color("#FAFAFA")
	Muted      = lipgloss.Color("#6C6C6C")
	Background = lipgloss.Color("#1A1A1A")
	Border     = lipgloss.Color("#3C3C3C")
)

// Base styles
var (
	// TitleStyle for wizard/component titles
	TitleStyle = lipgloss.NewStyle().
			Foreground(Primary).
			Bold(true).
			Padding(0, 1)

	// SubtitleStyle for step names
	SubtitleStyle = lipgloss.NewStyle().
			Foreground(Foreground).
			Bold(true)

	// PromptStyle for questions
	PromptStyle = lipgloss.NewStyle().
			Foreground(Primary).
			Bold(true)

	// DescriptionStyle for help text
	DescriptionStyle = lipgloss.NewStyle().
				Foreground(Muted)

	// SelectedStyle for selected list items
	SelectedStyle = lipgloss.NewStyle().
			Foreground(Primary).
			Bold(true)

	// UnselectedStyle for unselected list items
	UnselectedStyle = lipgloss.NewStyle().
			Foreground(Foreground)

	// SuccessStyle for success messages
	SuccessStyle = lipgloss.NewStyle().
			Foreground(Success).
			Bold(true)

	// ErrorStyle for error messages
	ErrorStyle = lipgloss.NewStyle().
			Foreground(Error).
			Bold(true)

	// HelpStyle for keyboard hints
	HelpStyle = lipgloss.NewStyle().
			Foreground(Muted).
			Italic(true)

	// ProgressStyle for progress indicators
	ProgressStyle = lipgloss.NewStyle().
			Foreground(Primary)

	// ProgressDotActive for active progress dots
	ProgressDotActive = lipgloss.NewStyle().
				Foreground(Primary).
				SetString("●")

	// ProgressDotInactive for inactive progress dots
	ProgressDotInactive = lipgloss.NewStyle().
				Foreground(Muted).
				SetString("○")
)

// BoxStyle creates a bordered box
var BoxStyle = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(Border).
	Padding(1, 2)

// FocusedBorder for focused inputs
var FocusedBorder = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(Primary).
	Padding(0, 1)

// BlurredBorder for unfocused inputs
var BlurredBorder = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(Border).
	Padding(0, 1)
