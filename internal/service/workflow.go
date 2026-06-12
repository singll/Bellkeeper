package service

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/singll/bellkeeper/internal/config"
	"github.com/singll/bellkeeper/internal/pkg/httpclient"
	"github.com/singll/bellkeeper/internal/repository"
)

type WorkflowService struct {
	cfg         config.N8NConfig
	settingRepo *repository.SettingRepository
	client      *httpclient.Client
}

func NewWorkflowService(cfg config.N8NConfig, settingRepo *repository.SettingRepository) *WorkflowService {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 30
	}
	return &WorkflowService{
		cfg:         cfg,
		settingRepo: settingRepo,
		client:      httpclient.NewClientWithTimeout(time.Duration(timeout) * time.Second),
	}
}

// getEffectiveConfig returns the n8n config, falling back to DB settings when startup config is empty.
func (s *WorkflowService) getEffectiveConfig() (apiKey, apiBaseURL, webhookBaseURL string) {
	apiKey = s.cfg.APIKey
	apiBaseURL = s.cfg.APIBaseURL
	webhookBaseURL = s.cfg.WebhookBaseURL

	if apiKey == "" && s.settingRepo != nil {
		if setting, err := s.settingRepo.GetByKey("n8n_api_key"); err == nil && setting.Value != "" {
			apiKey = setting.Value
		}
	}
	if apiBaseURL == "" && s.settingRepo != nil {
		if setting, err := s.settingRepo.GetByKey("n8n_api_base_url"); err == nil && setting.Value != "" {
			apiBaseURL = setting.Value
		}
	}
	if webhookBaseURL == "" && s.settingRepo != nil {
		if setting, err := s.settingRepo.GetByKey("n8n_webhook_base_url"); err == nil && setting.Value != "" {
			webhookBaseURL = setting.Value
		}
	}

	return
}

func (s *WorkflowService) workflowDefinitionDir() string {
	dir := strings.TrimSpace(s.cfg.WorkflowDir)
	if dir == "" && s.settingRepo != nil {
		if setting, err := s.settingRepo.GetByKey("n8n_workflow_dir"); err == nil && setting.Value != "" {
			dir = setting.Value
		}
	}
	if dir == "" {
		dir = "internal/n8n_workflows"
	}
	return filepath.Clean(dir)
}

type WorkflowStatus struct {
	ID        string                 `json:"id"`
	Name      string                 `json:"name"`
	Active    bool                   `json:"active"`
	CreatedAt string                 `json:"created_at,omitempty"`
	UpdatedAt string                 `json:"updated_at,omitempty"`
	Tags      []WorkflowTag          `json:"tags,omitempty"`
	Meta      map[string]interface{} `json:"meta,omitempty"`
}

type WorkflowTag struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type WorkflowExecution struct {
	ID         string `json:"id"`
	WorkflowID string `json:"workflow_id"`
	Finished   bool   `json:"finished"`
	Status     string `json:"status"` // success, error, waiting
	StartedAt  string `json:"started_at"`
	StoppedAt  string `json:"stopped_at,omitempty"`
}

type WorkflowDefinitionInventory struct {
	WorkflowDir  string               `json:"workflow_dir"`
	Definitions  []WorkflowDefinition `json:"definitions"`
	RemoteOnly   []WorkflowStatus     `json:"remote_only,omitempty"`
	LocalError   string               `json:"local_error,omitempty"`
	RuntimeError string               `json:"runtime_error,omitempty"`
}

type WorkflowDefinition struct {
	Key          string          `json:"key"`
	FileName     string          `json:"file_name"`
	Path         string          `json:"path"`
	Name         string          `json:"name"`
	Valid        bool            `json:"valid"`
	Error        string          `json:"error,omitempty"`
	NodeCount    int             `json:"node_count"`
	TriggerTypes []string        `json:"trigger_types,omitempty"`
	UpdatedAt    string          `json:"updated_at,omitempty"`
	Hash         string          `json:"hash,omitempty"`
	Runtime      *WorkflowStatus `json:"runtime,omitempty"`
	DriftStatus  string          `json:"drift_status"`
}

type WorkflowDefinitionDetail struct {
	Definition WorkflowDefinition     `json:"definition"`
	Content    map[string]interface{} `json:"content"`
	RawJSON    string                 `json:"raw_json"`
}

