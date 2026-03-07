package tui

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/google/uuid"

	"github.com/kevindutra/crit/internal/document"
	"github.com/kevindutra/crit/internal/review"
)

type pane int

const (
	contentPane pane = iota
	commentPane
)

type modalType int

const (
	noModal modalType = iota
	commentModal
	editModal
)

// gutterWidth is the total width of the left gutter: line number (5) + marker (1) + space (1).
const gutterWidth = 7

type AppModel struct {
	width, height int
	focused       pane
	modal         modalType

	filePath string
	doc      *document.Document
	state    *review.ReviewState
	detached bool

	contentViewport viewport.Model
	commentViewport viewport.Model
	modalTextarea   textarea.Model

	cursorLine int // 1-based

	// Visual selection mode (like vim's V)
	selecting    bool
	selectAnchor int // the line where selection started

	// Sidebar annotation list and cursor
	sidebarItems  []sidebarItem
	sidebarCursor int

	// Content pane annotation focus: when moving through lines, cursor can
	// land on annotation boxes between lines. cursorOnAnnotation means the
	// cursor is on an annotation box after cursorLine, at index cursorAnnoIdx.
	cursorOnAnnotation bool
	cursorAnnoIdx      int

	// Editing state
	editingID  string // ID of the comment being edited
	modalFocus int    // 0=textarea, 1=save button, 2=cancel button, 3=delete button (edit modal only)

	err error
}

func NewApp(filePath string) AppModel {
	ta := textarea.New()
	ta.Placeholder = "Type your comment..."
	ta.ShowLineNumbers = false
	ta.CharLimit = 2000

	return AppModel{
		filePath:        filePath,
		detached:        os.Getenv("CRIT_DETACHED") == "1",
		contentViewport: viewport.New(),
		commentViewport: viewport.New(),
		modalTextarea:   ta,
		cursorLine:      1,
	}
}

func (m AppModel) Init() tea.Cmd {
	return tea.Batch(m.loadDocument(), tea.RequestBackgroundColor)
}

func (m AppModel) loadDocument() tea.Cmd {
	return func() tea.Msg {
		_, err := document.Load(m.filePath)
		if err != nil {
			return errMsg{err}
		}
		return docRenderedMsg{}
	}
}

// selectionRange returns the ordered start/end of the current selection.
// If not selecting, returns cursorLine, cursorLine.
func (m *AppModel) selectionRange() (int, int) {
	if !m.selecting {
		return m.cursorLine, m.cursorLine
	}
	start, end := m.selectAnchor, m.cursorLine
	if start > end {
		start, end = end, start
	}
	return start, end
}

func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.BackgroundColorMsg:
		initAdaptiveStyles(msg.IsDark())
		if m.state != nil {
			m.rebuildContent()
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.recalculateLayout()
		if m.state != nil {
			m.rebuildContent()
		}
		return m, nil

	case docRenderedMsg:
		// Start each review session fresh — discard any stale comments
		// from prior sessions so Claude only sees current feedback.
		state := &review.ReviewState{
			File:     m.filePath,
			Comments: []review.Comment{},
		}

		doc, _ := document.Load(m.filePath)
		m.doc = doc
		m.state = state

		m.rebuildContent()
		m.updateCommentSidebar()
		return m, nil

	case errMsg:
		m.err = msg.err
		return m, nil

	case tea.KeyPressMsg:
		return m.handleKeyPress(msg)
	}

	var cmd tea.Cmd
	if m.modal != noModal {
		m.modalTextarea, cmd = m.modalTextarea.Update(msg)
		return m, cmd
	}

	switch m.focused {
	case contentPane:
		m.contentViewport, cmd = m.contentViewport.Update(msg)
	case commentPane:
		m.commentViewport, cmd = m.commentViewport.Update(msg)
	}

	return m, cmd
}

