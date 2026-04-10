package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/singll/bellkeeper/internal/matrix/command"
)

// TodoTxtHandler handles todo.txt export operations
type TodoTxtHandler struct {
	apiURL     string
	apiToken   string
	httpClient *http.Client
}

// NewTodoTxtHandler creates a new todo.txt handler
func NewTodoTxtHandler(apiURL, apiToken string) *TodoTxtHandler {
	return &TodoTxtHandler{
		apiURL:   apiURL,
		apiToken: apiToken,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Export exports all todos in todo.txt format (JSON response)
func (h *TodoTxtHandler) Export(c *gin.Context) {
	if h.apiURL == "" || h.apiToken == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Memos API not configured",
		})
		return
	}

	url := h.apiURL + "/api/v1/memos"
	req, err := http.NewRequestWithContext(c.Request.Context(), "GET", url, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to create request: " + err.Error(),
		})
		return
	}
	req.Header.Set("Authorization", "Bearer "+h.apiToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := h.httpClient.Do(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to call Memos API: " + err.Error(),
		})
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		c.JSON(http.StatusBadGateway, gin.H{
			"success": false,
			"message": "Memos API error: " + string(body),
		})
		return
	}

	// Parse Memos response
	var result struct {
		Memos []struct {
			ID          int    `json:"id"`
			Content     string `json:"content"`
			CreatedTs   int64  `json:"createdTs"`
			Done        bool   `json:"done"`
			CompletedTs *int64 `json:"completedTs"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to parse Memos response: " + err.Error(),
		})
		return
	}

	// Build todo.txt content
	var todoLines []string
	var doneLines []string

	for _, m := range result.Memos {
		// Skip non-todos
		isTodo := false
		if strings.Contains(m.Content, "#待办") || strings.Contains(m.Content, "#todo") {
			isTodo = true
		}

		if !isTodo {
			continue
		}

		todoTxt := command.MemosToTodoTxt(m.Content)

		// Build todo.txt line with dates
		if m.Done {
			var line strings.Builder
			line.WriteString("x ")
			if m.CompletedTs != nil && *m.CompletedTs > 0 {
				line.WriteString(formatTimestamp(*m.CompletedTs))
				line.WriteString(" ")
			}
			if m.CreatedTs > 0 {
				line.WriteString(formatTimestamp(m.CreatedTs))
				line.WriteString(" ")
			}
			line.WriteString(todoTxt)
			doneLines = append(doneLines, line.String())
		} else {
			var line strings.Builder
			if m.CreatedTs > 0 {
				line.WriteString(formatTimestamp(m.CreatedTs))
				line.WriteString(" ")
			}
			line.WriteString(todoTxt)
			todoLines = append(todoLines, line.String())
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"todo.txt":   strings.Join(todoLines, "\n"),
		"done.txt":   strings.Join(doneLines, "\n"),
		"todo_count": len(todoLines),
		"done_count": len(doneLines),
	})
}

// ExportPlain returns todo.txt in plain text format
func (h *TodoTxtHandler) ExportPlain(c *gin.Context) {
	if h.apiURL == "" || h.apiToken == "" {
		c.String(http.StatusBadRequest, "Memos API not configured")
		return
	}

	url := h.apiURL + "/api/v1/memos"
	req, err := http.NewRequestWithContext(c.Request.Context(), "GET", url, nil)
	if err != nil {
		c.String(http.StatusInternalServerError, "Failed to create request")
		return
	}
	req.Header.Set("Authorization", "Bearer "+h.apiToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := h.httpClient.Do(req)
	if err != nil {
		c.String(http.StatusInternalServerError, "Failed to call Memos API")
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		c.String(http.StatusBadGateway, "Memos API error")
		return
	}

	var result struct {
		Memos []struct {
			Content   string `json:"content"`
			CreatedTs int64  `json:"createdTs"`
			Done      bool   `json:"done"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		c.String(http.StatusInternalServerError, "Failed to parse response")
		return
	}

	var lines []string
	for _, m := range result.Memos {
		if strings.Contains(m.Content, "#待办") || strings.Contains(m.Content, "#todo") {
			todoTxt := command.MemosToTodoTxt(m.Content)
			if m.CreatedTs > 0 {
				lines = append(lines, formatTimestamp(m.CreatedTs)+" "+todoTxt)
			} else {
				lines = append(lines, todoTxt)
			}
		}
	}

	c.Header("Content-Type", "text/plain; charset=utf-8")
	c.String(http.StatusOK, strings.Join(lines, "\n"))
}

// formatTimestamp converts Unix timestamp to YYYY-MM-DD
func formatTimestamp(ts int64) string {
	t := time.Unix(ts, 0)
	return t.Format("2006-01-02")
}
