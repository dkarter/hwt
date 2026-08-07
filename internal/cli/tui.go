package cli

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type inputMode int

const (
	modeNormal inputMode = iota
	modeInsert
)

var (
	colorAccent = lipgloss.Color("#A6E3A1")
	colorPink   = lipgloss.Color("#F578C6")
	colorPurple = lipgloss.Color("#8983FF")
	colorText   = lipgloss.Color("#E7E7F0")
	colorMuted  = lipgloss.Color("#777784")
	colorPanel  = lipgloss.Color("#29292E")
	colorInk    = lipgloss.Color("#17171A")

	labelStyle  = lipgloss.NewStyle().Bold(true).Foreground(colorPurple)
	detailStyle = lipgloss.NewStyle().Foreground(colorMuted)
	helpStyle   = lipgloss.NewStyle().Foreground(colorMuted)
	promptStyle = lipgloss.NewStyle().Bold(true).Foreground(colorPink)
	inputStyle  = lipgloss.NewStyle().Foreground(colorText)
)

type vimInput struct {
	value       []rune
	position    int
	offset      int
	width       int
	mode        inputMode
	deleting    bool
	escapeArmed bool
	cancel      bool
	placeholder string
}

func newVimInput(placeholder string) vimInput {
	return vimInput{width: 56, mode: modeInsert, placeholder: placeholder}
}

func (v *vimInput) setWidth(width int) {
	v.width = max(8, width)
	v.ensureVisible()
}

func (v *vimInput) setValue(value string) {
	v.value = []rune(value)
	v.position = min(v.position, len(v.value))
	v.ensureVisible()
}

func (v vimInput) valueString() string {
	return string(v.value)
}

func (v *vimInput) setCursor(position int) {
	v.position = max(0, min(position, len(v.value)))
	v.ensureVisible()
}

func (v *vimInput) setMode(mode inputMode) {
	v.mode = mode
	v.deleting = false
	v.ensureVisible()
}

func (v *vimInput) leaveInsertMode() {
	if v.position > 0 {
		v.position--
	}
	v.setMode(modeNormal)
}

func (v *vimInput) update(msg tea.KeyPressMsg) bool {
	before := v.valueString()
	key := msg.String()
	if key == "esc" {
		if v.escapeArmed {
			v.cancel = true
			return false
		}
		v.escapeArmed = true
	} else {
		v.escapeArmed = false
	}
	if v.mode == modeInsert {
		switch key {
		case "esc":
			v.leaveInsertMode()
		case "left":
			v.setCursor(v.position - 1)
		case "right":
			v.setCursor(v.position + 1)
		case "home", "ctrl+a":
			v.setCursor(0)
		case "end", "ctrl+e":
			v.setCursor(len(v.value))
		case "backspace":
			v.deleteBeforeCursor()
		case "delete", "ctrl+d":
			v.deleteAtCursor()
		case "ctrl+w":
			v.deleteWordBeforeCursor()
		default:
			if msg.Text != "" {
				v.insert([]rune(msg.Text))
			}
		}
	} else {
		if v.deleting {
			if key == "esc" {
				v.deleting = false
			} else {
				v.deleteWithMotion(key)
			}
			v.ensureVisible()
			return before != v.valueString()
		}
		last := max(0, len(v.value)-1)
		switch key {
		case "i":
			v.setMode(modeInsert)
		case "a":
			v.setCursor(min(v.position+1, len(v.value)))
			v.setMode(modeInsert)
		case "I":
			v.setCursor(v.firstNonblank())
			v.setMode(modeInsert)
		case "A":
			v.setCursor(len(v.value))
			v.setMode(modeInsert)
		case "w":
			v.setCursor(min(last, v.nextWordStart(false)))
		case "W":
			v.setCursor(min(last, v.nextWordStart(true)))
		case "b":
			v.setCursor(v.previousWordStart(false))
		case "B":
			v.setCursor(v.previousWordStart(true))
		case "e":
			v.setCursor(v.wordEnd(false))
		case "E":
			v.setCursor(v.wordEnd(true))
		case "h", "left":
			v.setCursor(v.position - 1)
		case "l", "right":
			v.setCursor(min(last, v.position+1))
		case "0":
			v.setCursor(0)
		case "$":
			v.setCursor(last)
		case "x":
			v.deleteAtCursor()
		case "s":
			v.substitute()
			v.setMode(modeInsert)
		case "d":
			v.deleting = true
		}
	}
	v.ensureVisible()
	return before != v.valueString()
}