func (m *AppModel) handleKeyPress(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.modal == commentModal || m.modal == editModal {
		return m.handleTextModal(msg)
	}

	switch {
	case key.Matches(msg, keys.Quit):
		// Auto-save on quit
		if m.state != nil {
			review.Save(m.state)
		}
		return m, tea.Quit

	case key.Matches(msg, keys.Cancel):
		// Esc cancels selection
		if m.selecting {
			m.selecting = false
			m.rebuildContent()
			return m, nil
		}
		return m, nil

	case key.Matches(msg, keys.Tab):
		if !m.selecting {
			if m.focused == contentPane {
				m.focused = commentPane
			} else {
				m.focused = contentPane
			}
			m.updateCommentSidebar()
			m.rebuildContent()
		}
		return m, nil

	case key.Matches(msg, keys.VisualMode):
		if m.focused == contentPane && m.doc != nil {
			if m.selecting {
				m.selecting = false
			} else {
				m.selecting = true
				m.selectAnchor = m.cursorLine
			}
			m.rebuildContent()
			return m, nil
		}
	}

	// Content pane cursor movement (annotation-aware)
	if m.focused == contentPane && m.doc != nil {
		moved := false
		switch {
		case key.Matches(msg, keys.Down):
			if m.cursorOnAnnotation {
				anns := m.annotationsAfterLine(m.cursorLine)
				if m.cursorAnnoIdx < len(anns)-1 {
					m.cursorAnnoIdx++
				} else {
					m.cursorOnAnnotation = false
					m.cursorAnnoIdx = 0
					if m.cursorLine < m.doc.LineCount() {
						m.cursorLine++
					}
				}
			} else {
				anns := m.annotationsAfterLine(m.cursorLine)
				if len(anns) > 0 {
					m.cursorOnAnnotation = true
					m.cursorAnnoIdx = 0
				} else if m.cursorLine < m.doc.LineCount() {
					m.cursorLine++
				}
			}
			moved = true
		case key.Matches(msg, keys.Up):
			if m.cursorOnAnnotation {
				if m.cursorAnnoIdx > 0 {
					m.cursorAnnoIdx--
				} else {
					m.cursorOnAnnotation = false
					m.cursorAnnoIdx = 0
				}
			} else {
				prevLine := m.cursorLine - 1
				if prevLine >= 1 {
					anns := m.annotationsAfterLine(prevLine)
					if len(anns) > 0 {
						m.cursorLine = prevLine
						m.cursorOnAnnotation = true
						m.cursorAnnoIdx = len(anns) - 1
					} else {
						m.cursorLine = prevLine
					}
				}
			}
			moved = true
		case key.Matches(msg, keys.HalfPageDown):
			m.cursorOnAnnotation = false
			m.cursorAnnoIdx = 0
			jump := m.contentViewport.Height() / 2
			m.cursorLine += jump
			if m.cursorLine > m.doc.LineCount() {
				m.cursorLine = m.doc.LineCount()
			}
			moved = true
		case key.Matches(msg, keys.HalfPageUp):
			m.cursorOnAnnotation = false
			m.cursorAnnoIdx = 0
			jump := m.contentViewport.Height() / 2
			m.cursorLine -= jump
			if m.cursorLine < 1 {
				m.cursorLine = 1
			}
			moved = true
		case key.Matches(msg, keys.Top):
			m.cursorOnAnnotation = false
			m.cursorAnnoIdx = 0
			m.cursorLine = 1
			moved = true
		case key.Matches(msg, keys.Bottom):
			m.cursorOnAnnotation = false
			m.cursorAnnoIdx = 0
			m.cursorLine = m.doc.LineCount()
			moved = true
		case key.Matches(msg, keys.NextComment):
			if m.state != nil && len(m.state.Comments) > 0 {
				type target struct {
					endLine int
					idx     int
				}
				var best *target
				for _, c := range m.state.Comments {
					endAt := c.Line
					if c.EndLine > 0 {
						endAt = c.EndLine
					}
					if endAt > m.cursorLine || (endAt == m.cursorLine && !m.cursorOnAnnotation) {
						if best == nil || endAt < best.endLine {
							best = &target{endLine: endAt, idx: 0}
						}
					}
				}
				if best == nil {
					for _, c := range m.state.Comments {
						endAt := c.Line
						if c.EndLine > 0 {
							endAt = c.EndLine
						}
						if best == nil || endAt < best.endLine {
							best = &target{endLine: endAt, idx: 0}
						}
					}
				}
				if best != nil {
					m.cursorLine = best.endLine
					m.cursorOnAnnotation = true
					m.cursorAnnoIdx = best.idx
					moved = true
				}
			}
		case key.Matches(msg, keys.PrevComment):
			if m.state != nil && len(m.state.Comments) > 0 {
				type target struct {
					endLine int
					idx     int
				}
				var best *target
				for _, c := range m.state.Comments {
					endAt := c.Line
					if c.EndLine > 0 {
						endAt = c.EndLine
					}
					if endAt < m.cursorLine || (endAt == m.cursorLine && !m.cursorOnAnnotation) {
						if best == nil || endAt > best.endLine {
							best = &target{endLine: endAt, idx: 0}
						}
					}
				}
				if best == nil {
					for _, c := range m.state.Comments {
						endAt := c.Line
						if c.EndLine > 0 {
							endAt = c.EndLine
						}
						if best == nil || endAt > best.endLine {
							best = &target{endLine: endAt, idx: 0}
						}
					}
				}
				if best != nil {
					m.cursorLine = best.endLine
					m.cursorOnAnnotation = true
					m.cursorAnnoIdx = best.idx
					moved = true
				}
			}
		case key.Matches(msg, keys.Confirm):
			if m.cursorOnAnnotation {
				anns := m.annotationsAfterLine(m.cursorLine)
				if m.cursorAnnoIdx < len(anns) {
					ann := anns[m.cursorAnnoIdx]
					m.editingID = ann.id
					m.modal = editModal
					m.modalFocus = 0
					m.modalTextarea.Reset()
					m.modalTextarea.SetValue(ann.body)
					m.modalTextarea.Placeholder = "Edit comment..."
					m.modalTextarea.Focus()
					return m, nil
				}
			} else if m.state != nil {
				m.modal = commentModal
				m.modalFocus = 0
				m.modalTextarea.Placeholder = "Type your comment..."
				m.modalTextarea.Reset()
				m.modalTextarea.Focus()
				return m, nil
			}
		}

		if moved {
			m.rebuildContent()
			m.scrollToCursor()
			return m, nil
		}
	}

	// Comment pane navigation
	if m.focused == commentPane && len(m.sidebarItems) > 0 {
		sidebarMoved := false
		switch {
		case key.Matches(msg, keys.Down):
			if m.sidebarCursor < len(m.sidebarItems)-1 {
				m.sidebarCursor++
				sidebarMoved = true
			}
		case key.Matches(msg, keys.Up):
			if m.sidebarCursor > 0 {
				m.sidebarCursor--
				sidebarMoved = true
			}
		case key.Matches(msg, keys.Top):
			m.sidebarCursor = 0
			sidebarMoved = true
		case key.Matches(msg, keys.Bottom):
			m.sidebarCursor = len(m.sidebarItems) - 1
			sidebarMoved = true
		}
		if sidebarMoved {
			m.updateCommentSidebar()
			m.rebuildContent()
			sel := m.sidebarItems[m.sidebarCursor]
			m.cursorLine = sel.line
			m.scrollToAnnotation(sel.line, sel.endLine)
			return m, nil
		}

		// Enter to edit selected annotation
		if key.Matches(msg, keys.Confirm) {
			sel := m.sidebarItems[m.sidebarCursor]
			m.editingID = sel.id
			m.modal = editModal
			m.modalFocus = 0
			m.modalTextarea.Reset()
			m.modalTextarea.SetValue(sel.body)
			m.modalTextarea.Placeholder = "Edit comment..."
			m.modalTextarea.Focus()
			return m, nil
		}
	}

	return m, nil
}