type WorkflowDefinitionPushResult struct {
	Key        string          `json:"key"`
	Name       string          `json:"name"`
	Action     string          `json:"action"`
	WorkflowID string          `json:"workflow_id,omitempty"`
	Runtime    *WorkflowStatus `json:"runtime,omitempty"`
	Error      string          `json:"error,omitempty"`
}

// n8nWorkflow represents the workflow response from n8n API
type n8nWorkflow struct {
	ID        string                 `json:"id"`
	Name      string                 `json:"name"`
	Active    bool                   `json:"active"`
	CreatedAt string                 `json:"createdAt"`
	UpdatedAt string                 `json:"updatedAt"`
	Tags      []n8nTag               `json:"tags"`
	Meta      map[string]interface{} `json:"meta"`
}

type n8nTag struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type n8nWorkflowsResponse struct {
	Data       []n8nWorkflow `json:"data"`
	NextCursor string        `json:"nextCursor"`
}

type n8nExecutionsResponse struct {
	Data       []n8nExecution `json:"data"`
	NextCursor string         `json:"nextCursor"`
}

type n8nExecution struct {
	ID         string `json:"id"`
	WorkflowID string `json:"workflowId"`
	Finished   bool   `json:"finished"`
	Status     string `json:"status"`
	StartedAt  string `json:"startedAt"`
	StoppedAt  string `json:"stoppedAt"`
}