func (v vimInput) firstNonblank() int {
	for index, character := range v.value {
		if !unicode.IsSpace(character) {
			return index
		}
	}
	return 0
}

func (v *vimInput) deleteWithMotion(motion string) {
	start, end := v.position, v.position
	switch motion {
	case "d":
		start, end = 0, len(v.value)
	case "h", "left":
		start = max(0, v.position-1)
		end = v.position
	case "l", "right":
		end = min(len(v.value), v.position+1)
	case "0":
		start = 0
	case "$":
		end = len(v.value)
	case "w":
		end = v.nextWordStart(false)
	case "W":
		end = v.nextWordStart(true)
	case "e":
		end = min(len(v.value), v.wordEnd(false)+1)
	case "E":
		end = min(len(v.value), v.wordEnd(true)+1)
	case "b":
		start = v.previousWordStart(false)
	case "B":
		start = v.previousWordStart(true)
	default:
		v.deleting = false
		return
	}
	if end < start {
		start, end = end, start
	}
	v.value = append(v.value[:start], v.value[end:]...)
	v.position = min(start, max(0, len(v.value)-1))
	v.deleting = false
}

func (v vimInput) nextWordStart(big bool) int {
	if len(v.value) == 0 {
		return 0
	}
	index := min(v.position, len(v.value)-1)
	kind := wordKind(v.value[index], big)
	if kind != 0 {
		for index < len(v.value) && wordKind(v.value[index], big) == kind {
			index++
		}
	}
	for index < len(v.value) && wordKind(v.value[index], big) == 0 {
		index++
	}
	return index
}

func (v vimInput) previousWordStart(big bool) int {
	if len(v.value) == 0 || v.position == 0 {
		return 0
	}
	index := min(v.position-1, len(v.value)-1)
	for index > 0 && wordKind(v.value[index], big) == 0 {
		index--
	}
	kind := wordKind(v.value[index], big)
	for index > 0 && wordKind(v.value[index-1], big) == kind {
		index--
	}
	return index
}

func (v vimInput) wordEnd(big bool) int {
	if len(v.value) == 0 {
		return 0
	}
	index := min(v.position, len(v.value)-1)
	kind := wordKind(v.value[index], big)
	if kind == 0 || index == len(v.value)-1 || wordKind(v.value[index+1], big) != kind {
		if index < len(v.value)-1 {
			index++
		}
		for index < len(v.value) && wordKind(v.value[index], big) == 0 {
			index++
		}
		if index >= len(v.value) {
			return len(v.value) - 1
		}
		kind = wordKind(v.value[index], big)
	}
	for index < len(v.value)-1 && wordKind(v.value[index+1], big) == kind {
		index++
	}
	return index
}

func wordKind(value rune, big bool) int {
	if unicode.IsSpace(value) {
		return 0
	}
	if big || unicode.IsLetter(value) || unicode.IsDigit(value) || value == '_' {
		return 1
	}
	return 2
}

func (v *vimInput) insert(value []rune) {
	updated := make([]rune, 0, len(v.value)+len(value))
	updated = append(updated, v.value[:v.position]...)
	updated = append(updated, value...)
	updated = append(updated, v.value[v.position:]...)
	v.value = updated
	v.position += len(value)
}

func (v *vimInput) deleteBeforeCursor() {
	if v.position == 0 {
		return
	}
	v.value = append(v.value[:v.position-1], v.value[v.position:]...)
	v.position--
}

func (v *vimInput) deleteAtCursor() {
	if v.position >= len(v.value) {
		return
	}
	v.value = append(v.value[:v.position], v.value[v.position+1:]...)
	if v.mode == modeNormal && v.position == len(v.value) && v.position > 0 {
		v.position--
	}
}

