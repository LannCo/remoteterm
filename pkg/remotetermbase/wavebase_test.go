// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package remotetermbase

import (
	"path/filepath"
	"testing"
)

// The caches dir must move when REMOTETERM_DATA_HOME changes; it must not fall back to an
// independent OS-cache-convention path (XDG_CACHE_HOME, Library/Caches, LOCALAPPDATA) that
// ignores the data-dir override.
func TestResolveWaveCachesDirFollowsDataHomeOverride(t *testing.T) {
	saved := DataHome_VarCache
	t.Cleanup(func() { DataHome_VarCache = saved })

	DataHome_VarCache = filepath.Join(t.TempDir(), "data-a")
	first := resolveWaveCachesDir()
	wantFirst := filepath.Join(DataHome_VarCache, "caches")
	if first != wantFirst {
		t.Fatalf("resolveWaveCachesDir() = %q, want %q", first, wantFirst)
	}

	DataHome_VarCache = filepath.Join(t.TempDir(), "data-b")
	second := resolveWaveCachesDir()
	wantSecond := filepath.Join(DataHome_VarCache, "caches")
	if second != wantSecond {
		t.Fatalf("resolveWaveCachesDir() = %q, want %q", second, wantSecond)
	}

	if first == second {
		t.Fatalf("resolveWaveCachesDir() did not change when REMOTETERM_DATA_HOME changed")
	}
}
