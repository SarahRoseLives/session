package main

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"session/internal/session"
)

var errResumeCancelled = errors.New("resume cancelled")

type resumeSelectorModel struct {
	sessions  []session.Metadata
	cursor    int
	width     int
	height    int
	selected  *session.Metadata
	cancelled bool
}

func runResumeSelector(sessions []session.Metadata) (session.Metadata, error) {
	model := resumeSelectorModel{sessions: sessions}
	program := tea.NewProgram(model, tea.WithAltScreen())

	finalModel, err := program.Run()
	if err != nil {
		return session.Metadata{}, err
	}

	resumeModel, ok := finalModel.(resumeSelectorModel)
	if !ok {
		return session.Metadata{}, fmt.Errorf("unexpected resume selector model type %T", finalModel)
	}
	if resumeModel.cancelled || resumeModel.selected == nil {
		return session.Metadata{}, errResumeCancelled
	}

	return *resumeModel.selected, nil
}

func (m resumeSelectorModel) Init() tea.Cmd {
	return nil
}

func (m resumeSelectorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			m.cancelled = true
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.sessions)-1 {
				m.cursor++
			}
		case "enter":
			selected := m.sessions[m.cursor]
			m.selected = &selected
			return m, tea.Quit
		}
	}

	return m, nil
}

func (m resumeSelectorModel) View() string {
	if len(m.sessions) == 0 {
		return "\nNo resumable sessions.\n"
	}

	doc := newResumeStyles()
	viewportWidth := m.width
	if viewportWidth <= 0 {
		viewportWidth = 80
	}
	viewportHeight := m.height
	if viewportHeight <= 0 {
		viewportHeight = 24
	}

	contentWidth := max(16, viewportWidth-doc.frame.GetHorizontalFrameSize())
	contentHeight := max(6, viewportHeight-doc.frame.GetVerticalFrameSize())
	compact := contentWidth < 60 || contentHeight < 16

	var sections []string
	sections = append(sections, doc.title.MaxWidth(contentWidth).Render("session"))
	if !compact && contentHeight >= 10 {
		sections = append(sections, doc.subtitle.MaxWidth(contentWidth).Render("Resume a saved terminal session"))
	}

	var body string
	headerHeight := renderedHeight(strings.Join(sections, "\n"))
	helpHeight := 0
	helpText := "↑/↓ move • enter connect • q cancel"
	if contentWidth < 38 {
		helpText = "↑/↓ move • enter • q"
	}
	showHelp := contentHeight >= 8
	if showHelp {
		helpHeight = 1
	}
	bodyHeight := max(3, contentHeight-headerHeight-helpHeight)
	if !compact && len(sections) > 1 {
		bodyHeight--
	}
	if showHelp {
		bodyHeight--
	}

	if contentWidth < 96 {
		topOuterHeight, bottomOuterHeight := stackedPanelHeights(bodyHeight)
		panelInnerWidth := max(8, contentWidth-doc.panel.GetHorizontalFrameSize())
		detailInnerWidth := max(8, contentWidth-doc.detailPanel.GetHorizontalFrameSize())
		listInnerHeight := max(1, topOuterHeight-doc.panel.GetVerticalFrameSize())
		detailInnerHeight := max(1, bottomOuterHeight-doc.detailPanel.GetVerticalFrameSize())
		body = lipgloss.JoinVertical(
			lipgloss.Left,
			doc.panel.Width(panelInnerWidth).Height(listInnerHeight).Render(m.renderList(doc, panelInnerWidth, listInnerHeight)),
			doc.detailPanel.Width(detailInnerWidth).Height(detailInnerHeight).Render(m.renderDetails(doc, detailInnerWidth, detailInnerHeight)),
		)
	} else {
		listOuterWidth := max(28, contentWidth/2-1)
		detailOuterWidth := max(32, contentWidth-listOuterWidth-1)
		if listOuterWidth+detailOuterWidth+1 > contentWidth {
			detailOuterWidth = max(24, contentWidth-listOuterWidth-1)
		}
		listInnerWidth := max(8, listOuterWidth-doc.panel.GetHorizontalFrameSize())
		detailInnerWidth := max(8, detailOuterWidth-doc.detailPanel.GetHorizontalFrameSize())
		panelInnerHeight := max(1, bodyHeight-doc.panel.GetVerticalFrameSize())
		detailInnerHeight := max(1, bodyHeight-doc.detailPanel.GetVerticalFrameSize())
		body = lipgloss.JoinHorizontal(
			lipgloss.Top,
			doc.panel.Width(listInnerWidth).Height(panelInnerHeight).Render(m.renderList(doc, listInnerWidth, panelInnerHeight)),
			" ",
			doc.detailPanel.Width(detailInnerWidth).Height(detailInnerHeight).Render(m.renderDetails(doc, detailInnerWidth, detailInnerHeight)),
		)
	}

	sections = append(sections, body)
	if showHelp {
		sections = append(sections, doc.help.MaxWidth(contentWidth).Render(helpText))
	}

	content := strings.Join(sections, "\n")
	content = trimToHeight(content, contentHeight, contentWidth)
	return doc.frame.Width(contentWidth).MaxWidth(contentWidth).Height(contentHeight).Render(content)
}