func (v *vimInput) substitute() {
	if v.position >= len(v.value) {
		return
	}
	position := v.position
	v.value = append(v.value[:position], v.value[position+1:]...)
	v.position = min(position, len(v.value))
}

func (v *vimInput) deleteWordBeforeCursor() {
	end := v.position
	for v.position > 0 && isWordSeparator(v.value[v.position-1]) {
		v.position--
	}
	for v.position > 0 && !isWordSeparator(v.value[v.position-1]) {
		v.position--
	}
	v.value = append(v.value[:v.position], v.value[end:]...)
}

func isWordSeparator(value rune) bool {
	return unicode.IsSpace(value) || value == '/' || value == '-' || value == '_'
}

func (v *vimInput) ensureVisible() {
	if v.position < v.offset {
		v.offset = v.position
	}
	for v.offset < v.position && lipgloss.Width(string(v.value[v.offset:v.position])) >= v.width {
		v.offset++
	}
	if v.offset > len(v.value) {
		v.offset = len(v.value)
	}
}

func (v vimInput) visibleValue() string {
	width := 0
	end := v.offset
	for end < len(v.value) {
		characterWidth := lipgloss.Width(string(v.value[end]))
		if width+characterWidth > v.width {
			break
		}
		width += characterWidth
		end++
	}
	return string(v.value[v.offset:end])
}

func (v vimInput) view() string {
	value := v.visibleValue()
	if value == "" && len(v.value) == 0 {
		value = lipgloss.NewStyle().Foreground(colorMuted).Render(v.placeholder)
	} else {
		value = inputStyle.Render(value)
	}
	return promptStyle.Render("> ") + value
}

func (v vimInput) modeView() string {
	if v.deleting {
		return lipgloss.NewStyle().Bold(true).Foreground(colorText).Background(lipgloss.Color("#D65D6E")).Padding(0, 1).Render("DELETE")
	}
	if v.mode == modeInsert {
		return lipgloss.NewStyle().Bold(true).Foreground(colorInk).Background(colorAccent).Padding(0, 1).Render("INSERT")
	}
	return lipgloss.NewStyle().Bold(true).Foreground(colorText).Background(colorPurple).Padding(0, 1).Render("NORMAL")
}

func (v vimInput) cursor(y int) *tea.Cursor {
	cursor := tea.NewCursor(lipgloss.Width("> ")+lipgloss.Width(string(v.value[v.offset:v.position])), y)
	cursor.Blink = true
	if v.mode == modeInsert {
		cursor.Shape = tea.CursorBar
		cursor.Color = colorPink
	} else {
		cursor.Shape = tea.CursorBlock
		cursor.Color = colorPurple
	}
	return cursor
}

type textPromptModel struct {
	label     string
	input     vimInput
	submitted bool
	canceled  bool
}

func newTextPromptModel(label, placeholder string) textPromptModel {
	return textPromptModel{label: label, input: newVimInput(placeholder)}
}

func (m textPromptModel) Init() tea.Cmd { return nil }

func (m textPromptModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	if size, ok := message.(tea.WindowSizeMsg); ok {
		m.input.setWidth(min(72, size.Width-4))
		return m, nil
	}
	msg, ok := message.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	key := msg.String()
	if m.input.deleting && key != "ctrl+c" {
		m.input.update(msg)
		if m.input.cancel {
			m.canceled = true
			return m, tea.Quit
		}
		return m, nil
	}
	if key == "ctrl+c" || (m.input.mode == modeNormal && key == "q") {
		m.canceled = true
		return m, tea.Quit
	}
	if key == "enter" {
		m.submitted = true
		return m, tea.Quit
	}
	m.input.update(msg)
	if m.input.cancel {
		m.canceled = true
		return m, tea.Quit
	}
	return m, nil
}

func (m textPromptModel) View() tea.View {
	content := strings.Join([]string{
		labelStyle.Render(m.label),
		m.input.view(),
		"",
		m.input.modeView() + "  " + helpStyle.Render("esc normal  |  esc esc cancel  |  enter submit"),
	}, "\n")
	view := tea.NewView(content)
	view.AltScreen = true
	view.Cursor = m.input.cursor(1)
	return view
}

