package service

import (
	"fmt"
	"log"
)

// SyncDatasetMappings synchronizes local dataset_mappings with RAGFlow.
// For mappings with empty DatasetID, it tries to match by DisplayName against
// existing RAGFlow datasets. If no match is found, it creates a new dataset
// in RAGFlow. This eliminates the need to manually fill in RAGFlow UUIDs.
func (s *RagFlowService) SyncDatasetMappings() (*SyncResult, error) {
	result := &SyncResult{}

	// 1. Get all active local mappings
	mappings, err := s.datasetRepo.GetAll()
	if err != nil {
		return nil, fmt.Errorf("failed to list local mappings: %w", err)
	}

	// 2. List all RAGFlow datasets, build name→id lookup
	ragflowMap, err := s.buildRagFlowDatasetMap()
	if err != nil {
		return nil, fmt.Errorf("failed to list RAGFlow datasets: %w", err)
	}
	log.Printf("info: sync found %d RAGFlow datasets, %d local mappings", len(ragflowMap), len(mappings))

	// 3. For each mapping with empty DatasetID, try to match or create
	for _, m := range mappings {
		if m.DatasetID != "" {
			result.Skipped++
			continue
		}

		displayName := m.DisplayName
		if displayName == "" {
			displayName = m.Name
		}

		// Try to match by name in RAGFlow
		if rfID, ok := ragflowMap[displayName]; ok {
			if err := s.datasetRepo.UpdateDatasetID(m.ID, rfID); err != nil {
				log.Printf("warn: sync failed to update mapping %q: %v", m.Name, err)
				result.Failed++
				result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", m.Name, err))
				continue
			}
			log.Printf("info: sync matched %q -> RAGFlow dataset %s", displayName, rfID)
			result.Matched++
			continue
		}

		// No match — create dataset in RAGFlow
		params := map[string]interface{}{}
		if m.ParserID != "" {
			params["chunk_method"] = m.ParserID
		}
		createResp, err := s.CreateDataset(displayName, params)
		if err != nil {
			log.Printf("warn: sync failed to create RAGFlow dataset %q: %v", displayName, err)
			result.Failed++
			result.Errors = append(result.Errors, fmt.Sprintf("%s: create failed: %v", m.Name, err))
			continue
		}

		// Extract dataset ID from create response
		newID := extractDatasetID(createResp)
		if newID == "" {
			log.Printf("warn: sync created dataset %q but could not extract ID from response", displayName)
			result.Failed++
			result.Errors = append(result.Errors, fmt.Sprintf("%s: no ID in create response", m.Name))
			continue
		}

		if err := s.datasetRepo.UpdateDatasetID(m.ID, newID); err != nil {
			log.Printf("warn: sync created dataset %q (%s) but failed to update local mapping: %v", displayName, newID, err)
			result.Failed++
			result.Errors = append(result.Errors, fmt.Sprintf("%s: update failed: %v", m.Name, err))
			continue
		}
		log.Printf("info: sync created RAGFlow dataset %q -> %s", displayName, newID)
		result.Created++
	}

	log.Printf("info: sync complete — matched:%d created:%d skipped:%d failed:%d",
		result.Matched, result.Created, result.Skipped, result.Failed)
	return result, nil
}

// SyncResult holds the outcome of a dataset sync operation.
type SyncResult struct {
	Matched int      `json:"matched"`
	Created int      `json:"created"`
	Skipped int      `json:"skipped"`
	Failed  int      `json:"failed"`
	Errors  []string `json:"errors,omitempty"`
}

// buildRagFlowDatasetMap fetches all RAGFlow datasets and returns a name→id map.
func (s *RagFlowService) buildRagFlowDatasetMap() (map[string]string, error) {
	resp, err := s.ListDatasets(1, 1000)
	if err != nil {
		return nil, err
	}

	result := make(map[string]string)

	dataRaw, ok := resp["data"]
	if !ok {
		return result, nil
	}

	datasets, ok := dataRaw.([]interface{})
	if !ok {
		return result, nil
	}

	for _, d := range datasets {
		ds, ok := d.(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := ds["name"].(string)
		id, _ := ds["id"].(string)
		if name != "" && id != "" {
			result[name] = id
		}
	}
	return result, nil
}

// extractDatasetID extracts the dataset ID from a RAGFlow create response.
func extractDatasetID(resp map[string]interface{}) string {
	// RAGFlow returns {"code":0,"data":{"id":"...","name":"...",...}}
	dataRaw, ok := resp["data"]
	if !ok {
		return ""
	}
	switch data := dataRaw.(type) {
	case map[string]interface{}:
		id, _ := data["id"].(string)
		return id
	case []interface{}:
		if len(data) > 0 {
			if item, ok := data[0].(map[string]interface{}); ok {
				id, _ := item["id"].(string)
				return id
			}
		}
	}
	return ""
}