func (s *WorkflowService) doN8NRequest(method, endpoint string, body interface{}) ([]byte, error) {
	apiKey, apiBaseURL, _ := s.getEffectiveConfig()
	if apiKey == "" {
		return nil, fmt.Errorf("n8n API key not configured: please set N8N_API_KEY environment variable or configure n8n_api_key in settings")
	}
	if strings.TrimSpace(apiBaseURL) == "" {
		return nil, fmt.Errorf("n8n API base URL not configured")
	}

	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = bytes.NewReader(data)
	}

	requestURL := strings.TrimRight(apiBaseURL, "/") + endpoint
	req, err := http.NewRequest(method, requestURL, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("X-N8N-API-KEY", apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to n8n: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		var errResp struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(respBody, &errResp)
		if errResp.Message != "" {
			return nil, fmt.Errorf("n8n returned HTTP %d: %s", resp.StatusCode, errResp.Message)
		}
		return nil, fmt.Errorf("n8n returned HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

func cleanWorkflowDefinitionKey(key string) (string, error) {
	key = strings.TrimSpace(strings.TrimSuffix(key, ".json"))
	if key == "" {
		return "", fmt.Errorf("workflow definition key is required")
	}
	if key == "." || key == ".." || strings.Contains(key, "..") || strings.ContainsAny(key, `/\`) {
		return "", fmt.Errorf("invalid workflow definition key: %s", key)
	}
	return key, nil
}

func (s *WorkflowService) workflowDefinitionPath(key string) (string, error) {
	cleanKey, err := cleanWorkflowDefinitionKey(key)
	if err != nil {
		return "", err
	}
	return filepath.Join(s.workflowDefinitionDir(), cleanKey+".json"), nil
}

func (s *WorkflowService) loadLocalWorkflowDefinitions() ([]WorkflowDefinition, error) {
	dir := s.workflowDefinitionDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read workflow definition dir %s: %w", dir, err)
	}

	defs := make([]WorkflowDefinition, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" || entry.Name() == "00-config.json" {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		info, infoErr := entry.Info()
		if infoErr != nil {
			defs = append(defs, WorkflowDefinition{
				Key:         strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name())),
				FileName:    entry.Name(),
				Path:        path,
				Valid:       false,
				Error:       infoErr.Error(),
				DriftStatus: "invalid",
			})
			continue
		}

		data, readErr := os.ReadFile(path)
		def := parseWorkflowDefinition(entry.Name(), path, data, info.ModTime())
		if readErr != nil {
			def.Valid = false
			def.Error = readErr.Error()
			def.DriftStatus = "invalid"
		}
		defs = append(defs, def)
	}

	sort.Slice(defs, func(i, j int) bool {
		return defs[i].Key < defs[j].Key
	})

	return defs, nil
}

func parseWorkflowDefinition(fileName, path string, data []byte, modTime time.Time) WorkflowDefinition {
	key := strings.TrimSuffix(fileName, filepath.Ext(fileName))
	sum := sha256.Sum256(data)
	def := WorkflowDefinition{
		Key:         key,
		FileName:    fileName,
		Path:        path,
		Name:        key,
		Valid:       true,
		UpdatedAt:   modTime.UTC().Format(time.RFC3339),
		Hash:        fmt.Sprintf("%x", sum[:]),
		DriftStatus: "unknown",
	}

	var parsed struct {
		Name  string `json:"name"`
		Nodes []struct {
			Type string `json:"type"`
		} `json:"nodes"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		def.Valid = false
		def.Error = err.Error()
		def.DriftStatus = "invalid"
		return def
	}
	if strings.TrimSpace(parsed.Name) != "" {
		def.Name = parsed.Name
	}
	def.NodeCount = len(parsed.Nodes)

	triggerTypes := make(map[string]struct{})
	for _, node := range parsed.Nodes {
		lowerType := strings.ToLower(node.Type)
		if strings.Contains(lowerType, "trigger") || strings.Contains(lowerType, "webhook") {
			triggerTypes[node.Type] = struct{}{}
		}
	}
	for triggerType := range triggerTypes {
		def.TriggerTypes = append(def.TriggerTypes, triggerType)
	}
	sort.Strings(def.TriggerTypes)

	return def
}

// ListWorkflowDefinitions returns repository-managed n8n workflow JSON definitions and
// best-effort runtime status from n8n.
func (s *WorkflowService) ListWorkflowDefinitions() (*WorkflowDefinitionInventory, error) {
	defs, localErr := s.loadLocalWorkflowDefinitions()
	inventory := &WorkflowDefinitionInventory{
		WorkflowDir: s.workflowDefinitionDir(),
		Definitions: defs,
	}
	if localErr != nil {
		inventory.LocalError = localErr.Error()
	}

	runtimeWorkflows, runtimeErr := s.Status()
	if runtimeErr != nil {
		inventory.RuntimeError = runtimeErr.Error()
		for i := range inventory.Definitions {
			if inventory.Definitions[i].DriftStatus == "unknown" && inventory.Definitions[i].Valid {
				inventory.Definitions[i].DriftStatus = "runtime_unknown"
			}
		}
		return inventory, nil
	}

	remoteByName := make(map[string]WorkflowStatus, len(runtimeWorkflows))
	for _, wf := range runtimeWorkflows {
		remoteByName[wf.Name] = wf
	}

	matchedRemote := make(map[string]struct{})
	for i := range inventory.Definitions {
		if !inventory.Definitions[i].Valid {
			inventory.Definitions[i].DriftStatus = "invalid"
			continue
		}
		if runtime, ok := remoteByName[inventory.Definitions[i].Name]; ok {
			runtimeCopy := runtime
			inventory.Definitions[i].Runtime = &runtimeCopy
			inventory.Definitions[i].DriftStatus = "present"
			matchedRemote[runtime.Name] = struct{}{}
		} else {
			inventory.Definitions[i].DriftStatus = "missing_remote"
		}
	}

	for _, wf := range runtimeWorkflows {
		if _, ok := matchedRemote[wf.Name]; !ok {
			inventory.RemoteOnly = append(inventory.RemoteOnly, wf)
		}
	}
	sort.Slice(inventory.RemoteOnly, func(i, j int) bool {
		return inventory.RemoteOnly[i].Name < inventory.RemoteOnly[j].Name
	})

	return inventory, nil
}

func (s *WorkflowService) GetWorkflowDefinition(key string) (*WorkflowDefinitionDetail, error) {
	path, err := s.workflowDefinitionPath(key)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read workflow definition: %w", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("failed to stat workflow definition: %w", err)
	}

	var content map[string]interface{}
	if err := json.Unmarshal(data, &content); err != nil {
		return nil, fmt.Errorf("failed to parse workflow definition JSON: %w", err)
	}

	return &WorkflowDefinitionDetail{
		Definition: parseWorkflowDefinition(filepath.Base(path), path, data, info.ModTime()),
		Content:    content,
		RawJSON:    string(data),
	}, nil
}

func (s *WorkflowService) SaveWorkflowDefinition(key string, content map[string]interface{}) (*WorkflowDefinitionDetail, error) {
	cleanKey, err := cleanWorkflowDefinitionKey(key)
	if err != nil {
		return nil, err
	}
	if content == nil {
		return nil, fmt.Errorf("workflow definition JSON body is required")
	}
	if name, ok := content["name"].(string); !ok || strings.TrimSpace(name) == "" {
		content["name"] = cleanKey
	}
	if err := validateWorkflowContent(content); err != nil {
		return nil, err
	}

	dir := s.workflowDefinitionDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create workflow definition dir: %w", err)
	}

	path := filepath.Join(dir, cleanKey+".json")
	data, err := json.MarshalIndent(content, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal workflow definition: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0644); err != nil {
		return nil, fmt.Errorf("failed to write workflow definition: %w", err)
	}

	return s.GetWorkflowDefinition(cleanKey)
}

func (s *WorkflowService) DeleteWorkflowDefinition(key string) error {
	path, err := s.workflowDefinitionPath(key)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("failed to delete workflow definition: %w", err)
	}
	return nil
}