type branchMatch struct {
	value  string
	label  string
	score  int
	order  int
	custom bool
}

type branchPickerModel struct {
	input       vimInput
	candidates  []string
	matches     []branchMatch
	selected    int
	visibleRows int
	result      string
	canceled    bool
}

func newBranchPickerModel(candidates []string) branchPickerModel {
	m := branchPickerModel{input: newVimInput("Filter branches..."), candidates: candidates, visibleRows: 8}
	m.refilter()
	return m
}

func (m branchPickerModel) Init() tea.Cmd { return nil }

func (m branchPickerModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	if size, ok := message.(tea.WindowSizeMsg); ok {
		m.input.setWidth(min(72, size.Width-4))
		m.visibleRows = max(2, min(8, size.Height-9))
		return m, nil
	}
	msg, ok := message.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	key := msg.String()
	if m.input.deleting && key != "ctrl+c" {
		if m.input.update(msg) {
			m.selected = 0
			m.refilter()
		}
		if m.input.cancel {
			m.canceled = true
			return m, tea.Quit
		}
		return m, nil
	}
	if key == "ctrl+c" || (m.input.mode == modeNormal && key == "q") {
		m.canceled = true
		return m, tea.Quit
	}
	if key == "enter" && len(m.matches) > 0 {
		m.result = m.matches[m.selected].value
		return m, tea.Quit
	}

	move := 0
	if key == "up" || key == "ctrl+p" || (m.input.mode == modeNormal && key == "k") {
		move = -1
	}
	if key == "down" || key == "ctrl+n" || (m.input.mode == modeNormal && key == "j") {
		move = 1
	}
	if move != 0 && len(m.matches) > 0 {
		m.selected = max(0, min(len(m.matches)-1, m.selected+move))
		return m, nil
	}
	if m.input.mode == modeNormal && key == "g" {
		m.selected = 0
		return m, nil
	}
	if m.input.mode == modeNormal && key == "G" && len(m.matches) > 0 {
		m.selected = len(m.matches) - 1
		return m, nil
	}
	if m.input.update(msg) {
		m.selected = 0
		m.refilter()
	}
	if m.input.cancel {
		m.canceled = true
		return m, tea.Quit
	}
	return m, nil
}

func (m *branchPickerModel) refilter() {
	query := strings.TrimSpace(m.input.valueString())
	m.matches = m.matches[:0]
	if query == "" {
		for index, candidate := range m.candidates {
			m.matches = append(m.matches, branchMatch{value: candidate, label: candidate, order: index})
		}
		return
	}
	for index, candidate := range m.candidates {
		score, ok := fuzzyScore(query, candidate)
		if ok {
			m.matches = append(m.matches, branchMatch{value: candidate, label: candidate, score: score, order: index})
		}
	}
	sort.SliceStable(m.matches, func(i, j int) bool {
		if m.matches[i].score == m.matches[j].score {
			return m.matches[i].order < m.matches[j].order
		}
		return m.matches[i].score > m.matches[j].score
	})
	m.matches = append(m.matches, branchMatch{value: query, label: fmt.Sprintf("use %q as a ref", query), custom: true})
}

func fuzzyScore(query, candidate string) (int, bool) {
	queryRunes := []rune(strings.ToLower(query))
	candidateRunes := []rune(strings.ToLower(candidate))
	queryIndex := 0
	lastMatch := -2
	score := 0
	for index, character := range candidateRunes {
		if queryIndex >= len(queryRunes) || character != queryRunes[queryIndex] {
			continue
		}
		score += 10
		if index == 0 || candidateRunes[index-1] == '/' || candidateRunes[index-1] == '-' || candidateRunes[index-1] == '_' {
			score += 8
		}
		if index == lastMatch+1 {
			score += 6
		}
		lastMatch = index
		queryIndex++
	}
	if queryIndex != len(queryRunes) {
		return 0, false
	}
	return score - len(candidateRunes), true
}

