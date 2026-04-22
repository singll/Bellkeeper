package service

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/singll/bellkeeper/internal/matrix/gateway"
	"github.com/singll/bellkeeper/internal/repository"
	"github.com/yuin/goldmark"
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

// markdownToHTML converts markdown to HTML using goldmark.
func markdownToHTML(md string) string {
	var buf bytes.Buffer
	if err := goldmark.Convert([]byte(md), &buf); err != nil {
		return md
	}
	return buf.String()
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
