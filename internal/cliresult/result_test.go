package cliresult

import (
	"bytes"
	"strings"
	"testing"
)

func TestWriteHumanFallsBackToFieldList(t *testing.T) {
	var out bytes.Buffer
	response := OK("example", struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	}{
		Name:  "demo",
		Count: 2,
	})

	if err := WriteHuman(&out, response); err != nil {
		t.Fatalf("write human: %v", err)
	}

	got := out.String()
	for _, want := range []string{
		"Data:",
		"name: demo",
		"count: 2",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("human output = %q, want %q", got, want)
		}
	}
}

func TestWriteHumanFallsBackToPrettyJSONForNestedData(t *testing.T) {
	var out bytes.Buffer
	response := OK("example", struct {
		Name  string         `json:"name"`
		Items map[string]int `json:"items"`
	}{
		Name:  "demo",
		Items: map[string]int{"one": 1},
	})

	if err := WriteHuman(&out, response); err != nil {
		t.Fatalf("write human: %v", err)
	}

	got := out.String()
	for _, want := range []string{
		"Data:",
		`"name": "demo"`,
		`"one": 1`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("human output = %q, want %q", got, want)
		}
	}
}
