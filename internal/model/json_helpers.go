package model

import "encoding/json"

func parseJSONStringArray(s string) []string {
	var arr []string
	if s == "" {
		return arr
	}
	if err := json.Unmarshal([]byte(s), &arr); err != nil {
		return []string{}
	}
	return arr
}

func marshalJSONStringArray(arr []string) string {
	if arr == nil {
		return "[]"
	}
	data, err := json.Marshal(arr)
	if err != nil {
		return "[]"
	}
	return string(data)
}
