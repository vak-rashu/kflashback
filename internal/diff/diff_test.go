// Copyright 2024 The kflashback Authors
// SPDX-License-Identifier: Apache-2.0

package diff

import (
	"encoding/json"
	"testing"
)

func TestComputeMergePatch(t *testing.T) {
	old := []byte(`{"name":"nginx","replicas":1}`)
	new := []byte(`{"name":"nginx","replicas":3}`)

	patch, err := ComputeMergePatch(old, new)
	if err != nil {
		t.Fatalf("ComputeMergePatch: %v", err)
	}

	var p map[string]interface{}
	if err := json.Unmarshal(patch, &p); err != nil {
		t.Fatalf("unmarshal patch: %v", err)
	}
	if p["replicas"] != float64(3) {
		t.Errorf("patch replicas = %v, want 3", p["replicas"])
	}
	if _, exists := p["name"]; exists {
		t.Error("patch should not contain unchanged field 'name'")
	}
}

func TestApplyMergePatch(t *testing.T) {
	doc := []byte(`{"name":"nginx","replicas":1}`)
	patch := []byte(`{"replicas":3}`)

	result, err := ApplyMergePatch(doc, patch)
	if err != nil {
		t.Fatalf("ApplyMergePatch: %v", err)
	}

	var obj map[string]interface{}
	if err := json.Unmarshal(result, &obj); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if obj["replicas"] != float64(3) {
		t.Errorf("replicas = %v, want 3", obj["replicas"])
	}
	if obj["name"] != "nginx" {
		t.Errorf("name = %v, want nginx", obj["name"])
	}
}

func TestApplyMergePatches(t *testing.T) {
	base := []byte(`{"name":"nginx","replicas":1,"image":"nginx:1.0"}`)
	patches := [][]byte{
		[]byte(`{"replicas":2}`),
		[]byte(`{"image":"nginx:2.0"}`),
		[]byte(`{"replicas":3}`),
	}

	result, err := ApplyMergePatches(base, patches)
	if err != nil {
		t.Fatalf("ApplyMergePatches: %v", err)
	}

	var obj map[string]interface{}
	_ = json.Unmarshal(result, &obj)
	if obj["replicas"] != float64(3) {
		t.Errorf("replicas = %v, want 3", obj["replicas"])
	}
	if obj["image"] != "nginx:2.0" {
		t.Errorf("image = %v, want nginx:2.0", obj["image"])
	}
}

func TestStripFields(t *testing.T) {
	input := []byte(`{
		"metadata": {
			"name": "test",
			"resourceVersion": "123",
			"managedFields": [{"manager":"kubectl"}],
			"generation": 5,
			"uid": "abc",
			"creationTimestamp": "2024-01-01T00:00:00Z",
			"labels": {"app": "test"}
		},
		"spec": {"replicas": 3},
		"status": {"ready": true}
	}`)

	result, err := StripFields(input, nil, false)
	if err != nil {
		t.Fatalf("StripFields: %v", err)
	}

	var obj map[string]interface{}
	_ = json.Unmarshal(result, &obj)

	meta := obj["metadata"].(map[string]interface{})
	if _, exists := meta["resourceVersion"]; exists {
		t.Error("resourceVersion should be stripped")
	}
	if _, exists := meta["managedFields"]; exists {
		t.Error("managedFields should be stripped")
	}
	if _, exists := meta["labels"]; !exists {
		t.Error("labels should be preserved")
	}
	if meta["name"] != "test" {
		t.Error("name should be preserved")
	}
	if _, exists := obj["status"]; exists {
		t.Error("status should be stripped when trackStatus=false")
	}
}

func TestStripFields_TrackStatus(t *testing.T) {
	input := []byte(`{"metadata":{"name":"test"},"status":{"ready":true}}`)

	result, err := StripFields(input, nil, true)
	if err != nil {
		t.Fatalf("StripFields: %v", err)
	}

	var obj map[string]interface{}
	_ = json.Unmarshal(result, &obj)
	if _, exists := obj["status"]; !exists {
		t.Error("status should be preserved when trackStatus=true")
	}
}

func TestGetChangedPaths(t *testing.T) {
	old := []byte(`{"metadata":{"name":"a"},"spec":{"replicas":1,"image":"v1"}}`)
	new := []byte(`{"metadata":{"name":"a"},"spec":{"replicas":3,"image":"v1"}}`)

	paths, err := GetChangedPaths(old, new)
	if err != nil {
		t.Fatalf("GetChangedPaths: %v", err)
	}
	if len(paths) != 1 || paths[0] != "spec.replicas" {
		t.Errorf("paths = %v, want [spec.replicas]", paths)
	}
}

func TestGetChangedPaths_AddedField(t *testing.T) {
	old := []byte(`{"spec":{"replicas":1}}`)
	new := []byte(`{"spec":{"replicas":1},"metadata":{"name":"new"}}`)

	paths, err := GetChangedPaths(old, new)
	if err != nil {
		t.Fatalf("GetChangedPaths: %v", err)
	}
	found := false
	for _, p := range paths {
		if p == "metadata" || p == "metadata.name" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected metadata in changed paths, got %v", paths)
	}
}

func TestIsEmpty(t *testing.T) {
	tests := []struct {
		name  string
		patch []byte
		want  bool
	}{
		{"nil", nil, true},
		{"empty bytes", []byte{}, true},
		{"empty object", []byte(`{}`), true},
		{"has changes", []byte(`{"replicas":3}`), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsEmpty(tt.patch); got != tt.want {
				t.Errorf("IsEmpty(%q) = %v, want %v", tt.patch, got, tt.want)
			}
		})
	}
}

func TestCompressDecompress(t *testing.T) {
	original := []byte(`{"kind":"Deployment","spec":{"replicas":3}}`)

	compressed, err := Compress(original)
	if err != nil {
		t.Fatalf("Compress: %v", err)
	}
	if len(compressed) == 0 {
		t.Fatal("compressed data is empty")
	}

	decompressed, err := Decompress(compressed)
	if err != nil {
		t.Fatalf("Decompress: %v", err)
	}
	if string(decompressed) != string(original) {
		t.Errorf("round-trip failed: got %q, want %q", decompressed, original)
	}
}

func TestDecompress_InvalidData(t *testing.T) {
	_, err := Decompress([]byte("not gzip data"))
	if err == nil {
		t.Error("expected error for invalid gzip data")
	}
}
