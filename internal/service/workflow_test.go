package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/singll/bellkeeper/internal/config"
)

func writeWorkflowDefinition(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0644); err != nil {
		t.Fatalf("write workflow definition: %v", err)
	}
}

func TestListWorkflowDefinitionsMergesRuntime(t *testing.T) {
	dir := t.TempDir()
	writeWorkflowDefinition(t, dir, "A-test.json", `{
  "name": "A Test",
  "nodes": [{"id":"trigger","name":"Webhook","type":"n8n-nodes-base.webhook"}],
  "connections": {}
}`)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-N8N-API-KEY") != "secret" {
			t.Fatalf("missing API key header")
		}
		if r.URL.Path != "/api/v1/workflows" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{
				{"id": "wf-1", "name": "A Test", "active": true, "updatedAt": "2026-06-09T00:00:00Z"},
			},
		})
	}))
	defer server.Close()

	svc := NewWorkflowService(config.N8NConfig{
		APIBaseURL:  server.URL + "/api/v1",
		APIKey:      "secret",
		WorkflowDir: dir,
		Timeout:     1,
	}, nil)

	inventory, err := svc.ListWorkflowDefinitions()
	if err != nil {
		t.Fatalf("ListWorkflowDefinitions returned error: %v", err)
	}
	if inventory.RuntimeError != "" {
		t.Fatalf("unexpected runtime error: %s", inventory.RuntimeError)
	}
	if len(inventory.Definitions) != 1 {
		t.Fatalf("expected 1 definition, got %d", len(inventory.Definitions))
	}
	def := inventory.Definitions[0]
	if def.Name != "A Test" || def.DriftStatus != "present" {
		t.Fatalf("unexpected definition merge: %#v", def)
	}
	if def.Runtime == nil || !def.Runtime.Active || def.Runtime.ID != "wf-1" {
		t.Fatalf("runtime status not merged: %#v", def.Runtime)
	}
	if len(def.TriggerTypes) != 1 || def.TriggerTypes[0] != "n8n-nodes-base.webhook" {
		t.Fatalf("trigger types not detected: %#v", def.TriggerTypes)
	}
}

func TestListWorkflowDefinitionsSurvivesRuntimeError(t *testing.T) {
	dir := t.TempDir()
	writeWorkflowDefinition(t, dir, "A-test.json", `{
  "name": "A Test",
  "nodes": [{"id":"trigger","name":"Manual","type":"n8n-nodes-base.manualTrigger"}],
  "connections": {}
}`)

	svc := NewWorkflowService(config.N8NConfig{WorkflowDir: dir, Timeout: 1}, nil)

	inventory, err := svc.ListWorkflowDefinitions()
	if err != nil {
		t.Fatalf("ListWorkflowDefinitions returned error: %v", err)
	}
	if inventory.RuntimeError == "" {
		t.Fatal("expected runtime error when n8n is not configured")
	}
	if len(inventory.Definitions) != 1 {
		t.Fatalf("expected local definition despite runtime error, got %d", len(inventory.Definitions))
	}
	if inventory.Definitions[0].DriftStatus != "runtime_unknown" {
		t.Fatalf("unexpected drift status: %s", inventory.Definitions[0].DriftStatus)
	}
}

func TestPushWorkflowDefinitionUpdatesByNameAndPreservesActive(t *testing.T) {
	dir := t.TempDir()
	writeWorkflowDefinition(t, dir, "A-test.json", `{
  "id": "local-id",
  "name": "A Test",
  "meta": {"instanceId": ""},
  "tags": [{"name": "local"}],
  "nodes": [{"id":"trigger","name":"Manual","type":"n8n-nodes-base.manualTrigger"}],
  "connections": {},
  "settings": {}
}`)

	var putPayload map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/workflows":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"data": []map[string]interface{}{
					{"id": "wf-1", "name": "A Test", "active": true},
				},
			})
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/workflows/wf-1":
			if err := json.NewDecoder(r.Body).Decode(&putPayload); err != nil {
				t.Fatalf("decode put payload: %v", err)
			}
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"id":     "wf-1",
				"name":   "A Test",
				"active": true,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	svc := NewWorkflowService(config.N8NConfig{
		APIBaseURL:  server.URL + "/api/v1",
		APIKey:      "secret",
		WorkflowDir: dir,
		Timeout:     1,
	}, nil)

	result, err := svc.PushWorkflowDefinition("A-test")
	if err != nil {
		t.Fatalf("PushWorkflowDefinition returned error: %v", err)
	}
	if result.Action != "updated" || result.WorkflowID != "wf-1" {
		t.Fatalf("unexpected push result: %#v", result)
	}
	if _, ok := putPayload["active"]; ok {
		t.Fatalf("active is read-only and should not be pushed: %#v", putPayload)
	}
	if _, ok := putPayload["id"]; ok {
		t.Fatalf("id should not be pushed: %#v", putPayload)
	}
	if _, ok := putPayload["meta"]; ok {
		t.Fatalf("meta should not be pushed: %#v", putPayload)
	}
	settings, ok := putPayload["settings"].(map[string]interface{})
	if !ok || settings["executionOrder"] != "v1" {
		t.Fatalf("executionOrder default not set: %#v", putPayload["settings"])
	}
}
