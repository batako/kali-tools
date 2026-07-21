package ctx

import (
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-isatty"
)

type logDisplayMode int

const (
	logDisplayAuto logDisplayMode = iota
	logDisplayPlain
	logDisplayVerbose
	logDisplayInteractive
)

var (
	logStdin      io.Reader = os.Stdin
	logIsTerminal           = func(stdout io.Writer) bool {
		input, inputOK := logStdin.(*os.File)
		output, outputOK := stdout.(*os.File)
		return inputOK && outputOK &&
			isatty.IsTerminal(input.Fd()) &&
			isatty.IsTerminal(output.Fd())
	}
)

func writePlainTimeline(stdout io.Writer, entries []TimelineEntry) error {
	for _, entry := range entries {
		marker := " "
		switch {
		case !entry.IsCommand:
			marker = "#"
		case entry.Status == "failed":
			marker = "!"
		}
		if _, err := fmt.Fprintf(stdout, "%s  %s %s\n", compactTimelineTime(entry.Time), marker, oneLine(entry.Text)); err != nil {
			return err
		}
	}
	return nil
}

func writeVerboseTimeline(stdout io.Writer, entries []TimelineEntry) error {
	for _, entry := range entries {
		if entry.IsCommand {
			if _, err := fmt.Fprintf(stdout, "%s %s %s %d %s\n", entry.Ref, entry.Time, entry.Status, entry.ExitCode, entry.Text); err != nil {
				return err
			}
			continue
		}
		if _, err := fmt.Fprintf(stdout, "%s %s %s %s\n", entry.Ref, entry.Time, entry.Status, entry.Text); err != nil {
			return err
		}
	}
	return nil
}

func runLogTUI(workspace *Workspace, entries []TimelineEntry, stdout io.Writer) error {
	model := newLogModel(entries, func(id int64) (*CommandLog, error) {
		return GetCommandLog(workspace, fmt.Sprintf("%d", id))
	})
	program := tea.NewProgram(
		model,
		tea.WithInput(logStdin),
		tea.WithOutput(stdout),
		tea.WithAltScreen(),
	)
	_, err := program.Run()
	return err
}

type commandLogLoader func(id int64) (*CommandLog, error)

type logModel struct {
	entries      []TimelineEntry
	cursor       int
	offset       int
	width        int
	height       int
	detail       string
	detailOffset int
	loadCommand  commandLogLoader
	err          error
}

func newLogModel(entries []TimelineEntry, loader commandLogLoader) logModel {
	cursor := 0
	if len(entries) > 0 {
		cursor = len(entries) - 1
	}
	return logModel{
		entries:     entries,
		cursor:      cursor,
		width:       80,
		height:      24,
		loadCommand: loader,
	}
}

func (m logModel) Init() tea.Cmd {
	return nil
}

func (m logModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.keepCursorVisible()
	case tea.KeyMsg:
		key := msg.String()
		if m.detail != "" || m.err != nil {
			switch key {
			case "q", "esc", "backspace":
				m.detail = ""
				m.err = nil
				m.detailOffset = 0
			case "up", "k":
				if m.detailOffset > 0 {
					m.detailOffset--
				}
			case "down", "j":
				m.detailOffset++
			}
			return m, nil
		}

		switch key {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
			m.keepCursorVisible()
		case "down", "j":
			if m.cursor < len(m.entries)-1 {
				m.cursor++
			}
			m.keepCursorVisible()
		case "home", "g":
			m.cursor = 0
			m.keepCursorVisible()
		case "end", "G":
			if len(m.entries) > 0 {
				m.cursor = len(m.entries) - 1
			}
			m.keepCursorVisible()
		case "enter":
			m.openDetail()
		}
	}
	return m, nil
}

func (m logModel) View() string {
	if m.detail != "" || m.err != nil {
		return m.detailView()
	}

	var lines []string
	lines = append(lines, fitLine("ctx log", m.width))
	if len(m.entries) == 0 {
		lines = append(lines, "", "No timeline entries.")
	} else {
		end := min(len(m.entries), m.offset+m.listHeight())
		for i := m.offset; i < end; i++ {
			entry := m.entries[i]
			marker := " "
			switch {
			case !entry.IsCommand:
				marker = "#"
			case entry.Status == "failed":
				marker = "!"
			}
			cursor := "  "
			if i == m.cursor {
				cursor = "> "
			}
			line := fmt.Sprintf("%s%s %s %s", cursor, compactTimelineTime(entry.Time), marker, oneLine(entry.Text))
			lines = append(lines, fitLine(line, m.width))
		}
	}
	for len(lines) < max(1, m.height-1) {
		lines = append(lines, "")
	}
	lines = append(lines, fitLine("j/k move  enter details  q quit", m.width))
	return strings.Join(lines, "\n")
}

