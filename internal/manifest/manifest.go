// Package manifest embeds the agent-first-cli principles document and exposes
// it as a parsed-once typed value for every later consumer.
package manifest

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

//go:embed principles.json
var raw []byte

type Manifest struct {
	Name         string      `json:"name"`
	Version      string      `json:"version"`
	LastModified string      `json:"last_modified"`
	URL          string      `json:"url"`
	Description  string      `json:"description"`
	Principles   []Principle `json:"principles"`
}

type Principle struct {
	Number       int    `json:"number"`
	Title        string `json:"title"`
	Tagline      string `json:"tagline"`
	Category     string `json:"category"`
	LastModified string `json:"last_modified"`
	URL          string `json:"url"`
	MarkdownURL  string `json:"markdown_url"`
}

func (p Principle) PrincipleID() string {
	return fmt.Sprintf("P%d", p.Number)
}

var Embedded = mustLoad()

func mustLoad() *Manifest {
	var m Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		panic(fmt.Sprintf("manifest: failed to parse embedded principles.json: %v", err))
	}
	if len(m.Principles) != 16 {
		panic(fmt.Sprintf("manifest: expected 16 principles in embedded principles.json, got %d", len(m.Principles)))
	}
	return &m
}