func validateWorkflowContent(content map[string]interface{}) error {
	name, ok := content["name"].(string)
	if !ok || strings.TrimSpace(name) == "" {
		return fmt.Errorf("workflow definition requires a non-empty name")
	}
	nodes, ok := content["nodes"].([]interface{})
	if !ok || len(nodes) == 0 {
		return fmt.Errorf("workflow definition requires at least one node")
	}
	return nil
}

func (s *WorkflowService) PushWorkflowDefinition(key string) (*WorkflowDefinitionPushResult, error) {
	detail, err := s.GetWorkflowDefinition(key)
	if err != nil {
		return nil, err
	}
	if !detail.Definition.Valid {
		return nil, fmt.Errorf("workflow definition is invalid: %s", detail.Definition.Error)
	}
	if err := validateWorkflowContent(detail.Content); err != nil {
		return nil, err
	}

	runtimeWorkflows, err := s.Status()
	if err != nil {
		return nil, err
	}

	remoteByName := make(map[string]WorkflowStatus, len(runtimeWorkflows))
	for _, wf := range runtimeWorkflows {
		remoteByName[wf.Name] = wf
	}

	if runtime, ok := remoteByName[detail.Definition.Name]; ok {
		payload := prepareWorkflowPushPayload(detail.Content, &runtime, false)
		data, err := s.doN8NRequest("PUT", "/workflows/"+url.PathEscape(runtime.ID), payload)
		if err != nil {
			return nil, err
		}
		updated, parseErr := parseWorkflowStatus(data)
		if parseErr != nil {
			return nil, parseErr
		}
		return &WorkflowDefinitionPushResult{
			Key:        detail.Definition.Key,
			Name:       detail.Definition.Name,
			Action:     "updated",
			WorkflowID: updated.ID,
			Runtime:    updated,
		}, nil
	}

	payload := prepareWorkflowPushPayload(detail.Content, nil, true)
	data, err := s.doN8NRequest("POST", "/workflows", payload)
	if err != nil {
		return nil, err
	}
	created, parseErr := parseWorkflowStatus(data)
	if parseErr != nil {
		return nil, parseErr
	}
	return &WorkflowDefinitionPushResult{
		Key:        detail.Definition.Key,
		Name:       detail.Definition.Name,
		Action:     "created",
		WorkflowID: created.ID,
		Runtime:    created,
	}, nil
}

func (s *WorkflowService) PushAllWorkflowDefinitions() ([]WorkflowDefinitionPushResult, error) {
	defs, err := s.loadLocalWorkflowDefinitions()
	if err != nil {
		return nil, err
	}

	results := make([]WorkflowDefinitionPushResult, 0, len(defs))
	for _, def := range defs {
		if !def.Valid {
			results = append(results, WorkflowDefinitionPushResult{
				Key:    def.Key,
				Name:   def.Name,
				Action: "skipped",
				Error:  def.Error,
			})
			continue
		}

		result, err := s.PushWorkflowDefinition(def.Key)
		if err != nil {
			results = append(results, WorkflowDefinitionPushResult{
				Key:    def.Key,
				Name:   def.Name,
				Action: "failed",
				Error:  err.Error(),
			})
			continue
		}
		results = append(results, *result)
	}
	return results, nil
}

func prepareWorkflowPushPayload(content map[string]interface{}, runtime *WorkflowStatus, create bool) map[string]interface{} {
	payload := make(map[string]interface{}, len(content)+2)
	for key, value := range content {
		payload[key] = value
	}

	for _, key := range []string{"id", "createdAt", "updatedAt", "versionId", "triggerCount", "shared", "tags", "meta"} {
		delete(payload, key)
	}
	delete(payload, "active")

	if _, ok := payload["connections"]; !ok {
		payload["connections"] = map[string]interface{}{}
	}

	settings, ok := payload["settings"].(map[string]interface{})
	if !ok {
		settings = map[string]interface{}{}
		payload["settings"] = settings
	}
	if _, ok := settings["executionOrder"]; !ok {
		settings["executionOrder"] = "v1"
	}

	return payload
}