func (m *logModel) openDetail() {
	if len(m.entries) == 0 {
		return
	}
	entry := m.entries[m.cursor]
	m.detailOffset = 0
	if !entry.IsCommand {
		m.detail = fmt.Sprintf("note %s\n\ntime: %s\n\n%s", entry.Ref, entry.Time, entry.Text)
		return
	}
	if m.loadCommand == nil {
		m.err = fmt.Errorf("command log loader is unavailable")
		return
	}
	log, err := m.loadCommand(entry.ID)
	if err != nil {
		m.err = err
		return
	}
	m.detail = commandLogDetail(log)
}

func (m logModel) detailView() string {
	content := m.detail
	if m.err != nil {
		content = "error\n\n" + m.err.Error()
	}
	lines := strings.Split(content, "\n")
	available := max(1, m.height-1)
	maxOffset := max(0, len(lines)-available)
	offset := min(m.detailOffset, maxOffset)
	end := min(len(lines), offset+available)

	visible := make([]string, 0, available+1)
	for _, line := range lines[offset:end] {
		visible = append(visible, fitLine(line, m.width))
	}
	for len(visible) < available {
		visible = append(visible, "")
	}
	visible = append(visible, fitLine("j/k scroll  q back", m.width))
	return strings.Join(visible, "\n")
}

func (m *logModel) keepCursorVisible() {
	height := m.listHeight()
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+height {
		m.offset = m.cursor - height + 1
	}
	if m.offset < 0 {
		m.offset = 0
	}
}

func (m logModel) listHeight() int {
	return max(1, m.height-2)
}

func commandLogDetail(log *CommandLog) string {
	metadata := fmt.Sprintf(
		"command %d\n\ncommand: %s\nexpanded: %s\nstatus: %s\nexit code: %d\nstarted: %s\nended: %s\n",
		log.ID,
		log.Command,
		log.ExpandedCommand,
		log.Status,
		log.ExitCode,
		log.StartedAt,
		log.EndedAt,
	)
	if log.ParentID > 0 {
		metadata += fmt.Sprintf("parent: %d\n", log.ParentID)
	}
	if log.Phase != "" {
		metadata += fmt.Sprintf("phase: %s\n", log.Phase)
	}
	if log.Target != "" {
		metadata += fmt.Sprintf("target: %s\n", log.Target)
	}
	if len(log.Children) > 0 {
		metadata += "\nsteps:\n"
		for _, child := range log.Children {
			metadata += fmt.Sprintf("  [%s] %d %s", child.Status, child.ID, child.Command)
			if child.Phase != "" {
				metadata += fmt.Sprintf(" (%s)", child.Phase)
			}
			metadata += "\n"
		}
	}
	metadata += "\n"
	return metadata + commandOutputSections(log.Stdout, log.Stderr)
}

func commandOutputSections(stdout, stderr string) string {
	stdout = sanitizeTerminalOutput(stdout)
	stderr = sanitizeTerminalOutput(stderr)
	var output strings.Builder
	if stdout != "" {
		output.WriteString("---------------- stdout ----------------\n")
		output.WriteString(stdout)
		if !strings.HasSuffix(stdout, "\n") {
			output.WriteByte('\n')
		}
	}
	if stderr != "" {
		if output.Len() > 0 {
			output.WriteByte('\n')
		}
		output.WriteString("---------------- stderr ----------------\n")
		output.WriteString(stderr)
	}
	return output.String()
}

func sanitizeTerminalOutput(value string) string {
	output := make([]rune, 0, len(value))
	for i := 0; i < len(value); i++ {
		if value[i] != '\x1b' {
			switch value[i] {
			case '\r':
				if i+1 >= len(value) || value[i+1] != '\n' {
					output = append(output, '\n')
				}
			case '\b':
				if len(output) > 0 && output[len(output)-1] != '\n' {
					output = output[:len(output)-1]
				}
			case '\a':
				continue
			default:
				if value[i] < 0x20 && value[i] != '\n' && value[i] != '\t' {
					continue
				}
				r, size := utf8.DecodeRuneInString(value[i:])
				output = append(output, r)
				i += size - 1
			}
			continue
		}

		if i+1 >= len(value) {
			break
		}
		i++
		switch value[i] {
		case '[':
			sequenceStart := i + 1
			for i+1 < len(value) {
				i++
				if value[i] >= 0x40 && value[i] <= 0x7e {
					break
				}
			}
			if value[sequenceStart:i+1] == "?2004l" {
				for i+1 < len(value) && (value[i+1] == '\r' || value[i+1] == '\n') {
					i++
				}
			}
		case ']':
			for i+1 < len(value) {
				i++
				if value[i] == '\a' {
					break
				}
				if value[i] == '\x1b' && i+1 < len(value) && value[i+1] == '\\' {
					i++
					break
				}
			}
		}
		if i+1 < len(value) && value[i+1] == '\n' && len(output) > 0 && output[len(output)-1] == '\n' {
			i++
		}
	}
	return string(output)
}

func compactTimelineTime(value string) string {
	parsed, ok := timelineTime(value)
	if !ok {
		return value
	}
	return parsed.Local().Format("2006-01-02 15:04")
}

func oneLine(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func fitLine(value string, width int) string {
	if width < 1 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= width {
		return value
	}
	if width == 1 {
		return string(runes[:1])
	}
	return string(runes[:width-1]) + "~"
}