func (m resumeSelectorModel) renderList(doc resumeStyles, width, height int) string {
	height = max(1, height)
	compactRows := height < 6
	rowsPerItem := 2
	if compactRows {
		rowsPerItem = 1
	}

	availableRows := max(1, height-1)
	maxItems := max(1, availableRows/rowsPerItem)
	start := visibleWindowStart(len(m.sessions), m.cursor, maxItems)
	end := minInt(len(m.sessions), start+maxItems)

	var rows []string
	rows = append(rows, doc.sectionTitle.MaxWidth(width).Render("Sessions"))
	for i := start; i < end; i++ {
		item := m.sessions[i]
		cursor := " "
		style := doc.item
		if i == m.cursor {
			cursor = "›"
			style = doc.selectedItem
		}

		badge := ""
		if i == 0 {
			badge = " " + doc.newestBadge.Render("NEWEST")
		}

		line := ellipsize(fmt.Sprintf("%s %s%s", cursor, sessionListTitle(item), badge), width-2)
		rowText := line
		if !compactRows {
			meta := ellipsize(
				fmt.Sprintf("%s • %s • %s", sessionRunningSummary(item), compactCWD(item), humanizeAge(time.Since(item.CreatedAt))),
				width-2,
			)
			rowText += "\n" + doc.muted.MaxWidth(width).Render(meta)
		}
		rows = append(rows, style.Width(width).Render(rowText))
	}
	if end < len(m.sessions) && len(rows) < height {
		rows = append(rows, doc.muted.MaxWidth(width).Render(fmt.Sprintf("… %d more", len(m.sessions)-end)))
	}

	return trimToHeight(strings.Join(rows, "\n"), height, width)
}

func (m resumeSelectorModel) renderDetails(doc resumeStyles, width, height int) string {
	item := m.sessions[m.cursor]
	lines := []string{doc.sectionTitle.MaxWidth(width).Render("Session details")}

	details := []string{
		renderDetailLine(doc, width, "Name", sessionNameDetail(item)),
		renderDetailLine(doc, width, "ID", item.ID),
		renderDetailLine(doc, width, "Status", sessionStatusLabel(item)),
		renderDetailLine(doc, width, "App", sessionRunningSummary(item)),
		renderDetailLine(doc, width, "Cwd", sessionCWD(item)),
		renderDetailLine(doc, width, "Started", item.CreatedAt.Local().Format("2006-01-02 15:04:05 MST")),
		renderDetailLine(doc, width, "Uptime", humanizeDuration(time.Since(item.CreatedAt))),
		renderDetailLine(doc, width, "Shell", item.Shell),
		renderDetailLine(doc, width, "Last", lastConnectedDetail(item)),
		renderDetailLine(doc, width, "Daemon", fmt.Sprintf("%d", item.DaemonPID)),
		renderDetailLine(doc, width, "Shell PID", fmt.Sprintf("%d", item.ShellPID)),
		renderDetailLine(doc, width, "Socket", item.SocketPath),
		renderDetailLine(doc, width, "Log", item.LogPath),
	}

	maxDetailLines := max(0, height-1)
	if maxDetailLines < len(details) {
		if maxDetailLines > 0 {
			lines = append(lines, details[:maxDetailLines-1]...)
			lines = append(lines, doc.muted.MaxWidth(width).Render("…"))
		}
	} else {
		lines = append(lines, details...)
	}

	return trimToHeight(strings.Join(lines, "\n"), height, width)
}

type resumeStyles struct {
	frame        lipgloss.Style
	title        lipgloss.Style
	subtitle     lipgloss.Style
	panel        lipgloss.Style
	detailPanel  lipgloss.Style
	item         lipgloss.Style
	selectedItem lipgloss.Style
	newestBadge  lipgloss.Style
	sectionTitle lipgloss.Style
	detailKey    lipgloss.Style
	detailValue  lipgloss.Style
	muted        lipgloss.Style
	help         lipgloss.Style
}

