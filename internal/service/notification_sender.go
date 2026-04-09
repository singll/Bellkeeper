package service

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/singll/bellkeeper/internal/matrix/gateway"
	"github.com/singll/bellkeeper/internal/repository"
)

// NotificationSender sends notifications to Matrix
type NotificationSender struct {
	client *gateway.Client
	repos  *repository.Repositories
}

// NewNotificationSender creates a new notification sender
func NewNotificationSender(client *gateway.Client, repos *repository.Repositories) *NotificationSender {
	return &NotificationSender{
		client: client,
		repos:  repos,
	}
}

// UpdateClient updates the matrix client
func (s *NotificationSender) UpdateClient(client *gateway.Client) {
	s.client = client
}

// Send delivers a notification to Matrix
func (s *NotificationSender) Send(ctx context.Context, msg *NotificationQueueMessage) error {
	// Determine message format
	var htmlBody string
	var textBody string

	switch msg.MessageType {
	case "html":
		// Assume message is HTML, extract plain text
		htmlBody = msg.Message
		textBody = stripHTML(msg.Message)
	case "markdown":
		// Convert markdown to HTML
		htmlBody = markdownToHTML(msg.Message)
		textBody = msg.Message
	default:
		// Plain text
		htmlBody = escapeHTML(msg.Message)
		textBody = msg.Message
	}

	// Send to Matrix
	eventID, err := s.client.SendHTMLMessage(ctx, msg.RoomID, htmlBody, textBody)
	if err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}

	// Update notification status
	if err := s.repos.MatrixNotification.MarkSent(ctx, msg.NotificationID, eventID); err != nil {
		log.Printf("[NotifySender] failed to update notification status: %v", err)
	}

	log.Printf("[NotifySender] sent notification %s to room %s (event: %s)", msg.NotificationID, msg.RoomID, eventID)
	return nil
}

// stripHTML removes HTML tags from a string (basic implementation)
func stripHTML(html string) string {
	// Simple implementation - remove common tags
	html = strings.ReplaceAll(html, "<br>", "\n")
	html = strings.ReplaceAll(html, "<br/>", "\n")
	html = strings.ReplaceAll(html, "<p>", "")
	html = strings.ReplaceAll(html, "</p>", "\n")
	html = strings.ReplaceAll(html, "<b>", "")
	html = strings.ReplaceAll(html, "</b>", "")
	html = strings.ReplaceAll(html, "<strong>", "")
	html = strings.ReplaceAll(html, "</strong>", "")
	html = strings.ReplaceAll(html, "<i>", "")
	html = strings.ReplaceAll(html, "</i>", "")
	html = strings.ReplaceAll(html, "<em>", "")
	html = strings.ReplaceAll(html, "</em>", "")
	html = strings.ReplaceAll(html, "<code>", "")
	html = strings.ReplaceAll(html, "</code>", "")
	html = strings.ReplaceAll(html, "<pre>", "")
	html = strings.ReplaceAll(html, "</pre>", "")
	html = strings.ReplaceAll(html, "<li>", "• ")
	html = strings.ReplaceAll(html, "</li>", "\n")
	html = strings.ReplaceAll(html, "<ul>", "")
	html = strings.ReplaceAll(html, "</ul>", "")
	html = strings.ReplaceAll(html, "<ol>", "")
	html = strings.ReplaceAll(html, "</ol>", "")
	html = strings.ReplaceAll(html, "<a href=\"", "")
	html = strings.ReplaceAll(html, "\">", " (")
	html = strings.ReplaceAll(html, "</a>", ")")
	html = strings.ReplaceAll(html, "<span>", "")
	html = strings.ReplaceAll(html, "</span>", "")
	html = strings.ReplaceAll(html, "<div>", "")
	html = strings.ReplaceAll(html, "</div>", "")
	html = strings.ReplaceAll(html, "<h1>", "# ")
	html = strings.ReplaceAll(html, "</h1>", "")
	html = strings.ReplaceAll(html, "<h2>", "## ")
	html = strings.ReplaceAll(html, "</h2>", "")
	html = strings.ReplaceAll(html, "<h3>", "### ")
	html = strings.ReplaceAll(html, "</h3>", "")

	// Remove any remaining tags
	inTag := false
	result := make([]byte, 0, len(html))
	for i := 0; i < len(html); i++ {
		if html[i] == '<' {
			inTag = true
			continue
		}
		if html[i] == '>' {
			inTag = false
			continue
		}
		if !inTag {
			result = append(result, html[i])
		}
	}

	return strings.TrimSpace(string(result))
}

