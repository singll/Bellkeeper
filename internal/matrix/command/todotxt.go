package command

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// todo.txt format converter for Memos integration

// TodoItem represents a parsed todo item
type TodoItem struct {
	ID         int
	Content    string
	Priority   string // A-Z or empty
	Done       bool
	CreatedAt  string // YYYY-MM-DD
	CompletedAt string // YYYY-MM-DD
	Projects   []string // +project
	Contexts   []string // @context
	DueDate    string // due:YYYY-MM-DD
	RawContent string
}

// Parse todo.txt format
// Examples:
//   (A) 完成报告 +工作 @办公室 due:2026-04-20
//   x 2026-04-20 2026-04-19 完成报告 +工作
func ParseTodoTxt(line string) *TodoItem {
	item := &TodoItem{RawContent: line}

	// Skip empty lines
	if strings.TrimSpace(line) == "" {
		return nil
	}

	line = strings.TrimSpace(line)

	// Check if done
	if strings.HasPrefix(line, "x ") {
		item.Done = true
		line = strings.TrimPrefix(line, "x ")
	}

	// Parse completed date (first YYYY-MM-DD after x)
	if item.Done {
		if date := extractDate(line); date != "" {
			item.CompletedAt = date
			line = strings.TrimPrefix(line, date+" ")
		}
	}

	// Parse priority (A) to (Z)
	priorityRegex := regexp.MustCompile(`^\(([A-Z])\)\s*`)
	if matches := priorityRegex.FindStringSubmatch(line); len(matches) > 0 {
		item.Priority = matches[1]
		line = priorityRegex.ReplaceAllString(line, "")
	}

	// Parse creation date if present (and not done)
	if !item.Done {
		if date := extractDate(line); date != "" {
			item.CreatedAt = date
			line = strings.TrimPrefix(line, date+" ")
		}
	}

	// Extract due date: due:YYYY-MM-DD
	dueRegex := regexp.MustCompile(`due:(\d{4}-\d{2}-\d{2})`)
	if matches := dueRegex.FindStringSubmatch(line); len(matches) > 0 {
		item.DueDate = matches[1]
		line = dueRegex.ReplaceAllString(line, "")
	}

	// Extract projects +project
	projectRegex := regexp.MustCompile(`\+(\S+)`)
	for _, m := range projectRegex.FindAllStringSubmatch(line, -1) {
		item.Projects = append(item.Projects, m[1])
	}
	line = projectRegex.ReplaceAllString(line, "")

	// Extract contexts @context
	contextRegex := regexp.MustCompile(`@(\S+)`)
	for _, m := range contextRegex.FindAllStringSubmatch(line, -1) {
		item.Contexts = append(item.Contexts, m[1])
	}
	line = contextRegex.ReplaceAllString(line, "")

	// Remaining is the content
	item.Content = strings.TrimSpace(line)

	return item
}

// Convert to todo.txt format for display
func (t *TodoItem) ToTodoTxt() string {
	var parts []string

	if t.Done {
		parts = append(parts, "x")
		if t.CompletedAt != "" {
			parts = append(parts, t.CompletedAt)
		}
		if t.CreatedAt != "" {
			parts = append(parts, t.CreatedAt)
		}
	} else {
		if t.Priority != "" {
			parts = append(parts, "("+t.Priority+")")
		}
		if t.CreatedAt != "" {
			parts = append(parts, t.CreatedAt)
		}
	}

	parts = append(parts, t.Content)

	// Add projects
	for _, p := range t.Projects {
		parts = append(parts, "+"+p)
	}

	// Add contexts
	for _, c := range t.Contexts {
		parts = append(parts, "@"+c)
	}

	// Add due date
	if t.DueDate != "" {
		parts = append(parts, "due:"+t.DueDate)
	}

	return strings.Join(parts, " ")
}