func (m *AppModel) modalSubmit() {
	body := strings.TrimSpace(m.modalTextarea.Value())
	if body == "" || m.state == nil {
		return
	}

	if m.modal == editModal {
		for i := range m.state.Comments {
			if m.state.Comments[i].ID == m.editingID {
				m.state.Comments[i].Body = body
				break
			}
		}
		m.editingID = ""
	} else {
		startLine, endLine := m.selectionRange()
		snippet := ""
		if m.doc != nil {
			snippet = strings.TrimSpace(m.doc.LineAt(startLine))
		}

		c := review.Comment{
			ID:             uuid.NewString()[:8],
			Line:           startLine,
			ContentSnippet: snippet,
			Body:           body,
			CreatedAt:      time.Now(),
		}
		if startLine != endLine {
			c.EndLine = endLine
		}
		m.state.AddComment(c)
	}

	review.Save(m.state)
	m.modal = noModal
	m.modalTextarea.Blur()
	m.selecting = false
	m.rebuildContent()
	m.updateCommentSidebar()
}

func (m *AppModel) modalDelete() {
	if m.state == nil || m.editingID == "" {
		return
	}
	m.state.DeleteComment(m.editingID)
	m.editingID = ""
	review.Save(m.state)
	m.modal = noModal
	m.modalTextarea.Blur()
	m.cursorOnAnnotation = false
	m.cursorAnnoIdx = 0
	m.rebuildContent()
	m.updateCommentSidebar()
}

