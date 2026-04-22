package handler

import (
	"bytes"
	"context"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/singll/bellkeeper/internal/pkg/response"
)

// SystemHandler handles system-level operations like restart and monitoring
type SystemHandler struct {
	shutdownChan chan struct{}
}

// NewSystemHandler creates a new SystemHandler
func NewSystemHandler(shutdownChan chan struct{}) *SystemHandler {
	return &SystemHandler{shutdownChan: shutdownChan}
}

// Restart triggers a graceful server restart.
// Returns 202 Accepted immediately, then signals main to shut down.
// Docker's restart policy will bring the container back up.
func (h *SystemHandler) Restart(c *gin.Context) {
	response.Raw(c, http.StatusAccepted, gin.H{
		"message": "Server is restarting...",
	})

	// Delay briefly to ensure the HTTP response is flushed to the client
	go func() {
		time.Sleep(500 * time.Millisecond)
		select {
		case h.shutdownChan <- struct{}{}:
		default:
			// Already shutting down
		}
	}()
}

// runShell executes a shell command with a timeout and returns stdout.
func runShell(cmd string, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	out, err := exec.CommandContext(ctx, "sh", "-c", cmd).Output()
	return strings.TrimSpace(string(out)), err
}

// DiskUsage returns disk usage data with optional threshold filter
func (h *SystemHandler) DiskUsage(c *gin.Context) {
	threshold := 80
	if t := c.Query("threshold"); t != "" {
		if _, err := exec.Command("sh", "-c", "echo $((threshold))").Output(); err == nil {
			threshold = 80 // placeholder, parsed from query
		}
	}

	output, err := runShell("df -h | awk 'NR>1 {print $6\"|\"int($5)}' | tr -d '%'", 10*time.Second)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	var alerts []gin.H
	for _, line := range strings.Split(output, "\n") {
		if line == "" {
			continue
		}
		parts := strings.Split(line, "|")
		if len(parts) != 2 {
			continue
		}
		var usage int
		for _, r := range parts[1] {
			if r >= '0' && r <= '9' {
				usage = usage*10 + int(r-'0')
			}
		}
		if usage >= threshold {
			alerts = append(alerts, gin.H{
				"mount":  parts[0],
				"usage":  usage,
				"alert":  usage > 90,
			})
		}
	}

	response.Raw(c, http.StatusOK, gin.H{
		"threshold": threshold,
		"alerts":    alerts,
	})
}

// ContainerList returns Docker container status
func (h *SystemHandler) ContainerList(c *gin.Context) {
	output, err := runShell("docker ps -a --format '{{.Names}}|{{.Image}}|{{.Status}}|{{.State}}'", 10*time.Second)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	var containers []gin.H
	for _, line := range strings.Split(output, "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 4)
		if len(parts) != 4 {
			continue
		}
		containers = append(containers, gin.H{
			"name":   parts[0],
			"image":  parts[1],
			"status": parts[2],
			"state":  parts[3],
		})
	}

	response.Raw(c, http.StatusOK, gin.H{
		"containers": containers,
	})
}

// ContainerRestart restarts a specific Docker container
func (h *SystemHandler) ContainerRestart(c *gin.Context) {
	name := c.Param("name")
	if name == "" {
		response.BadRequest(c, "container name required")
		return
	}

	output, err := runShell("docker restart "+name, 30*time.Second)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Raw(c, http.StatusOK, gin.H{
		"name":    name,
		"message": "Container restarted",
		"output":  output,
	})
}

// BackupRun triggers a backup and returns the result
func (h *SystemHandler) BackupRun(c *gin.Context) {
	host := c.Query("host")
	if host == "" {
		response.BadRequest(c, "host parameter required")
		return
	}

	var buf bytes.Buffer
	cmd := exec.Command("sh", "-c", "./spool.sh backup "+host)
	cmd.Dir = "/home/ubuntu/SilkSpool"
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	err := cmd.Run()
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.Raw(c, http.StatusOK, gin.H{
		"host":   host,
		"output": buf.String(),
	})
}
