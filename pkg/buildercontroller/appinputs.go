// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package buildercontroller

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"strings"

	"github.com/LannCo/remoteterm/pkg/remotetermappstore"
)

// IsRelevantAppPath is shared by the watcher and the input hash so that what triggers
// a rebuild and what counts as "already built" can never disagree.
func IsRelevantAppPath(rel string) bool {
	rel = filepath.ToSlash(rel)
	if rel == "" || rel == "." {
		return false
	}
	parts := strings.Split(rel, "/")
	for _, part := range parts {
		if part == "" || strings.HasPrefix(part, ".") || part == "node_modules" {
			return false
		}
	}
	base := parts[len(parts)-1]
	if isEditorTempName(base) {
		return false
	}
	if len(parts) == 1 {
		return strings.HasSuffix(base, ".go")
	}
	if parts[0] != "static" {
		return false
	}
	return rel != "static/tw.css"
}

// Editors write backups and swap files next to the real file, and vim probes
// writability with a numeric name (4913); none of these are app inputs.
func isEditorTempName(name string) bool {
	for _, suffix := range []string{"~", ".swp", ".swx", ".tmp"} {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}
	for _, r := range name {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func shouldSkipAppDir(rel string) bool {
	base := path.Base(rel)
	if strings.HasPrefix(base, ".") || base == "node_modules" {
		return true
	}
	return !strings.Contains(rel, "/") && rel != "static"
}

// Root .go files are hashed by content: they are small, and a save that rewrites the
// same bytes must not look like a change. Files under static/ can be large media, so
// they are hashed by path, size and modification time and their contents never read.
// Everything is read through an os.Root, so a symlink or FIFO planted by an agent can
// neither redirect the walk nor stall it.
func ComputeAppInputHash(appDir string) (string, error) {
	root, err := remotetermappstore.OpenAppRoot(appDir)
	if err != nil {
		return "", fmt.Errorf("cannot scan app folder %s: %w", appDir, err)
	}
	defer root.Close()
	hasher := sha256.New()
	err = fs.WalkDir(root.FS(), ".", func(rel string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if rel == "." {
				return walkErr
			}
			return nil
		}
		if rel == "." {
			return nil
		}
		if d.IsDir() {
			if shouldSkipAppDir(rel) {
				return fs.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() || !IsRelevantAppPath(rel) {
			return nil
		}
		if strings.HasPrefix(rel, "static/") {
			info, err := root.Lstat(rel)
			if err != nil || !info.Mode().IsRegular() {
				return nil
			}
			fmt.Fprintf(hasher, "%s\x00%d\x00%d\x00", rel, info.Size(), info.ModTime().UnixNano())
			return nil
		}
		data, err := remotetermappstore.ReadAppRootFile(root, rel, remotetermappstore.MaxAppFileReadSize)
		if err != nil {
			return nil
		}
		fmt.Fprintf(hasher, "%s\x00%d\x00", rel, len(data))
		hasher.Write(data)
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("cannot scan app folder %s: %w", appDir, err)
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}