// Convert Memos content to todo.txt format
func MemosToTodoTxt(content string) string {
	item := &TodoItem{}

	// Remove #待办 tag if present
	content = strings.ReplaceAll(content, "#待办", "")
	content = strings.ReplaceAll(content, "#todo", "")

	// Parse priority: #P1, #P2, #P3
	priorityRegex := regexp.MustCompile(`#P([1-3])\b`)
	if matches := priorityRegex.FindStringSubmatch(content); len(matches) > 0 {
		switch matches[1] {
		case "1":
			item.Priority = "A"
		case "2":
			item.Priority = "B"
		case "3":
			item.Priority = "C"
		}
		content = priorityRegex.ReplaceAllString(content, "")
	}

	// Parse due date: #D:YYYY-MM-DD or #D:MM/DD
	dueRegex := regexp.MustCompile(`#D:(\d{4}-\d{2}-\d{2})\b`)
	if matches := dueRegex.FindStringSubmatch(content); len(matches) > 0 {
		item.DueDate = matches[1]
		content = dueRegex.ReplaceAllString(content, "")
	}

	// Short date format: #D:MM/DD
	shortDueRegex := regexp.MustCompile(`#D:(\d{1,2})/(\d{1,2})\b`)
	if matches := shortDueRegex.FindStringSubmatch(content); len(matches) > 0 {
		month, _ := strconv.Atoi(matches[1])
		day, _ := strconv.Atoi(matches[2])
		item.DueDate = "2026-" + zeroPad(month) + "-" + zeroPad(day)
		content = shortDueRegex.ReplaceAllString(content, "")
	}

	// Parse projects: #+project or #项目名
	projectRegex := regexp.MustCompile(`#\+(\S+)`)
	for _, m := range projectRegex.FindAllStringSubmatch(content, -1) {
		item.Projects = append(item.Projects, m[1])
	}
	content = projectRegex.ReplaceAllString(content, "")

	// Parse contexts: #@context
	contextRegex := regexp.MustCompile(`#@(\S+)`)
	for _, m := range contextRegex.FindAllStringSubmatch(content, -1) {
		item.Contexts = append(item.Contexts, m[1])
	}
	content = contextRegex.ReplaceAllString(content, "")

	// Clean remaining content
	content = strings.TrimSpace(content)
	content = regexp.MustCompile(`#\S+`).ReplaceAllString(content, "")
	content = strings.TrimSpace(content)

	item.Content = content

	return item.ToTodoTxt()
}

// Convert todo.txt format back to Memos format
func TodoTxtToMemos(line string) string {
	item := ParseTodoTxt(line)
	if item == nil {
		return ""
	}

	var parts []string

	// Add todo tag
	parts = append(parts, "#待办")

	// Add priority
	if item.Priority != "" {
		var p string
		switch item.Priority {
		case "A":
			p = "1"
		case "B":
			p = "2"
		case "C":
			p = "3"
		default:
			p = "3"
		}
		parts = append(parts, "#P"+p)
	}

	// Add due date
	if item.DueDate != "" {
		parts = append(parts, "#D:"+item.DueDate)
	}

	// Add projects
	for _, p := range item.Projects {
		parts = append(parts, "#+"+p)
	}

	// Add contexts
	for _, c := range item.Contexts {
		parts = append(parts, "#@"+c)
	}

	// Add content
	parts = append(parts, item.Content)

	return strings.Join(parts, " ")
}

// Parse command input to extract todo parameters
// Input examples:
//   "完成报告"
//   "P1 完成报告"
//   "P2 D4/20 完成报告"
//   "P1 +工作 @办公室 完成报告"
//   "D4/20 +项目 完成"
type CommandParams struct {
	Content   string
	Priority  string // A, B, C or empty
	DueDate   string // YYYY-MM-DD
	Project   string
	Context   string
}

func ParseCommandInput(input string) *CommandParams {
	params := &CommandParams{}
	input = strings.TrimSpace(input)

	// Parse priority: P1, P2, P3
	priorityRegex := regexp.MustCompile(`^(P[1-3])\s*`)
	if matches := priorityRegex.FindStringSubmatch(input); len(matches) > 0 {
		switch matches[1] {
		case "P1":
			params.Priority = "A"
		case "P2":
			params.Priority = "B"
		case "P3":
			params.Priority = "C"
		}
		input = priorityRegex.ReplaceAllString(input, "")
	}

	// Parse short date: D4/20 or D04/20
	shortDateRegex := regexp.MustCompile(`^(D(\d{1,2})/(\d{1,2}))\s*`)
	if matches := shortDateRegex.FindStringSubmatch(input); len(matches) > 0 {
		month, _ := strconv.Atoi(matches[2])
		day, _ := strconv.Atoi(matches[3])
		params.DueDate = "2026-" + zeroPad(month) + "-" + zeroPad(day)
		input = shortDateRegex.ReplaceAllString(input, "")
	}

	// Parse long date: D2026-04-20
	longDateRegex := regexp.MustCompile(`^(D(\d{4}-\d{2}-\d{2}))\s*`)
	if matches := longDateRegex.FindStringSubmatch(input); len(matches) > 0 {
		params.DueDate = matches[2]
		input = longDateRegex.ReplaceAllString(input, "")
	}

	// Parse project: +project
	projectRegex := regexp.MustCompile(`^\+(\S+)\s*`)
	if matches := projectRegex.FindStringSubmatch(input); len(matches) > 0 {
		params.Project = matches[1]
		input = projectRegex.ReplaceAllString(input, "")
	}

	// Parse context: @context
	contextRegex := regexp.MustCompile(`^@(\S+)\s*`)
	if matches := contextRegex.FindStringSubmatch(input); len(matches) > 0 {
		params.Context = matches[1]
		input = contextRegex.ReplaceAllString(input, "")
	}

	params.Content = strings.TrimSpace(input)
	return params
}

