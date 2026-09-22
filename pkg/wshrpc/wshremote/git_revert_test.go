// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package wshremote

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/LannCo/remoteterm/pkg/wshrpc"
)

type revertTestRepo struct {
	t    *testing.T
	dir  string
	file string
}

func makeRevertTestRepo(t *testing.T, lines []string) *revertTestRepo {
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	r := &revertTestRepo{t: t, dir: t.TempDir()}
	r.file = filepath.Join(r.dir, "a.txt")
	r.git("init", "-q")
	r.git("config", "user.email", "t@t")
	r.git("config", "user.name", "t")
	r.write(lines)
	r.git("add", ".")
	r.git("commit", "-qm", "init")
	return r
}

func (r *revertTestRepo) git(args ...string) string {
	cmd := exec.Command("git", args...)
	cmd.Dir = r.dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		r.t.Fatalf("git %v: %v %s", args, err, out)
	}
	return string(out)
}

func (r *revertTestRepo) write(lines []string) {
	if err := os.WriteFile(r.file, []byte(strings.Join(lines, "\n")+"\n"), 0644); err != nil {
		r.t.Fatal(err)
	}
}

func (r *revertTestRepo) hunkCount(staged bool) int {
	if staged {
		return len(parseDiffHunks(r.git("diff", "--cached", "--", "a.txt")))
	}
	return len(parseDiffHunks(r.git("diff", "--", "a.txt")))
}

func (r *revertTestRepo) revert(index int, staged bool) error {
	impl := &ServerImpl{}
	return impl.GitRevertHunkCommand(context.Background(), wshrpc.CommandGitRevertHunkData{
		Dir: r.dir, Path: "a.txt", HunkIndex: index, Staged: staged,
	})
}

func numberedLines(n int) []string {
	var lines []string
	for i := 0; i < n; i++ {
		lines = append(lines, fmt.Sprintf("line %d", i))
	}
	return lines
}

// The frontend's "Revert file" reverts every hunk one RPC at a time and the server
// re-diffs on each call, so it must go from the last hunk to the first: reverting
// hunk 0 first shifts every later hunk down one index.
func TestGitRevertHunkDescendingClearsEveryHunk(t *testing.T) {
	lines := numberedLines(80)
	r := makeRevertTestRepo(t, lines)
	for _, i := range []int{5, 25, 45, 65} {
		lines[i] += " CHANGED"
	}
	r.write(lines)

	count := r.hunkCount(false)
	if count != 4 {
		t.Fatalf("expected 4 hunks, got %d", count)
	}
	for i := count - 1; i >= 0; i-- {
		if err := r.revert(i, false); err != nil {
			t.Fatalf("revert hunk %d: %v", i, err)
		}
	}
	if left := r.hunkCount(false); left != 0 {
		t.Fatalf("%d of %d hunks survived", left, count)
	}
}

// A hunk whose removed and added line counts differ (here: 2 lines inserted) must still
// revert; the inverse patch cannot reuse the original "-a,b +c,d" header unchanged.
func TestGitRevertHunkWithUnequalLineCounts(t *testing.T) {
	lines := numberedLines(20)
	r := makeRevertTestRepo(t, lines)
	modified := append([]string{}, lines[:10]...)
	modified = append(modified, "inserted 1", "inserted 2")
	modified = append(modified, lines[10:]...)
	r.write(modified)

	if err := r.revert(0, false); err != nil {
		t.Fatalf("revert: %v", err)
	}
	if left := r.hunkCount(false); left != 0 {
		t.Fatalf("hunk survived revert")
	}
}

func TestGitRevertHunkWithUnequalLineCountsAfterEarlierHunk(t *testing.T) {
	lines := numberedLines(60)
	r := makeRevertTestRepo(t, lines)
	modified := append([]string{}, lines[:5]...)
	modified = append(modified, lines[8:40]...)
	modified = append(modified, "inserted 1", "inserted 2", "inserted 3")
	modified = append(modified, lines[40:]...)
	r.write(modified)

	if count := r.hunkCount(false); count != 2 {
		t.Fatalf("expected 2 hunks, got %d", count)
	}
	if err := r.revert(1, false); err != nil {
		t.Fatalf("revert hunk 1: %v", err)
	}
	if err := r.revert(0, false); err != nil {
		t.Fatalf("revert hunk 0: %v", err)
	}
	if left := r.hunkCount(false); left != 0 {
		t.Fatalf("%d hunks survived", left)
	}
	if got := r.git("diff", "HEAD", "--", "a.txt"); got != "" {
		t.Fatalf("file differs from HEAD after revert:\n%s", got)
	}
}