func (m *AppModel) handleTextModal(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	// Number of focusable elements: edit modal has 4 (textarea, save, cancel, delete), comment modal has 3
	focusCount := 3
	if m.modal == editModal {
		focusCount = 4
	}

	switch msg.String() {
	case "esc":
		m.modal = noModal
		m.modalTextarea.Blur()
		return m, nil
	case "tab", "shift+tab":
		if msg.String() == "shift+tab" {
			m.modalFocus = (m.modalFocus + focusCount - 1) % focusCount
		} else {
			m.modalFocus = (m.modalFocus + 1) % focusCount
		}
		if m.modalFocus == 0 {
			m.modalTextarea.Focus()
		} else {
			m.modalTextarea.Blur()
		}
		return m, nil
	case "enter":
		if m.modalFocus == 1 {
			m.modalSubmit()
			return m, nil
		} else if m.modalFocus == 2 {
			m.modal = noModal
			m.modalTextarea.Blur()
			return m, nil
		} else if m.modalFocus == 3 && m.modal == editModal {
			m.modalDelete()
			return m, nil
		}
	case "ctrl+s":
		m.modalSubmit()
		return m, nil
	}

	if m.modalFocus == 0 {
		var cmd tea.Cmd
		m.modalTextarea, cmd = m.modalTextarea.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m *AppModel) recalculateLayout() {
	headerHeight := 1
	if m.detached {
		headerHeight = 2
	}
	footerHeight := 1
	tmuxPadding := 0
	if os.Getenv("TMUX") != "" {
		tmuxPadding = 1
	}
	mainHeight := m.height - headerHeight - footerHeight - 2 - tmuxPadding

	commentWidth := m.width / 4
	if commentWidth < 20 {
		commentWidth = 20
	}
	contentWidth := m.width - commentWidth

	m.contentViewport.SetWidth(contentWidth - 4)
	m.contentViewport.SetHeight(mainHeight)
	m.commentViewport.SetWidth(commentWidth - 4)
	m.commentViewport.SetHeight(mainHeight - 1) // -1 for the "Comments (N)" header line

	modalWidth := m.width * 2 / 3
	if modalWidth < 50 {
		modalWidth = 50
	}
	if modalWidth > m.width-4 {
		modalWidth = m.width - 4
	}
	m.modalTextarea.SetWidth(modalWidth - 10)
	m.modalTextarea.SetHeight(6)
}

// annotationsAfterLine returns annotations that render after the given line
// (keyed by their endLine).
func (m *AppModel) annotationsAfterLine(lineNum int) []annotation {
	if m.state == nil {
		return nil
	}
	var anns []annotation
	for _, c := range m.state.Comments {
		endAt := c.Line
		if c.EndLine > 0 {
			endAt = c.EndLine
		}
		if endAt == lineNum {
			anns = append(anns, annotation{
				id: c.ID, body: c.Body,
				line: c.Line, endLine: c.EndLine,
			})
		}
	}
	return anns
}

// sidebarItem represents a comment in the sidebar list.
type sidebarItem struct {
	id      string
	line    int
	endLine int
	body    string
}

// annotation represents an inline comment to render.
type annotation struct {
	id      string
	body    string
	line    int
	endLine int
}

// rebuildContent renders the document line-by-line with cursor, selection,
// line numbers, and bordered inline annotations.
func (m *AppModel) rebuildContent() {
	if m.doc == nil {
		return
	}

	// Collect annotations keyed by the line they appear AFTER
	annosByEndLine := make(map[int][]annotation)
	if m.state != nil {
		for _, c := range m.state.Comments {
			endAt := c.Line
			if c.EndLine > 0 {
				endAt = c.EndLine
			}
			annosByEndLine[endAt] = append(annosByEndLine[endAt], annotation{
				id: c.ID, body: c.Body,
				line: c.Line, endLine: c.EndLine,
			})
		}
	}

	// Count how many comments cover each line (for overlap detection)
	annotatedLines := make(map[int]int)
	if m.state != nil {
		for _, c := range m.state.Comments {
			end := c.Line
			if c.EndLine > 0 {
				end = c.EndLine
			}
			for l := c.Line; l <= end; l++ {
				annotatedLines[l]++
			}
		}
	}

	selStart, selEnd := m.selectionRange()

	// Determine which lines to highlight from selected annotation
	var sidebarHighlightStart, sidebarHighlightEnd int
	if m.focused == commentPane && len(m.sidebarItems) > 0 && m.sidebarCursor < len(m.sidebarItems) {
		sel := m.sidebarItems[m.sidebarCursor]
		sidebarHighlightStart = sel.line
		sidebarHighlightEnd = sel.line
		if sel.endLine > 0 {
			sidebarHighlightEnd = sel.endLine
		}
	} else if m.focused == contentPane && m.cursorOnAnnotation {
		anns := m.annotationsAfterLine(m.cursorLine)
		if m.cursorAnnoIdx < len(anns) {
			ann := anns[m.cursorAnnoIdx]
			sidebarHighlightStart = ann.line
			sidebarHighlightEnd = ann.line
			if ann.endLine > 0 {
				sidebarHighlightEnd = ann.endLine
			}
		}
	}

	contentWidth := m.contentViewport.Width()
	boxWidth := contentWidth - 7
	if boxWidth < 20 {
		boxWidth = 20
	}

	textWidth := contentWidth - 8
	if textWidth < 10 {
		textWidth = 10
	}

	// Detect table blocks so we can align columns across rows
	tableBlocks := detectTableBlocks(m.doc.Lines)
	tableBlockMap := make(map[int]*tableBlock)
	for i := range tableBlocks {
		tb := &tableBlocks[i]
		for l := tb.startLine; l <= tb.endLine; l++ {
			tableBlockMap[l] = tb
		}
	}

	var b strings.Builder
	for i, line := range m.doc.Lines {
		lineNum := i + 1

		isCursor := lineNum == m.cursorLine
		isSelected := m.selecting && lineNum >= selStart && lineNum <= selEnd
		isSidebarHighlight := sidebarHighlightStart > 0 && lineNum >= sidebarHighlightStart && lineNum <= sidebarHighlightEnd

		// Marker column
		var marker string
		if isCursor && !m.cursorOnAnnotation {
			marker = cursorMarker.Render(">")
		} else if isSelected {
			marker = selectedMarker.Render("|")
		} else if isSidebarHighlight {
			marker = cursorMarker.Render(">")
		} else if count, ok := annotatedLines[lineNum]; ok && count > 0 {
			if count > 1 {
				marker = gutterOverlap.Render("◆")
			} else {
				marker = annotationGutter.Render("■")
			}
		} else {
			marker = " "
		}

		// Line number
		var numStr string
		if isCursor {
			numStr = cursorLineNumStyle.Render(fmt.Sprintf("%d", lineNum))
		} else if isSelected {
			numStr = selectedLineNumStyle.Render(fmt.Sprintf("%d", lineNum))
		} else {
			numStr = lineNumStyle.Render(fmt.Sprintf("%d", lineNum))
		}

		// Check if this line is part of a table block
		if tb, inTable := tableBlockMap[lineNum]; inTable {
			var styledLine string
			if reTableSep.MatchString(line) {
				styledLine = formatTableSep(tb.colWidths)
			} else {
				isHeader := lineNum == tb.startLine
				styledLine = formatTableRow(line, tb.colWidths, isHeader)
			}

			if isSelected {
				styledLine = selectedLineBg.Render(styledLine)
			} else if isSidebarHighlight {
				styledLine = sidebarHighlightBg.Render(styledLine)
			}

			b.WriteString(fmt.Sprintf("%s%s %s\n", marker, numStr, styledLine))
		} else {
			lineContent := line

			styleFunc := func(s string) string { return highlightMarkdown(s) }
			if isSelected {
				styleFunc = func(s string) string { return selectedLineBg.Render(s) }
			} else if isSidebarHighlight {
				styleFunc = func(s string) string { return sidebarHighlightBg.Render(s) }
			}

			wrapped := lipgloss.Wrap(lineContent, textWidth, "")
			wrappedLines := strings.Split(wrapped, "\n")
			for wi, wl := range wrappedLines {
				if wi == 0 {
					b.WriteString(fmt.Sprintf("%s%s %s\n", marker, numStr, styleFunc(wl)))
				} else {
					b.WriteString(fmt.Sprintf(" %s %s\n", continuationGutter, styleFunc(wl)))
				}
			}
		}

		// Render inline annotations after this line
		if anns, ok := annosByEndLine[lineNum]; ok {
			for idx, ann := range anns {
				focused := m.focused == contentPane && m.cursorOnAnnotation && m.cursorLine == lineNum && m.cursorAnnoIdx == idx
				b.WriteString(m.renderAnnotationBox(ann, boxWidth, focused))
			}
		}
	}

	m.contentViewport.SetContent(b.String())
}

// renderAnnotationBox renders a bordered annotation box indented under the gutter.
func (m *AppModel) renderAnnotationBox(ann annotation, maxWidth int, focused bool) string {
	var lineLabel string
	if ann.endLine > 0 {
		lineLabel = fmt.Sprintf("L%d-%d", ann.line, ann.endLine)
	} else {
		lineLabel = fmt.Sprintf("L%d", ann.line)
	}

	var boxContent strings.Builder
	label := inlineLabelComment.Render("comment")
	lineRef := commentLineStyle.Render(lineLabel)
	boxContent.WriteString(fmt.Sprintf("%s %s\n", label, lineRef))
	boxContent.WriteString(clampLines(ann.body, 3))
	boxStyle := inlineCommentBox

	if focused {
		boxStyle = boxStyle.BorderForeground(warning)
	}
	box := boxStyle.Width(maxWidth).Render(boxContent.String())

	var prefix string
	if focused {
		cursor := lipgloss.NewStyle().Width(2).Render(cursorMarker.Render(">"))
		prefix = cursor + strings.Repeat(" ", gutterWidth-2)
	} else {
		prefix = strings.Repeat(" ", gutterWidth)
	}

	var b strings.Builder
	for _, line := range strings.Split(box, "\n") {
		b.WriteString(prefix + line + "\n")
	}
	return b.String()
}

var (
	reBold       = regexp.MustCompile(`\*\*(.+?)\*\*`)
	reItalic     = regexp.MustCompile(`(?:^|[^*])\*([^*]+?)\*(?:[^*]|$)`)
	reCode       = regexp.MustCompile("`([^`]+)`")
	reLink       = regexp.MustCompile(`\[([^\]]+)\]\([^)]+\)`)
	reListItem   = regexp.MustCompile(`^(\s*[-*+]\s)(.*)$`)
	reCheckbox   = regexp.MustCompile(`^(\s*[-*+]\s)\[([ xX])\]\s(.*)$`)
	reNumList    = regexp.MustCompile(`^(\s*\d+\.\s)(.*)$`)
	reBlockquote = regexp.MustCompile(`^(\s*>\s?)(.*)$`)
	reHr         = regexp.MustCompile(`^(\s*)([-*_]{3,})\s*$`)
	reTableRow   = regexp.MustCompile(`^\s*\|.*\|\s*$`)
	reTableSep   = regexp.MustCompile(`^\s*\|[\s:]*[-]+[\s:|-]*\|\s*$`)
)

// highlightMarkdown applies markdown syntax highlighting to a single line.
func highlightMarkdown(line string) string {
	trimmed := strings.TrimSpace(line)

	if reHr.MatchString(line) {
		return mdHrStyle.Render("─────────────────────────────────")
	}

	if strings.HasPrefix(trimmed, "#### ") {
		return mdH4Style.Render(line)
	}
	if strings.HasPrefix(trimmed, "### ") {
		return mdH3Style.Render(line)
	}
	if strings.HasPrefix(trimmed, "## ") {
		return mdH2Style.Render(line)
	}
	if strings.HasPrefix(trimmed, "# ") {
		return mdH1Style.Render(line)
	}

	if reTableSep.MatchString(line) {
		return mdTableSepStyle.Render(line)
	}
	if reTableRow.MatchString(line) {
		cells := strings.Split(line, "|")
		var parts []string
		for i, cell := range cells {
			if i == 0 || i == len(cells)-1 {
				parts = append(parts, cell)
			} else {
				parts = append(parts, highlightInline(cell))
			}
		}
		return strings.Join(parts, mdTablePipe.Render("|"))
	}

	if loc := reBlockquote.FindStringSubmatchIndex(line); loc != nil {
		rest := line[loc[4]:loc[5]]
		return mdBlockquoteBar.Render("▎") + " " + mdBlockquoteStyle.Render(rest)
	}

	if loc := reCheckbox.FindStringSubmatchIndex(line); loc != nil {
		indent := line[loc[2]:loc[3]]
		checked := line[loc[4]:loc[5]]
		rest := line[loc[6]:loc[7]]
		if checked == "x" || checked == "X" {
			return indent + mdCheckboxDone.Render("✓") + " " + mdCheckboxDoneText.Render(rest)
		}
		return indent + mdCheckboxOpen.Render("☐") + " " + highlightInline(rest)
	}

	if loc := reListItem.FindStringSubmatchIndex(line); loc != nil {
		indent := line[loc[2]:loc[3]]
		rest := line[loc[4]:loc[5]]
		return mdListMarkerStyle.Render(indent) + highlightInline(rest)
	}
	if loc := reNumList.FindStringSubmatchIndex(line); loc != nil {
		marker := line[loc[2]:loc[3]]
		rest := line[loc[4]:loc[5]]
		return mdListMarkerStyle.Render(marker) + highlightInline(rest)
	}

	return highlightInline(line)
}

// tableBlock represents a contiguous range of markdown table lines.
type tableBlock struct {
	startLine int
	endLine   int
	colWidths []int
}

func detectTableBlocks(lines []string) []tableBlock {
	var blocks []tableBlock
	inTable := false
	var current tableBlock

	for i, line := range lines {
		isTable := reTableRow.MatchString(line) || reTableSep.MatchString(line)
		if isTable {
			if !inTable {
				inTable = true
				current = tableBlock{startLine: i + 1}
			}
			current.endLine = i + 1

			if !reTableSep.MatchString(line) {
				cells := parseTableCells(line)
				for len(current.colWidths) < len(cells) {
					current.colWidths = append(current.colWidths, 0)
				}
				for ci, cell := range cells {
					if len(cell) > current.colWidths[ci] {
						current.colWidths[ci] = len(cell)
					}
				}
			}
		} else {
			if inTable {
				blocks = append(blocks, current)
				inTable = false
			}
		}
	}
	if inTable {
		blocks = append(blocks, current)
	}
	return blocks
}

func parseTableCells(line string) []string {
	line = strings.TrimSpace(line)
	line = strings.Trim(line, "|")
	parts := strings.Split(line, "|")
	cells := make([]string, len(parts))
	for i, p := range parts {
		cells[i] = strings.TrimSpace(p)
	}
	return cells
}

func formatTableRow(line string, colWidths []int, isHeader bool) string {
	cells := parseTableCells(line)
	pipe := mdTablePipe.Render("│")

	var parts []string
	for ci := 0; ci < len(colWidths); ci++ {
		w := colWidths[ci]
		cell := ""
		if ci < len(cells) {
			cell = cells[ci]
		}
		padded := lipgloss.NewStyle().Width(w).Render(cell)
		if isHeader {
			parts = append(parts, mdTableHeaderStyle.Render(" "+padded+" "))
		} else {
			parts = append(parts, mdTableCellStyle.Render(" "+padded+" "))
		}
	}

	return pipe + strings.Join(parts, pipe) + pipe
}

func formatTableSep(colWidths []int) string {
	pipe := mdTablePipe.Render("│")
	var parts []string
	for _, w := range colWidths {
		parts = append(parts, mdTableSepStyle.Render(strings.Repeat("─", w+2)))
	}
	return pipe + strings.Join(parts, mdTablePipe.Render("┼")) + pipe
}

func highlightInline(line string) string {
	line = reCode.ReplaceAllStringFunc(line, func(match string) string {
		inner := match[1 : len(match)-1]
		return mdCodeStyle.Render(" " + inner + " ")
	})

	line = reBold.ReplaceAllStringFunc(line, func(match string) string {
		inner := match[2 : len(match)-2]
		return mdBoldStyle.Render(inner)
	})

	line = reItalic.ReplaceAllStringFunc(line, func(match string) string {
		start := 0
		end := len(match)
		if match[0] != '*' {
			start = 1
		}
		if match[end-1] != '*' {
			end--
		}
		inner := match[start+1 : end-1]
		prefix := match[:start]
		suffix := match[end:]
		return prefix + mdItalicStyle.Render(inner) + suffix
	})

	line = reLink.ReplaceAllStringFunc(line, func(match string) string {
		idx := strings.Index(match, "](")
		if idx < 0 {
			return match
		}
		text := match[1:idx]
		return mdLinkStyle.Render(text)
	})

	return line
}

func (m *AppModel) scrollToCursor() {
	if m.doc == nil {
		return
	}

	renderedLine := 0
	extraCounts := m.extraLinesPerDocLine()
	for i := 1; i < m.cursorLine; i++ {
		renderedLine++
		renderedLine += extraCounts[i]
	}

	cursorBottom := renderedLine + 1 + extraCounts[m.cursorLine]

	vpHeight := m.contentViewport.Height()
	currentTop := m.contentViewport.YOffset()

	if renderedLine < currentTop {
		m.contentViewport.SetYOffset(renderedLine)
	}
	if cursorBottom > currentTop+vpHeight {
		m.contentViewport.SetYOffset(cursorBottom - vpHeight)
	}
}

func (m *AppModel) scrollToAnnotation(startLine, endLine int) {
	if m.doc == nil {
		return
	}
	if endLine == 0 {
		endLine = startLine
	}

	extraCounts := m.extraLinesPerDocLine()

	startRendered := 0
	for i := 1; i < startLine; i++ {
		startRendered++
		startRendered += extraCounts[i]
	}

	endRendered := 0
	for i := 1; i <= endLine; i++ {
		endRendered++
		endRendered += extraCounts[i]
	}

	vpHeight := m.contentViewport.Height()

	offset := endRendered - vpHeight
	if offset < 0 {
		offset = 0
	}
	if offset > startRendered {
		offset = startRendered
	}

	m.contentViewport.SetYOffset(offset)
}

func (m *AppModel) extraLinesPerDocLine() map[int]int {
	counts := make(map[int]int)
	if m.doc == nil {
		return counts
	}

	contentWidth := m.contentViewport.Width()
	textWidth := contentWidth - 8
	if textWidth < 10 {
		textWidth = 10
	}

	for i, line := range m.doc.Lines {
		lineNum := i + 1
		wrapped := lipgloss.Wrap(line, textWidth, "")
		wrapCount := strings.Count(wrapped, "\n")
		if wrapCount > 0 {
			counts[lineNum] += wrapCount
		}
	}

	if m.state != nil {
		for _, c := range m.state.Comments {
			endAt := c.Line
			if c.EndLine > 0 {
				endAt = c.EndLine
			}
			bodyLines := strings.Count(c.Body, "\n") + 1
			counts[endAt] += bodyLines + 3
		}
	}
	return counts
}

func (m *AppModel) updateCommentSidebar() {
	if m.state == nil {
		return
	}

	m.sidebarItems = nil
	for _, c := range m.state.Comments {
		m.sidebarItems = append(m.sidebarItems, sidebarItem{
			id: c.ID, line: c.Line, endLine: c.EndLine,
			body: c.Body,
		})
	}
	sort.Slice(m.sidebarItems, func(i, j int) bool { return m.sidebarItems[i].line < m.sidebarItems[j].line })

	if m.sidebarCursor >= len(m.sidebarItems) {
		m.sidebarCursor = len(m.sidebarItems) - 1
	}
	if m.sidebarCursor < 0 {
		m.sidebarCursor = 0
	}

	var b strings.Builder

	if len(m.sidebarItems) == 0 {
		b.WriteString(commentStyle.Render("No comments yet.\n\nPress enter to comment.\n\nUse 'v' to select\nmultiple lines first."))
		m.commentViewport.SetContent(b.String())
		return
	}

	for idx, it := range m.sidebarItems {
		isSelected := m.focused == commentPane && idx == m.sidebarCursor

		var lineInfo string
		if it.endLine > 0 {
			lineInfo = fmt.Sprintf("L%d-%d", it.line, it.endLine)
		} else {
			lineInfo = fmt.Sprintf("L%d", it.line)
		}
		lineInfo = commentLineStyle.Render(lineInfo)

		cursorCol := lipgloss.NewStyle().Width(2)
		prefix := cursorCol.Render("")
		if isSelected {
			prefix = cursorCol.Render(cursorMarker.Render(">"))
		}

		b.WriteString(fmt.Sprintf("%s%s\n", prefix, lineInfo))

		clamped := clampLines(it.body, 3)
		bodyLines := strings.Split(clamped, "\n")
		for i, bl := range bodyLines {
			styled := bl
			if isSelected {
				styled = sidebarSelectedText.Render(bl)
			} else {
				styled = commentStyle.Render(bl)
			}
			b.WriteString(" " + styled)
			if i < len(bodyLines)-1 {
				b.WriteString("\n")
			}
		}
		b.WriteString("\n\n")
	}

	m.commentViewport.SetContent(b.String())
}

func (m AppModel) View() tea.View {
	if m.err != nil {
		v := tea.NewView(fmt.Sprintf("Error: %v\n\nPress q to quit.", m.err))
		v.AltScreen = true
		return v
	}

	if m.width == 0 || m.state == nil {
		v := tea.NewView("Loading...")
		v.AltScreen = true
		return v
	}

	// Header
	commentCount := len(m.state.Comments)
	var headerContent string
	if m.selecting {
		start, end := m.selectionRange()
		selLabel := visualModeIndicator.Render("VISUAL")
		headerContent = fmt.Sprintf(" Crit: %s  %s L%d-%d", m.filePath, selLabel, start, end)
	} else {
		headerContent = fmt.Sprintf(" Crit: %s  %d comments  L%d/%d", m.filePath, commentCount, m.cursorLine, m.doc.LineCount())
	}
	var header string
	if m.detached {
		claudeBanner := claudeStatusBar.Width(m.width).Render(" Claude Code is paused — review the document, then press q to submit")
		header = claudeBanner + "\n" + headerStyle.Width(m.width).Render(headerContent)
	} else {
		header = headerStyle.Width(m.width).Render(headerContent)
	}

	// Content pane
	contentBorder := blurredBorder
	if m.focused == contentPane {
		contentBorder = focusedBorder
	}

	commentWidth := m.width / 4
	if commentWidth < 20 {
		commentWidth = 20
	}
	contentWidth := m.width - commentWidth

	panelHeight := m.contentViewport.Height() + 2

	contentBox := contentBorder.
		Width(contentWidth - 2).
		Height(panelHeight).
		Render(m.contentViewport.View())

	// Comment sidebar
	commentBorder := blurredBorder
	if m.focused == commentPane {
		commentBorder = focusedBorder
	}

	commentHeader := lipgloss.NewStyle().Bold(true).Foreground(accent).Render(fmt.Sprintf("Comments (%d)", commentCount))
	commentBox := commentBorder.
		Width(commentWidth - 2).
		Height(panelHeight).
		Render(commentHeader + "\n" + m.commentViewport.View())

	mainRow := lipgloss.JoinHorizontal(lipgloss.Top, contentBox, commentBox)

	footer := m.renderFooter()

	full := lipgloss.JoinVertical(lipgloss.Left, header, mainRow, footer)

	if m.modal != noModal {
		full = m.renderWithModal(full)
	}

	v := tea.NewView(full)
	v.AltScreen = true
	return v
}

func (m AppModel) renderFooter() string {
	k := func(key, desc string) string {
		return footerKeyStyle.Render(key) + " " + footerStyle.Render(desc)
	}

	var items []string
	if m.selecting {
		items = []string{
			k("j/k", "extend"),
			k("enter", "comment selection"),
			k("esc", "cancel"),
			k("v", "toggle select"),
		}
	} else {
		items = []string{
			k("j/k", "move"),
			k("[/]", "prev/next comment"),
			k("shift+↑↓", "page"),
			k("tab", "pane"),
			k("v", "select"),
			k("enter", "comment"),
			k("q", "save & quit"),
		}
	}

	return footerStyle.Width(m.width).Render(strings.Join(items, "  "))
}

func (m AppModel) renderModalButton(label, hint string, focused bool) string {
	btn := modalBtnLabel.Render(label)
	h := modalBtnHint.Render(hint)
	content := btn + " " + h
	if focused {
		return modalBtnFocused.Render(content)
	}
	return modalBtnNormal.Render(content)
}

func (m AppModel) renderDeleteButton(label string, focused bool) string {
	if focused {
		return modalDeleteBtnFocused.Render(label)
	}
	return modalBtnNormal.Render(modalDeleteBtnLabel.Render(label))
}

func (m AppModel) renderContextPreview(start, end, maxWidth int) string {
	if m.doc == nil {
		return ""
	}
	var lines []string
	maxLineText := maxWidth - 7
	if maxLineText < 10 {
		maxLineText = 10
	}
	for i := start; i <= end && i <= m.doc.LineCount(); i++ {
		lineText := m.doc.LineAt(i)
		wrapped := lipgloss.Wrap(lineText, maxLineText, "")
		num := lineNumStyle.Render(fmt.Sprintf("%d", i))
		wrapLines := strings.Split(wrapped, "\n")
		for wi, wl := range wrapLines {
			if wi == 0 {
				lines = append(lines, num+" "+wl)
			} else {
				lines = append(lines, lipgloss.NewStyle().Width(6).Render("")+wl)
			}
		}
	}
	if len(lines) > 8 {
		lines = append(lines[:7], footerStyle.Render(fmt.Sprintf("  ... +%d more lines", len(lines)-7)))
	}
	return strings.Join(lines, "\n")
}

func (m AppModel) renderWithModal(background string) string {
	var modalContent string
	modalWidth := m.width * 2 / 3
	if modalWidth < 50 {
		modalWidth = 50
	}
	if modalWidth > m.width-4 {
		modalWidth = m.width - 4
	}
	innerWidth := modalWidth - 6

	switch m.modal {
	case commentModal:
		start, end := m.selectionRange()
		var title string
		if start != end {
			title = modalTitleStyle.Render(fmt.Sprintf("Add Comment (lines %d-%d)", start, end))
		} else {
			title = modalTitleStyle.Render(fmt.Sprintf("Add Comment (line %d)", start))
		}
		contextBox := contextBoxStyle.
			Width(innerWidth - 2).
			Render(m.renderContextPreview(start, end, innerWidth-4))

		saveBtn := m.renderModalButton("Save", "ctrl+s", m.modalFocus == 1)
		cancelBtn := m.renderModalButton("Cancel", "esc", m.modalFocus == 2)
		buttons := lipgloss.JoinHorizontal(lipgloss.Center, saveBtn, "  ", cancelBtn)

		modalContent = modalStyle.Width(modalWidth).Render(
			title + "\n" + contextBox + "\n\n" + m.modalTextarea.View() + "\n\n" + buttons)

	case editModal:
		title := modalTitleStyle.Render("Edit Comment")
		var contextSection string
		for _, c := range m.state.Comments {
			if c.ID == m.editingID {
				start := c.Line
				end := c.EndLine
				if end == 0 {
					end = start
				}
				contextSection = contextBoxStyle.
					Width(innerWidth - 2).
					Render(m.renderContextPreview(start, end, innerWidth-4))
				break
			}
		}
		saveBtn := m.renderModalButton("Save", "ctrl+s", m.modalFocus == 1)
		cancelBtn := m.renderModalButton("Cancel", "esc", m.modalFocus == 2)
		deleteBtn := m.renderDeleteButton("Delete", m.modalFocus == 3)
		buttons := lipgloss.JoinHorizontal(lipgloss.Center, saveBtn, "  ", cancelBtn, "  ", deleteBtn)

		content := title + "\n"
		if contextSection != "" {
			content += contextSection + "\n\n"
		}
		content += m.modalTextarea.View() + "\n\n" + buttons
		modalContent = modalStyle.Width(modalWidth).Render(content)
	}

	bgW := lipgloss.Width(background)
	bgH := lipgloss.Height(background)

	modalW := lipgloss.Width(modalContent)
	modalH := lipgloss.Height(modalContent)

	mx := (bgW - modalW) / 2
	my := (bgH - modalH) / 2
	if mx < 0 {
		mx = 0
	}
	if my < 0 {
		my = 0
	}

	background = dimRendered(background, bgW, bgH)

	bgLayer := lipgloss.NewLayer(background)
	modalLayer := lipgloss.NewLayer(modalContent).X(mx).Y(my).Z(1)

	comp := lipgloss.NewCompositor(bgLayer, modalLayer)
	return comp.Render()
}

func dimRendered(s string, w, h int) string {
	canvas := lipgloss.NewCanvas(w, h)
	canvas.Compose(lipgloss.NewLayer(s))

	dim := lipgloss.Color("#555555")
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			cell := canvas.CellAt(x, y)
			if cell != nil {
				cell.Style.Fg = dim
			}
		}
	}
	return canvas.Render()
}

// clampLines truncates text to maxLines and appends "…" if truncated.
func clampLines(text string, maxLines int) string {
	lines := strings.Split(text, "\n")
	if len(lines) <= maxLines {
		return text
	}
	return strings.Join(lines[:maxLines], "\n") + "…"
}