func newResumeStyles() resumeStyles {
	border := lipgloss.Color("#3B3F56")
	accent := lipgloss.Color("#7C3AED")
	highlight := lipgloss.Color("#22D3EE")
	muted := lipgloss.Color("#94A3B8")

	return resumeStyles{
		frame: lipgloss.NewStyle().
			Padding(0, 1),
		title: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#F8FAFC")).
			Background(accent).
			Padding(0, 1),
		subtitle: lipgloss.NewStyle().
			Foreground(muted),
		panel: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(border).
			Padding(0, 1),
		detailPanel: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(highlight).
			Padding(0, 1),
		item: lipgloss.NewStyle().
			Padding(0, 1),
		selectedItem: lipgloss.NewStyle().
			Padding(0, 1).
			Background(lipgloss.Color("#1E1B4B")).
			Foreground(lipgloss.Color("#E2E8F0")),
		newestBadge: lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#020617")).
			Background(lipgloss.Color("#F59E0B")).
			Padding(0, 1),
		sectionTitle: lipgloss.NewStyle().
			Bold(true).
			Foreground(highlight),
		detailKey: lipgloss.NewStyle().
			Foreground(muted),
		detailValue: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#E2E8F0")),
		muted: lipgloss.NewStyle().
			Foreground(muted),
		help: lipgloss.NewStyle().
			Foreground(muted),
	}
}

func humanizeDuration(duration time.Duration) string {
	if duration < 0 {
		duration = 0
	}

	days := int(duration / (24 * time.Hour))
	hours := int(duration%(24*time.Hour)) / int(time.Hour)
	minutes := int(duration%time.Hour) / int(time.Minute)

	switch {
	case days > 0:
		return fmt.Sprintf("%dd %dh", days, hours)
	case hours > 0:
		return fmt.Sprintf("%dh %dm", hours, minutes)
	case minutes > 0:
		return fmt.Sprintf("%dm", minutes)
	default:
		return "under a minute"
	}
}

func sessionStatusLabel(meta session.Metadata) string {
	if meta.ActiveClient {
		return "Connected elsewhere"
	}
	return "Ready to resume"
}

func lastConnectedDetail(meta session.Metadata) string {
	if meta.LastConnectedAt == nil {
		return "Never"
	}
	return fmt.Sprintf("%s (%s)", meta.LastConnectedAt.Local().Format("2006-01-02 15:04:05 MST"), humanizeAge(time.Since(*meta.LastConnectedAt)))
}

func sessionListTitle(meta session.Metadata) string {
	if meta.Name == "" {
		return meta.ID
	}
	return fmt.Sprintf("%s — %s", meta.Name, meta.ID)
}

func sessionNameDetail(meta session.Metadata) string {
	if meta.Name == "" {
		return "—"
	}
	return meta.Name
}

func compactCWD(meta session.Metadata) string {
	if meta.CurrentDir == "" {
		return "cwd unknown"
	}
	base := filepath.Base(meta.CurrentDir)
	if base == "." || base == string(filepath.Separator) {
		return meta.CurrentDir
	}
	return base
}

func renderDetailLine(doc resumeStyles, width int, label, value string) string {
	prefix := label + ": "
	remaining := max(1, width-lipgloss.Width(prefix))
	return doc.detailKey.Render(prefix) + doc.detailValue.Render(ellipsize(value, remaining))
}

func ellipsize(value string, width int) string {
	if width <= 0 {
		return ""
	}

	runes := []rune(value)
	if len(runes) <= width {
		return value
	}
	if width == 1 {
		return string(runes[:1])
	}

	return string(runes[:width-1]) + "…"
}

func trimToHeight(text string, height, width int) string {
	if height <= 0 {
		return ""
	}

	lines := strings.Split(text, "\n")
	if len(lines) <= height {
		return text
	}
	if height == 1 {
		return ellipsize(lines[0], width)
	}

	trimmed := append([]string(nil), lines[:height]...)
	trimmed[height-1] = ellipsize("…", width)
	return strings.Join(trimmed, "\n")
}

func renderedHeight(text string) int {
	if text == "" {
		return 0
	}
	return len(strings.Split(text, "\n"))
}

func visibleWindowStart(total, cursor, maxItems int) int {
	if total <= maxItems {
		return 0
	}

	start := cursor - maxItems/2
	if start < 0 {
		start = 0
	}
	maxStart := total - maxItems
	if start > maxStart {
		start = maxStart
	}
	return start
}

func stackedPanelHeights(total int) (int, int) {
	if total <= 4 {
		return total, 0
	}

	top := total / 2
	if top < 3 {
		top = 3
	}
	if top > total-3 {
		top = total - 3
	}
	bottom := total - top
	return top, bottom
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
