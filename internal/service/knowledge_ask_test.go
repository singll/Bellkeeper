package service

import (
	"strings"
	"testing"
)

func TestAskServiceValidateLayersRejectsUnknownLayer(t *testing.T) {
	svc := &AskService{}
	svc.SetAllowedLayers([]string{"archive", "vault"})

	err := svc.validateLayers([]string{"raw"})
	if err == nil || !strings.Contains(err.Error(), "invalid knowledge layer") {
		t.Fatalf("validateLayers err = %v, want invalid layer", err)
	}
}

func TestAskServiceValidateLayersAllowsConfiguredLayers(t *testing.T) {
	svc := &AskService{}
	svc.SetAllowedLayers([]string{"archive", "vault"})

	if err := svc.validateLayers([]string{"archive", "vault"}); err != nil {
		t.Fatalf("validateLayers returned error: %v", err)
	}
}