// markdownToHTML converts basic markdown to HTML
func markdownToHTML(md string) string {
	html := md

	// Headers
	html = strings.ReplaceAll(html, "<", "&lt;")
	html = strings.ReplaceAll(html, ">", "&gt;")

	// Unescape first for headers
	html = strings.ReplaceAll(html, "&lt;h1&gt;", "<h1>")
	html = strings.ReplaceAll(html, "&lt;/h1&gt;", "</h1>")
	html = strings.ReplaceAll(html, "&lt;h2&gt;", "<h2>")
	html = strings.ReplaceAll(html, "&lt;/h2&gt;", "</h2>")
	html = strings.ReplaceAll(html, "&lt;h3&gt;", "<h3>")
	html = strings.ReplaceAll(html, "&lt;/h3&gt;", "</h3>")

	// Headers (must come after angle bracket escaping)
	lines := strings.Split(html, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "### ") {
			lines[i] = "<h3>" + strings.TrimPrefix(trimmed, "### ") + "</h3>"
		} else if strings.HasPrefix(trimmed, "## ") {
			lines[i] = "<h2>" + strings.TrimPrefix(trimmed, "## ") + "</h2>"
		} else if strings.HasPrefix(trimmed, "# ") {
			lines[i] = "<h1>" + strings.TrimPrefix(trimmed, "# ") + "</h1>"
		}
	}
	html = strings.Join(lines, "\n")

	// Bold
	html = strings.ReplaceAll(html, "**", "<strong>")
	html = strings.ReplaceAll(html, "__", "<strong>")
	// Close bold (simplified - replace odd occurrences)
	parts := strings.Split(html, "<strong>")
	if len(parts) > 1 {
		for i := 1; i < len(parts); i++ {
			parts[i] = strings.Replace(parts[i], "<strong>", "", 1) // close previous
		}
		html = strings.Join(parts, "</strong><strong>")
		// Now close all
		html = strings.ReplaceAll(html, "<strong>", "")
		html = strings.ReplaceAll(html, "</strong>", "")
		// Re-process
		for i, part := range strings.Split(md, "**") {
			if i%2 == 1 {
				html += "<strong>" + part + "</strong>"
			} else {
				html += part
			}
		}
	}

	// Italic
	html = strings.ReplaceAll(html, "_text_", "<em>text</em>")
	html = strings.ReplaceAll(html, "*", "<em>")
	html = strings.ReplaceAll(html, "</em><em>", "")

	// Code blocks
	for strings.Contains(html, "```") {
		html = strings.Replace(html, "```", "<pre>", 1)
		html = strings.Replace(html, "```", "</pre>", 1)
	}

	// Inline code
	html = strings.ReplaceAll(html, "`", "<code>")

	// Links [text](url)
	for strings.Contains(html, "[") && strings.Contains(html, "](") {
		start := strings.Index(html, "[")
		linkStart := strings.Index(html, "][")
		urlStart := linkStart + 2
		urlEnd := strings.Index(html[urlStart:], ")")
		if urlStart > 0 && urlEnd > 0 {
			text := html[start+1 : linkStart]
			url := html[urlStart : urlStart+urlEnd]
			html = html[:start] + fmt.Sprintf(`<a href="%s">%s</a>`, url, text) + html[urlStart+urlEnd+1:]
		} else {
			break
		}
	}

	// Line breaks
	html = strings.ReplaceAll(html, "\n\n", "</p><p>")
	html = "<p>" + html + "</p>"
	html = strings.ReplaceAll(html, "<p></p>", "")

	return html
}

// escapeHTML escapes special HTML characters
func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "'", "&#39;")
	return s
}

// RetryFailedNotifications retries failed notifications
func (s *NotificationSender) RetryFailedNotifications(ctx context.Context, maxRetries int) error {
	notifications, err := s.repos.MatrixNotification.GetFailed(ctx, maxRetries)
	if err != nil {
		return fmt.Errorf("failed to get failed notifications: %w", err)
	}

	log.Printf("[NotifySender] retrying %d failed notifications", len(notifications))

	for _, n := range notifications {
		msg := &NotificationQueueMessage{
			NotificationID: n.NotificationID,
			RoomID:        n.RoomID,
			Message:       n.MessageContent,
			MessageType:   n.MessageType,
			RetryCount:    n.RetryCount + 1,
		}

		if err := s.Send(ctx, msg); err != nil {
			log.Printf("[NotifySender] retry failed for %s: %v", n.NotificationID, err)
			s.repos.MatrixNotification.UpdateStatus(ctx, n.NotificationID, "retrying", err.Error())
		}

		// Rate limit between retries
		time.Sleep(100 * time.Millisecond)
	}

	return nil
}