func (m branchPickerModel) View() tea.View {
	lines := []string{
		labelStyle.Render("Fuzzy search"),
		m.input.view(),
		"",
	}
	if len(m.matches) == 0 {
		lines = append(lines, detailStyle.Render("No branches found"))
	} else {
		rows := max(1, m.visibleRows)
		start := max(0, min(m.selected-rows/2, len(m.matches)-rows))
		end := min(len(m.matches), start+rows)
		for index := start; index < end; index++ {
			prefix := "  "
			style := lipgloss.NewStyle().Foreground(colorText)
			if m.matches[index].custom {
				style = style.Italic(true).Foreground(colorMuted)
			}
			if index == m.selected {
				prefix = "> "
				style = style.Bold(true).Foreground(colorPink)
			}
			lines = append(lines, style.Render(prefix+m.matches[index].label))
		}
	}
	lines = append(lines, "", m.input.modeView()+"  "+helpStyle.Render("j/k select  |  esc esc cancel  |  enter choose"))
	view := tea.NewView(strings.Join(lines, "\n"))
	view.AltScreen = true
	view.Cursor = m.input.cursor(1)
	return view
}

type confirmModel struct {
	question string
	detail   string
	width    int
	yes      bool
	result   bool
	done     bool
}

func (m confirmModel) Init() tea.Cmd { return nil }

func (m confirmModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	if size, ok := message.(tea.WindowSizeMsg); ok {
		m.width = size.Width
		return m, nil
	}
	msg, ok := message.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	switch msg.String() {
	case "ctrl+c", "esc", "q":
		m.done = true
		return m, tea.Quit
	case "h", "left":
		m.yes = true
	case "l", "right":
		m.yes = false
	case "tab", "shift+tab":
		m.yes = !m.yes
	case "y", "Y":
		m.result = true
		m.done = true
		return m, tea.Quit
	case "n", "N":
		m.done = true
		return m, tea.Quit
	case "enter":
		m.result = m.yes
		m.done = true
		return m, tea.Quit
	}
	return m, nil
}

func (m confirmModel) View() tea.View {
	optionWidth := 18
	if m.width > 0 {
		optionWidth = max(8, min(18, (m.width-6)/2))
	}
	selected := lipgloss.NewStyle().Bold(true).Foreground(colorInk).Background(colorPink).Align(lipgloss.Center).Width(optionWidth)
	unselected := lipgloss.NewStyle().Foreground(colorText).Background(colorPanel).Align(lipgloss.Center).Width(optionWidth)
	yesStyle, noStyle := unselected, selected
	if m.yes {
		yesStyle, noStyle = selected, unselected
	}
	options := lipgloss.JoinHorizontal(lipgloss.Top, yesStyle.Render("Yes"), "    ", noStyle.Render("No"))
	lines := []string{labelStyle.Render(m.question)}
	if m.detail != "" {
		style := detailStyle
		if m.width > 0 {
			style = style.Width(max(16, m.width-4))
		}
		lines = append(lines, style.Render(m.detail))
	}
	lines = append(lines, "", options, "", helpStyle.Render("h/l toggle  |  enter submit  |  y yes  |  n no"))
	view := tea.NewView(strings.Join(lines, "\n"))
	view.AltScreen = true
	return view
}

func runTextPrompt(input io.Reader, output io.Writer, label, placeholder string) (string, bool, error) {
	result, err := tea.NewProgram(newTextPromptModel(label, placeholder), tea.WithInput(input), tea.WithOutput(output)).Run()
	if err != nil {
		return "", false, err
	}
	model := result.(textPromptModel)
	return strings.TrimSpace(model.input.valueString()), model.submitted && !model.canceled, nil
}

func runBranchPicker(input io.Reader, output io.Writer, candidates []string) (string, bool, error) {
	result, err := tea.NewProgram(newBranchPickerModel(candidates), tea.WithInput(input), tea.WithOutput(output)).Run()
	if err != nil {
		return "", false, err
	}
	model := result.(branchPickerModel)
	return model.result, model.result != "" && !model.canceled, nil
}

func runConfirm(input io.Reader, output io.Writer, question, detail string) (bool, error) {
	result, err := tea.NewProgram(confirmModel{question: question, detail: detail}, tea.WithInput(input), tea.WithOutput(output)).Run()
	if err != nil {
		return false, err
	}
	model := result.(confirmModel)
	return model.done && model.result, nil
}