// Build Memos content from command params
func (p *CommandParams) ToMemosContent() string {
	var parts []string

	parts = append(parts, "#待办")

	if p.Priority != "" {
		switch p.Priority {
		case "A":
			parts = append(parts, "#P1")
		case "B":
			parts = append(parts, "#P2")
		case "C":
			parts = append(parts, "#P3")
		}
	}

	if p.DueDate != "" {
		parts = append(parts, "#D:"+p.DueDate)
	}

	if p.Project != "" {
		parts = append(parts, "#+"+p.Project)
	}

	if p.Context != "" {
		parts = append(parts, "#@"+p.Context)
	}

	parts = append(parts, p.Content)

	return strings.Join(parts, " ")
}

// Build todo.txt display format from command params
func (p *CommandParams) ToTodoTxtDisplay() string {
	var parts []string

	if p.Priority != "" {
		parts = append(parts, "("+p.Priority+")")
	}

	parts = append(parts, p.Content)

	if p.Project != "" {
		parts = append(parts, "+"+p.Project)
	}

	if p.Context != "" {
		parts = append(parts, "@"+p.Context)
	}

	if p.DueDate != "" {
		parts = append(parts, "due:"+p.DueDate)
	}

	return strings.Join(parts, " ")
}

// Format todo list for Matrix display
func FormatTodoList(items []*TodoItem, showDone bool) string {
	var lines []string

	// Separate active and done
	var active []*TodoItem
	var done []*TodoItem

	for _, item := range items {
		if item.Done {
			done = append(done, item)
		} else {
			active = append(active, item)
		}
	}

	// Sort active by priority
	sortByPriority(active)

	// Format active todos
	if len(active) > 0 {
		lines = append(lines, "**📋 待办列表 ("+strconv.Itoa(len(active))+"项)**\n")
		for i, item := range active {
			priorityEmoji := ""
			switch item.Priority {
			case "A":
				priorityEmoji = "🔴"
			case "B":
				priorityEmoji = "🟡"
			case "C":
				priorityEmoji = "⚪"
			}

			parts := []string{fmt.Sprintf("%d.", i+1)}
			if priorityEmoji != "" {
				parts = append(parts, priorityEmoji)
			}
			parts = append(parts, item.Content)

			for _, p := range item.Projects {
				parts = append(parts, "+"+p)
			}
			for _, c := range item.Contexts {
				parts = append(parts, "@"+c)
			}

			if item.DueDate != "" {
				parts = append(parts, "📅"+formatShortDate(item.DueDate))
			}

			parts = append(parts, fmt.Sprintf("[#%d]", item.ID))
			lines = append(lines, strings.Join(parts, " "))
		}
	}

	// Format done todos if requested
	if showDone && len(done) > 0 {
		lines = append(lines, "\n**✅ 已完成 ("+strconv.Itoa(len(done))+"项)**\n")
		for i, item := range done {
			line := fmt.Sprintf("%d. ~~%s~~", i+1, item.Content)
			if item.CompletedAt != "" {
				line += fmt.Sprintf(" ✓%s", formatShortDate(item.CompletedAt))
			}
			line += fmt.Sprintf(" [#%d]", item.ID)
			lines = append(lines, line)
		}
	}

	if len(lines) == 0 {
		return "📋 暂无待办事项\n\n使用 `!待办 新增 <内容>` 添加待办"
	}

	return strings.Join(lines, "\n")
}

func sortByPriority(items []*TodoItem) {
	// Simple bubble sort
	for i := 0; i < len(items)-1; i++ {
		for j := 0; j < len(items)-i-1; j++ {
			p1 := priorityOrder(items[j].Priority)
			p2 := priorityOrder(items[j+1].Priority)
			if p1 > p2 {
				items[j], items[j+1] = items[j+1], items[j]
			}
		}
	}
}

func priorityOrder(p string) int {
	switch p {
	case "A":
		return 1
	case "B":
		return 2
	case "C":
		return 3
	default:
		return 4
	}
}

func extractDate(s string) string {
	dateRegex := regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})\s*`)
	if matches := dateRegex.FindStringSubmatch(s); len(matches) > 0 {
		return matches[1]
	}
	return ""
}

func zeroPad(n int) string {
	if n < 10 {
		return "0" + strconv.Itoa(n)
	}
	return strconv.Itoa(n)
}

func formatShortDate(date string) string {
	// Convert YYYY-MM-DD to M/D
	if len(date) >= 10 {
		month := date[5:7]
		day := date[8:10]
		m, _ := strconv.Atoi(month)
		d, _ := strconv.Atoi(day)
		return fmt.Sprintf("%d/%d", m, d)
	}
	return date
}