func parseWorkflowStatus(data []byte) (*WorkflowStatus, error) {
	var wf n8nWorkflow
	if err := json.Unmarshal(data, &wf); err != nil {
		return nil, fmt.Errorf("failed to parse workflow response: %w", err)
	}

	tags := make([]WorkflowTag, len(wf.Tags))
	for i, t := range wf.Tags {
		tags[i] = WorkflowTag(t)
	}

	return &WorkflowStatus{
		ID:        wf.ID,
		Name:      wf.Name,
		Active:    wf.Active,
		CreatedAt: wf.CreatedAt,
		UpdatedAt: wf.UpdatedAt,
		Tags:      tags,
		Meta:      wf.Meta,
	}, nil
}

// Status retrieves the list of workflows from n8n
func (s *WorkflowService) Status() ([]WorkflowStatus, error) {
	body, err := s.doN8NRequest("GET", "/workflows", nil)
	if err != nil {
		return nil, err
	}

	var n8nResp n8nWorkflowsResponse
	if err := json.Unmarshal(body, &n8nResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	workflows := make([]WorkflowStatus, len(n8nResp.Data))
	for i, wf := range n8nResp.Data {
		tags := make([]WorkflowTag, len(wf.Tags))
		for j, t := range wf.Tags {
			tags[j] = WorkflowTag(t)
		}
		workflows[i] = WorkflowStatus{
			ID:        wf.ID,
			Name:      wf.Name,
			Active:    wf.Active,
			CreatedAt: wf.CreatedAt,
			UpdatedAt: wf.UpdatedAt,
			Tags:      tags,
			Meta:      wf.Meta,
		}
	}

	return workflows, nil
}

// GetWorkflow retrieves a single workflow by ID
func (s *WorkflowService) GetWorkflow(id string) (*WorkflowStatus, error) {
	body, err := s.doN8NRequest("GET", "/workflows/"+url.PathEscape(id), nil)
	if err != nil {
		return nil, err
	}

	return parseWorkflowStatus(body)
}

// ActivateWorkflow activates a workflow
func (s *WorkflowService) ActivateWorkflow(id string) error {
	_, err := s.doN8NRequest("POST", "/workflows/"+url.PathEscape(id)+"/activate", nil)
	return err
}

// DeactivateWorkflow deactivates a workflow
func (s *WorkflowService) DeactivateWorkflow(id string) error {
	_, err := s.doN8NRequest("POST", "/workflows/"+url.PathEscape(id)+"/deactivate", nil)
	return err
}

// GetExecutions retrieves workflow executions
func (s *WorkflowService) GetExecutions(workflowID string, limit int) ([]WorkflowExecution, error) {
	values := url.Values{}
	values.Set("limit", fmt.Sprintf("%d", limit))
	if workflowID != "" {
		values.Set("workflowId", workflowID)
	}

	body, err := s.doN8NRequest("GET", "/executions?"+values.Encode(), nil)
	if err != nil {
		return nil, err
	}

	var n8nResp n8nExecutionsResponse
	if err := json.Unmarshal(body, &n8nResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	executions := make([]WorkflowExecution, len(n8nResp.Data))
	for i, ex := range n8nResp.Data {
		executions[i] = WorkflowExecution(ex)
	}

	return executions, nil
}

// Trigger triggers a workflow via webhook
func (s *WorkflowService) Trigger(name string, payload map[string]interface{}) (map[string]interface{}, error) {
	_, _, webhookBaseURL := s.getEffectiveConfig()

	url := fmt.Sprintf("%s/webhook/%s", webhookBaseURL, name)

	var body []byte
	if payload != nil {
		body, _ = json.Marshal(payload)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close() //nolint:errcheck

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		// Try to include raw response in error message
		return nil, fmt.Errorf("failed to parse response: %w, body: %s", err, string(respBody))
	}

	if resp.StatusCode >= 400 {
		return result, fmt.Errorf("trigger failed with status %d", resp.StatusCode)
	}

	return result, nil
}
