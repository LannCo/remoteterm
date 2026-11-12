// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package starter

import (
	"embed"
	"fmt"
)

const (
	AppGoFileName  = "app.go"
	AgentsFileName = "AGENTS.md"
	ClaudeFileName = "CLAUDE.md"
	GuideFileName  = "TSUNAMI_GUIDE.md"
)

//go:embed files/app.go.tmpl files/AGENTS.md files/CLAUDE.md files/TSUNAMI_GUIDE.md
var starterFS embed.FS

type StarterFile struct {
	Name string
	Data []byte
}

// app.go is stored as app.go.tmpl because a real .go file here would be compiled
// into this package.
var starterSources = []struct {
	name string
	src  string
}{
	{AppGoFileName, "files/app.go.tmpl"},
	{AgentsFileName, "files/AGENTS.md"},
	{ClaudeFileName, "files/CLAUDE.md"},
	{GuideFileName, "files/TSUNAMI_GUIDE.md"},
}

func GetStarterFiles() ([]StarterFile, error) {
	files := make([]StarterFile, 0, len(starterSources))
	for _, source := range starterSources {
		data, err := starterFS.ReadFile(source.src)
		if err != nil {
			return nil, fmt.Errorf("missing embedded starter file %s: %w", source.src, err)
		}
		files = append(files, StarterFile{Name: source.name, Data: data})
	}
	return files, nil
}
