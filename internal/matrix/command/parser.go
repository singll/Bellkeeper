package command

import (
	"fmt"
	"strings"
)

// ParsedCommand represents a parsed command from user input
type ParsedCommand struct {
	Name    string            // 命令名称（不含前缀）
	Args    string            // 原始参数字符串
	Argv    []string          // 参数列表（按空格分割）
	Raw     string            // 原始消息
	Prefix  string            // 使用的前缀（! 或 ！）
}

// Parser parses Matrix messages into commands
type Parser struct {
	prefixes []string
}

// NewParser creates a new command parser
func NewParser(prefixes string) *Parser {
	// Support multiple prefixes
	p := strings.Split(prefixes, ",")
	prefixesList := make([]string, 0, len(p))
	for _, pref := range p {
		pref = strings.TrimSpace(pref)
		if pref != "" {
			prefixesList = append(prefixesList, pref)
		}
	}
	return &Parser{prefixes: prefixesList}
}

// Parse parses a message and returns a ParsedCommand if it matches a command pattern
func (p *Parser) Parse(content string) (*ParsedCommand, bool) {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, false
	}

	// Find matching prefix
	var prefix string
	var rest string
	for _, pref := range p.prefixes {
		if strings.HasPrefix(content, pref) {
			prefix = pref
			rest = strings.TrimSpace(content[len(pref):])
			break
		}
	}

	// No prefix matched
	if prefix == "" {
		return nil, false
	}

	// Empty command after prefix
	if rest == "" {
		return nil, false
	}

	// Parse command name and arguments
	parts := strings.SplitN(rest, " ", 2)
	name := parts[0]
	args := ""
	if len(parts) > 1 {
		args = strings.TrimSpace(parts[1])
	}

	// Build argv
	argv := parseArgs(args)

	return &ParsedCommand{
		Name:   strings.ToLower(name),
		Args:   args,
		Argv:   argv,
		Raw:    content,
		Prefix: prefix,
	}, true
}

// parseArgs splits arguments by spaces, respecting quoted strings
func parseArgs(args string) []string {
	if args == "" {
		return []string{}
	}

	var argv []string
	var current strings.Builder
	inQuote := false
	var quoteChar byte

	for i := 0; i < len(args); i++ {
		c := args[i]

		if !inQuote && (c == '"' || c == '\'') {
			inQuote = true
			quoteChar = c
			continue
		}

		if inQuote && c == quoteChar {
			inQuote = false
			continue
		}

		if !inQuote && c == ' ' {
			if current.Len() > 0 {
				argv = append(argv, current.String())
				current.Reset()
			}
			continue
		}

		current.WriteByte(c)
	}

	if current.Len() > 0 {
		argv = append(argv, current.String())
	}

	return argv
}

// FormatHelp generates a help text for a command
func FormatHelp(name string, description string, usage string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("**%s%s** — %s\n", "!", name, description))
	if usage != "" {
		sb.WriteString(fmt.Sprintf("使用: `%s%s %s`\n", "!", name, usage))
	}
	return sb.String()
}
