# Builder by Hand Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the Tsunami app builder produce a running app with no built-in AI: bundled SDK, reliable Go discovery, starter files, live rebuild from outside edits, terminal and folder actions, and safe app-folder handling.

**Architecture:** The Go side changes live in `tsunami/build` (SDK bundle, Go floor, Go discovery, go.mod generation), `pkg/remotetermapputil` (SDK path and pre-build checks), `pkg/remotetermappstore` (safe paths, secret bindings, seeding, embedded starter files) and `pkg/buildercontroller` (coalesced rebuilds, build log, fsnotify watcher). New RPCs go through `pkg/wshrpc/wshrpctypes_builder.go` and `pkg/wshrpc/wshserver/wshserver.go`, then `npx task generate`. The frontend changes are confined to `frontend/builder/**`, plus three Electron IPCs in `emain/`.

**Tech Stack:** Go 1.25.6+ (stdlib `go/version`; `golang.org/x/mod/modfile` and `github.com/fsnotify/fsnotify` v1.9.0 are already dependencies), Electron, React 19, Jotai, Tailwind v4, vitest 3, go-task (`npx task`, from `node_modules/.bin/task`).

**Spec:** `/media/owner/Workspace/remoteterm/remoteterm-builder-by-hand/.planning/builder-by-hand/SPEC.md` (v2). Read it alongside this plan; where this plan differs, the "Spec deviations" section at the end says why.

## Global Constraints

Every task's requirements include this section.

- Repo root: `/media/owner/Workspace/remoteterm/remoteterm-builder-by-hand` (branch `feat/builder-by-hand`). Read `CLAUDE.md` and `.kilocode/rules/rules.md` before writing code.
- Go: string constants, never custom enum types; constructors named `Make...`/`make...`, never `New...`; consts at the top of the file; `Printf` not `Println`; locking via small helpers using `lock.Lock(); defer lock.Unlock()`; early returns.
- Comments explain why, never what. Never remove existing comments.
- TypeScript: Jotai singleton models (atoms on the model, `globalStore.get/set`), `@/` imports across directories, named exports only, 4-space indent, `cursor-pointer` on every clickable, hooks at the top of components, `== null` not `=== undefined`. Accent buttons use `bg-accent/80 text-onaccent rounded hover:bg-accent transition-colors cursor-pointer`. Never `cursor-help` or `cursor-not-allowed`.
- New files carry `// Copyright 2026, Command Line Inc.` and `// SPDX-License-Identifier: Apache-2.0`.
- Generated files (`frontend/types/gotypes.d.ts`, `frontend/types/remotetermevent.d.ts`, `frontend/app/store/wshclientapi.ts`, `pkg/wshrpc/wshclient/wshclient.go`, `pkg/rtconfig/metaconsts.go`, `schema/settings.json`) are never hand-edited. After changing RPC types, settings or WPS events, run `npx task generate` from the repo root (`task` is not on PATH; `npx task` uses `node_modules/.bin/task`, which uses the repo's `golang-1.26.2/bin/go`).
- Go floor: read from the effective SDK `go.mod` `go` directive (today `go 1.25.6`, `tsunami/go.mod:3`). Every builder `go` invocation gets `GOTOOLCHAIN=local`.
- Go discovery bounds (spec D2): shells `bash`, `zsh`, `fish` run with `-l -i -c`; `sh`, `dash`, `ksh` with `-l -c`; anything else skipped; script `command -v go`; stdin `/dev/null`; own session (`Setsid`; the spec says `Setpgid`, see Spec deviations), so the process group id equals the shell's pid; 3 s timeout kills the group; `cmd.WaitDelay` 1 s; stdout capped at 64 KiB; accept only the last non-empty line, an absolute path to an existing regular executable file; success cached for the process lifetime; failure cached 30 s; GOROOT canonicalisation via `<go> env GOROOT` with a 2 s timeout and `GOTOOLCHAIN=local`.
- Watcher bounds (spec D4): at most 8 watchers in total, at most 1000 watched directories, 300 ms trailing debounce, root poll every 1 s, "unavailable" after 10 s.
- Read cap for app files: 2 MiB.
- Secret bindings path: `<data dir>/builder/secret-bindings/<ns>/<name>.json`.
- Build log: `<app>/.tsunami/build.log`, truncated per build.
- Setting `builder:liverebuild`: bool, default `false`. Toggle label: `Rebuild on external changes`.
- Events: `rtapp:appgoupdated` scoped to the app id; `rtapp:watchstatus` scoped to the builder oref (`builder:<id>`), status `active` or `unavailable` with a reason.
- D6 messages, verbatim (`<min>`, `<found>`, `<path>` substituted):
  - `Go toolchain not found. Install Go <min> or newer, or set "tsunami:gopath" in Settings to the full path of the go binary.`
  - `Go <found> is older than <min>, which the Tsunami SDK requires. Install a newer Go, or set "tsunami:gopath" to one.`
  - `Tsunami SDK not found at <path>. Rebuild with "task build:tsunamisdk", or set "tsunami:sdkreplacepath" in Settings.`
  - `Tsunami scaffold not found at <path>. Rebuild with "task build:tsunamiscaffold", or set "tsunami:scaffoldpath" in Settings.`
- UI copy, verbatim: `Changed on disk`, `Rebuild`, `app.go changed on disk.`, `Load disk version`, `Keep my edits`, `app.go is missing`, `Create starter app`, `Open terminal`, `Open folder`, `Rebuild on external changes`.
- Starter files: `AGENTS.md` under 120 lines; `CLAUDE.md` is the single line `@AGENTS.md`; no shell commands, no `go get`, no network instructions in either.
- Safety, binding on every task: never start the app, Electron, `task dev`, `npm run dev` or anything on a display. Never write to `~/.config`, `~/.local/share`, `~/waveapps` or dotfiles. Tests use `t.TempDir()` and `t.Setenv("HOME", ...)`. Do not run `go build` to check compilation; `go test` and `go vet` on specific packages are fine (the starter compile test in Task 7 runs `go build` inside a temp module because the spec requires that test).
- Test commands run from the repo root: `go test ./pkg/<pkg>/...`, `cd tsunami && go test ./build/...` (tsunami is a separate module), `npx vitest run <path>`, `npx tsc --noEmit`.
- Commits: stage only the files the task names (`git add <paths>`). The worktree has untracked `golang-1.26.2` and `zig-0.14.0`; never add them. End every commit message with the line `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.
- Prose in British English. No em-dashes anywhere (use `-`, commas, colons or parentheses).
- If a step's expected output differs from what you see, stop and report the difference with the command output rather than adapting silently. If you cannot verify a claim (for example, an SDK identifier in Task 7), say so in your report instead of asserting it.

## Review Focus

Five conditions the spec implies that no task's spec-derived tests exercised; each has a pinning test in the named task.

1. Editors and agents that save by writing a temp file and renaming it over `app.go`: one change, one rebuild, temp names ignored. Test: Task 10, `TestWatcherAtomicRenameSaveFiresOnce`.
2. An SDK path containing a space (Windows `C:\Program Files\...`) and a stale `replace` from a previous AppImage mount in the app's `go.mod`: exactly one replace, pointing at the current path, correctly quoted. Test: Task 4, `TestMakeGoModContentRewritesStaleReplace` and `TestMakeGoModContentQuotesPathWithSpace`.
3. GUI launches with `$SHELL` unset, relative, pointing at a missing file, or at a shell outside the allowlist: discovery fails within a second without running any shell (the resulting D6 "not found" text is pinned in Task 4). Test: Task 3, `TestProbeSkipsUnsetMissingAndUnknownShells`.
4. The build log cannot be written (for example `.tsunami` is a file, or the folder is read-only): the build outcome and status are unchanged. Test: Task 9, `TestBuildLogFailureDoesNotChangeOutcome`.
5. An app folder with more directories under `static/` than the watch cap (or an exhausted inotify limit): live reload reports unavailable with a reason and the watcher slot is released, so the Code-tab save path is unaffected. Test: Task 10, `TestStartWatchingReportsUnavailableOverDirCap`.

---

### Task 1: Bundled SDK (`CopySdkBundle`, `sdkbundle` CLI, Taskfile, packaging, `GetTsunamiSdkPath`)

**Files:**
- Create: `tsunami/build/sdkbundle.go`
- Create: `tsunami/build/sdkbundle_test.go`
- Modify: `tsunami/cmd/main-tsunami.go:127-171` (add `sdkBundleCmd`, register in `init()`)
- Modify: `Taskfile.yml:26-30` (`electron:dev` deps), `Taskfile.yml:129-136` (`package` deps), insert new task after `build:tsunamiscaffold` (ends at line 366)
- Modify: `electron-builder.config.cjs:25` (files filter) and `:34-39` (`extraResources`)
- Modify: `pkg/remotetermapputil/waveapputil.go` (add `TsunamiSdkDirName`, `GetTsunamiSdkPath`)
- Create: `pkg/remotetermapputil/waveapputil_test.go`
- Create: `frontend/builder/tsunamisdk-packaging.test.ts`

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces:
  - `build.SdkBundlePackageDirs []string` = `{"app","engine","vdom","rpctypes","util","tsunamibase","ui"}`
  - `build.CopySdkBundle(srcDir string, dstDir string) error`
  - test helper `writeTestFile(t *testing.T, path string, content string)` in `tsunami/build/sdkbundle_test.go` (package `build`), reused by Tasks 2-4
  - `remotetermapputil.TsunamiSdkDirName = "tsunamisdk"` and `remotetermapputil.GetTsunamiSdkPath() string` returning `<resources>/tsunamisdk`
  - Taskfile task `build:tsunamisdk` writing `dist/tsunamisdk/`

- [ ] **Step 1: Write the failing Go tests**

Create `tsunami/build/sdkbundle_test.go`:

```go
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package build

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"testing"
)

func writeTestFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func listBundleFiles(t *testing.T, root string) []string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(files)
	return files
}

func TestCopySdkBundleCopiesOnlyRuntimePackages(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink fixtures need a Unix filesystem")
	}
	src := t.TempDir()
	writeTestFile(t, filepath.Join(src, "go.mod"), "module github.com/LannCo/remoteterm/tsunami\n\ngo 1.25.6\n")
	writeTestFile(t, filepath.Join(src, "go.sum"), "")
	writeTestFile(t, filepath.Join(src, "app", "app.go"), "package app\n")
	writeTestFile(t, filepath.Join(src, "app", "app_test.go"), "package app\n")
	writeTestFile(t, filepath.Join(src, "app", "sub", "sub.go"), "package sub\n")
	writeTestFile(t, filepath.Join(src, "engine", "engine.go"), "package engine\n")
	writeTestFile(t, filepath.Join(src, "engine", "render.md"), "notes\n")
	for _, dir := range []string{"vdom", "rpctypes", "util", "tsunamibase", "ui"} {
		writeTestFile(t, filepath.Join(src, dir, dir+".go"), "package "+dir+"\n")
	}
	for _, dir := range []string{"build", "cmd", "demo/todo", "frontend", "templates"} {
		writeTestFile(t, filepath.Join(src, dir, "x.go"), "package x\n")
	}
	if err := os.Symlink(filepath.Join(src, "app", "app.go"), filepath.Join(src, "app", "link.go")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(src, "build"), filepath.Join(src, "app", "linkdir")); err != nil {
		t.Fatal(err)
	}

	dst := filepath.Join(t.TempDir(), "tsunamisdk")
	if err := CopySdkBundle(src, dst); err != nil {
		t.Fatalf("CopySdkBundle: %v", err)
	}

	want := []string{
		"app/app.go",
		"app/sub/sub.go",
		"engine/engine.go",
		"go.mod",
		"go.sum",
		"rpctypes/rpctypes.go",
		"tsunamibase/tsunamibase.go",
		"ui/ui.go",
		"util/util.go",
		"vdom/vdom.go",
	}
	got := listBundleFiles(t, dst)
	if !slices.Equal(got, want) {
		t.Fatalf("bundle files:\n got %v\nwant %v", got, want)
	}
}

func TestCopySdkBundleRequiresGoMod(t *testing.T) {
	src := t.TempDir()
	for _, dir := range SdkBundlePackageDirs {
		writeTestFile(t, filepath.Join(src, dir, dir+".go"), "package "+dir+"\n")
	}
	err := CopySdkBundle(src, filepath.Join(t.TempDir(), "out"))
	if err == nil || !strings.Contains(err.Error(), "go.mod") {
		t.Fatalf("expected an error naming go.mod, got %v", err)
	}
}

func TestCopySdkBundleFromRepoSdk(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "tsunamisdk")
	if err := CopySdkBundle("..", dst); err != nil {
		t.Fatalf("CopySdkBundle(..): %v", err)
	}
	for _, rel := range []string{"go.mod", "go.sum", "app/defaultclient.go", "vdom/vdom.go", "engine/clientimpl.go"} {
		if _, err := os.Stat(filepath.Join(dst, rel)); err != nil {
			t.Errorf("expected %s in bundle: %v", rel, err)
		}
	}
	for _, rel := range []string{"build", "cmd", "demo", "frontend", "templates"} {
		if _, err := os.Stat(filepath.Join(dst, rel)); err == nil {
			t.Errorf("%s must not be in the bundle", rel)
		}
	}
	for _, rel := range listBundleFiles(t, dst) {
		if strings.HasSuffix(rel, "_test.go") {
			t.Errorf("test file %s must not be in the bundle", rel)
		}
	}
}
```

Create `pkg/remotetermapputil/waveapputil_test.go`:

```go
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package remotetermapputil

import (
	"path/filepath"
	"testing"

	"github.com/LannCo/remoteterm/pkg/remotetermbase"
)

func setResourcesPath(t *testing.T, path string) {
	t.Helper()
	orig := remotetermbase.AppResourcesPath_VarCache
	remotetermbase.AppResourcesPath_VarCache = path
	t.Cleanup(func() { remotetermbase.AppResourcesPath_VarCache = orig })
}

func TestGetTsunamiSdkPath(t *testing.T) {
	resources := t.TempDir()
	setResourcesPath(t, resources)
	want := filepath.Join(resources, "tsunamisdk")
	if got := GetTsunamiSdkPath(); got != want {
		t.Fatalf("GetTsunamiSdkPath() = %q, want %q", got, want)
	}
}
```

- [ ] **Step 2: Run the Go tests to verify they fail**

Run: `cd tsunami && go test ./build/... -run CopySdkBundle -v`
Expected: FAIL, `undefined: CopySdkBundle` and `undefined: SdkBundlePackageDirs`.

Run: `go test ./pkg/remotetermapputil/... -run GetTsunamiSdkPath -v`
Expected: FAIL, `undefined: GetTsunamiSdkPath`.

- [ ] **Step 3: Implement `CopySdkBundle`**

Create `tsunami/build/sdkbundle.go`:

```go
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package build

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Only the packages an app build imports go into the bundle; build/, cmd/, demo/,
// frontend/ and templates/ belong to the toolchain side and stay out.
var SdkBundlePackageDirs = []string{"app", "engine", "vdom", "rpctypes", "util", "tsunamibase", "ui"}

// CopySdkBundle is the single definition of the bundle contents: the Taskfile, the
// packaged app and the starter compile test all go through it, so they cannot drift.
func CopySdkBundle(srcDir string, dstDir string) error {
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		return fmt.Errorf("failed to create SDK bundle directory %s: %w", dstDir, err)
	}
	if err := copyBundleFile(filepath.Join(srcDir, "go.mod"), filepath.Join(dstDir, "go.mod"), true); err != nil {
		return err
	}
	if err := copyBundleFile(filepath.Join(srcDir, "go.sum"), filepath.Join(dstDir, "go.sum"), false); err != nil {
		return err
	}
	for _, pkgDir := range SdkBundlePackageDirs {
		if err := copyBundlePackageDir(srcDir, dstDir, pkgDir); err != nil {
			return err
		}
	}
	return nil
}

func copyBundlePackageDir(srcDir string, dstDir string, pkgDir string) error {
	root := filepath.Join(srcDir, pkgDir)
	info, err := os.Lstat(root)
	if err != nil {
		return fmt.Errorf("SDK package directory %s: %w", root, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("SDK package path %s is not a directory", root)
	}
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || !d.Type().IsRegular() {
			return nil
		}
		name := d.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			return nil
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		return copyBundleFile(path, filepath.Join(dstDir, rel), true)
	})
}

func copyBundleFile(srcPath string, dstPath string, required bool) error {
	data, err := os.ReadFile(srcPath)
	if err != nil {
		if !required && os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read %s: %w", srcPath, err)
	}
	if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
		return fmt.Errorf("failed to create directory for %s: %w", dstPath, err)
	}
	if err := os.WriteFile(dstPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write %s: %w", dstPath, err)
	}
	return nil
}
```

`filepath.WalkDir` never follows symlinked directories, and the `!d.Type().IsRegular()` check skips symlinked files, so symlinks never reach the bundle.

- [ ] **Step 4: Implement `GetTsunamiSdkPath`**

In `pkg/remotetermapputil/waveapputil.go`, change the const line (line 19) and add the function after `GetTsunamiScaffoldPath` (ends line 28):

```go
const (
	DefaultTsunamiSdkVersion = "v0.12.4"
	TsunamiSdkDirName        = "tsunamisdk"
)
```

```go
func GetTsunamiSdkPath() string {
	return filepath.Join(remotetermbase.GetWaveAppResourcesPath(), TsunamiSdkDirName)
}
```

- [ ] **Step 5: Run the Go tests to verify they pass**

Run: `cd tsunami && go test ./build/... -run CopySdkBundle -v`
Expected: PASS for all three tests.

Run: `go test ./pkg/remotetermapputil/... -v`
Expected: PASS.

- [ ] **Step 6: Add the `sdkbundle` CLI subcommand**

In `tsunami/cmd/main-tsunami.go`, add after `packageCmd` (ends line 153):

```go
var sdkBundleCmd = &cobra.Command{
	Use:          "sdkbundle [dstdir]",
	Short:        "Copy the Tsunami SDK runtime packages into a directory",
	Long:         `Copy go.mod, go.sum and the runtime packages of the Tsunami SDK into a directory that app builds can use as a replace target.`,
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	Run: func(cmd *cobra.Command, args []string) {
		srcDir, _ := cmd.Flags().GetString("src")
		if err := build.CopySdkBundle(srcDir, args[0]); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
	},
}
```

In `init()`, after `rootCmd.AddCommand(packageCmd)` (line 170):

```go
	sdkBundleCmd.Flags().String("src", ".", "Tsunami SDK source directory (the one containing go.mod)")
	rootCmd.AddCommand(sdkBundleCmd)
```

- [ ] **Step 7: Add the Taskfile task and wire it**

In `Taskfile.yml`, insert after the `build:tsunamiscaffold` task (after its `generates:` block, line 366):

```yaml
    build:tsunamisdk:
        desc: Copy the Tsunami SDK runtime packages to the dist directory.
        cmds:
            - cmd: "{{.RMRF}} dist/tsunamisdk"
              ignore_error: true
            - "{{.GO}} run -C tsunami ./cmd sdkbundle --src {{.ROOT_DIR}}/tsunami {{.ROOT_DIR}}/dist/tsunamisdk"
        sources:
            - "tsunami/go.mod"
            - "tsunami/go.sum"
            - "tsunami/**/*.go"
        generates:
            - "dist/tsunamisdk/**/*"
```

Add `- build:tsunamisdk` directly after `- build:tsunamiscaffold` in the `deps` of `electron:dev` (line 30) and of `package` (line 136). Those are the only two places `build:tsunamiscaffold` is wired (`grep -n 'build:tsunamiscaffold' Taskfile.yml` confirms lines 30, 136 and the task itself at 353).

- [ ] **Step 8: Add the electron-builder entries**

In `electron-builder.config.cjs`, line 25, extend the filter:

```js
            filter: [
                "**/*",
                "!bin/*",
                "bin/remotetermsrv.${arch}*",
                "bin/wsh*",
                "!tsunamiscaffold/**/*",
                "!tsunamisdk/**/*",
            ],
```

and extend `extraResources` (lines 34-39):

```js
    extraResources: [
        {
            from: "dist/tsunamiscaffold",
            to: "tsunamiscaffold",
        },
        {
            from: "dist/tsunamisdk",
            to: "tsunamisdk",
        },
    ],
```

- [ ] **Step 9: Write the packaging config test**

Create `frontend/builder/tsunamisdk-packaging.test.ts`:

```ts
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import * as fs from "fs";
import { createRequire } from "module";
import * as path from "path";
import { describe, expect, it } from "vitest";

const RepoRoot = process.cwd();

function taskBlock(taskfile: string, taskName: string): string {
    const start = taskfile.indexOf(`\n    ${taskName}:\n`);
    if (start < 0) {
        return "";
    }
    const rest = taskfile.slice(start + 1);
    const next = rest.slice(1).search(/\n    [a-z][^\n]*:\n/);
    return next < 0 ? rest : rest.slice(0, next + 1);
}

describe("tsunami SDK packaging", () => {
    it("ships dist/tsunamisdk as an extra resource and keeps it out of the asar", () => {
        const require = createRequire(import.meta.url);
        const config = require(path.join(RepoRoot, "electron-builder.config.cjs"));
        expect(config.extraResources).toContainEqual({ from: "dist/tsunamisdk", to: "tsunamisdk" });
        const distEntry = config.files.find((entry: any) => entry?.from === "./dist");
        expect(distEntry.filter).toContain("!tsunamisdk/**/*");
    });

    it("builds the SDK bundle wherever the scaffold is built", () => {
        const taskfile = fs.readFileSync(path.join(RepoRoot, "Taskfile.yml"), "utf8");
        expect(taskBlock(taskfile, "build:tsunamisdk")).toContain("sdkbundle");
        for (const name of ["electron:dev", "package"]) {
            const block = taskBlock(taskfile, name);
            expect(block).toContain("- build:tsunamiscaffold");
            expect(block).toContain("- build:tsunamisdk");
        }
    });
});
```

- [ ] **Step 10: Run the packaging test**

Run: `npx vitest run frontend/builder/tsunamisdk-packaging.test.ts`
Expected: PASS (2 tests). If `require` of the config fails because `electron-builder` cannot be resolved, report it; do not replace the require with a text match.

- [ ] **Step 11: Run the Taskfile task once**

Run: `npx task build:tsunamisdk && test -f dist/tsunamisdk/go.mod && test -f dist/tsunamisdk/app/defaultclient.go && test ! -e dist/tsunamisdk/build && echo BUNDLE-OK`
Expected: `BUNDLE-OK`. (`dist/` is gitignored; nothing to stage from it.)

- [ ] **Step 12: Commit**

```bash
git add tsunami/build/sdkbundle.go tsunami/build/sdkbundle_test.go tsunami/cmd/main-tsunami.go Taskfile.yml electron-builder.config.cjs pkg/remotetermapputil/waveapputil.go pkg/remotetermapputil/waveapputil_test.go frontend/builder/tsunamisdk-packaging.test.ts
git commit -m "feat(builder): bundle the Tsunami SDK as an app resource

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 2: Go floor from the SDK `go.mod` and `GOTOOLCHAIN=local`

**Files:**
- Create: `tsunami/build/goversion.go`
- Create: `tsunami/build/goversion_test.go`
- Modify: `tsunami/build/build.go:31` (const), `:92-106` (`BuildOpts`), `:127-132` (`GoVersionCheckResult`), `:171-251` (`CheckGoVersion`), `:267-294` (`verifyEnvironment` Go section), `:420` (tidy env), `:761` (build env)
- Modify: `pkg/wshrpc/wshserver/wshserver.go:1235` (`CheckGoVersion` call site)

**Interfaces:**
- Consumes: `writeTestFile` (Task 1, `tsunami/build/sdkbundle_test.go`).
- Produces:
  - consts `build.GoStatus_Ok = "ok"`, `GoStatus_NotFound = "notfound"`, `GoStatus_BadVersion = "badversion"`, `GoStatus_Error = "error"`, `build.DefaultMinGoVersion = "1.22"`
  - `build.ReadSdkGoVersion(sdkDir string) (string, error)`: the `go` directive of `<sdkDir>/go.mod`, e.g. `"1.25.6"`
  - `build.ParseGoVersionOutput(out string) (string, bool)`: `"go version go1.26.3 linux/amd64"` gives `"1.26.3", true`
  - `build.CompareGoVersions(a string, b string) int`: accepts `1.25.6` or `go1.25.6` forms; -1, 0, +1
  - unexported `goCmdEnv() []string` (environment with `GOTOOLCHAIN=local`)
  - `build.CheckGoVersion(customGoPath string, minGoVersion string) GoVersionCheckResult` (new second parameter; `""` means `DefaultMinGoVersion`); a set but missing `customGoPath` returns `GoStatus_NotFound`
  - `GoVersionCheckResult.Version string` (parsed version, e.g. `"1.26.3"`)
  - `BuildOpts.MinGoVersion string`
  - `BuildEnv.GoVersion` now holds the full version (`"1.26.3"`), not the minor (`"1.26"`)

- [ ] **Step 1: Write the failing tests**

Create `tsunami/build/goversion_test.go`:

```go
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package build

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func writeFakeGo(t *testing.T, path string, versionLine string, markerPath string) {
	t.Helper()
	script := "#!/bin/sh\n"
	if markerPath != "" {
		script += "printf '%s' \"$GOTOOLCHAIN\" > '" + markerPath + "'\n"
	}
	script += "if [ \"$1\" = version ]; then echo '" + versionLine + "'; exit 0; fi\nexit 1\n"
	writeTestFile(t, path, script)
	if err := os.Chmod(path, 0755); err != nil {
		t.Fatal(err)
	}
}

func TestReadSdkGoVersion(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "go.mod"), "module github.com/LannCo/remoteterm/tsunami\n\ngo 1.25.6\n")
	got, err := ReadSdkGoVersion(dir)
	if err != nil || got != "1.25.6" {
		t.Fatalf("ReadSdkGoVersion = %q, %v; want 1.25.6", got, err)
	}

	noGoLine := t.TempDir()
	writeTestFile(t, filepath.Join(noGoLine, "go.mod"), "module example.com/x\n")
	if _, err := ReadSdkGoVersion(noGoLine); err == nil {
		t.Fatal("expected an error for a go.mod without a go directive")
	}

	if _, err := ReadSdkGoVersion(t.TempDir()); err == nil {
		t.Fatal("expected an error for a missing go.mod")
	}

	repo, err := ReadSdkGoVersion("..")
	if err != nil || repo == "" {
		t.Fatalf("ReadSdkGoVersion(..) = %q, %v", repo, err)
	}
}

func TestCompareGoVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.25.5", "1.25.6", -1},
		{"1.26.0", "1.25.6", 1},
		{"1.25.6", "1.25.6", 0},
		{"go1.25rc1", "1.25.6", -1},
		{"1.25rc1", "1.25.0", -1},
		{"1.9", "1.10", -1},
		{"go1.26.1", "go1.26.0", 1},
	}
	for _, tc := range cases {
		if got := CompareGoVersions(tc.a, tc.b); got != tc.want {
			t.Errorf("CompareGoVersions(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestParseGoVersionOutput(t *testing.T) {
	cases := map[string]string{
		"go version go1.26.3 linux/amd64":          "1.26.3",
		"go version go1.25rc1 darwin/arm64":        "1.25rc1",
		"go version go1.22 linux/amd64":            "1.22",
		"go version devel go1.27-abcdef Tue linux": "1.27",
	}
	for in, want := range cases {
		got, ok := ParseGoVersionOutput(in)
		if !ok || got != want {
			t.Errorf("ParseGoVersionOutput(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}
	if _, ok := ParseGoVersionOutput("not a version"); ok {
		t.Error("expected ok=false for unparseable output")
	}
}

func TestCheckGoVersionAgainstFloor(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake go uses a shell script")
	}
	dir := t.TempDir()
	marker := filepath.Join(dir, "gotoolchain")
	newGo := filepath.Join(dir, "new", "go")
	writeFakeGo(t, newGo, "go version go1.26.0 linux/amd64", marker)
	res := CheckGoVersion(newGo, "1.25.6")
	if res.GoStatus != GoStatus_Ok || res.Version != "1.26.0" {
		t.Fatalf("new go: status %q version %q (%s)", res.GoStatus, res.Version, res.ErrorString)
	}
	toolchain, err := os.ReadFile(marker)
	if err != nil || string(toolchain) != "local" {
		t.Fatalf("GOTOOLCHAIN seen by go = %q, %v; want local", toolchain, err)
	}

	oldGo := filepath.Join(dir, "old", "go")
	writeFakeGo(t, oldGo, "go version go1.25.5 linux/amd64", "")
	res = CheckGoVersion(oldGo, "1.25.6")
	if res.GoStatus != GoStatus_BadVersion || res.Version != "1.25.5" {
		t.Fatalf("old go: status %q version %q", res.GoStatus, res.Version)
	}

	res = CheckGoVersion(filepath.Join(dir, "missing", "go"), "1.25.6")
	if res.GoStatus != GoStatus_NotFound {
		t.Fatalf("missing custom go: status %q, want notfound", res.GoStatus)
	}
}

func TestVerifyEnvironmentUsesMinGoVersion(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake go uses a shell script")
	}
	oldGo := filepath.Join(t.TempDir(), "go")
	writeFakeGo(t, oldGo, "go version go1.25.5 linux/amd64", "")
	_, err := verifyEnvironment(false, BuildOpts{SdkReplacePath: "/sdk", GoPath: oldGo, MinGoVersion: "1.25.6"})
	if err == nil || !strings.Contains(err.Error(), "1.25.6") {
		t.Fatalf("expected an error naming 1.25.6, got %v", err)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd tsunami && go test ./build/... -run 'ReadSdkGoVersion|CompareGoVersions|ParseGoVersionOutput|CheckGoVersionAgainstFloor|VerifyEnvironmentUsesMinGoVersion' -v`
Expected: FAIL to compile: `undefined: ReadSdkGoVersion`, `undefined: CompareGoVersions`, `undefined: ParseGoVersionOutput`, `undefined: GoStatus_Ok`, `too many arguments in call to CheckGoVersion`, `unknown field MinGoVersion`.

- [ ] **Step 3: Implement `goversion.go`**

Create `tsunami/build/goversion.go`:

```go
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package build

import (
	"fmt"
	"go/version"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/mod/modfile"
)

var goVersionOutputRe = regexp.MustCompile(`go(1\.\d+(?:\.\d+)?(?:(?:rc|beta)\d+)?)`)

func ReadSdkGoVersion(sdkDir string) (string, error) {
	goModPath := filepath.Join(sdkDir, "go.mod")
	data, err := os.ReadFile(goModPath)
	if err != nil {
		return "", fmt.Errorf("failed to read %s: %w", goModPath, err)
	}
	modFile, err := modfile.ParseLax(goModPath, data, nil)
	if err != nil {
		return "", fmt.Errorf("failed to parse %s: %w", goModPath, err)
	}
	if modFile.Go == nil || modFile.Go.Version == "" {
		return "", fmt.Errorf("%s has no go directive", goModPath)
	}
	return modFile.Go.Version, nil
}

func ParseGoVersionOutput(out string) (string, bool) {
	matches := goVersionOutputRe.FindStringSubmatch(out)
	if len(matches) < 2 {
		return "", false
	}
	return matches[1], true
}

// go/version understands release candidates and toolchain suffixes, which a
// hand-rolled minor-number comparison got wrong.
func CompareGoVersions(a string, b string) int {
	return version.Compare(normalizeGoVersion(a), normalizeGoVersion(b))
}

func normalizeGoVersion(v string) string {
	v = strings.TrimSpace(v)
	if strings.HasPrefix(v, "go") {
		return v
	}
	return "go" + v
}

// GOTOOLCHAIN=local makes an older Go fail with our version message instead of
// silently downloading a newer toolchain.
func goCmdEnv() []string {
	return append(os.Environ(), "GOTOOLCHAIN=local")
}
```

- [ ] **Step 4: Update `build.go`**

Replace line 31 (`const MinSupportedGoMinorVersion = 22`) with:

```go
const (
	DefaultMinGoVersion = "1.22"
	goVersionTimeout    = 10 * time.Second

	GoStatus_Ok         = "ok"
	GoStatus_NotFound   = "notfound"
	GoStatus_BadVersion = "badversion"
	GoStatus_Error      = "error"
)
```

Add to `BuildOpts` (after `GoPath string`, line 103):

```go
	MinGoVersion   string
```

Add to `GoVersionCheckResult` (after `GoVersion string`, line 130):

```go
	Version     string
```

Replace `CheckGoVersion` (lines 171-251) with:

```go
func CheckGoVersion(customGoPath string, minGoVersion string) GoVersionCheckResult {
	if minGoVersion == "" {
		minGoVersion = DefaultMinGoVersion
	}
	goPath := customGoPath
	if goPath != "" {
		if _, err := os.Stat(goPath); err != nil {
			return GoVersionCheckResult{GoStatus: GoStatus_NotFound, GoPath: goPath}
		}
	} else {
		found, err := FindGoExecutable()
		if err != nil {
			return GoVersionCheckResult{GoStatus: GoStatus_NotFound}
		}
		goPath = found
	}

	ctx, cancel := context.WithTimeout(context.Background(), goVersionTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, goPath, "version")
	cmd.Env = goCmdEnv()
	output, err := cmd.Output()
	if err != nil {
		return GoVersionCheckResult{
			GoStatus:    GoStatus_Error,
			GoPath:      goPath,
			ErrorString: fmt.Sprintf("failed to run '%s version': %v", goPath, err),
		}
	}

	versionStr := strings.TrimSpace(string(output))
	goVer, ok := ParseGoVersionOutput(versionStr)
	if !ok {
		return GoVersionCheckResult{
			GoStatus:    GoStatus_Error,
			GoPath:      goPath,
			GoVersion:   versionStr,
			ErrorString: fmt.Sprintf("unable to parse go version from: %s", versionStr),
		}
	}

	status := GoStatus_Ok
	if CompareGoVersions(goVer, minGoVersion) < 0 {
		status = GoStatus_BadVersion
	}
	return GoVersionCheckResult{
		GoStatus:  status,
		GoPath:    goPath,
		GoVersion: versionStr,
		Version:   goVer,
	}
}
```

In `verifyEnvironment`, replace lines 267-294 (from `result := CheckGoVersion(opts.GoPath)` through `goVersion := matches[1]`) with:

```go
	minGoVersion := opts.MinGoVersion
	if minGoVersion == "" {
		minGoVersion = DefaultMinGoVersion
	}
	result := CheckGoVersion(opts.GoPath, minGoVersion)

	switch result.GoStatus {
	case GoStatus_NotFound:
		return nil, fmt.Errorf("go command not found")
	case GoStatus_BadVersion:
		return nil, fmt.Errorf("go version %s or higher required, found: %s", minGoVersion, result.GoVersion)
	case GoStatus_Error:
		return nil, fmt.Errorf("%s", result.ErrorString)
	case GoStatus_Ok:
		if verbose {
			if opts.GoPath != "" {
				oc.Printf("[debug] Using custom go path: %s", result.GoPath)
			} else {
				oc.Printf("[debug] Using go path: %s", result.GoPath)
			}
			oc.Printf("[debug] Found %s", result.GoVersion)
		}
	default:
		return nil, fmt.Errorf("unexpected go status: %s", result.GoStatus)
	}

	goVersion := result.Version
```

In `createGoMod`, after `tidyCmd.Dir = tempDir` (line 421) add `tidyCmd.Env = goCmdEnv()`. In `runGoBuild`, after `buildCmd.Dir = tempDir` (line 762) add `buildCmd.Env = goCmdEnv()`.

Add `"context"` to the imports. Keep `strconv` and `regexp`: `ParseTsunamiPort` and the `SdkVersion` check still use them. `go vet` in Step 6 reports any import that does become unused.

- [ ] **Step 5: Update the RPC call site**

In `pkg/wshrpc/wshserver/wshserver.go:1235`, change `result := build.CheckGoVersion(goPath)` to `result := build.CheckGoVersion(goPath, "")`. (Task 4 replaces `""` with the SDK floor.)

- [ ] **Step 6: Run the tests and vet**

Run: `cd tsunami && go test ./build/... -v && go vet ./build/...`
Expected: PASS; vet prints nothing.

Run: `go vet ./pkg/wshrpc/wshserver/...`
Expected: no output.

- [ ] **Step 7: Commit**

```bash
git add tsunami/build/goversion.go tsunami/build/goversion_test.go tsunami/build/build.go pkg/wshrpc/wshserver/wshserver.go
git commit -m "feat(tsunami): read the Go floor from the SDK go.mod and pin GOTOOLCHAIN=local

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 3: Go discovery (extra dirs, bounded login-shell probe, caches, GOROOT canonicalisation)

**Files:**
- Create: `tsunami/build/godiscovery.go`
- Create: `tsunami/build/godiscovery_unix.go`
- Create: `tsunami/build/godiscovery_windows.go`
- Create: `tsunami/build/godiscovery_test.go`
- Modify: `tsunami/build/build.go:134-169` (delete the old `FindGoExecutable`)
- Modify: `pkg/remotetermapputil/waveapputil.go:30-63` (`ResolveGoFmtPath` uses the cache only)
- Modify: `pkg/remotetermapputil/waveapputil_test.go` (add a test)

**Interfaces:**
- Consumes: `CompareGoVersions`, `goCmdEnv` (Task 2); `writeTestFile` (Task 1).
- Produces:
  - `build.FindGoExecutable(minGoVersion string) (string, error)`: takes the floor; each candidate (PATH, search paths, probe) is version-checked and skipped if older; returns the canonical `$GOROOT/bin/go` of the first that qualifies, or a `*GoTooOldError` naming the newest too-old Go when none qualifies; results are cached per floor
  - `build.GoTooOldError struct { GoPath, Version, MinVersion string }`
  - `CheckGoVersion` with no custom path passes its floor to discovery and maps `*GoTooOldError` to `GoStatus_BadVersion` (with that Go's path and version)
  - `build.GetCachedGoFmtPath() string`: `""` until a discovery succeeded; never starts a probe
  - unexported test seams in package `build`: `var goSearchPaths func(home string) []string`, `var goProbeTimeout time.Duration`, `func resetGoDiscoveryCache()`, `var goCache *goDiscoveryCache` with fields `failedAt time.Time` and `lock sync.Mutex`
  - `remotetermapputil.ResolveGoFmtPath() (string, error)`: unchanged signature; with `tsunami:gopath` unset it reads only `build.GetCachedGoFmtPath()`

- [ ] **Step 1: Write the failing tests**

Create `tsunami/build/godiscovery_test.go`:

```go
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package build

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

func isolateGoDiscovery(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("discovery fixtures use shell scripts")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", t.TempDir())
	t.Setenv("SHELL", "")
	origSearch := goSearchPaths
	origTimeout := goProbeTimeout
	goSearchPaths = func(string) []string { return nil }
	resetGoDiscoveryCache()
	t.Cleanup(func() {
		goSearchPaths = origSearch
		goProbeTimeout = origTimeout
		resetGoDiscoveryCache()
	})
	return home
}

func writeExecutable(t *testing.T, path string, script string) {
	t.Helper()
	writeTestFile(t, path, script)
	if err := os.Chmod(path, 0755); err != nil {
		t.Fatal(err)
	}
}

func fakeGoScript(version string) string {
	return "#!/bin/sh\nif [ \"$1\" = version ]; then echo 'go version go" + version + " linux/amd64'; exit 0; fi\nexit 1\n"
}

var plainFakeGo = fakeGoScript("1.26.0")

func TestFindGoUsesPath(t *testing.T) {
	isolateGoDiscovery(t)
	binDir := t.TempDir()
	writeExecutable(t, filepath.Join(binDir, "go"), plainFakeGo)
	t.Setenv("PATH", binDir)
	got, err := FindGoExecutable("")
	if err != nil || got != filepath.Join(binDir, "go") {
		t.Fatalf("FindGoExecutable = %q, %v", got, err)
	}
}

func TestFindGoUsesFirstExecutableSearchPath(t *testing.T) {
	isolateGoDiscovery(t)
	dir := t.TempDir()
	notExec := filepath.Join(dir, "b", "go")
	writeTestFile(t, notExec, plainFakeGo)
	first := filepath.Join(dir, "c", "go")
	second := filepath.Join(dir, "d", "go")
	writeExecutable(t, first, plainFakeGo)
	writeExecutable(t, second, plainFakeGo)
	goSearchPaths = func(string) []string {
		return []string{filepath.Join(dir, "a", "go"), notExec, first, second}
	}
	got, err := FindGoExecutable("")
	if err != nil || got != first {
		t.Fatalf("FindGoExecutable = %q, %v; want %q", got, err, first)
	}
}

func TestDefaultGoSearchPathsOrder(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix search list")
	}
	home := t.TempDir()
	for _, v := range []string{"go1.9.2", "go1.26.1", "go1.25.6"} {
		writeExecutable(t, filepath.Join(home, "sdk", v, "bin", "go"), plainFakeGo)
	}
	want := []string{
		"/opt/homebrew/bin/go",
		"/usr/local/bin/go",
		"/usr/local/go/bin/go",
		"/usr/bin/go",
		filepath.Join(home, ".local", "go", "bin", "go"),
		"/usr/lib/go/bin/go",
		filepath.Join(home, "sdk", "go1.26.1", "bin", "go"),
		"/snap/bin/go",
		filepath.Join(home, ".local", "share", "mise", "shims", "go"),
		filepath.Join(home, ".asdf", "shims", "go"),
	}
	if got := defaultGoSearchPaths(home); !slices.Equal(got, want) {
		t.Fatalf("defaultGoSearchPaths:\n got %v\nwant %v", got, want)
	}
}

func TestProbeTakesLastLineAndPassesLoginFlags(t *testing.T) {
	isolateGoDiscovery(t)
	dir := t.TempDir()
	fakeGo := filepath.Join(dir, "real", "go")
	writeExecutable(t, fakeGo, plainFakeGo)
	argsFile := filepath.Join(dir, "args")
	shell := filepath.Join(dir, "shells", "bash")
	writeExecutable(t, shell, "#!/bin/sh\nprintf '%s ' \"$@\" > '"+argsFile+"'\necho 'welcome to the machine'\necho\necho '"+fakeGo+"'\n")
	t.Setenv("SHELL", shell)
	got, err := FindGoExecutable("")
	if err != nil || got != fakeGo {
		t.Fatalf("FindGoExecutable = %q, %v; want %q", got, err, fakeGo)
	}
	args, _ := os.ReadFile(argsFile)
	if strings.TrimSpace(string(args)) != "-l -i -c command -v go" {
		t.Fatalf("bash probe args = %q", args)
	}

	resetGoDiscoveryCache()
	shShell := filepath.Join(dir, "shells2", "sh")
	writeExecutable(t, shShell, "#!/bin/sh\nprintf '%s ' \"$@\" > '"+argsFile+"'\necho '"+fakeGo+"'\n")
	t.Setenv("SHELL", shShell)
	if _, err := FindGoExecutable(""); err != nil {
		t.Fatal(err)
	}
	args, _ = os.ReadFile(argsFile)
	if strings.TrimSpace(string(args)) != "-l -c command -v go" {
		t.Fatalf("sh probe args = %q", args)
	}
}

func TestProbeRejectsRelativePath(t *testing.T) {
	isolateGoDiscovery(t)
	shell := filepath.Join(t.TempDir(), "zsh")
	writeExecutable(t, shell, "#!/bin/sh\necho go\n")
	got, err := probeLoginShellForGo(shell)
	if err == nil || !strings.Contains(err.Error(), "not an absolute path") {
		t.Fatalf("probeLoginShellForGo = %q, %v; want a not-absolute error", got, err)
	}
}

func TestProbeTimesOutWithBackgroundChild(t *testing.T) {
	isolateGoDiscovery(t)
	goProbeTimeout = 300 * time.Millisecond
	shell := filepath.Join(t.TempDir(), "bash")
	// The test PATH is an empty temp dir, so the script sets its own to reach sleep.
	writeExecutable(t, shell, "#!/bin/sh\nPATH=/usr/bin:/bin\n(sleep 30) &\nsleep 30\n")
	start := time.Now()
	_, err := probeLoginShellForGo(shell)
	elapsed := time.Since(start)
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("expected a timed-out error, got %v", err)
	}
	if elapsed < goProbeTimeout {
		t.Fatalf("probe returned after %v, before the timeout; the fixture did not hang", elapsed)
	}
	if elapsed > goProbeTimeout+goProbeWaitDelay+time.Second {
		t.Fatalf("probe took %v; the process group was not killed", elapsed)
	}
}

func TestProbeSkipsUnsetMissingAndUnknownShells(t *testing.T) {
	isolateGoDiscovery(t)
	dir := t.TempDir()
	marker := filepath.Join(dir, "ran")
	unknown := filepath.Join(dir, "tcsh")
	writeExecutable(t, unknown, "#!/bin/sh\n: > '"+marker+"'\n")
	for _, shell := range []string{"", "bash", filepath.Join(dir, "missing", "bash"), unknown} {
		resetGoDiscoveryCache()
		t.Setenv("SHELL", shell)
		start := time.Now()
		if _, err := FindGoExecutable(""); err == nil {
			t.Fatalf("SHELL=%q: expected not found", shell)
		}
		if time.Since(start) > time.Second {
			t.Fatalf("SHELL=%q: discovery took %v", shell, time.Since(start))
		}
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("a shell outside the allowlist was executed")
	}
}

func TestFindGoCachesFailureFor30Seconds(t *testing.T) {
	isolateGoDiscovery(t)
	dir := t.TempDir()
	counter := filepath.Join(dir, "count")
	shell := filepath.Join(dir, "bash")
	writeExecutable(t, shell, "#!/bin/sh\necho x >> '"+counter+"'\n")
	t.Setenv("SHELL", shell)
	countRuns := func() int {
		data, _ := os.ReadFile(counter)
		return strings.Count(string(data), "x")
	}
	FindGoExecutable("")
	FindGoExecutable("")
	if n := countRuns(); n != 1 {
		t.Fatalf("probe ran %d times within the failure TTL, want 1", n)
	}
	goCache.lock.Lock()
	goCache.failedAt = time.Now().Add(-31 * time.Second)
	goCache.lock.Unlock()
	FindGoExecutable("")
	if n := countRuns(); n != 2 {
		t.Fatalf("probe ran %d times after the TTL expired, want 2", n)
	}
}

func TestFindGoCanonicalisesGoroot(t *testing.T) {
	isolateGoDiscovery(t)
	dir := t.TempDir()
	goroot := filepath.Join(dir, "goroot")
	writeExecutable(t, filepath.Join(goroot, "bin", "go"), plainFakeGo)
	writeExecutable(t, filepath.Join(goroot, "bin", "gofmt"), plainFakeGo)
	shimDir := filepath.Join(dir, "shims")
	writeExecutable(t, filepath.Join(shimDir, "go"), "#!/bin/sh\nif [ \"$1\" = env ] && [ \"$2\" = GOROOT ]; then echo '"+goroot+"'; exit 0; fi\nif [ \"$1\" = version ]; then echo 'go version go1.26.0 linux/amd64'; exit 0; fi\nexit 1\n")
	t.Setenv("PATH", shimDir)
	if got := GetCachedGoFmtPath(); got != "" {
		t.Fatalf("GetCachedGoFmtPath before discovery = %q, want empty", got)
	}
	got, err := FindGoExecutable("")
	if err != nil || got != filepath.Join(goroot, "bin", "go") {
		t.Fatalf("FindGoExecutable = %q, %v", got, err)
	}
	if fmtPath := GetCachedGoFmtPath(); fmtPath != filepath.Join(goroot, "bin", "gofmt") {
		t.Fatalf("GetCachedGoFmtPath = %q", fmtPath)
	}
}

func TestFindGoSkipsTooOldCandidate(t *testing.T) {
	isolateGoDiscovery(t)
	dir := t.TempDir()
	binDir := filepath.Join(dir, "path")
	writeExecutable(t, filepath.Join(binDir, "go"), fakeGoScript("1.24.0"))
	t.Setenv("PATH", binDir)
	newer := filepath.Join(dir, "newer", "go")
	writeExecutable(t, newer, fakeGoScript("1.26.1"))
	goSearchPaths = func(string) []string { return []string{newer} }
	got, err := FindGoExecutable("1.25.6")
	if err != nil || got != newer {
		t.Fatalf("FindGoExecutable = %q, %v; want the newer %q", got, err, newer)
	}
}

func TestFindGoReportsNewestTooOld(t *testing.T) {
	isolateGoDiscovery(t)
	dir := t.TempDir()
	binDir := filepath.Join(dir, "path")
	writeExecutable(t, filepath.Join(binDir, "go"), fakeGoScript("1.24.0"))
	t.Setenv("PATH", binDir)
	older := filepath.Join(dir, "older", "go")
	writeExecutable(t, older, fakeGoScript("1.25.1"))
	goSearchPaths = func(string) []string { return []string{older} }

	_, err := FindGoExecutable("1.25.6")
	var tooOld *GoTooOldError
	if !errors.As(err, &tooOld) || tooOld.Version != "1.25.1" || tooOld.GoPath != older {
		t.Fatalf("err = %v; want GoTooOldError naming 1.25.1 at %s", err, older)
	}

	resetGoDiscoveryCache()
	res := CheckGoVersion("", "1.25.6")
	if res.GoStatus != GoStatus_BadVersion || res.Version != "1.25.1" || res.GoPath != older {
		t.Fatalf("CheckGoVersion = %+v; want badversion 1.25.1", res)
	}
}
```

Append to `pkg/remotetermapputil/waveapputil_test.go` (add `"os"` and `"runtime"` to its imports):

```go
func TestResolveGoFmtPathNeverProbes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake shell is a shell script")
	}
	dir := t.TempDir()
	marker := filepath.Join(dir, "probed")
	shell := filepath.Join(dir, "bash")
	if err := os.WriteFile(shell, []byte("#!/bin/sh\n: > '"+marker+"'\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SHELL", shell)
	t.Setenv("PATH", t.TempDir())
	if _, err := ResolveGoFmtPath(); err == nil {
		t.Fatal("expected an error before any discovery has run")
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("ResolveGoFmtPath started a login-shell probe")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd tsunami && go test ./build/... -run 'FindGo|Probe|DefaultGoSearchPaths' -v`
Expected: FAIL to compile: `undefined: goSearchPaths`, `undefined: resetGoDiscoveryCache`, `undefined: GetCachedGoFmtPath`, `undefined: defaultGoSearchPaths`, `undefined: goProbeTimeout`, `undefined: GoTooOldError`, `too many arguments in call to FindGoExecutable`.

- [ ] **Step 3: Implement discovery**

Delete `FindGoExecutable` from `tsunami/build/build.go` (lines 134-169, the function and its comment lines). Create `tsunami/build/godiscovery.go`:

```go
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package build

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	goProbeWaitDelay      = 1 * time.Second
	goProbeMaxOutput      = 64 * 1024
	goProbeScript         = "command -v go"
	goDiscoveryFailureTTL = 30 * time.Second
	goEnvTimeout          = 2 * time.Second
)

var goProbeTimeout = 3 * time.Second

var goSearchPaths = defaultGoSearchPaths

type goDiscoveryCache struct {
	findLock  sync.Mutex
	lock      sync.Mutex
	floor     string
	goPath    string
	gofmtPath string
	failErr   error
	failedAt  time.Time
}

// GoTooOldError is returned only when every Go found is older than the floor; it names
// the newest one so the message can say what was found.
type GoTooOldError struct {
	GoPath     string
	Version    string
	MinVersion string
}

func (e *GoTooOldError) Error() string {
	return fmt.Sprintf("the newest Go found (%s at %s) is older than %s", e.Version, e.GoPath, e.MinVersion)
}

var goCache = &goDiscoveryCache{}

type cappedWriter struct {
	buf bytes.Buffer
	max int
}

// Writes past the cap are reported as accepted so a chatty rc file sees no EPIPE.
func (w *cappedWriter) Write(p []byte) (int, error) {
	remain := w.max - w.buf.Len()
	if remain > 0 {
		if len(p) > remain {
			w.buf.Write(p[:remain])
		} else {
			w.buf.Write(p)
		}
	}
	return len(p), nil
}

// FindGoExecutable holds findLock for the whole discovery so concurrent callers share
// one login-shell probe instead of each starting their own. Results are cached per
// floor; in practice the floor is the SDK's go line and never changes.
func FindGoExecutable(minGoVersion string) (string, error) {
	goCache.findLock.Lock()
	defer goCache.findLock.Unlock()
	if hit, goPath, failErr := goCache.lookup(minGoVersion); hit {
		return goPath, failErr
	}
	found, err := discoverGo(minGoVersion)
	if err != nil {
		goCache.setFailure(minGoVersion, err)
		return "", err
	}
	goPath, gofmtPath := canonicalizeGoPath(found)
	goCache.setSuccess(minGoVersion, goPath, gofmtPath)
	return goPath, nil
}

// GetCachedGoFmtPath never discovers; Code-tab saves call it and must not wait on a probe.
func GetCachedGoFmtPath() string {
	goCache.lock.Lock()
	defer goCache.lock.Unlock()
	return goCache.gofmtPath
}

func (c *goDiscoveryCache) lookup(floor string) (bool, string, error) {
	c.lock.Lock()
	defer c.lock.Unlock()
	if c.floor != floor {
		return false, "", nil
	}
	if c.goPath != "" {
		return true, c.goPath, nil
	}
	if c.failErr != nil && time.Since(c.failedAt) < goDiscoveryFailureTTL {
		return true, "", c.failErr
	}
	return false, "", nil
}

func (c *goDiscoveryCache) setFailure(floor string, err error) {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.floor = floor
	c.goPath = ""
	c.failErr = err
	c.failedAt = time.Now()
}

func (c *goDiscoveryCache) setSuccess(floor string, goPath string, gofmtPath string) {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.floor = floor
	c.goPath = goPath
	c.gofmtPath = gofmtPath
	c.failErr = nil
}

func resetGoDiscoveryCache() {
	goCache.lock.Lock()
	defer goCache.lock.Unlock()
	goCache.floor = ""
	goCache.goPath = ""
	goCache.gofmtPath = ""
	goCache.failErr = nil
	goCache.failedAt = time.Time{}
}

func goExeName() string {
	if runtime.GOOS == "windows" {
		return "go.exe"
	}
	return "go"
}

func gofmtExeName() string {
	if runtime.GOOS == "windows" {
		return "gofmt.exe"
	}
	return "gofmt"
}

// Candidates are tried in order and each must meet the floor; a too-old Go early in
// PATH must not hide a newer one installed elsewhere.
func discoverGo(minGoVersion string) (string, error) {
	var newestTooOld *GoTooOldError
	accept := func(candidate string) bool {
		goVer, err := readGoVersion(candidate)
		if err != nil {
			return false
		}
		if minGoVersion == "" || CompareGoVersions(goVer, minGoVersion) >= 0 {
			return true
		}
		if newestTooOld == nil || CompareGoVersions(goVer, newestTooOld.Version) > 0 {
			newestTooOld = &GoTooOldError{GoPath: candidate, Version: goVer, MinVersion: minGoVersion}
		}
		return false
	}
	if goPath, err := exec.LookPath(goExeName()); err == nil {
		if absPath, err := filepath.Abs(goPath); err == nil && accept(absPath) {
			return absPath, nil
		}
	}
	home, _ := os.UserHomeDir()
	for _, candidate := range goSearchPaths(home) {
		if isExecutableFile(candidate) && accept(candidate) {
			return candidate, nil
		}
	}
	if runtime.GOOS == "linux" || runtime.GOOS == "darwin" {
		goPath, err := probeLoginShellForGo(os.Getenv("SHELL"))
		if err == nil && accept(goPath) {
			return goPath, nil
		}
		if err != nil {
			log.Printf("go discovery: %v", err)
		}
	}
	if newestTooOld != nil {
		return "", newestTooOld
	}
	return "", fmt.Errorf("go command not found in PATH, common installation locations, or the login shell")
}

func readGoVersion(goPath string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), goVersionTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, goPath, "version")
	cmd.Env = goCmdEnv()
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	goVer, ok := ParseGoVersionOutput(string(out))
	if !ok {
		return "", errors.New("unparseable go version output")
	}
	return goVer, nil
}

func defaultGoSearchPaths(home string) []string {
	if runtime.GOOS == "windows" {
		return []string{
			`c:\go\bin\go.exe`,
			`c:\program files\go\bin\go.exe`,
		}
	}
	paths := []string{
		"/opt/homebrew/bin/go",
		"/usr/local/bin/go",
		"/usr/local/go/bin/go",
		"/usr/bin/go",
	}
	if home != "" {
		paths = append(paths, filepath.Join(home, ".local", "go", "bin", "go"))
	}
	paths = append(paths, "/usr/lib/go/bin/go")
	if home != "" {
		if sdkGo := newestSdkGo(home); sdkGo != "" {
			paths = append(paths, sdkGo)
		}
	}
	paths = append(paths, "/snap/bin/go")
	if home != "" {
		paths = append(paths,
			filepath.Join(home, ".local", "share", "mise", "shims", "go"),
			filepath.Join(home, ".asdf", "shims", "go"),
		)
	}
	return paths
}

func newestSdkGo(home string) string {
	matches, err := filepath.Glob(filepath.Join(home, "sdk", "go*", "bin", goExeName()))
	if err != nil || len(matches) == 0 {
		return ""
	}
	sdkVersion := func(goPath string) string {
		return filepath.Base(filepath.Dir(filepath.Dir(goPath)))
	}
	sort.Slice(matches, func(i, j int) bool {
		return CompareGoVersions(sdkVersion(matches[i]), sdkVersion(matches[j])) > 0
	})
	return matches[0]
}

func isExecutableFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	if runtime.GOOS == "windows" {
		return true
	}
	return info.Mode()&0111 != 0
}

func loginShellArgs(shellPath string) ([]string, bool) {
	switch filepath.Base(shellPath) {
	case "bash", "zsh", "fish":
		return []string{"-l", "-i", "-c", goProbeScript}, true
	case "sh", "dash", "ksh":
		return []string{"-l", "-c", goProbeScript}, true
	}
	return nil, false
}

// The probe is the last resort because it runs the user's rc files; every bound
// here (timeout, group kill, output cap) exists because rc files can hang or spam.
func probeLoginShellForGo(shellPath string) (string, error) {
	if shellPath == "" {
		return "", fmt.Errorf("SHELL is not set")
	}
	if !filepath.IsAbs(shellPath) {
		return "", fmt.Errorf("SHELL %q is not an absolute path", shellPath)
	}
	args, ok := loginShellArgs(shellPath)
	if !ok {
		return "", fmt.Errorf("shell %q is not on the probe allowlist", filepath.Base(shellPath))
	}
	if !isExecutableFile(shellPath) {
		return "", fmt.Errorf("shell %q is not an executable file", shellPath)
	}
	ctx, cancel := context.WithTimeout(context.Background(), goProbeTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, shellPath, args...)
	output := &cappedWriter{max: goProbeMaxOutput}
	cmd.Stdout = output
	cmd.WaitDelay = goProbeWaitDelay
	setProbeProcessGroup(cmd)
	runErr := cmd.Run()
	if ctx.Err() != nil {
		return "", fmt.Errorf("login shell probe timed out after %v", goProbeTimeout)
	}
	if runErr != nil {
		return "", fmt.Errorf("login shell probe failed: %w", runErr)
	}
	line := lastNonEmptyLine(output.buf.String())
	if !filepath.IsAbs(line) {
		return "", fmt.Errorf("login shell probe returned %q, not an absolute path", line)
	}
	if !isExecutableFile(line) {
		return "", fmt.Errorf("login shell probe returned %q, which is not an executable file", line)
	}
	return line, nil
}

func lastNonEmptyLine(s string) string {
	lines := strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if line := strings.TrimSpace(lines[i]); line != "" {
			return line
		}
	}
	return ""
}

// Shims (mise, asdf) and /snap/bin wrappers are not the toolchain; asking the found
// binary for its GOROOT gives the real go and a gofmt that sits beside it.
func canonicalizeGoPath(found string) (string, string) {
	ctx, cancel := context.WithTimeout(context.Background(), goEnvTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, found, "env", "GOROOT")
	cmd.Env = goCmdEnv()
	out, err := cmd.Output()
	if err == nil {
		goroot := strings.TrimSpace(string(out))
		if filepath.IsAbs(goroot) {
			rootGo := filepath.Join(goroot, "bin", goExeName())
			if isExecutableFile(rootGo) {
				rootFmt := filepath.Join(goroot, "bin", gofmtExeName())
				if !isExecutableFile(rootFmt) {
					rootFmt = ""
				}
				return rootGo, rootFmt
			}
		}
	}
	fmtPath := filepath.Join(filepath.Dir(found), gofmtExeName())
	if !isExecutableFile(fmtPath) {
		fmtPath = ""
	}
	return found, fmtPath
}
```

Create `tsunami/build/godiscovery_unix.go`:

```go
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

//go:build !windows

package build

import (
	"os/exec"
	"syscall"
)

// A new session detaches the probe from any controlling tty, so an interactive shell
// cannot stop itself on SIGTTIN; its group id equals its pid, and rc files can leave
// background jobs holding stdout, so killing the whole group is what ends the probe.
func setProbeProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
```

Create `tsunami/build/godiscovery_windows.go`:

```go
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

//go:build windows

package build

import "os/exec"

func setProbeProcessGroup(cmd *exec.Cmd) {}
```

- [ ] **Step 4: Pass the floor from `CheckGoVersion` into discovery**

In `tsunami/build/build.go` `CheckGoVersion` (the Task 2 version), replace the discovery branch:

```go
		found, err := FindGoExecutable()
		if err != nil {
			return GoVersionCheckResult{GoStatus: GoStatus_NotFound}
		}
		goPath = found
```

with:

```go
		found, err := FindGoExecutable(minGoVersion)
		var tooOld *GoTooOldError
		if errors.As(err, &tooOld) {
			return GoVersionCheckResult{
				GoStatus:  GoStatus_BadVersion,
				GoPath:    tooOld.GoPath,
				GoVersion: "go" + tooOld.Version,
				Version:   tooOld.Version,
			}
		}
		if err != nil {
			return GoVersionCheckResult{GoStatus: GoStatus_NotFound}
		}
		goPath = found
```

and add `"errors"` to `build.go`'s imports. `readGoVersion` in `godiscovery.go` uses `goVersionTimeout` from Task 2.

- [ ] **Step 4b: Make `ResolveGoFmtPath` cache-only**

In `pkg/remotetermapputil/waveapputil.go`, replace lines 34-40 (the `if goPath == "" { ... build.FindGoExecutable() ... }` block) so the function body reads:

```go
func ResolveGoFmtPath() (string, error) {
	settings := rtconfig.GetWatcher().GetFullConfig().Settings
	goPath := settings.TsunamiGoPath

	if goPath == "" {
		gofmtPath := build.GetCachedGoFmtPath()
		if gofmtPath == "" {
			return "", fmt.Errorf("go toolchain has not been located yet (the first build locates it)")
		}
		return gofmtPath, nil
	}

	goDir := filepath.Dir(goPath)
```

and keep the rest of the existing function (from `gofmtName := "gofmt"` to the end) unchanged.

- [ ] **Step 5: Run the tests and vet**

Run: `cd tsunami && go test ./build/... -v && go vet ./build/...`
Expected: PASS for all tests (the timeout test takes about 0.3 to 1.3 s), including `TestFindGoSkipsTooOldCandidate` and `TestFindGoReportsNewestTooOld`; vet silent.

Run: `go test ./pkg/remotetermapputil/... -v && go vet ./pkg/remotetermapputil/...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add tsunami/build/godiscovery.go tsunami/build/godiscovery_unix.go tsunami/build/godiscovery_windows.go tsunami/build/godiscovery_test.go tsunami/build/build.go pkg/remotetermapputil/waveapputil.go pkg/remotetermapputil/waveapputil_test.go
git commit -m "feat(tsunami): find Go from GUI launches via user dirs and a bounded login-shell probe

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---
### Task 4: Effective SDK replace path, pre-build checks and D6 messages

**Files:**
- Create: `tsunami/build/gomod.go`
- Create: `tsunami/build/gomod_test.go`
- Modify: `tsunami/build/build.go:341-397` (`createGoMod` uses `makeGoModContent`), `:385`, `:394`, `:413`, `:415` (use `TsunamiSdkModulePath`)
- Create: `pkg/remotetermapputil/tsunamisdk.go`
- Create: `pkg/remotetermapputil/tsunamisdk_test.go`
- Modify: `pkg/remotetermapputil/waveapputil.go:21-28` (`GetTsunamiScaffoldPath` via `scaffoldPathFromSettings`)
- Modify: `pkg/buildercontroller/buildercontroller.go:193-251` (`buildAndRun` pre-build check), `:415-435` (`handleBuildError` output line)
- Create: `pkg/buildercontroller/buildercontroller_test.go`
- Modify: `pkg/blockcontroller/tsunamicontroller.go:118-185` (same pre-build check for app blocks)
- Modify: `pkg/wshrpc/wshserver/wshserver.go:1230-1244` (`CheckGoVersionCommand` uses the SDK floor)

**Interfaces:**
- Consumes: `build.ReadSdkGoVersion`, `build.CheckGoVersion(customGoPath, minGoVersion)`, `build.GoStatus_*`, `GoVersionCheckResult.Version`, `BuildOpts.MinGoVersion`, `build.CompareGoVersions` (Task 2); `remotetermapputil.GetTsunamiSdkPath`, `setResourcesPath` test helper in `pkg/remotetermapputil/waveapputil_test.go` (Task 1); `writeTestFile` (Task 1).
- Produces:
  - `build.TsunamiSdkModulePath = "github.com/LannCo/remoteterm/tsunami"`
  - unexported `build.makeGoModContent(existing []byte, params goModParams) ([]byte, error)` with `goModParams{ModulePath, GoVersion, MinGoVersion, SdkVersion, SdkReplacePath string}`
  - `remotetermapputil.TsunamiBuildEnv struct { ScaffoldPath, SdkReplacePath, MinGoVersion, GoPath string }`
  - `remotetermapputil.ResolveTsunamiSdkPath(settingPath string) (string, error)` (D6 SDK message on failure)
  - `remotetermapputil.PrepareTsunamiBuild(settings rtconfig.SettingsType) (*TsunamiBuildEnv, error)` (order: SDK, scaffold, Go; D6 messages)
  - `handleBuildError` appends `"[error] " + err.Error()` to the build output so the Build panel shows it
  - test helpers in `pkg/buildercontroller/buildercontroller_test.go`: `setupBuilderTest(t) (home string, resources string)` and `makeTestApp(t, home, appName string) string` (returns the app dir `~/waveapps/draft/<appName>` with an `app.go`), reused by Tasks 9, 10 and 12

- [ ] **Step 1: Write the failing go.mod tests**

Create `tsunami/build/gomod_test.go`:

```go
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package build

import (
	"testing"

	"golang.org/x/mod/modfile"
)

func parseGoMod(t *testing.T, data []byte) *modfile.File {
	t.Helper()
	mf, err := modfile.Parse("go.mod", data, nil)
	if err != nil {
		t.Fatalf("generated go.mod does not parse: %v\n%s", err, data)
	}
	return mf
}

func TestMakeGoModContentQuotesPathWithSpace(t *testing.T) {
	sdkPath := "/Program Files/RemoteTerm/resources/tsunamisdk"
	data, err := makeGoModContent(nil, goModParams{
		ModulePath:     "tsunami/draft/demo",
		GoVersion:      "1.25.6",
		MinGoVersion:   "1.25.6",
		SdkVersion:     "v0.12.4",
		SdkReplacePath: sdkPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	mf := parseGoMod(t, data)
	if mf.Module.Mod.Path != "tsunami/draft/demo" || mf.Go.Version != "1.25.6" {
		t.Fatalf("module %q go %q", mf.Module.Mod.Path, mf.Go.Version)
	}
	if len(mf.Replace) != 1 || mf.Replace[0].New.Path != sdkPath {
		t.Fatalf("replace = %+v", mf.Replace)
	}
}

func TestMakeGoModContentRewritesStaleReplace(t *testing.T) {
	existing := []byte("module tsunami/draft/demo\n\ngo 1.22\n\nrequire github.com/LannCo/remoteterm/tsunami v0.12.4\n\nreplace github.com/LannCo/remoteterm/tsunami => /tmp/.mount_RemoteOLD/resources/tsunamisdk\n")
	data, err := makeGoModContent(existing, goModParams{
		ModulePath:     "tsunami/draft/demo",
		GoVersion:      "1.26.3",
		MinGoVersion:   "1.25.6",
		SdkVersion:     "v0.12.4",
		SdkReplacePath: "/tmp/.mount_RemoteNEW/resources/tsunamisdk",
	})
	if err != nil {
		t.Fatal(err)
	}
	mf := parseGoMod(t, data)
	if len(mf.Replace) != 1 || mf.Replace[0].New.Path != "/tmp/.mount_RemoteNEW/resources/tsunamisdk" {
		t.Fatalf("replace = %+v", mf.Replace)
	}
	if mf.Go.Version != "1.25.6" {
		t.Fatalf("go line = %q, want it raised to the floor 1.25.6", mf.Go.Version)
	}
}

func TestMakeGoModContentKeepsNewerGoLine(t *testing.T) {
	existing := []byte("module tsunami/draft/demo\n\ngo 1.26\n")
	data, err := makeGoModContent(existing, goModParams{
		ModulePath:     "tsunami/draft/demo",
		GoVersion:      "1.26.3",
		MinGoVersion:   "1.25.6",
		SdkVersion:     "v0.12.4",
		SdkReplacePath: "/sdk",
	})
	if err != nil {
		t.Fatal(err)
	}
	if mf := parseGoMod(t, data); mf.Go.Version != "1.26" {
		t.Fatalf("go line = %q, want 1.26 unchanged", mf.Go.Version)
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `cd tsunami && go test ./build/... -run MakeGoModContent -v`
Expected: FAIL to compile, `undefined: makeGoModContent`, `undefined: goModParams`.

- [ ] **Step 3: Implement `makeGoModContent` and use it in `createGoMod`**

Create `tsunami/build/gomod.go`:

```go
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package build

import (
	"fmt"

	"golang.org/x/mod/modfile"
)

const TsunamiSdkModulePath = "github.com/LannCo/remoteterm/tsunami"

type goModParams struct {
	ModulePath     string
	GoVersion      string
	MinGoVersion   string
	SdkVersion     string
	SdkReplacePath string
}

// The replace target is rewritten on every build because packaged resource paths
// (AppImage mounts in particular) change between launches; AddReplace updates an
// existing replace in place, so a stale one never survives.
func makeGoModContent(existing []byte, params goModParams) ([]byte, error) {
	var modFile *modfile.File
	if existing != nil {
		parsed, err := modfile.Parse("go.mod", existing, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to parse existing go.mod: %w", err)
		}
		modFile = parsed
		if params.MinGoVersion != "" && (modFile.Go == nil || CompareGoVersions(modFile.Go.Version, params.MinGoVersion) < 0) {
			if err := modFile.AddGoStmt(params.MinGoVersion); err != nil {
				return nil, fmt.Errorf("failed to raise go version: %w", err)
			}
		}
	} else {
		modFile = &modfile.File{}
		if err := modFile.AddModuleStmt(params.ModulePath); err != nil {
			return nil, fmt.Errorf("failed to add module statement: %w", err)
		}
		if err := modFile.AddGoStmt(params.GoVersion); err != nil {
			return nil, fmt.Errorf("failed to add go version: %w", err)
		}
		if err := modFile.AddRequire(TsunamiSdkModulePath, params.SdkVersion); err != nil {
			return nil, fmt.Errorf("failed to add require directive: %w", err)
		}
	}
	if params.SdkReplacePath != "" {
		if err := modFile.AddReplace(TsunamiSdkModulePath, "", params.SdkReplacePath, ""); err != nil {
			return nil, fmt.Errorf("failed to add replace directive: %w", err)
		}
	}
	modFile.Cleanup()
	return modFile.Format()
}
```

In `tsunami/build/build.go`, replace the body of `createGoMod` from line 348 (`// Check if go.mod already exists in temp directory`) through line 409 (end of the `os.WriteFile(goModPath, ...)` error block) with:

```go
	tempGoModPath := filepath.Join(tempDir, "go.mod")
	existing, err := os.ReadFile(tempGoModPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("error checking for go.mod in temp directory: %w", err)
	}
	if verbose {
		if existing != nil {
			oc.Printf("[debug] Found existing go.mod in temp directory, parsing it")
		} else {
			oc.Printf("[debug] No existing go.mod found, creating new one")
		}
	}

	goLine := opts.MinGoVersion
	if goLine == "" {
		goLine = buildEnv.GoVersion
	}
	goModContent, err := makeGoModContent(existing, goModParams{
		ModulePath:     modulePath,
		GoVersion:      goLine,
		MinGoVersion:   opts.MinGoVersion,
		SdkVersion:     opts.SdkVersion,
		SdkReplacePath: opts.SdkReplacePath,
	})
	if err != nil {
		return err
	}
	if err := os.WriteFile(tempGoModPath, goModContent, 0644); err != nil {
		return fmt.Errorf("failed to write go.mod file: %w", err)
	}
```

Keep the verbose `[debug] Created go.mod ...` block and the `go mod tidy` section that follow unchanged, but replace the literal `github.com/LannCo/remoteterm/tsunami` in the two debug `Printf`s with `TsunamiSdkModulePath`. Remove the `golang.org/x/mod/modfile` import from `build.go` if nothing else there uses it; `go vet ./build/...` reports that and any variable (`modFile`, `goModPath`) left unused.

- [ ] **Step 4: Run the go.mod tests**

Run: `cd tsunami && go test ./build/... -v && go vet ./build/...`
Expected: PASS, vet silent.

- [ ] **Step 5: Write the failing pre-build check tests**

Create `pkg/remotetermapputil/tsunamisdk_test.go`:

```go
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package remotetermapputil

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/LannCo/remoteterm/pkg/rtconfig"
	"github.com/LannCo/remoteterm/tsunami/build"
)

func makeBuildFixture(t *testing.T) (string, string) {
	t.Helper()
	resources := t.TempDir()
	setResourcesPath(t, resources)
	sdk := filepath.Join(resources, "tsunamisdk")
	scaffold := filepath.Join(resources, "tsunamiscaffold")
	for dir, files := range map[string]map[string]string{
		sdk:      {"go.mod": "module github.com/LannCo/remoteterm/tsunami\n\ngo 1.25.6\n"},
		scaffold: {"app-main.go.tmpl": "package main\n"},
	} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		for name, content := range files {
			if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
				t.Fatal(err)
			}
		}
	}
	return sdk, scaffold
}

func fakeCheckGo(status string, version string) func(string, string) build.GoVersionCheckResult {
	return func(customGoPath string, minGoVersion string) build.GoVersionCheckResult {
		return build.GoVersionCheckResult{GoStatus: status, GoPath: "/fake/go", Version: version}
	}
}

func TestResolveTsunamiSdkPath(t *testing.T) {
	sdk, _ := makeBuildFixture(t)
	if got, err := ResolveTsunamiSdkPath(""); err != nil || got != sdk {
		t.Fatalf("bundle: got %q, %v", got, err)
	}
	custom := t.TempDir()
	if err := os.WriteFile(filepath.Join(custom, "go.mod"), []byte("module x\n\ngo 1.25.6\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if got, err := ResolveTsunamiSdkPath(custom); err != nil || got != custom {
		t.Fatalf("setting: got %q, %v", got, err)
	}
	if err := os.Remove(filepath.Join(sdk, "go.mod")); err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprintf(`Tsunami SDK not found at %s. Rebuild with "task build:tsunamisdk", or set "tsunami:sdkreplacepath" in Settings.`, sdk)
	if _, err := ResolveTsunamiSdkPath(""); err == nil || err.Error() != want {
		t.Fatalf("missing bundle: got %v\nwant %s", err, want)
	}
}

func TestPrepareTsunamiBuildMessages(t *testing.T) {
	sdk, scaffold := makeBuildFixture(t)
	settings := rtconfig.SettingsType{}

	env, err := prepareTsunamiBuild(settings, fakeCheckGo(build.GoStatus_Ok, "1.26.3"))
	if err != nil {
		t.Fatal(err)
	}
	if env.SdkReplacePath != sdk || env.ScaffoldPath != scaffold || env.MinGoVersion != "1.25.6" || env.GoPath != "/fake/go" {
		t.Fatalf("env = %+v", env)
	}

	_, err = prepareTsunamiBuild(settings, fakeCheckGo(build.GoStatus_NotFound, ""))
	wantNotFound := `Go toolchain not found. Install Go 1.25.6 or newer, or set "tsunami:gopath" in Settings to the full path of the go binary.`
	if err == nil || err.Error() != wantNotFound {
		t.Fatalf("not found: got %v", err)
	}

	_, err = prepareTsunamiBuild(settings, fakeCheckGo(build.GoStatus_BadVersion, "1.25.5"))
	wantOld := `Go 1.25.5 is older than 1.25.6, which the Tsunami SDK requires. Install a newer Go, or set "tsunami:gopath" to one.`
	if err == nil || err.Error() != wantOld {
		t.Fatalf("too old: got %v", err)
	}

	if err := os.Remove(filepath.Join(scaffold, "app-main.go.tmpl")); err != nil {
		t.Fatal(err)
	}
	_, err = prepareTsunamiBuild(settings, fakeCheckGo(build.GoStatus_Ok, "1.26.3"))
	wantScaffold := fmt.Sprintf(`Tsunami scaffold not found at %s. Rebuild with "task build:tsunamiscaffold", or set "tsunami:scaffoldpath" in Settings.`, scaffold)
	if err == nil || err.Error() != wantScaffold {
		t.Fatalf("scaffold: got %v", err)
	}

	if err := os.Remove(filepath.Join(sdk, "go.mod")); err != nil {
		t.Fatal(err)
	}
	_, err = prepareTsunamiBuild(settings, fakeCheckGo(build.GoStatus_NotFound, ""))
	if err == nil || err.Error() != fmt.Sprintf(`Tsunami SDK not found at %s. Rebuild with "task build:tsunamisdk", or set "tsunami:sdkreplacepath" in Settings.`, sdk) {
		t.Fatalf("SDK check must come first, got %v", err)
	}
}
```

Create `pkg/buildercontroller/buildercontroller_test.go`:

```go
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package buildercontroller

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/LannCo/remoteterm/pkg/remotetermbase"
	"github.com/LannCo/remoteterm/pkg/utilds"
)

func setupBuilderTest(t *testing.T) (string, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	resources := t.TempDir()
	orig := remotetermbase.AppResourcesPath_VarCache
	remotetermbase.AppResourcesPath_VarCache = resources
	t.Cleanup(func() { remotetermbase.AppResourcesPath_VarCache = orig })
	return home, resources
}

func makeTestApp(t *testing.T, home string, appName string) string {
	t.Helper()
	appDir := filepath.Join(home, "waveapps", "draft", appName)
	if err := os.MkdirAll(appDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appDir, "app.go"), []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	return appDir
}

func TestBuildAndRunReportsMissingSdk(t *testing.T) {
	home, resources := setupBuilderTest(t)
	makeTestApp(t, home, "demo")
	bc := &BuilderController{
		builderId:    "test-missing-sdk",
		appId:        "draft/demo",
		status:       BuilderStatus_Building,
		outputBuffer: utilds.MakeMultiReaderLineBuffer(100),
	}
	resultCh := make(chan *BuildResult, 1)
	bc.buildAndRun(context.Background(), "draft/demo", nil, resultCh)

	want := fmt.Sprintf(`Tsunami SDK not found at %s. Rebuild with "task build:tsunamisdk", or set "tsunami:sdkreplacepath" in Settings.`, filepath.Join(resources, "tsunamisdk"))
	result := <-resultCh
	if result.Success || result.ErrorMessage != want {
		t.Fatalf("result = %+v\nwant error %s", result, want)
	}
	status := bc.GetStatus()
	if status.Status != BuilderStatus_Error || status.ErrorMsg != want {
		t.Fatalf("status = %q / %q", status.Status, status.ErrorMsg)
	}
	lines := bc.GetOutput()
	if len(lines) == 0 || lines[len(lines)-1] != "[error] "+want {
		t.Fatalf("build output = %q; want a final [error] line", lines)
	}
}
```

- [ ] **Step 6: Run them to verify they fail**

Run: `go test ./pkg/remotetermapputil/... ./pkg/buildercontroller/... -v`
Expected: FAIL. `remotetermapputil`: `undefined: ResolveTsunamiSdkPath`, `undefined: prepareTsunamiBuild`. `buildercontroller`: the test compiles but fails because the error message is `build failed: ...` or a scaffold/electron error, not the SDK message.

- [ ] **Step 7: Implement the pre-build check**

Create `pkg/remotetermapputil/tsunamisdk.go`:

```go
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package remotetermapputil

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/LannCo/remoteterm/pkg/rtconfig"
	"github.com/LannCo/remoteterm/tsunami/build"
)

const ScaffoldCheckFileName = "app-main.go.tmpl"

type TsunamiBuildEnv struct {
	ScaffoldPath   string
	SdkReplacePath string
	MinGoVersion   string
	GoPath         string
}

func ResolveTsunamiSdkPath(settingPath string) (string, error) {
	sdkPath := settingPath
	if sdkPath == "" {
		sdkPath = GetTsunamiSdkPath()
	}
	if _, err := os.Stat(filepath.Join(sdkPath, "go.mod")); err != nil {
		return "", fmt.Errorf(`Tsunami SDK not found at %s. Rebuild with "task build:tsunamisdk", or set "tsunami:sdkreplacepath" in Settings.`, sdkPath)
	}
	return sdkPath, nil
}

// Each missing prerequisite gets its own message naming the fix; checked here, before
// the build starts, because the build itself reports them as generic failures.
func PrepareTsunamiBuild(settings rtconfig.SettingsType) (*TsunamiBuildEnv, error) {
	return prepareTsunamiBuild(settings, build.CheckGoVersion)
}

func prepareTsunamiBuild(settings rtconfig.SettingsType, checkGo func(customGoPath string, minGoVersion string) build.GoVersionCheckResult) (*TsunamiBuildEnv, error) {
	sdkPath, err := ResolveTsunamiSdkPath(settings.TsunamiSdkReplacePath)
	if err != nil {
		return nil, err
	}
	minGoVersion, err := build.ReadSdkGoVersion(sdkPath)
	if err != nil {
		return nil, fmt.Errorf("Tsunami SDK at %s has an unusable go.mod: %w", sdkPath, err)
	}
	scaffoldPath := scaffoldPathFromSettings(settings)
	if _, err := os.Stat(filepath.Join(scaffoldPath, ScaffoldCheckFileName)); err != nil {
		return nil, fmt.Errorf(`Tsunami scaffold not found at %s. Rebuild with "task build:tsunamiscaffold", or set "tsunami:scaffoldpath" in Settings.`, scaffoldPath)
	}
	result := checkGo(settings.TsunamiGoPath, minGoVersion)
	switch result.GoStatus {
	case build.GoStatus_NotFound:
		return nil, fmt.Errorf(`Go toolchain not found. Install Go %s or newer, or set "tsunami:gopath" in Settings to the full path of the go binary.`, minGoVersion)
	case build.GoStatus_BadVersion:
		return nil, fmt.Errorf(`Go %s is older than %s, which the Tsunami SDK requires. Install a newer Go, or set "tsunami:gopath" to one.`, result.Version, minGoVersion)
	case build.GoStatus_Error:
		return nil, fmt.Errorf("%s", result.ErrorString)
	}
	return &TsunamiBuildEnv{
		ScaffoldPath:   scaffoldPath,
		SdkReplacePath: sdkPath,
		MinGoVersion:   minGoVersion,
		GoPath:         result.GoPath,
	}, nil
}
```

In `pkg/remotetermapputil/waveapputil.go`, replace `GetTsunamiScaffoldPath` (lines 21-28) with:

```go
func GetTsunamiScaffoldPath() string {
	return scaffoldPathFromSettings(rtconfig.GetWatcher().GetFullConfig().Settings)
}

func scaffoldPathFromSettings(settings rtconfig.SettingsType) string {
	if settings.TsunamiScaffoldPath != "" {
		return settings.TsunamiScaffoldPath
	}
	return filepath.Join(remotetermbase.GetWaveAppResourcesPath(), "tsunamiscaffold")
}
```

- [ ] **Step 8: Use it in the builder controller**

In `pkg/buildercontroller/buildercontroller.go` `buildAndRun`, insert directly after the `GetAppDir` error block (after line 204):

```go
	settings := rtconfig.GetWatcher().GetFullConfig().Settings
	buildEnv, err := remotetermapputil.PrepareTsunamiBuild(settings)
	if err != nil {
		bc.handleBuildError(err, resultCh)
		return
	}
```

Then replace lines 218-225 (from `scaffoldPath := remotetermapputil.GetTsunamiScaffoldPath()` through `goPath := settings.TsunamiGoPath`) with:

```go
	sdkVersion := settings.TsunamiSdkVersion
	if sdkVersion == "" {
		sdkVersion = remotetermapputil.DefaultTsunamiSdkVersion
	}
```

and in the `build.BuildOpts{...}` literal (lines 228-242) set `ScaffoldPath: buildEnv.ScaffoldPath`, `SdkReplacePath: buildEnv.SdkReplacePath`, `MinGoVersion: buildEnv.MinGoVersion`, `GoPath: buildEnv.GoPath` (other fields unchanged).

In `handleBuildError` (line 415), after `bc.setStatus_nolock(BuilderStatus_Error, 0, 1, err.Error())`, add:

```go
	if bc.outputBuffer != nil {
		bc.outputBuffer.AddLine("[error] " + err.Error())
	}
```

- [ ] **Step 9: Use it for tsunami app blocks**

In `pkg/blockcontroller/tsunamicontroller.go` `Start`: delete line 119 (`scaffoldPath := ...`), line 121 (`sdkReplacePath := ...`) and line 126 (`goPath := ...`). Inside `if !upToDate || force {`, after the `nodePath` check (line 172), add:

```go
		buildEnv, err := remotetermapputil.PrepareTsunamiBuild(settings)
		if err != nil {
			return err
		}
```

and in the `build.BuildOpts{...}` literal set `ScaffoldPath: buildEnv.ScaffoldPath`, `SdkReplacePath: buildEnv.SdkReplacePath`, `MinGoVersion: buildEnv.MinGoVersion`, `GoPath: buildEnv.GoPath`.

- [ ] **Step 10: Report the SDK floor from `CheckGoVersionCommand`**

In `pkg/wshrpc/wshserver/wshserver.go` `CheckGoVersionCommand` (line 1230), replace `result := build.CheckGoVersion(goPath, "")` with:

```go
	minGoVersion := ""
	if sdkPath, err := remotetermapputil.ResolveTsunamiSdkPath(fullConfig.Settings.TsunamiSdkReplacePath); err == nil {
		minGoVersion, _ = build.ReadSdkGoVersion(sdkPath)
	}
	result := build.CheckGoVersion(goPath, minGoVersion)
```

- [ ] **Step 11: Run the tests and vet**

Run: `go test ./pkg/remotetermapputil/... ./pkg/buildercontroller/... -v`
Expected: PASS.

Run: `go vet ./pkg/remotetermapputil/... ./pkg/buildercontroller/... ./pkg/blockcontroller/... ./pkg/wshrpc/wshserver/... && (cd tsunami && go vet ./build/...)`
Expected: no output.

- [ ] **Step 12: Commit**

```bash
git add tsunami/build/gomod.go tsunami/build/gomod_test.go tsunami/build/build.go pkg/remotetermapputil/tsunamisdk.go pkg/remotetermapputil/tsunamisdk_test.go pkg/remotetermapputil/waveapputil.go pkg/buildercontroller/buildercontroller.go pkg/buildercontroller/buildercontroller_test.go pkg/blockcontroller/tsunamicontroller.go pkg/wshrpc/wshserver/wshserver.go
git commit -m "feat(builder): build against the bundled SDK and name each missing prerequisite

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 5: App folder safety (D7) for reads, writes, deletes and renames

**Files:**
- Create: `pkg/remotetermappstore/safepath.go`
- Create: `pkg/remotetermappstore/safepath_test.go`
- Modify: `pkg/remotetermappstore/waveappstore.go:263-287` (`WriteAppFile`), `:289-318` (`ReadAppFile`), `:320-340` (`DeleteAppFile`), `:378-407` (`RenameAppFile`)

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces (package `remotetermappstore`):
  - `MaxAppFileReadSize = 2 * 1024 * 1024`
  - `GetWaveAppsRoot() string` (`<home>/waveapps`)
  - `CheckNoSymlinks(target string) error`: every existing component from `~/waveapps/<ns>` down to `target` (inclusive) is `Lstat`ed and must not be a symlink; `~/waveapps` itself may be a symlink; missing trailing components are allowed; a target outside `~/waveapps` is an error
  - unexported `readRegularFileCapped(path string, maxSize int64) ([]byte, int64, error)` (data, mod time in ms)
  - unexported `writeAppFileSafe(path string, contents []byte) error` (new file: `O_EXCL`; existing: must be a regular file, checked with `Lstat` right before writing)
  - unexported `createFileExclusive(path string, contents []byte) error` (`O_CREATE|O_EXCL|O_WRONLY`, 0644; an existing path, including a dangling symlink, returns an error satisfying `errors.Is(err, fs.ErrExist)`)
  - test helper `setupAppStoreTest(t *testing.T) string` (temp HOME, returns it) and `makeAppDir(t, home, ns, name string) string`, reused by Tasks 6 and 8
  - `ReadAppFile` still returns errors satisfying `errors.Is(err, os.ErrNotExist)` for a missing file (`wshserver.ReadAppFileCommand` relies on it)

- [ ] **Step 1: Write the failing tests**

Create `pkg/remotetermappstore/safepath_test.go`:

```go
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package remotetermappstore

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func setupAppStoreTest(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	return home
}

func makeAppDir(t *testing.T, home string, ns string, name string) string {
	t.Helper()
	dir := filepath.Join(home, "waveapps", ns, name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func skipWithoutSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink fixtures need a Unix filesystem")
	}
}

func TestReadAppFileCapsSize(t *testing.T) {
	home := setupAppStoreTest(t)
	dir := makeAppDir(t, home, "draft", "demo")
	if err := os.WriteFile(filepath.Join(dir, "ok.bin"), bytes.Repeat([]byte("a"), MaxAppFileReadSize), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "big.bin"), bytes.Repeat([]byte("a"), MaxAppFileReadSize+1), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadAppFile("draft/demo", "ok.bin"); err != nil {
		t.Fatalf("2 MiB file: %v", err)
	}
	if _, err := ReadAppFile("draft/demo", "big.bin"); err == nil {
		t.Fatal("expected an error for a file over 2 MiB")
	}
}

func TestReadAppFileMissingIsErrNotExist(t *testing.T) {
	home := setupAppStoreTest(t)
	makeAppDir(t, home, "draft", "demo")
	_, err := ReadAppFile("draft/demo", "app.go")
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("got %v, want an os.ErrNotExist error", err)
	}
}

func TestReadAppFileRejectsNonRegular(t *testing.T) {
	skipWithoutSymlinks(t)
	home := setupAppStoreTest(t)
	dir := makeAppDir(t, home, "draft", "demo")
	if err := os.Mkdir(filepath.Join(dir, "app.go"), 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadAppFile("draft/demo", "app.go"); err == nil {
		t.Fatal("expected an error reading a directory")
	}
	outside := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "link.go")); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadAppFile("draft/demo", "link.go"); err == nil {
		t.Fatal("expected an error reading through a symlink")
	}
}

func TestSymlinkedParentRejected(t *testing.T) {
	skipWithoutSymlinks(t)
	home := setupAppStoreTest(t)
	dir := makeAppDir(t, home, "draft", "demo")
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.css"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "static")); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadAppFile("draft/demo", "static/secret.css"); err == nil {
		t.Error("read through a symlinked parent succeeded")
	}
	if err := WriteAppFile("draft/demo", "static/new.css", []byte("x")); err == nil {
		t.Error("write through a symlinked parent succeeded")
	}
	if _, err := os.Stat(filepath.Join(outside, "new.css")); err == nil {
		t.Error("a file was created outside the app folder")
	}
	if err := DeleteAppFile("draft/demo", "static/secret.css"); err == nil {
		t.Error("delete through a symlinked parent succeeded")
	}
	if _, err := os.Stat(filepath.Join(outside, "secret.css")); err != nil {
		t.Error("a file outside the app folder was deleted")
	}
}

func TestSymlinkedAppDirRejected(t *testing.T) {
	skipWithoutSymlinks(t)
	home := setupAppStoreTest(t)
	real := t.TempDir()
	if err := os.WriteFile(filepath.Join(real, "app.go"), []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, "waveapps", "draft"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, filepath.Join(home, "waveapps", "draft", "demo")); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadAppFile("draft/demo", "app.go"); err == nil {
		t.Error("read from a symlinked app folder succeeded")
	}
	if err := WriteAppFile("draft/demo", "x.go", []byte("x")); err == nil {
		t.Error("write into a symlinked app folder succeeded")
	}
	if _, err := os.Stat(filepath.Join(real, "x.go")); err == nil {
		t.Error("a file was created in the symlink target")
	}
}

func TestWriteAppFileRefusesDanglingSymlink(t *testing.T) {
	skipWithoutSymlinks(t)
	home := setupAppStoreTest(t)
	dir := makeAppDir(t, home, "draft", "demo")
	target := filepath.Join(t.TempDir(), "created-through-link")
	if err := os.Symlink(target, filepath.Join(dir, "app.go")); err != nil {
		t.Fatal(err)
	}
	if err := WriteAppFile("draft/demo", "app.go", []byte("package main\n")); err == nil {
		t.Fatal("write through a dangling symlink succeeded")
	}
	if _, err := os.Stat(target); err == nil {
		t.Fatal("the symlink target was created")
	}
}

func TestWriteAppFileRefusesNonRegularTarget(t *testing.T) {
	home := setupAppStoreTest(t)
	dir := makeAppDir(t, home, "draft", "demo")
	if err := os.Mkdir(filepath.Join(dir, "app.go"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := WriteAppFile("draft/demo", "app.go", []byte("x")); err == nil {
		t.Fatal("overwriting a directory succeeded")
	}
}

func TestWriteAppFileCreatesAndOverwrites(t *testing.T) {
	home := setupAppStoreTest(t)
	makeAppDir(t, home, "draft", "demo")
	if err := WriteAppFile("draft/demo", "static/css/a.css", []byte("one")); err != nil {
		t.Fatal(err)
	}
	if err := WriteAppFile("draft/demo", "static/css/a.css", []byte("two")); err != nil {
		t.Fatal(err)
	}
	data, err := ReadAppFile("draft/demo", "static/css/a.css")
	if err != nil || string(data.Contents) != "two" {
		t.Fatalf("read back %q, %v", data, err)
	}
}

func TestSymlinkedWaveappsRootAllowed(t *testing.T) {
	skipWithoutSymlinks(t)
	home := setupAppStoreTest(t)
	realRoot := t.TempDir()
	if err := os.Symlink(realRoot, filepath.Join(home, "waveapps")); err != nil {
		t.Fatal(err)
	}
	if err := WriteAppFile("draft/demo", "app.go", []byte("package main\n")); err != nil {
		t.Fatalf("write through a symlinked ~/waveapps: %v", err)
	}
	data, err := ReadAppFile("draft/demo", "app.go")
	if err != nil || string(data.Contents) != "package main\n" {
		t.Fatalf("read through a symlinked ~/waveapps: %q, %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(realRoot, "draft", "demo", "app.go")); err != nil {
		t.Fatalf("app.go not under the real root: %v", err)
	}
}

func TestSymlinkedNamespaceRejected(t *testing.T) {
	skipWithoutSymlinks(t)
	home := setupAppStoreTest(t)
	realNs := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "waveapps"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realNs, filepath.Join(home, "waveapps", "draft")); err != nil {
		t.Fatal(err)
	}
	if err := WriteAppFile("draft/demo", "app.go", []byte("x")); err == nil {
		t.Error("write through a symlinked namespace succeeded")
	}
	if _, err := ReadAppFile("draft/demo", "app.go"); err == nil {
		t.Error("read through a symlinked namespace succeeded")
	}
	if entries, _ := os.ReadDir(realNs); len(entries) != 0 {
		t.Fatalf("files created in the symlink target: %v", entries)
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./pkg/remotetermappstore/... -v`
Expected: FAIL to compile, `undefined: MaxAppFileReadSize`. After adding only the constant, the symlink and size tests would fail; do not stop to check that.

- [ ] **Step 3: Implement the safe-path helpers**

Create `pkg/remotetermappstore/safepath.go`:

```go
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package remotetermappstore

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/LannCo/remoteterm/pkg/remotetermbase"
)

const MaxAppFileReadSize = 2 * 1024 * 1024

func GetWaveAppsRoot() string {
	return filepath.Join(remotetermbase.GetHomeDir(), "waveapps")
}

// A process that can write the app folder (an agent, an editor plugin) can plant
// symlinks; refusing every symlinked component keeps our reads and writes inside it.
// The check starts at ~/waveapps/<ns>: ~/waveapps itself is the user's choice (it may
// live on another disk) and is out of reach of anything confined to an app folder.
func CheckNoSymlinks(target string) error {
	root := GetWaveAppsRoot()
	rel, err := filepath.Rel(root, filepath.Clean(target))
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return fmt.Errorf("path %s is outside %s", target, root)
	}
	var components []string
	if rel != "." {
		cur := root
		for _, part := range strings.Split(rel, string(filepath.Separator)) {
			cur = filepath.Join(cur, part)
			components = append(components, cur)
		}
	}
	for _, component := range components {
		info, err := os.Lstat(component)
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("cannot inspect %s: %w", component, err)
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			return fmt.Errorf("refusing to use %s: it is a symbolic link", component)
		}
	}
	return nil
}

func readRegularFileCapped(path string, maxSize int64) ([]byte, int64, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to stat file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, 0, fmt.Errorf("%s is not a regular file", filepath.Base(path))
	}
	if info.Size() > maxSize {
		return nil, 0, fmt.Errorf("%s is larger than %d bytes", filepath.Base(path), maxSize)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to open file: %w", err)
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxSize+1))
	if err != nil {
		return nil, 0, fmt.Errorf("failed to read file: %w", err)
	}
	if int64(len(data)) > maxSize {
		return nil, 0, fmt.Errorf("%s is larger than %d bytes", filepath.Base(path), maxSize)
	}
	return data, info.ModTime().UnixMilli(), nil
}

func createFileExclusive(path string, contents []byte) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	if _, err := f.Write(contents); err != nil {
		f.Close()
		return fmt.Errorf("failed to write %s: %w", filepath.Base(path), err)
	}
	return f.Close()
}

func writeAppFileSafe(path string, contents []byte) error {
	if err := CheckNoSymlinks(path); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return createFileExclusive(path, contents)
	}
	if err != nil {
		return fmt.Errorf("failed to stat %s: %w", filepath.Base(path), err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("refusing to overwrite %s: not a regular file", filepath.Base(path))
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_TRUNC, 0)
	if err != nil {
		return fmt.Errorf("failed to open %s: %w", filepath.Base(path), err)
	}
	if _, err := f.Write(contents); err != nil {
		f.Close()
		return fmt.Errorf("failed to write %s: %w", filepath.Base(path), err)
	}
	return f.Close()
}
```

- [ ] **Step 4: Apply the helpers in `waveappstore.go`**

In `WriteAppFile`, replace lines 278-284 (the `MkdirAll` block and the `os.WriteFile` block) with:

```go
	if err := CheckNoSymlinks(filepath.Dir(filePath)); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}
	if err := writeAppFileSafe(filePath, contents); err != nil {
		return err
	}
```

In `ReadAppFile`, replace lines 304-317 (from `fileInfo, err := os.Stat(filePath)` to the `return &FileData{...}, nil` block) with:

```go
	if err := CheckNoSymlinks(filePath); err != nil {
		return nil, err
	}
	contents, modTs, err := readRegularFileCapped(filePath, MaxAppFileReadSize)
	if err != nil {
		return nil, err
	}
	return &FileData{
		Contents: contents,
		ModTs:    modTs,
	}, nil
```

In `DeleteAppFile`, insert before `if err := os.Remove(filePath)` (line 335):

```go
	if err := CheckNoSymlinks(filepath.Dir(filePath)); err != nil {
		return err
	}
```

In `RenameAppFile`, insert before the `MkdirAll` (line 398):

```go
	if err := CheckNoSymlinks(filepath.Dir(fromPath)); err != nil {
		return err
	}
	if err := CheckNoSymlinks(filepath.Dir(toPath)); err != nil {
		return err
	}
```

- [ ] **Step 5: Run the tests and vet**

Run: `go test ./pkg/remotetermappstore/... -v && go vet ./pkg/remotetermappstore/...`
Expected: PASS, vet silent.

- [ ] **Step 6: Commit**

```bash
git add pkg/remotetermappstore/safepath.go pkg/remotetermappstore/safepath_test.go pkg/remotetermappstore/waveappstore.go
git commit -m "fix(builder): refuse symlinks and special files in app folder reads and writes

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 6: Secret bindings out of the app folder (D8)

**Files:**
- Create: `pkg/remotetermappstore/secretbindings.go`
- Create: `pkg/remotetermappstore/secretbindings_test.go`
- Modify: `pkg/remotetermappstore/waveappstore.go:29-31` (remove `SecretBindingsFileName`), `:148` (`PublishDraft`), `:176` (`RevertDraft`), `:215` (`MakeDraftFromLocal`), `:228-232` (`DeleteApp`), `:719` (`RenameLocalApp`), `:746-802` (move `ReadAppSecretBindings`/`WriteAppSecretBindings` out)

**Interfaces:**
- Consumes: `setupAppStoreTest`, `makeAppDir` (Task 5 test helpers).
- Produces (package `remotetermappstore`):
  - `GetSecretBindingsPath(appId string) (string, error)`: `<data dir>/builder/secret-bindings/<ns>/<name>.json`; errors if the data dir is unset
  - `ReadAppSecretBindings(appId string) (map[string]string, error)` and `WriteAppSecretBindings(appId string, bindings map[string]string) error`: same signatures as today, new location
  - unexported `copySecretBindings(fromAppId, toAppId string) error`, `moveSecretBindings(fromAppId, toAppId string) error`, `deleteSecretBindings(appId string) error`
  - test helper `setDataDir(t *testing.T) string`

- [ ] **Step 1: Write the failing tests**

Create `pkg/remotetermappstore/secretbindings_test.go`:

```go
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package remotetermappstore

import (
	"maps"
	"os"
	"path/filepath"
	"testing"

	"github.com/LannCo/remoteterm/pkg/remotetermbase"
)

func setDataDir(t *testing.T) string {
	t.Helper()
	dataDir := t.TempDir()
	orig := remotetermbase.DataHome_VarCache
	remotetermbase.DataHome_VarCache = dataDir
	t.Cleanup(func() { remotetermbase.DataHome_VarCache = orig })
	return dataDir
}

func TestSecretBindingsLiveUnderDataDir(t *testing.T) {
	home := setupAppStoreTest(t)
	dataDir := setDataDir(t)
	appDir := makeAppDir(t, home, "draft", "demo")
	bindings := map[string]string{"API_KEY": "my-api-key"}
	if err := WriteAppSecretBindings("draft/demo", bindings); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dataDir, "builder", "secret-bindings", "draft", "demo.json")
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("bindings not at %s: %v", want, err)
	}
	if _, err := os.Stat(filepath.Join(appDir, "secret-bindings.json")); err == nil {
		t.Fatal("bindings were written into the app folder")
	}
	got, err := ReadAppSecretBindings("draft/demo")
	if err != nil || !maps.Equal(got, bindings) {
		t.Fatalf("read back %v, %v", got, err)
	}
}

func TestLegacyAppDirBindingsIgnored(t *testing.T) {
	home := setupAppStoreTest(t)
	setDataDir(t)
	appDir := makeAppDir(t, home, "draft", "demo")
	if err := os.WriteFile(filepath.Join(appDir, "secret-bindings.json"), []byte(`{"API_KEY":"stolen"}`), 0644); err != nil {
		t.Fatal(err)
	}
	got, err := ReadAppSecretBindings("draft/demo")
	if err != nil || len(got) != 0 {
		t.Fatalf("got %v, %v; want no bindings", got, err)
	}
}

func TestPublishCopiesBindings(t *testing.T) {
	home := setupAppStoreTest(t)
	setDataDir(t)
	appDir := makeAppDir(t, home, "draft", "demo")
	if err := os.WriteFile(filepath.Join(appDir, "app.go"), []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	bindings := map[string]string{"API_KEY": "my-api-key"}
	if err := WriteAppSecretBindings("draft/demo", bindings); err != nil {
		t.Fatal(err)
	}
	if _, err := PublishDraft("draft/demo"); err != nil {
		t.Fatal(err)
	}
	got, err := ReadAppSecretBindings("local/demo")
	if err != nil || !maps.Equal(got, bindings) {
		t.Fatalf("published bindings %v, %v", got, err)
	}
}

func TestSecretBindingsNeedDataDir(t *testing.T) {
	setupAppStoreTest(t)
	orig := remotetermbase.DataHome_VarCache
	remotetermbase.DataHome_VarCache = ""
	t.Cleanup(func() { remotetermbase.DataHome_VarCache = orig })
	if err := WriteAppSecretBindings("draft/demo", map[string]string{"A": "b"}); err == nil {
		t.Fatal("expected an error with no data dir")
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./pkg/remotetermappstore/... -run 'SecretBindings|LegacyAppDir|PublishCopies' -v`
Expected: FAIL: `TestSecretBindingsLiveUnderDataDir` (file written into the app folder), `TestLegacyAppDirBindingsIgnored` (returns `stolen`), `TestSecretBindingsNeedDataDir` (no error).

- [ ] **Step 3: Implement the new storage**

Delete `ReadAppSecretBindings` and `WriteAppSecretBindings` from `waveappstore.go` (lines 746-802) and remove `SecretBindingsFileName` from the const block (line 30). Create `pkg/remotetermappstore/secretbindings.go`:

```go
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package remotetermappstore

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/LannCo/remoteterm/pkg/remotetermbase"
)

// Bindings live outside the app folder so a process that can write the folder (an
// agent, an editor plugin) cannot bind the user's stored secrets into its own code.
func GetSecretBindingsPath(appId string) (string, error) {
	if err := ValidateAppId(appId); err != nil {
		return "", fmt.Errorf("invalid appId: %w", err)
	}
	dataDir := remotetermbase.GetWaveDataDir()
	if dataDir == "" {
		return "", fmt.Errorf("data directory is not set")
	}
	appNS, appName, _ := ParseAppId(appId)
	return filepath.Join(dataDir, "builder", "secret-bindings", appNS, appName+".json"), nil
}

func ReadAppSecretBindings(appId string) (map[string]string, error) {
	bindingsPath, err := GetSecretBindingsPath(appId)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(bindingsPath)
	if errors.Is(err, fs.ErrNotExist) {
		return make(map[string]string), nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read secret bindings: %w", err)
	}
	var bindings map[string]string
	if err := json.Unmarshal(data, &bindings); err != nil {
		return nil, fmt.Errorf("failed to parse secret bindings: %w", err)
	}
	if bindings == nil {
		bindings = make(map[string]string)
	}
	return bindings, nil
}

func WriteAppSecretBindings(appId string, bindings map[string]string) error {
	bindingsPath, err := GetSecretBindingsPath(appId)
	if err != nil {
		return err
	}
	if bindings == nil {
		bindings = make(map[string]string)
	}
	data, err := json.MarshalIndent(bindings, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal bindings: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(bindingsPath), 0700); err != nil {
		return fmt.Errorf("failed to create secret bindings directory: %w", err)
	}
	if err := os.WriteFile(bindingsPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write secret bindings: %w", err)
	}
	return nil
}

// The bindings file used to travel with the app folder on publish, draft and revert;
// copying it here keeps that behaviour now that it lives elsewhere.
func copySecretBindings(fromAppId string, toAppId string) error {
	fromPath, err := GetSecretBindingsPath(fromAppId)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(fromPath)
	if errors.Is(err, fs.ErrNotExist) {
		return deleteSecretBindings(toAppId)
	}
	if err != nil {
		return fmt.Errorf("failed to read secret bindings: %w", err)
	}
	var bindings map[string]string
	if err := json.Unmarshal(data, &bindings); err != nil {
		return fmt.Errorf("failed to parse secret bindings: %w", err)
	}
	return WriteAppSecretBindings(toAppId, bindings)
}

func moveSecretBindings(fromAppId string, toAppId string) error {
	if err := copySecretBindings(fromAppId, toAppId); err != nil {
		return err
	}
	return deleteSecretBindings(fromAppId)
}

func deleteSecretBindings(appId string) error {
	bindingsPath, err := GetSecretBindingsPath(appId)
	if err != nil {
		return err
	}
	if err := os.Remove(bindingsPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("failed to remove secret bindings: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Keep bindings with the app on publish, draft, revert, rename and delete**

In `waveappstore.go`:

- `PublishDraft`, before `return localAppId, nil` (line 148):

```go
	if err := copySecretBindings(draftAppId, localAppId); err != nil {
		return "", err
	}
```

- `RevertDraft`, replace `return copyDir(localDir, draftDir)` (line 176) with:

```go
	if err := copyDir(localDir, draftDir); err != nil {
		return err
	}
	return copySecretBindings(localAppId, draftAppId)
```

- `MakeDraftFromLocal`, before the final `return draftAppId, nil` (line 215):

```go
	if err := copySecretBindings(localAppId, draftAppId); err != nil {
		return "", err
	}
```

- `DeleteApp`, before the final `return nil` (line 233):

```go
	if err := deleteSecretBindings(appId); err != nil {
		return err
	}
```

- `RenameLocalApp`, before the final `return nil` (line 719):

```go
	if localExists {
		if err := moveSecretBindings(oldLocalAppId, newLocalAppId); err != nil {
			return err
		}
	}
	if draftExists {
		if err := moveSecretBindings(MakeAppId(AppNSDraft, appName), MakeAppId(AppNSDraft, newAppName)); err != nil {
			return err
		}
	}
```

- [ ] **Step 5: Run the tests and vet**

Run: `go test ./pkg/remotetermappstore/... ./pkg/buildercontroller/... -v && go vet ./pkg/remotetermappstore/... ./pkg/buildercontroller/... ./pkg/wshrpc/wshserver/...`
Expected: PASS, vet silent (`buildercontroller` and `wshserver` call `ReadAppSecretBindings`/`WriteAppSecretBindings` with unchanged signatures).

- [ ] **Step 6: Commit**

```bash
git add pkg/remotetermappstore/secretbindings.go pkg/remotetermappstore/secretbindings_test.go pkg/remotetermappstore/waveappstore.go
git commit -m "fix(builder): store app secret bindings under the data dir, not the app folder

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---
### Task 7: Starter package (`app.go`, `AGENTS.md`, `CLAUDE.md`, `TSUNAMI_GUIDE.md`) with symbol and compile tests

**Files:**
- Create: `pkg/remotetermappstore/starter/starter.go`
- Create: `pkg/remotetermappstore/starter/files/app.go.tmpl`
- Create: `pkg/remotetermappstore/starter/files/AGENTS.md`
- Create: `pkg/remotetermappstore/starter/files/CLAUDE.md`
- Create: `pkg/remotetermappstore/starter/files/TSUNAMI_GUIDE.md`
- Create: `pkg/remotetermappstore/starter/starter_test.go`

**Interfaces:**
- Consumes: `build.CopySdkBundle` (Task 1), `build.ReadSdkGoVersion`, `build.CompareGoVersions` (Task 2), `build.TsunamiSdkModulePath` (Task 4).
- Produces (package `starter`, import path `github.com/LannCo/remoteterm/pkg/remotetermappstore/starter`):
  - consts `AppGoFileName = "app.go"`, `AgentsFileName = "AGENTS.md"`, `ClaudeFileName = "CLAUDE.md"`, `GuideFileName = "TSUNAMI_GUIDE.md"`
  - `type StarterFile struct { Name string; Data []byte }`
  - `GetStarterFiles() ([]StarterFile, error)`: always in the order app.go, AGENTS.md, CLAUDE.md, TSUNAMI_GUIDE.md

The app source is embedded as `files/app.go.tmpl` because a file literally named `app.go` in this directory would be compiled into package `starter`.

- [ ] **Step 1: Write the failing tests**

Create `pkg/remotetermappstore/starter/starter_test.go`:

```go
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package starter

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/LannCo/remoteterm/tsunami/build"
)

var sdkIdentRe = regexp.MustCompile(`\b(app|vdom|ui)\.([A-Z][A-Za-z0-9_]*)`)

func repoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate the test file")
	}
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "..")
}

func starterFile(t *testing.T, name string) []byte {
	t.Helper()
	files, err := GetStarterFiles()
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if f.Name == name {
			return f.Data
		}
	}
	t.Fatalf("starter file %s not found", name)
	return nil
}

func exportedIdents(t *testing.T, pkgDir string) map[string]bool {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(pkgDir, "*.go"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("no Go files in %s: %v", pkgDir, err)
	}
	idents := make(map[string]bool)
	fset := token.NewFileSet()
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, decl := range file.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				if d.Recv == nil && d.Name.IsExported() {
					idents[d.Name.Name] = true
				}
			case *ast.GenDecl:
				for _, spec := range d.Specs {
					switch s := spec.(type) {
					case *ast.TypeSpec:
						if s.Name.IsExported() {
							idents[s.Name.Name] = true
						}
					case *ast.ValueSpec:
						for _, name := range s.Names {
							if name.IsExported() {
								idents[name.Name] = true
							}
						}
					}
				}
			}
		}
	}
	return idents
}

func TestGetStarterFiles(t *testing.T) {
	files, err := GetStarterFiles()
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, f := range files {
		names = append(names, f.Name)
		if len(f.Data) == 0 {
			t.Errorf("%s is empty", f.Name)
		}
	}
	if strings.Join(names, ",") != "app.go,AGENTS.md,CLAUDE.md,TSUNAMI_GUIDE.md" {
		t.Fatalf("names = %v", names)
	}
	if got := string(starterFile(t, ClaudeFileName)); got != "@AGENTS.md\n" {
		t.Fatalf("CLAUDE.md = %q, want the single line @AGENTS.md", got)
	}
}

func TestAgentsMdConstraints(t *testing.T) {
	text := string(starterFile(t, AgentsFileName))
	if lines := strings.Count(text, "\n"); lines >= 120 {
		t.Fatalf("AGENTS.md has %d lines, must be under 120", lines)
	}
	fence := strings.Repeat("`", 3)
	for _, banned := range []string{"go get", fence + "sh", fence + "bash", fence + "shell", "http://", "https://", "curl ", "wget "} {
		if strings.Contains(text, banned) {
			t.Errorf("AGENTS.md contains %q; it must not carry shell commands or network instructions", banned)
		}
	}
	for _, required := range []string{".tsunami/build.log", "TSUNAMI_GUIDE.md", "AppMeta", "AppInit", "go.mod", "manifest.json", "static/tw.css", "bin/"} {
		if !strings.Contains(text, required) {
			t.Errorf("AGENTS.md does not mention %q", required)
		}
	}
}

func TestStarterAppGoShape(t *testing.T) {
	src := starterFile(t, AppGoFileName)
	file, err := parser.ParseFile(token.NewFileSet(), "app.go", src, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	if file.Name.Name != "main" {
		t.Fatalf("package %s, want main", file.Name.Name)
	}
	vars := make(map[string]bool)
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "init" {
			t.Fatal("starter app.go defines init(); the build rejects that")
		}
		if gen, ok := decl.(*ast.GenDecl); ok && gen.Tok == token.VAR {
			for _, spec := range gen.Specs {
				for _, name := range spec.(*ast.ValueSpec).Names {
					vars[name.Name] = true
				}
			}
		}
	}
	if !vars["AppMeta"] || !vars["App"] {
		t.Fatalf("starter app.go must declare var AppMeta and var App, found %v", vars)
	}
}

func TestStarterDocsReferenceOnlyExistingSdkSymbols(t *testing.T) {
	sdk := filepath.Join(repoRoot(t), "tsunami")
	exported := map[string]map[string]bool{
		"app":  exportedIdents(t, filepath.Join(sdk, "app")),
		"vdom": exportedIdents(t, filepath.Join(sdk, "vdom")),
		"ui":   exportedIdents(t, filepath.Join(sdk, "ui")),
	}
	for _, name := range []string{AgentsFileName, GuideFileName} {
		text := string(starterFile(t, name))
		matches := sdkIdentRe.FindAllStringSubmatch(text, -1)
		if name == GuideFileName && len(matches) < 50 {
			t.Errorf("%s references only %d SDK identifiers; the port looks truncated", name, len(matches))
		}
		for _, m := range matches {
			if !exported[m[1]][m[2]] {
				t.Errorf("%s references %s.%s, which the SDK does not export", name, m[1], m[2])
			}
		}
	}
}

func TestGuideHasNoStaleApiOrChatFraming(t *testing.T) {
	text := string(starterFile(t, GuideFileName))
	for _, stale := range []string{"wavetermdev", "const AppTitle", "const AppShortDesc", "SetShortDescription", "UseData", "Built for AI", "AI agent", "AI model", "global-keyboard-handling.md", "graphing.md"} {
		if strings.Contains(text, stale) {
			t.Errorf("TSUNAMI_GUIDE.md still contains %q", stale)
		}
	}
	if !strings.Contains(text, "github.com/LannCo/remoteterm/tsunami/app") {
		t.Error("TSUNAMI_GUIDE.md does not use the LannCo import path")
	}
}

func TestStarterAppCompilesAgainstBundledSdk(t *testing.T) {
	goBin, err := exec.LookPath("go")
	if err != nil {
		// go test runs with the toolchain that built it, even when PATH lacks go.
		goBin = filepath.Join(runtime.GOROOT(), "bin", "go")
		if _, statErr := os.Stat(goBin); statErr != nil {
			t.Skip("go not found: not on PATH and not at runtime.GOROOT()/bin/go")
		}
	}
	root := repoRoot(t)
	sdkSrc := filepath.Join(root, "tsunami")
	minGo, err := build.ReadSdkGoVersion(sdkSrc)
	if err != nil {
		t.Fatal(err)
	}
	env := append(os.Environ(), "GOTOOLCHAIN=local", "GOFLAGS=-mod=mod", "GOPROXY=off", "GOWORK=off")
	verCmd := exec.Command(goBin, "env", "GOVERSION")
	verCmd.Env = env
	verOut, err := verCmd.Output()
	if err != nil {
		t.Skipf("cannot read the local Go version: %v", err)
	}
	if localGo := strings.TrimSpace(string(verOut)); build.CompareGoVersions(localGo, minGo) < 0 {
		t.Skipf("local %s is older than the SDK's go %s", localGo, minGo)
	}

	work := t.TempDir()
	sdk := filepath.Join(work, "sdk")
	if err := build.CopySdkBundle(sdkSrc, sdk); err != nil {
		t.Fatal(err)
	}
	appDir := filepath.Join(work, "app")
	mainTmpl, err := os.ReadFile(filepath.Join(sdkSrc, "templates", "app-main.go.tmpl"))
	if err != nil {
		t.Fatal(err)
	}
	goMod := fmt.Sprintf("module tsunami/draft/starter\n\ngo %s\n\nrequire %s v0.12.4\n\nreplace %s => %q\n", minGo, build.TsunamiSdkModulePath, build.TsunamiSdkModulePath, sdk)
	for rel, content := range map[string][]byte{
		"go.mod":          []byte(goMod),
		"app.go":          starterFile(t, AppGoFileName),
		"app-main.go":     mainTmpl,
		"dist/index.html": []byte("<!doctype html>\n"),
		"static/tw.css":   []byte(""),
	} {
		path := filepath.Join(appDir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, content, 0644); err != nil {
			t.Fatal(err)
		}
	}
	run := func(args ...string) (string, error) {
		cmd := exec.Command(goBin, args...)
		cmd.Dir = appDir
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	if out, err := run("mod", "tidy"); err != nil {
		if strings.Contains(out, "GOPROXY=off") || strings.Contains(out, "module lookup disabled") {
			t.Skipf("module cache is cold, cannot resolve SDK dependencies offline:\n%s", out)
		}
		t.Fatalf("go mod tidy: %v\n%s", err, out)
	}
	if out, err := run("build", "-o", filepath.Join(work, "starter-bin"), "."); err != nil {
		t.Fatalf("the starter app does not compile against the bundled SDK: %v\n%s", err, out)
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./pkg/remotetermappstore/starter/... -v`
Expected: FAIL to compile, `undefined: GetStarterFiles`.

- [ ] **Step 3: Implement the package**

Create `pkg/remotetermappstore/starter/starter.go`:

```go
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
```

Create `pkg/remotetermappstore/starter/files/app.go.tmpl`:

```go
package main

import (
	"github.com/LannCo/remoteterm/tsunami/app"
	"github.com/LannCo/remoteterm/tsunami/vdom"
)

var AppMeta = app.AppMeta{
	Title:     "Starter App",
	ShortDesc: "A counter and a text field to build on",
}

var App = app.DefineComponent("App", func(_ any) any {
	count := app.UseLocal(0)
	name := app.UseLocal("")

	greeting := "Type your name below."
	if name.Get() != "" {
		greeting = "Hello, " + name.Get() + "!"
	}

	return vdom.H("div", map[string]any{
		"className": "max-w-md m-6 flex flex-col gap-4 font-sans",
	},
		vdom.H("h1", map[string]any{"className": "text-2xl font-bold"}, "Starter App"),
		vdom.H("p", nil, greeting),
		vdom.H("input", map[string]any{
			"className":   "px-3 py-2 border border-border rounded",
			"type":        "text",
			"placeholder": "Your name",
			"value":       name.Get(),
			"onChange": func(e vdom.VDomEvent) {
				name.Set(e.TargetValue)
			},
		}),
		vdom.H("button", map[string]any{
			"className": "px-4 py-2 border border-border rounded cursor-pointer",
			"onClick": func() {
				count.Set(count.Get() + 1)
			},
		}, "Clicked ", count.Get(), " times"),
	)
},
)
```

Create `pkg/remotetermappstore/starter/files/CLAUDE.md` with exactly one line and a trailing newline:

```
@AGENTS.md
```

Create `pkg/remotetermappstore/starter/files/AGENTS.md` (outer fence below is four backticks so the inner Go block shows; the file itself starts at `# Tsunami app`):

````markdown
# Tsunami app: notes for coding agents

This folder is a Tsunami app. Tsunami is a Go framework that renders a React-style UI from Go code; the RemoteTerm app builder compiles and previews it. `TSUNAMI_GUIDE.md` in this folder documents the API with examples.

## What the build compiles

- Every `*.go` file directly in this folder, as `package main`. Files in subfolders are not compiled.
- Everything under `static/`, embedded and served at `/static/<path>`.
- The builder supplies `main()` and the module setup. Do not write a `main` function.
- Do not define `func init()`; the build rejects it. For start-up work, define `func AppInit() error`. It runs once before the app starts, and returning an error stops the app.

## Required declarations

`app.go` declares the app's metadata and its root component, which must be named `App`:

```go
package main

import (
	"github.com/LannCo/remoteterm/tsunami/app"
	"github.com/LannCo/remoteterm/tsunami/vdom"
)

var AppMeta = app.AppMeta{
	Title:     "My App",
	ShortDesc: "One line about what it does",
}

var App = app.DefineComponent("App", func(_ any) any {
	return vdom.H("div", map[string]any{"className": "p-4"}, "Hello")
})
```

## Files the build writes

The builder rewrites these on every build. Do not edit them; changes are lost:

- `go.mod` and `go.sum`. The SDK location in `go.mod` is set per build and can change between launches.
- `manifest.json`, generated from `AppMeta` and the app's config, data and secret declarations.
- `static/tw.css`, the generated Tailwind stylesheet.
- `bin/`, the compiled app.

The `.tsunami/` folder belongs to the builder.

## After you change a file

- The result of the most recent build is in `.tsunami/build.log`. Its last line is the build status.
- If that status is an error, the lines above it hold the compiler output. Fix the file and line it reports.
- If the user has "Rebuild on external changes" turned off, saving a file does not start a build; the user clicks Rebuild in the builder. Until then `.tsunami/build.log` describes the previous build.

## Styling

Use Tailwind utility classes in `className`. Tailwind only generates classes it finds written out in the source, so write each class name in full (`"text-red-500"`), never assembled from parts at run time.

## Rules that prevent most bugs

- Components are values created with `app.DefineComponent`; props are plain Go structs with `json` tags.
- Call hooks (`app.UseLocal`, `app.UseEffect`, `app.UseRef` and the others in the guide) at the top of a component body, unconditionally, in the same order on every render.
- Read an atom with `.Get()` and change it with `.Set()` or `.SetFn()`. Never modify a value returned by `.Get()` in place; copy it first.
- State that several components share goes in atoms declared at package level with `app.SharedAtom`, `app.ConfigAtom` or `app.DataAtom`.
- Stay inside this folder: do not read or write files elsewhere.

## Where to look next

`TSUNAMI_GUIDE.md` covers elements and attributes, conditional rendering and lists, hooks, atoms, async work with goroutines, static files and keyboard handling.
````

- [ ] **Step 4: Port the Tsunami guide into `TSUNAMI_GUIDE.md`**

This is editorial work against a moving SDK; the tests catch identifiers and stale strings, not wrong prose, so verify as you go.

Read the sources with `git show e9bc34a0^:pkg/aiusechat/tsunami/system.md` (1404 lines), `git show e9bc34a0^:pkg/aiusechat/tsunami/global-keyboard-handling.md` (71 lines) and `git show e9bc34a0^:pkg/aiusechat/tsunami/graphing.md` (337 lines). Start `files/TSUNAMI_GUIDE.md` from `system.md` and apply, in order:

1. Replace every `github.com/wavetermdev/waveterm/tsunami/` with `github.com/LannCo/remoteterm/tsunami/`.
2. Delete the section `## Built for AI Development` (source lines 23-44). Rephrase or cut every other sentence aimed at an AI assistant or chat (source lines 63, 84, 467, 551, 557, 584 mention AI agents or models): "external tools" is the neutral replacement where the sentence still says something true.
3. Replace the `const AppTitle` / `const AppShortDesc` convention (source lines 49-86 and the template at line 1211 onward) with `var AppMeta = app.AppMeta{Title: "...", ShortDesc: "..."}`. Ground truth: `tsunami/templates/app-main.go.tmpl` calls `app.SetAppMeta(AppMeta)`; `tsunami/demo/todo/app.go:11-14` shows the declaration.
4. Source line 553 names `app.SetShortDescription`; the SDK function is `app.SetShortDesc` (`tsunami/app/defaultclient.go:179`).
5. Source line 250 says `app.Use\*`; the symbol test reads `app.Use` as an identifier. Rewrite as "functions whose names start with `Use` (for example `app.UseLocal`)".
6. Replace the pointer to `global-keyboard-handling.md` in `## Global Keyboard Handling` (source line 1120) with the body of that file. Its identifiers (`app.SetGlobalEventHandler`, `vdom.VDomEvent`, `vdom.H`) exist; check its prose against `tsunami/app/defaultclient.go:47` and `tsunami/vdom/vdom_types.go`.
7. `graphing.md` calls `app.UseData`, which does not exist. If you can rewrite its examples on top of `tsunami/demo/recharts/app.go` and `tsunami/demo/cpuchart/app.go` and confirm each call against the SDK, append it as a `## Charts` section. If not, leave it out entirely. Either way, the file names `graphing.md` and `global-keyboard-handling.md` must not appear in the guide.
8. For every code block, check hook signatures against `tsunami/app/hooks.go` (`UseEffect(fn func() func(), deps []any)`, `UseTicker(interval time.Duration, tickFn func(), deps []any)`, `UseAfter(duration time.Duration, timeoutFn func(), deps []any)`, `UseGoRoutine(fn func(ctx context.Context), deps []any)`, `UseRef[T any](val T) *vdom.VDomSimpleRef[T]`, `UseVDomRef() *vdom.VDomRef`), atom methods against `tsunami/app/atom.go` (`Get`, `Set`, `SetFn`), and atom constructors against `tsunami/app/defaultclient.go:73-97` (`ConfigAtom[T](name, defaultValue, meta *AtomMeta)`, `DataAtom[T](name, defaultValue, meta *AtomMeta)`, `SharedAtom[T](name, defaultValue)`).
9. Cut any statement you cannot verify against the SDK source rather than guessing. Keep a list of what you cut or changed beyond steps 1-7.

If the result is still recognisably the original guide with the corrections above, the port is done; trimming for length is not required.

- [ ] **Step 5: Run the starter tests**

Run: `go test ./pkg/remotetermappstore/starter/... -v`
Expected: PASS for all six tests. `TestStarterAppCompilesAgainstBundledSdk` finds `go` on PATH or, failing that, at `runtime.GOROOT()/bin/go` (the toolchain running the test). It may skip only for a Go older than the SDK or a cold module cache; report any skip and its reason. If `TestStarterDocsReferenceOnlyExistingSdkSymbols` fails, fix the document, never the test's identifier list.

Run: `go vet ./pkg/remotetermappstore/...`
Expected: no output.

- [ ] **Step 6: Commit**

Put your list from Step 4 item 9 in the commit body.

```bash
git add pkg/remotetermappstore/starter
git commit -m "feat(builder): starter app, agent notes and Tsunami guide for new apps

<list of guide statements cut or rewritten beyond the mechanical fixes>

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 8: `SeedApp`, `SeedBuilderAppCommand` and its frontend callers

**Files:**
- Create: `pkg/remotetermappstore/seed.go`
- Create: `pkg/remotetermappstore/seed_test.go`
- Modify: `pkg/wshrpc/wshrpctypes_builder.go:11-30` (interface), append types after line 169
- Modify: `pkg/wshrpc/wshserver/wshserver.go` (add `SeedBuilderAppCommand` after `WriteAppGoFileCommand`, line 1127)
- Generated: `frontend/types/gotypes.d.ts`, `frontend/app/store/wshclientapi.ts`, `pkg/wshrpc/wshclient/wshclient.go`
- Modify: `frontend/builder/app-selection-modal.tsx:149-160` (`handleCreateNew`)
- Modify: `frontend/builder/store/builder-apppanel-model.ts` (add `seedStarterApp`)
- Modify: `frontend/builder/tabs/builder-previewtab.tsx:9-28` (`EmptyStateView`), `:157-158` (pass prop)

**Interfaces:**
- Consumes: `CheckNoSymlinks`, `createFileExclusive`, and the test helpers `setupAppStoreTest`, `makeAppDir`, `skipWithoutSymlinks` (Task 5); `deleteSecretBindings`, `WriteAppSecretBindings`, `ReadAppSecretBindings` and the test helper `setDataDir` (Task 6); `starter.GetStarterFiles`, `starter.AppGoFileName` (Task 7).
- Produces:
  - `remotetermappstore.SeedApp(appId string) ([]string, error)`: names of files written; existing files (including dangling symlinks) are skipped, never written through, except that an existing regular, empty `app.go` is filled (opened `O_WRONLY|O_TRUNC` after an `Lstat` confirms it is a regular file); when the app folder did not exist before the call, any secret bindings stored for that app id are deleted first
  - RPC `SeedBuilderAppCommand(ctx, CommandSeedBuilderAppData{AppId}) (*CommandSeedBuilderAppRtnData{Files []string}, error)`; TS: `RpcApi.SeedBuilderAppCommand(TabRpcClient, { appid })` returns `{ files: string[] }`
  - `BuilderAppPanelModel.seedStarterApp(): Promise<void>`

Read `.kilocode/skills/add-rpc/SKILL.md` before Step 5; this task follows its steps 1-4 (interface, types, `npx task generate`, `wshserver.go`).

- [ ] **Step 1: Write the failing tests**

Create `pkg/remotetermappstore/seed_test.go`:

```go
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package remotetermappstore

import (
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/LannCo/remoteterm/pkg/remotetermappstore/starter"
)

func TestSeedAppWritesStarterFiles(t *testing.T) {
	home := setupAppStoreTest(t)
	setDataDir(t)
	written, err := SeedApp("draft/fresh")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"app.go", "AGENTS.md", "CLAUDE.md", "TSUNAMI_GUIDE.md"}
	if !slices.Equal(written, want) {
		t.Fatalf("written = %v, want %v", written, want)
	}
	files, _ := starter.GetStarterFiles()
	for _, f := range files {
		data, err := os.ReadFile(filepath.Join(home, "waveapps", "draft", "fresh", f.Name))
		if err != nil || string(data) != string(f.Data) {
			t.Errorf("%s not written as embedded: %v", f.Name, err)
		}
	}
}

func TestSeedAppNeverOverwrites(t *testing.T) {
	home := setupAppStoreTest(t)
	setDataDir(t)
	dir := makeAppDir(t, home, "draft", "mine")
	if err := os.WriteFile(filepath.Join(dir, "app.go"), []byte("package main // mine\n"), 0644); err != nil {
		t.Fatal(err)
	}
	written, err := SeedApp("draft/mine")
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(written, "app.go") {
		t.Fatal("app.go reported as written")
	}
	data, _ := os.ReadFile(filepath.Join(dir, "app.go"))
	if string(data) != "package main // mine\n" {
		t.Fatalf("app.go was overwritten: %q", data)
	}
}

func TestSeedAppSkipsDanglingSymlink(t *testing.T) {
	skipWithoutSymlinks(t)
	home := setupAppStoreTest(t)
	setDataDir(t)
	dir := makeAppDir(t, home, "draft", "linked")
	target := filepath.Join(t.TempDir(), "outside.go")
	if err := os.Symlink(target, filepath.Join(dir, "app.go")); err != nil {
		t.Fatal(err)
	}
	written, err := SeedApp("draft/linked")
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(written, "app.go") {
		t.Fatal("app.go reported as written through a symlink")
	}
	if _, err := os.Stat(target); err == nil {
		t.Fatal("the symlink target was created")
	}
}

func TestSeedAppRefusesSymlinkedAppDir(t *testing.T) {
	skipWithoutSymlinks(t)
	home := setupAppStoreTest(t)
	setDataDir(t)
	real := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "waveapps", "draft"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, filepath.Join(home, "waveapps", "draft", "evil")); err != nil {
		t.Fatal(err)
	}
	if _, err := SeedApp("draft/evil"); err == nil {
		t.Fatal("seeding a symlinked app folder succeeded")
	}
	if entries, _ := os.ReadDir(real); len(entries) != 0 {
		t.Fatalf("files written into the symlink target: %v", entries)
	}
}

func TestSeedAppRejectsInvalidIds(t *testing.T) {
	setupAppStoreTest(t)
	setDataDir(t)
	for _, id := range []string{"", "nonamespace", "draft/../escape", "draft/has space", "draft/a/b"} {
		if _, err := SeedApp(id); err == nil {
			t.Errorf("SeedApp(%q) succeeded", id)
		}
	}
}

func TestSeedAppFillsEmptyAppGo(t *testing.T) {
	home := setupAppStoreTest(t)
	setDataDir(t)
	dir := makeAppDir(t, home, "draft", "empty")
	if err := os.WriteFile(filepath.Join(dir, "app.go"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	written, err := SeedApp("draft/empty")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(written, "app.go") {
		t.Fatalf("written = %v; an empty app.go should be filled", written)
	}
	files, _ := starter.GetStarterFiles()
	data, _ := os.ReadFile(filepath.Join(dir, "app.go"))
	if string(data) != string(files[0].Data) {
		t.Fatal("empty app.go was not replaced with the starter app")
	}
}

func TestSeedAppLeavesSymlinkToEmptyFile(t *testing.T) {
	skipWithoutSymlinks(t)
	home := setupAppStoreTest(t)
	setDataDir(t)
	dir := makeAppDir(t, home, "draft", "emptylink")
	target := filepath.Join(t.TempDir(), "empty.go")
	if err := os.WriteFile(target, nil, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, "app.go")); err != nil {
		t.Fatal(err)
	}
	written, err := SeedApp("draft/emptylink")
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(written, "app.go") {
		t.Fatal("wrote through a symlink to an empty file")
	}
	if data, _ := os.ReadFile(target); len(data) != 0 {
		t.Fatal("the symlink target was modified")
	}
}

func TestSeedAppClearsStaleBindingsForNewApp(t *testing.T) {
	setupAppStoreTest(t)
	setDataDir(t)
	if err := WriteAppSecretBindings("draft/reborn", map[string]string{"API_KEY": "old-binding"}); err != nil {
		t.Fatal(err)
	}
	if _, err := SeedApp("draft/reborn"); err != nil {
		t.Fatal(err)
	}
	got, err := ReadAppSecretBindings("draft/reborn")
	if err != nil || len(got) != 0 {
		t.Fatalf("bindings after seeding a new app = %v, %v; want none", got, err)
	}
}

func TestSeedAppKeepsBindingsForExistingApp(t *testing.T) {
	home := setupAppStoreTest(t)
	setDataDir(t)
	makeAppDir(t, home, "draft", "kept")
	bindings := map[string]string{"API_KEY": "my-binding"}
	if err := WriteAppSecretBindings("draft/kept", bindings); err != nil {
		t.Fatal(err)
	}
	if _, err := SeedApp("draft/kept"); err != nil {
		t.Fatal(err)
	}
	got, err := ReadAppSecretBindings("draft/kept")
	if err != nil || !maps.Equal(got, bindings) {
		t.Fatalf("bindings of an existing app changed: %v, %v", got, err)
	}
}

func TestSeedAppThroughSymlinkedRootButNotNamespace(t *testing.T) {
	skipWithoutSymlinks(t)
	home := setupAppStoreTest(t)
	setDataDir(t)
	realRoot := t.TempDir()
	if err := os.Symlink(realRoot, filepath.Join(home, "waveapps")); err != nil {
		t.Fatal(err)
	}
	if _, err := SeedApp("draft/seeded"); err != nil {
		t.Fatalf("seed through a symlinked ~/waveapps: %v", err)
	}
	if _, err := os.Stat(filepath.Join(realRoot, "draft", "seeded", "app.go")); err != nil {
		t.Fatalf("seeded app.go not under the real root: %v", err)
	}
	realNs := t.TempDir()
	if err := os.Symlink(realNs, filepath.Join(realRoot, "local")); err != nil {
		t.Fatal(err)
	}
	if _, err := SeedApp("local/seeded"); err == nil {
		t.Fatal("seed through a symlinked namespace succeeded")
	}
	if entries, _ := os.ReadDir(realNs); len(entries) != 0 {
		t.Fatalf("files created in the symlink target: %v", entries)
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./pkg/remotetermappstore/... -run SeedApp -v`
Expected: FAIL to compile, `undefined: SeedApp`.

- [ ] **Step 3: Implement `SeedApp`**

Create `pkg/remotetermappstore/seed.go`:

```go
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package remotetermappstore

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/LannCo/remoteterm/pkg/remotetermappstore/starter"
)

// O_EXCL means an existing file, or a symlink planted where a starter file would go,
// is left alone rather than written through. The one exception is an empty regular
// app.go (an editor or agent touched it), which counts as missing.
func SeedApp(appId string) ([]string, error) {
	if err := ValidateAppId(appId); err != nil {
		return nil, fmt.Errorf("invalid appId: %w", err)
	}
	appDir, err := GetAppDir(appId)
	if err != nil {
		return nil, err
	}
	if err := CheckNoSymlinks(appDir); err != nil {
		return nil, err
	}
	_, statErr := os.Lstat(appDir)
	isNewApp := errors.Is(statErr, fs.ErrNotExist)
	if err := os.MkdirAll(appDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create app directory: %w", err)
	}
	// MkdirAll follows a symlink created after the first check, so check again.
	if err := CheckNoSymlinks(appDir); err != nil {
		return nil, err
	}
	// Bindings are keyed by app id and outlive a deleted folder; a new app with an old
	// name must not inherit them.
	if isNewApp {
		if err := deleteSecretBindings(appId); err != nil {
			return nil, err
		}
	}
	files, err := starter.GetStarterFiles()
	if err != nil {
		return nil, err
	}
	written := make([]string, 0, len(files))
	for _, f := range files {
		path := filepath.Join(appDir, f.Name)
		err := createFileExclusive(path, f.Data)
		if errors.Is(err, fs.ErrExist) && f.Name == starter.AppGoFileName {
			filled, fillErr := fillEmptyRegularFile(path, f.Data)
			if fillErr != nil {
				return written, fmt.Errorf("failed to write %s: %w", f.Name, fillErr)
			}
			if filled {
				written = append(written, f.Name)
			}
			continue
		}
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return written, fmt.Errorf("failed to write %s: %w", f.Name, err)
		}
		written = append(written, f.Name)
	}
	return written, nil
}

func fillEmptyRegularFile(path string, contents []byte) (bool, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return false, err
	}
	if !info.Mode().IsRegular() || info.Size() != 0 {
		return false, nil
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_TRUNC, 0)
	if err != nil {
		return false, err
	}
	if _, err := f.Write(contents); err != nil {
		f.Close()
		return false, err
	}
	return true, f.Close()
}
```

- [ ] **Step 4: Run the seed tests**

Run: `go test ./pkg/remotetermappstore/... -v`
Expected: PASS.

- [ ] **Step 5: Add the RPC**

In `pkg/wshrpc/wshrpctypes_builder.go`, add to `WshRpcBuilderInterface` (after `WriteAppGoFileCommand`, line 17):

```go
	SeedBuilderAppCommand(ctx context.Context, data CommandSeedBuilderAppData) (*CommandSeedBuilderAppRtnData, error)
```

and append to the file:

```go
type CommandSeedBuilderAppData struct {
	AppId string `json:"appid"`
}

type CommandSeedBuilderAppRtnData struct {
	Files []string `json:"files"`
}
```

In `pkg/wshrpc/wshserver/wshserver.go`, add after `WriteAppGoFileCommand` (ends line 1127):

```go
func (ws *WshServer) SeedBuilderAppCommand(ctx context.Context, data wshrpc.CommandSeedBuilderAppData) (*wshrpc.CommandSeedBuilderAppRtnData, error) {
	if data.AppId == "" {
		return nil, fmt.Errorf("must provide an appId to SeedBuilderAppCommand")
	}
	files, err := remotetermappstore.SeedApp(data.AppId)
	if err != nil {
		return nil, fmt.Errorf("failed to create starter files: %w", err)
	}
	return &wshrpc.CommandSeedBuilderAppRtnData{Files: files}, nil
}
```

Run: `npx task generate`
Expected: exits 0. Then `git status --short` lists exactly `frontend/types/gotypes.d.ts`, `frontend/app/store/wshclientapi.ts`, `pkg/wshrpc/wshclient/wshclient.go` among generated files (plus your edits). If other generated files change, report them and do not commit them.

Run: `go vet ./pkg/wshrpc/... && go test ./pkg/wshrpc/... ./pkg/tsgen/...`
Expected: vet silent; tests PASS.

- [ ] **Step 6: Seed on "Create new"**

In `frontend/builder/app-selection-modal.tsx`, replace `handleCreateNew` (lines 149-160) with:

```tsx
    const handleCreateNew = async (appName: string) => {
        const draftAppId = `draft/${appName}`;
        try {
            await RpcApi.SeedBuilderAppCommand(TabRpcClient, { appid: draftAppId });
        } catch (err) {
            console.error("Failed to create starter files:", err);
            setError(`Failed to create ${appName}: ${err.message || String(err)}`);
            return;
        }
        const builderId = globalStore.get(atoms.builderId);
        const oref = WOS.makeORef("builder", builderId);
        await RpcApi.SetRTInfoCommand(TabRpcClient, {
            oref,
            data: { "builder:appid": draftAppId },
        });
        globalStore.set(atoms.builderAppId, draftAppId);
        document.title = `RTApp Builder (${draftAppId})`;
        getApi().setBuilderWindowAppId(draftAppId);
    };
```

- [ ] **Step 7: Add `seedStarterApp` and the Preview button**

In `frontend/builder/store/builder-apppanel-model.ts`, add after `loadAppFile` (ends line 277):

```ts
    async seedStarterApp() {
        const appId = globalStore.get(atoms.builderAppId);
        if (!appId) {
            return;
        }
        try {
            await RpcApi.SeedBuilderAppCommand(TabRpcClient, { appid: appId });
            await this.loadAppFile(appId);
        } catch (err) {
            console.error("Failed to create starter app:", err);
            globalStore.set(this.errorAtom, `Failed to create starter app: ${err.message || "Unknown error"}`);
        }
    }
```

In `frontend/builder/tabs/builder-previewtab.tsx`, replace `EmptyStateView` (lines 9-26) with:

```tsx
const EmptyStateView = memo(({ showCreate }: { showCreate: boolean }) => {
    const model = BuilderAppPanelModel.getInstance();
    return (
        <div className="w-full h-full flex items-center justify-center bg-background">
            <div className="flex flex-col items-center gap-6 max-w-[500px] text-center px-8">
                <div className="text-6xl">🏗️</div>
                <div className="flex flex-col gap-3">
                    <h2 className="text-2xl font-semibold text-primary">No App to Preview</h2>
                    <p className="text-base text-secondary leading-relaxed">
                        Create an <span className="font-mono">app.go</span> file to get started.
                    </p>
                </div>
                {showCreate ? (
                    <button
                        onClick={() => model.seedStarterApp()}
                        className="px-6 py-2 font-semibold bg-accent/80 text-onaccent rounded hover:bg-accent transition-colors cursor-pointer"
                    >
                        Create starter app
                    </button>
                ) : (
                    <div className="text-base text-secondary mt-2">
                        Your app will appear here once <span className="font-mono">app.go</span> is created
                    </div>
                )}
            </div>
        </div>
    );
});
```

and at line 158 change `overlay = <EmptyStateView />;` to `overlay = <EmptyStateView showCreate={!fileExists} />;`.

`fileExists` is `originalContent.length > 0` (line 140), so an empty `app.go` also shows the button, and `SeedApp` now fills an empty `app.go`, so the button works in that case too.

- [ ] **Step 8: Type-check**

Run: `npx tsc --noEmit`
Expected: exits 0 with no output.

- [ ] **Step 9: Commit**

```bash
git add pkg/remotetermappstore/seed.go pkg/remotetermappstore/seed_test.go pkg/wshrpc/wshrpctypes_builder.go pkg/wshrpc/wshserver/wshserver.go pkg/wshrpc/wshclient/wshclient.go frontend/types/gotypes.d.ts frontend/app/store/wshclientapi.ts frontend/builder/app-selection-modal.tsx frontend/builder/store/builder-apppanel-model.ts frontend/builder/tabs/builder-previewtab.tsx
git commit -m "feat(builder): seed new apps with a starter app and agent notes

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---
### Task 9: `RequestRebuild` (coalescing, input hash, build log, stop previous process) and the non-blocking RPC

**Files:**
- Create: `pkg/buildercontroller/appinputs.go`
- Create: `pkg/buildercontroller/appinputs_test.go`
- Create: `pkg/buildercontroller/rebuild_test.go`
- Modify: `pkg/buildercontroller/buildercontroller.go:32-38` (const), `:54-66` (struct), `:73-90` (`GetOrCreateController`), `:159-191` (`Start`), `:270-273` (success path log), `:415-435` (`handleBuildError` log), `:480-490` (`Stop`), `:508-554` (`GetStatus` race fix)
- Modify: `pkg/buildercontroller/buildercontroller_test.go` (two build-log tests)
- Modify: `pkg/buildercontroller/buildercontroller.go:98-137` (`DeleteController`, `Shutdown` mark closed), `:139-157` (`waitForBuildDone`), `:437-478` (delete `RestartAndWaitForBuild`)
- Modify: `pkg/wshrpc/wshrpctypes_builder.go` (interface + `CommandRequestBuilderRebuildData`; remove `RestartBuilderAndWaitCommand` and its two types)
- Modify: `pkg/wshrpc/wshserver/wshserver.go` (add `RequestBuilderRebuildCommand` after `StartBuilderCommand`, line 1172; delete `RestartBuilderAndWaitCommand`, lines 1185-1211; record the input hash in `WriteAppGoFileCommand`, line 1109)
- Generated: `frontend/types/gotypes.d.ts`, `frontend/app/store/wshclientapi.ts`, `pkg/wshrpc/wshclient/wshclient.go`

**Interfaces:**
- Consumes: `remotetermappstore.WriteAppFile` with D7 checks (Task 5); `setupBuilderTest`, `makeTestApp` (Task 4 test helpers); `PrepareTsunamiBuild` path in `buildAndRun` and the `[error]` output line (Task 4).
- Produces (package `buildercontroller`):
  - `BuildLogFileName = ".tsunami/build.log"`
  - `IsRelevantAppPath(rel string) bool` (slash-separated path relative to the app dir; root `*.go`, `static/**` except `static/tw.css`; ignores dotfiles/dot-dirs, `node_modules`, names ending `~`, `.swp`, `.swx`, `.tmp`, all-digit names)
  - `ComputeAppInputHash(appDir string) (string, error)`: sha256 over relevant regular files; root `*.go` files contribute path and content, files under `static/` contribute path, size and modification time in nanoseconds (their contents are never read)
  - `RecordAppInputHash(appId string)`: records the current input hash on every controller whose app is `appId`; called by `WriteAppGoFileCommand` after a Code-tab save
  - `(*BuilderController).markClosed()` / `isClosed() bool`: a closed controller never queues or starts another build; `DeleteController` and `Shutdown` call `markClosed()` before `Stop()`, and `Stop()` waits until the build loop has finished (`!isBuilding()`)
  - `RestartAndWaitForBuild` and the `RestartBuilderAndWaitCommand` RPC are deleted (no non-generated callers)
  - `(*BuilderController).RequestRebuild(appId string, builderEnv map[string]string)`: never blocks; a request during a build sets `rebuildPending` and exactly one follow-up build runs
  - `(*BuilderController).getLastBuildInputHash() string`, `setLastBuildInputHash(hash string)`, `isBuilding() bool`, `hasProcess() bool`
  - unexported `makeBuilderController(builderId string) *BuilderController`; field `runBuildFn func(ctx context.Context, appId string, builderEnv map[string]string)` (test seam, defaults to `buildAndRun`)
  - test helper `waitUntil(t *testing.T, timeout time.Duration, cond func() bool, what string)` in `rebuild_test.go`, reused by Task 10
  - RPC `RequestBuilderRebuildCommand(ctx, CommandRequestBuilderRebuildData{BuilderId}) error`; TS `RpcApi.RequestBuilderRebuildCommand(TabRpcClient, { builderid })`
  - `StartBuilderCommand` keeps working: `Start` now calls `RequestRebuild`

- [ ] **Step 1: Write the failing tests**

Create `pkg/buildercontroller/appinputs_test.go`:

```go
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package buildercontroller

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestIsRelevantAppPath(t *testing.T) {
	relevant := []string{"app.go", "helpers.go", "static/logo.png", "static/css/site.css", "static/tw.css.map"}
	ignored := []string{
		"go.mod", "go.sum", "manifest.json", "static/tw.css", "bin/app", ".tsunami/build.log",
		".git/HEAD", ".app.go.swp", "app.go~", "app.go.swp", "app.go.swx", "app.go.tmp", "4913",
		"static/.DS_Store", "static/node_modules/x.js", "sub/x.go", "README.md", "AGENTS.md", "static", "",
	}
	for _, rel := range relevant {
		if !IsRelevantAppPath(rel) {
			t.Errorf("%q should be relevant", rel)
		}
	}
	for _, rel := range ignored {
		if IsRelevantAppPath(rel) {
			t.Errorf("%q should be ignored", rel)
		}
	}
}

func TestComputeAppInputHash(t *testing.T) {
	dir := t.TempDir()
	write := func(rel string, content string) {
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	hash := func() string {
		h, err := ComputeAppInputHash(dir)
		if err != nil {
			t.Fatal(err)
		}
		return h
	}
	write("app.go", "package main\n")
	write("static/site.css", "body{}")
	base := hash()

	for _, rel := range []string{"go.mod", "go.sum", "manifest.json", "static/tw.css", ".tsunami/build.log", "bin/app"} {
		write(rel, "generated")
		if hash() != base {
			t.Fatalf("writing %s changed the input hash", rel)
		}
	}
	write("app.go", "package main // edited\n")
	edited := hash()
	if edited == base {
		t.Fatal("editing app.go did not change the input hash")
	}
	write("static/site.css", "body{color:red}")
	if hash() == edited {
		t.Fatal("editing a static file did not change the input hash")
	}
}

func TestComputeAppInputHashDoesNotReadStaticContents(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs an unreadable file, which root and Windows ignore")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "app.go"), []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	media := filepath.Join(dir, "static", "video.mp4")
	if err := os.MkdirAll(filepath.Dir(media), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(media, []byte("frames"), 0000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(media, 0644) })
	before, err := ComputeAppInputHash(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(media, time.Now(), time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	after, err := ComputeAppInputHash(dir)
	if err != nil {
		t.Fatal(err)
	}
	if before == after {
		t.Fatal("an unreadable static file's mtime change did not change the hash; it must be hashed from metadata")
	}
}

func TestComputeAppInputHashStaticSameSizeNewMtime(t *testing.T) {
	dir := t.TempDir()
	css := filepath.Join(dir, "static", "site.css")
	if err := os.MkdirAll(filepath.Dir(css), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(css, []byte("aaaa"), 0644); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-time.Hour)
	if err := os.Chtimes(css, past, past); err != nil {
		t.Fatal(err)
	}
	before, _ := ComputeAppInputHash(dir)
	if err := os.WriteFile(css, []byte("bbbb"), 0644); err != nil {
		t.Fatal(err)
	}
	after, _ := ComputeAppInputHash(dir)
	if before == after {
		t.Fatal("a same-size rewrite of a static file did not change the hash")
	}
}

func TestComputeAppInputHashAppGoSameBytesUnchanged(t *testing.T) {
	dir := t.TempDir()
	appGo := filepath.Join(dir, "app.go")
	if err := os.WriteFile(appGo, []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	before, _ := ComputeAppInputHash(dir)
	if err := os.Chtimes(appGo, time.Now(), time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(appGo, []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	after, _ := ComputeAppInputHash(dir)
	if before != after {
		t.Fatal("rewriting app.go with identical bytes changed the hash")
	}
}
```

Create `pkg/buildercontroller/rebuild_test.go`:

```go
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package buildercontroller

import (
	"context"
	"os/exec"
	"runtime"
	"sync/atomic"
	"testing"
	"time"
)

func waitUntil(t *testing.T, timeout time.Duration, cond func() bool, what string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out after %v waiting for %s", timeout, what)
}

func waitSignal(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
	}
}

func TestRequestRebuildCoalescesIntoOneFollowUp(t *testing.T) {
	home, _ := setupBuilderTest(t)
	makeTestApp(t, home, "demo")
	bc := makeBuilderController("test-coalesce")
	started := make(chan struct{}, 10)
	release := make(chan struct{})
	var calls atomic.Int32
	bc.runBuildFn = func(ctx context.Context, appId string, builderEnv map[string]string) {
		calls.Add(1)
		started <- struct{}{}
		<-release
	}

	begin := time.Now()
	bc.RequestRebuild("draft/demo", nil)
	if elapsed := time.Since(begin); elapsed > 200*time.Millisecond {
		t.Fatalf("RequestRebuild blocked for %v", elapsed)
	}
	waitSignal(t, started, "the first build")
	for i := 0; i < 5; i++ {
		bc.RequestRebuild("draft/demo", nil)
	}
	release <- struct{}{}
	waitSignal(t, started, "the follow-up build")
	release <- struct{}{}
	waitUntil(t, 2*time.Second, func() bool { return !bc.isBuilding() }, "the build loop to finish")
	time.Sleep(100 * time.Millisecond)
	if n := calls.Load(); n != 2 {
		t.Fatalf("%d builds ran, want 2 (the first and one coalesced follow-up)", n)
	}
}

func TestRequestRebuildRecordsInputHash(t *testing.T) {
	home, _ := setupBuilderTest(t)
	appDir := makeTestApp(t, home, "demo")
	bc := makeBuilderController("test-hash")
	done := make(chan struct{}, 1)
	bc.runBuildFn = func(ctx context.Context, appId string, builderEnv map[string]string) {
		done <- struct{}{}
	}
	bc.RequestRebuild("draft/demo", nil)
	waitSignal(t, done, "the build")
	want, err := ComputeAppInputHash(appDir)
	if err != nil {
		t.Fatal(err)
	}
	if got := bc.getLastBuildInputHash(); got != want {
		t.Fatalf("last build input hash %q, want %q", got, want)
	}
}

func TestRebuildStopsPreviousProcess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses the sleep command")
	}
	home, _ := setupBuilderTest(t)
	makeTestApp(t, home, "demo")
	bc := makeBuilderController("test-stop-previous")
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	exited := make(chan struct{})
	go func() {
		cmd.Wait()
		close(exited)
	}()
	bc.process = &BuilderProcess{Cmd: cmd, WaitCh: exited}

	sawProcess := make(chan bool, 1)
	bc.runBuildFn = func(ctx context.Context, appId string, builderEnv map[string]string) {
		sawProcess <- bc.hasProcess()
	}
	bc.RequestRebuild("draft/demo", nil)
	select {
	case had := <-sawProcess:
		if had {
			t.Fatal("the previous app process was still attached when the new build started")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the build did not start")
	}
	waitSignal(t, exited, "the previous app process to exit")
}

func TestDeleteControllerDuringBuildLeavesNoProcess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses the sleep command")
	}
	home, _ := setupBuilderTest(t)
	makeTestApp(t, home, "teardown-build")
	bc := GetOrCreateController("test-teardown-build")
	started := make(chan struct{}, 4)
	release := make(chan struct{})
	var calls atomic.Int32
	var appProcess *exec.Cmd
	bc.runBuildFn = func(ctx context.Context, appId string, builderEnv map[string]string) {
		calls.Add(1)
		started <- struct{}{}
		<-release
		// what a successful buildAndRun leaves behind: a running app attached to the controller
		appProcess = exec.Command("sleep", "30")
		if err := appProcess.Start(); err != nil {
			t.Error(err)
			return
		}
		attachTestProcess(bc, appProcess)
	}
	bc.RequestRebuild("draft/teardown-build", nil)
	waitSignal(t, started, "the first build")
	bc.RequestRebuild("draft/teardown-build", nil)

	deleted := make(chan struct{})
	go func() {
		DeleteController("test-teardown-build")
		close(deleted)
	}()
	waitUntil(t, 2*time.Second, func() bool { return bc.isClosed() }, "the controller to be marked closed")
	bc.RequestRebuild("draft/teardown-build", nil)
	release <- struct{}{}
	waitSignal(t, deleted, "DeleteController to return")

	time.Sleep(200 * time.Millisecond)
	if n := calls.Load(); n != 1 {
		t.Fatalf("%d builds ran, want 1: a queued build ran after teardown", n)
	}
	if bc.hasProcess() {
		t.Fatal("an app process is still attached after teardown")
	}
	exited := make(chan struct{})
	go func() {
		appProcess.Wait()
		close(exited)
	}()
	waitSignal(t, exited, "the app process to be killed")
}

func attachTestProcess(bc *BuilderController, cmd *exec.Cmd) {
	bc.lock.Lock()
	defer bc.lock.Unlock()
	bc.process = &BuilderProcess{Cmd: cmd}
}

func TestRecordAppInputHashUpdatesMatchingControllers(t *testing.T) {
	home, _ := setupBuilderTest(t)
	appDir := makeTestApp(t, home, "recorded")
	match := GetOrCreateController("test-record-match")
	other := GetOrCreateController("test-record-other")
	t.Cleanup(func() {
		DeleteController("test-record-match")
		DeleteController("test-record-other")
	})
	setTestAppId(match, "draft/recorded")
	setTestAppId(other, "draft/elsewhere")
	other.setLastBuildInputHash("untouched")

	RecordAppInputHash("draft/recorded")
	want, err := ComputeAppInputHash(appDir)
	if err != nil {
		t.Fatal(err)
	}
	if got := match.getLastBuildInputHash(); got != want {
		t.Fatalf("matching controller hash %q, want %q", got, want)
	}
	if got := other.getLastBuildInputHash(); got != "untouched" {
		t.Fatalf("a controller for another app was updated: %q", got)
	}
}

func setTestAppId(bc *BuilderController, appId string) {
	bc.lock.Lock()
	defer bc.lock.Unlock()
	bc.appId = appId
}
```

Append to `pkg/buildercontroller/buildercontroller_test.go`:

```go
func TestBuildLogRecordsLatestBuildOnly(t *testing.T) {
	home, resources := setupBuilderTest(t)
	appDir := makeTestApp(t, home, "demo")
	bc := makeBuilderController("test-build-log")
	bc.appId = "draft/demo"
	bc.outputBuffer = utilds.MakeMultiReaderLineBuffer(100)
	bc.buildAndRun(context.Background(), "draft/demo", nil, nil)

	logPath := filepath.Join(appDir, ".tsunami", "build.log")
	first, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("build log not written: %v", err)
	}
	if !strings.Contains(string(first), "Tsunami SDK not found") || !strings.HasSuffix(string(first), "status: error\n") {
		t.Fatalf("first build log:\n%s", first)
	}

	sdk := filepath.Join(resources, "tsunamisdk")
	if err := os.MkdirAll(sdk, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sdk, "go.mod"), []byte("module github.com/LannCo/remoteterm/tsunami\n\ngo 1.25.6\n"), 0644); err != nil {
		t.Fatal(err)
	}
	bc.outputBuffer = utilds.MakeMultiReaderLineBuffer(100)
	bc.buildAndRun(context.Background(), "draft/demo", nil, nil)
	second, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(second), "Tsunami SDK not found") || !strings.Contains(string(second), "Tsunami scaffold not found") {
		t.Fatalf("second build log was not truncated or is wrong:\n%s", second)
	}
}

func TestBuildLogFailureDoesNotChangeOutcome(t *testing.T) {
	home, resources := setupBuilderTest(t)
	appDir := makeTestApp(t, home, "demo")
	if err := os.WriteFile(filepath.Join(appDir, ".tsunami"), []byte("not a directory"), 0644); err != nil {
		t.Fatal(err)
	}
	bc := makeBuilderController("test-build-log-fail")
	bc.appId = "draft/demo"
	bc.outputBuffer = utilds.MakeMultiReaderLineBuffer(100)
	resultCh := make(chan *BuildResult, 1)
	bc.buildAndRun(context.Background(), "draft/demo", nil, resultCh)
	want := fmt.Sprintf(`Tsunami SDK not found at %s. Rebuild with "task build:tsunamisdk", or set "tsunami:sdkreplacepath" in Settings.`, filepath.Join(resources, "tsunamisdk"))
	if result := <-resultCh; result.ErrorMessage != want {
		t.Fatalf("result error %q, want %q", result.ErrorMessage, want)
	}
	if status := bc.GetStatus(); status.Status != BuilderStatus_Error || status.ErrorMsg != want {
		t.Fatalf("status %q / %q", status.Status, status.ErrorMsg)
	}
}
```

Add `"strings"` to that file's imports.

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./pkg/buildercontroller/... -v`
Expected: FAIL to compile: `undefined: IsRelevantAppPath`, `undefined: ComputeAppInputHash`, `undefined: makeBuilderController`.

- [ ] **Step 3: Implement the input rules and hash**

Create `pkg/buildercontroller/appinputs.go`:

```go
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package buildercontroller

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
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
func ComputeAppInputHash(appDir string) (string, error) {
	hasher := sha256.New()
	err := filepath.WalkDir(appDir, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if p == appDir {
				return walkErr
			}
			return nil
		}
		rel, err := filepath.Rel(appDir, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
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
			info, err := d.Info()
			if err != nil {
				return nil
			}
			fmt.Fprintf(hasher, "%s\x00%d\x00%d\x00", rel, info.Size(), info.ModTime().UnixNano())
			return nil
		}
		data, err := os.ReadFile(p)
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
```

`filepath.WalkDir` visits entries in lexical order, so the hash is deterministic without sorting.

- [ ] **Step 4: Rework the controller's build entry points**

In `pkg/buildercontroller/buildercontroller.go`:

Add to the const block (lines 32-38):

```go
	BuildLogFileName = ".tsunami/build.log"
```

Add to the `BuilderController` struct (after `errorMsg string`, line 65):

```go
	closed             bool
	building           bool
	rebuildPending     bool
	pendingAppId       string
	pendingEnv         map[string]string
	lastBuildInputHash string
	runBuildFn         func(ctx context.Context, appId string, builderEnv map[string]string)
```

In `GetOrCreateController`, replace lines 82-86 (the struct literal) with `bc = makeBuilderController(builderId)` and add:

```go
func makeBuilderController(builderId string) *BuilderController {
	bc := &BuilderController{
		builderId: builderId,
		status:    BuilderStatus_Init,
	}
	bc.runBuildFn = func(ctx context.Context, appId string, builderEnv map[string]string) {
		bc.buildAndRun(ctx, appId, builderEnv, nil)
	}
	return bc
}
```

Replace `Start` (lines 159-191) with:

```go
func (bc *BuilderController) Start(ctx context.Context, appId string, builderEnv map[string]string) error {
	bc.RequestRebuild(appId, builderEnv)
	return nil
}

// RequestRebuild never waits for a build, so the RPC that calls it returns at once and
// the RPC timeout never applies to a build. Requests that arrive during a build
// collapse into a single follow-up build.
func (bc *BuilderController) RequestRebuild(appId string, builderEnv map[string]string) {
	bc.recordInputHash(appId)
	if !bc.queueBuild(appId, builderEnv) {
		return
	}
	go func() {
		defer func() {
			panichandler.PanicHandler(fmt.Sprintf("buildercontroller[%s].buildLoop", bc.builderId), recover())
		}()
		bc.buildLoop()
	}()
}

func (bc *BuilderController) queueBuild(appId string, builderEnv map[string]string) bool {
	bc.lock.Lock()
	defer bc.lock.Unlock()
	if bc.closed {
		return false
	}
	bc.pendingAppId = appId
	bc.pendingEnv = builderEnv
	if bc.building {
		bc.rebuildPending = true
		return false
	}
	bc.building = true
	return true
}

func (bc *BuilderController) buildLoop() {
	for {
		appId, builderEnv, ok := bc.beginBuild()
		if !ok {
			return
		}
		bc.recordInputHash(appId)
		buildCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		bc.runBuildFn(buildCtx, appId, builderEnv)
		cancel()
		if !bc.endBuild() {
			return
		}
	}
}

func (bc *BuilderController) beginBuild() (string, map[string]string, bool) {
	bc.lock.Lock()
	defer bc.lock.Unlock()
	if bc.closed {
		bc.building = false
		return "", nil, false
	}
	bc.rebuildPending = false
	if bc.process != nil {
		log.Printf("BuilderController: stopping previous app %s for builder %s", bc.appId, bc.builderId)
		bc.stopProcess_nolock()
	}
	bc.appId = bc.pendingAppId
	bc.outputBuffer = utilds.MakeMultiReaderLineBuffer(1000)
	bc.setStatus_nolock(BuilderStatus_Building, 0, 0, "")
	bc.publishOutputLine("", true)
	bc.outputBuffer.SetLineCallback(func(line string) {
		bc.publishOutputLine(line, false)
	})
	return bc.pendingAppId, bc.pendingEnv, true
}

func (bc *BuilderController) endBuild() bool {
	bc.lock.Lock()
	defer bc.lock.Unlock()
	if bc.rebuildPending && !bc.closed {
		return true
	}
	bc.building = false
	return false
}

// Closing is permanent: a controller being torn down must not start a queued build
// or act on a late watcher callback, either of which would leave an orphan app process.
func (bc *BuilderController) markClosed() {
	bc.lock.Lock()
	defer bc.lock.Unlock()
	bc.closed = true
	bc.rebuildPending = false
}

func (bc *BuilderController) isClosed() bool {
	bc.lock.Lock()
	defer bc.lock.Unlock()
	return bc.closed
}

func (bc *BuilderController) clearPendingRebuild() {
	bc.lock.Lock()
	defer bc.lock.Unlock()
	bc.rebuildPending = false
}

func (bc *BuilderController) isBuilding() bool {
	bc.lock.Lock()
	defer bc.lock.Unlock()
	return bc.building
}

func (bc *BuilderController) hasProcess() bool {
	bc.lock.Lock()
	defer bc.lock.Unlock()
	return bc.process != nil
}

// The hash is taken when a build is requested as well as when it starts: a save that
// lands during a running build is then recognised as ours, not as an outside change.
func (bc *BuilderController) recordInputHash(appId string) {
	appDir, err := remotetermappstore.GetAppDir(appId)
	if err != nil {
		return
	}
	hash, err := ComputeAppInputHash(appDir)
	if err != nil {
		return
	}
	bc.setLastBuildInputHash(hash)
}

// RecordAppInputHash is called after the builder itself writes app files (a Code-tab
// save), so the watcher treats that write as ours whichever RPC reaches us first.
func RecordAppInputHash(appId string) {
	controllers := getControllersForApp(appId)
	if len(controllers) == 0 {
		return
	}
	appDir, err := remotetermappstore.GetAppDir(appId)
	if err != nil {
		return
	}
	hash, err := ComputeAppInputHash(appDir)
	if err != nil {
		return
	}
	for _, bc := range controllers {
		bc.setLastBuildInputHash(hash)
	}
}

func getControllersForApp(appId string) []*BuilderController {
	mapLock.Lock()
	defer mapLock.Unlock()
	var controllers []*BuilderController
	for _, bc := range controllerMap {
		if bc.getAppId() == appId {
			controllers = append(controllers, bc)
		}
	}
	return controllers
}

func (bc *BuilderController) setLastBuildInputHash(hash string) {
	bc.lock.Lock()
	defer bc.lock.Unlock()
	bc.lastBuildInputHash = hash
}

func (bc *BuilderController) getLastBuildInputHash() string {
	bc.lock.Lock()
	defer bc.lock.Unlock()
	return bc.lastBuildInputHash
}

// Agents working in the app folder cannot see the Build panel; this file is how they
// read compile errors. Failing to write it must never change the build result.
func writeBuildLog(appId string, lines []string, statusLine string) {
	if appId == "" {
		return
	}
	var buf strings.Builder
	for _, line := range lines {
		buf.WriteString(line)
		buf.WriteString("\n")
	}
	buf.WriteString(statusLine)
	buf.WriteString("\n")
	if err := remotetermappstore.WriteAppFile(appId, BuildLogFileName, []byte(buf.String())); err != nil {
		log.Printf("BuilderController: cannot write build log for %s: %v", appId, err)
	}
}
```

In `buildAndRun`, directly after the `bc.lock.Unlock()` that follows `bc.setStatus_nolock(BuilderStatus_Running, process.Port, 0, "")` (original line 273; not the one inside the `<-process.WaitCh` goroutine), add:

```go
	writeBuildLog(appId, outputCapture.GetLines(), fmt.Sprintf("status: running on port %d", process.Port))
```

In `handleBuildError`, after the `[error]` output line added in Task 4, add:

```go
	var lines []string
	if bc.outputBuffer != nil {
		lines = bc.outputBuffer.GetLines()
	}
	writeBuildLog(bc.appId, lines, "status: error")
```

In `Stop` (line 480), add `bc.clearPendingRebuild()` as the first statement. In `waitForBuildDone` (lines 139-157), replace the status read (`bc.statusLock.Lock()` through the `if status != BuilderStatus_Building { return nil }` block) with:

```go
		if !bc.isBuilding() {
			return nil
		}
```

so `Stop` waits for the build loop itself (including a coalesced follow-up), not for a status value that a follow-up build may not have set yet.

In `DeleteController` (lines 98-107) and `Shutdown` (lines 126-137), call `bc.markClosed()` before `bc.Stop()`:

```go
	if bc != nil {
		bc.markClosed()
		bc.Stop()
	}
```

```go
	for _, bc := range controllers {
		bc.markClosed()
		bc.Stop()
	}
```

Delete `RestartAndWaitForBuild` (lines 437-478). Its only caller is `RestartBuilderAndWaitCommand` (`pkg/wshrpc/wshserver/wshserver.go:1185-1211`), and nothing outside generated code calls that RPC (`grep -rn RestartBuilderAndWait frontend emain cmd` finds only `frontend/app/store/wshclientapi.ts` and `frontend/types/gotypes.d.ts`, both generated). Remove the RPC as well: the `RestartBuilderAndWaitCommand` line from `WshRpcBuilderInterface` (`wshrpctypes_builder.go:24`), the types `CommandRestartBuilderAndWaitData` and `RestartBuilderAndWaitResult` (lines 108-116), and the server method. `BuildResult` stays: `buildAndRun` and the tests use it. `npx task generate` in Step 5 drops the generated client functions.

In `pkg/wshrpc/wshserver/wshserver.go` `WriteAppGoFileCommand` (line 1109), after the `WriteAppFile` error check, add:

```go
	buildercontroller.RecordAppInputHash(data.AppId)
```

The Code-tab save then records its input hash on the server before it returns, so the watcher recognises the write as ours whether the save or the rebuild request arrives first.

`GetStatus` (line 508) reads `bc.appId` under `statusLock` while `Start` (today) and `beginBuild` (now) write it under `bc.lock`; the new tests expose that existing race under `-race`. Take a snapshot through `bc.lock` before taking `statusLock`, and use the local `appId` for every later read in the function (`ReadAppManifest`, `ReadAppSecretBindings`, `BuildAppSecretEnv`):

```go
func (bc *BuilderController) GetStatus() wshrpc.BuilderStatusData {
	appId := bc.getAppId()
	bc.statusLock.Lock()
	defer bc.statusLock.Unlock()
```

```go
func (bc *BuilderController) getAppId() string {
	bc.lock.Lock()
	defer bc.lock.Unlock()
	return bc.appId
}
```

This is safe because nothing calls `GetStatus` while holding `bc.lock`: `setStatus_nolock` publishes from a new goroutine (`go bc.publishStatus()`).

- [ ] **Step 5: Add the RPC**

Read `.kilocode/skills/add-rpc/SKILL.md`. In `pkg/wshrpc/wshrpctypes_builder.go`, add to the interface (after `StartBuilderCommand`, line 22):

```go
	RequestBuilderRebuildCommand(ctx context.Context, data CommandRequestBuilderRebuildData) error
```

and append:

```go
type CommandRequestBuilderRebuildData struct {
	BuilderId string `json:"builderid"`
}
```

In `pkg/wshrpc/wshserver/wshserver.go`, add after `StartBuilderCommand` (ends line 1172):

```go
func (ws *WshServer) RequestBuilderRebuildCommand(ctx context.Context, data wshrpc.CommandRequestBuilderRebuildData) error {
	if data.BuilderId == "" {
		return fmt.Errorf("must provide a builderId to RequestBuilderRebuildCommand")
	}
	rtInfo := rtstore.GetRTInfo(remotetermobj.MakeORef("builder", data.BuilderId))
	if rtInfo == nil {
		return fmt.Errorf("builder rtinfo not found for builderid: %s", data.BuilderId)
	}
	if rtInfo.BuilderAppId == "" {
		return fmt.Errorf("builder appid not set for builderid: %s", data.BuilderId)
	}
	buildercontroller.GetOrCreateController(data.BuilderId).RequestRebuild(rtInfo.BuilderAppId, rtInfo.BuilderEnv)
	return nil
}
```

Run: `npx task generate`
Expected: exits 0; `git status --short` shows the three generated files from the Files list changed and nothing else generated.

- [ ] **Step 6: Run the tests and vet**

Run: `go test ./pkg/buildercontroller/... -v && go vet ./pkg/buildercontroller/... ./pkg/wshrpc/...`
Expected: PASS; vet silent.

Run: `go test -race -count=2 ./pkg/buildercontroller/...`
Expected: `ok`, no `DATA RACE` report.

Run: `grep -rn 'RestartAndWaitForBuild\|RestartBuilderAndWait' pkg frontend emain cmd`
Expected: no output.

Run: `npx tsc --noEmit`
Expected: exits 0.

- [ ] **Step 7: Commit**

```bash
git add pkg/buildercontroller/appinputs.go pkg/buildercontroller/appinputs_test.go pkg/buildercontroller/rebuild_test.go pkg/buildercontroller/buildercontroller.go pkg/buildercontroller/buildercontroller_test.go pkg/wshrpc/wshrpctypes_builder.go pkg/wshrpc/wshserver/wshserver.go pkg/wshrpc/wshclient/wshclient.go frontend/types/gotypes.d.ts frontend/app/store/wshclientapi.ts
git commit -m "feat(builder): coalesce rebuild requests and write .tsunami/build.log

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 10: App folder watcher, `WatchBuilderAppCommand`, `rtapp:watchstatus`, `builder:liverebuild`

**Files:**
- Create: `pkg/buildercontroller/appwatcher.go`
- Create: `pkg/buildercontroller/appwatcher_test.go`
- Create: `pkg/buildercontroller/watch.go`
- Create: `pkg/buildercontroller/watch_test.go`
- Modify: `pkg/buildercontroller/buildercontroller.go:98-107` (`DeleteController`), `:126-137` (`Shutdown`), struct (add `watcher *AppWatcher`)
- Modify: `pkg/rtconfig/settingsconfig.go:139` (add the `builder:` group before the `tsunami:` group)
- Modify: `pkg/wps/wpstypes.go:17-55` (event constant and `AllEvents`)
- Modify: `pkg/tsgen/tsgenevent.go:23-41` (event data type)
- Modify: `pkg/wshrpc/wshrpctypes_builder.go` (interface, `CommandWatchBuilderAppData`, `BuilderWatchStatusData`)
- Modify: `pkg/wshrpc/wshserver/wshserver.go` (add `WatchBuilderAppCommand` after `RequestBuilderRebuildCommand`)
- Modify: `docs/docs/config.mdx:106` (settings table row)
- Generated: `frontend/types/gotypes.d.ts`, `frontend/types/remotetermevent.d.ts`, `frontend/app/store/wshclientapi.ts`, `pkg/wshrpc/wshclient/wshclient.go`, `pkg/rtconfig/metaconsts.go`, `schema/settings.json`

**Interfaces:**
- Consumes: `IsRelevantAppPath`, `ComputeAppInputHash`, `RequestRebuild`, `getLastBuildInputHash`, `setLastBuildInputHash`, `makeBuilderController`, `runBuildFn`, `markClosed`, `isClosed`, `waitUntil` (Task 9); `setupBuilderTest`, `makeTestApp` (Task 4); `remotetermappstore.CheckNoSymlinks` (Task 5).
- Produces:
  - consts `WatchStatus_Active = "active"`, `WatchStatus_Unavailable = "unavailable"`, `MaxAppWatchers = 8`
  - `MakeAppWatcher(appDir string, onChange func(), onStatus func(status string, reason string)) (*AppWatcher, error)` and `(*AppWatcher).Close()`
  - `(*BuilderController).StartWatching(appId string) wshrpc.BuilderWatchStatusData`, `(*BuilderController).StopWatching()`
  - unexported seams: `var watchDebounce, rootPollInterval, rootUnavailableAfter time.Duration`, `var maxWatchedDirs int`, `var liveRebuildEnabled func() bool`, `var publishAppGoUpdated func(appId string)`, `func activeWatcherCount() int`
  - `wps.Event_BuilderWatchStatus = "rtapp:watchstatus"` (data `wshrpc.BuilderWatchStatusData`)
  - `wshrpc.BuilderWatchStatusData{Status string "status"; Reason string "reason,omitempty"}`; TS type `BuilderWatchStatusData`
  - RPC `WatchBuilderAppCommand(ctx, CommandWatchBuilderAppData{BuilderId}) (*BuilderWatchStatusData, error)`; TS `RpcApi.WatchBuilderAppCommand(TabRpcClient, { builderid })`
  - setting `builder:liverebuild` (Go `SettingsType.BuilderLiveRebuild bool`, TS `SettingsType["builder:liverebuild"]?: boolean`)

Read `.kilocode/skills/wps-events/SKILL.md`, `.kilocode/skills/add-config/SKILL.md` and `.kilocode/skills/add-rpc/SKILL.md` before Step 5. The add-config guide names `pkg/wconfig`; in this repo the package is `pkg/rtconfig`.

- [ ] **Step 1: Write the failing watcher tests**

Create `pkg/buildercontroller/appwatcher_test.go`:

```go
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package buildercontroller

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

type watchRecorder struct {
	lock     sync.Mutex
	changes  int
	statuses []string
}

func (r *watchRecorder) onChange() {
	r.lock.Lock()
	defer r.lock.Unlock()
	r.changes++
}

func (r *watchRecorder) onStatus(status string, reason string) {
	r.lock.Lock()
	defer r.lock.Unlock()
	r.statuses = append(r.statuses, status)
}

func (r *watchRecorder) changeCount() int {
	r.lock.Lock()
	defer r.lock.Unlock()
	return r.changes
}

func (r *watchRecorder) lastStatus() string {
	r.lock.Lock()
	defer r.lock.Unlock()
	if len(r.statuses) == 0 {
		return ""
	}
	return r.statuses[len(r.statuses)-1]
}

func shortWatchTimings(t *testing.T) {
	t.Helper()
	origDebounce, origPoll, origUnavailable, origMaxDirs := watchDebounce, rootPollInterval, rootUnavailableAfter, maxWatchedDirs
	watchDebounce = 100 * time.Millisecond
	rootPollInterval = 50 * time.Millisecond
	rootUnavailableAfter = 300 * time.Millisecond
	t.Cleanup(func() {
		watchDebounce, rootPollInterval, rootUnavailableAfter, maxWatchedDirs = origDebounce, origPoll, origUnavailable, origMaxDirs
	})
}

func startTestWatcher(t *testing.T, appDir string) *watchRecorder {
	t.Helper()
	rec := &watchRecorder{}
	w, err := MakeAppWatcher(appDir, rec.onChange, rec.onStatus)
	if err != nil {
		t.Fatalf("MakeAppWatcher: %v", err)
	}
	t.Cleanup(w.Close)
	return rec
}

func writeAppFileForTest(t *testing.T, appDir string, rel string, content string) {
	t.Helper()
	path := filepath.Join(appDir, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestWatcherBurstDebouncedToOneChange(t *testing.T) {
	shortWatchTimings(t)
	home, _ := setupBuilderTest(t)
	appDir := makeTestApp(t, home, "burst")
	rec := startTestWatcher(t, appDir)
	for i := 0; i < 5; i++ {
		writeAppFileForTest(t, appDir, "app.go", fmt.Sprintf("package main // %d\n", i))
		time.Sleep(20 * time.Millisecond)
	}
	waitUntil(t, 2*time.Second, func() bool { return rec.changeCount() >= 1 }, "a change notification")
	time.Sleep(300 * time.Millisecond)
	if n := rec.changeCount(); n != 1 {
		t.Fatalf("%d change notifications for one burst, want 1", n)
	}
}

func TestWatcherIgnoresGeneratedAndTempFiles(t *testing.T) {
	shortWatchTimings(t)
	home, _ := setupBuilderTest(t)
	appDir := makeTestApp(t, home, "ignored")
	writeAppFileForTest(t, appDir, "static/keep.txt", "x")
	rec := startTestWatcher(t, appDir)
	for _, rel := range []string{"go.mod", "go.sum", "manifest.json", "static/tw.css", ".tsunami/build.log", "app.go~", ".app.go.swp", "4913", "bin/app"} {
		writeAppFileForTest(t, appDir, rel, "generated")
	}
	time.Sleep(500 * time.Millisecond)
	if n := rec.changeCount(); n != 0 {
		t.Fatalf("%d change notifications for ignored files, want 0", n)
	}
}

func TestWatcherAtomicRenameSaveFiresOnce(t *testing.T) {
	shortWatchTimings(t)
	home, _ := setupBuilderTest(t)
	appDir := makeTestApp(t, home, "rename")
	rec := startTestWatcher(t, appDir)
	writeAppFileForTest(t, appDir, "app.go.tmp.4242", "package main // new\n")
	time.Sleep(3 * watchDebounce)
	if n := rec.changeCount(); n != 0 {
		t.Fatalf("%d change notifications for the temp file alone, want 0", n)
	}
	if err := os.Rename(filepath.Join(appDir, "app.go.tmp.4242"), filepath.Join(appDir, "app.go")); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, 2*time.Second, func() bool { return rec.changeCount() >= 1 }, "a change notification")
	time.Sleep(300 * time.Millisecond)
	if n := rec.changeCount(); n != 1 {
		t.Fatalf("%d change notifications for one rename-save, want 1", n)
	}
}

func TestWatcherWatchesNewStaticSubdir(t *testing.T) {
	shortWatchTimings(t)
	home, _ := setupBuilderTest(t)
	appDir := makeTestApp(t, home, "subdir")
	writeAppFileForTest(t, appDir, "static/keep.txt", "x")
	rec := startTestWatcher(t, appDir)
	writeAppFileForTest(t, appDir, "static/img/a.png", "a")
	waitUntil(t, 2*time.Second, func() bool { return rec.changeCount() >= 1 }, "a change for the new directory")
	time.Sleep(300 * time.Millisecond)
	before := rec.changeCount()
	writeAppFileForTest(t, appDir, "static/img/b.png", "b")
	waitUntil(t, 2*time.Second, func() bool { return rec.changeCount() > before }, "a change inside the new directory")
}

func TestWatcherRootRemovedThenRecreated(t *testing.T) {
	shortWatchTimings(t)
	home, _ := setupBuilderTest(t)
	appDir := makeTestApp(t, home, "gone")
	rec := startTestWatcher(t, appDir)
	if err := os.RemoveAll(appDir); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, 3*time.Second, func() bool { return rec.lastStatus() == WatchStatus_Unavailable }, "the unavailable status")
	before := rec.changeCount()
	writeAppFileForTest(t, appDir, "app.go", "package main // back\n")
	waitUntil(t, 3*time.Second, func() bool { return rec.lastStatus() == WatchStatus_Active }, "the active status after recreation")
	waitUntil(t, 2*time.Second, func() bool { return rec.changeCount() > before }, "a rescan change after recreation")
}

func TestWatcherCapOfEight(t *testing.T) {
	home, _ := setupBuilderTest(t)
	if n := activeWatcherCount(); n != 0 {
		t.Fatalf("%d watchers leaked from earlier tests", n)
	}
	var watchers []*AppWatcher
	for i := 0; i < MaxAppWatchers; i++ {
		w, err := MakeAppWatcher(makeTestApp(t, home, fmt.Sprintf("cap%d", i)), func() {}, func(string, string) {})
		if err != nil {
			t.Fatalf("watcher %d: %v", i, err)
		}
		watchers = append(watchers, w)
	}
	extraDir := makeTestApp(t, home, "cap-extra")
	if _, err := MakeAppWatcher(extraDir, func() {}, func(string, string) {}); err == nil || !strings.Contains(err.Error(), "limit 8") {
		t.Fatalf("ninth watcher: got %v, want a limit error", err)
	}
	watchers[0].Close()
	extra, err := MakeAppWatcher(extraDir, func() {}, func(string, string) {})
	if err != nil {
		t.Fatalf("watcher after closing one: %v", err)
	}
	extra.Close()
	for _, w := range watchers[1:] {
		w.Close()
	}
	if n := activeWatcherCount(); n != 0 {
		t.Fatalf("%d watchers still counted after Close", n)
	}
}

func TestWatcherCloseReleasesFds(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("counts /proc/self/fd")
	}
	home, _ := setupBuilderTest(t)
	appDir := makeTestApp(t, home, "fds")
	countFds := func() int {
		entries, err := os.ReadDir("/proc/self/fd")
		if err != nil {
			t.Fatal(err)
		}
		return len(entries)
	}
	before := countFds()
	for i := 0; i < 3; i++ {
		w, err := MakeAppWatcher(appDir, func() {}, func(string, string) {})
		if err != nil {
			t.Fatal(err)
		}
		w.Close()
	}
	if after := countFds(); after > before {
		t.Fatalf("open fds went from %d to %d after closing watchers", before, after)
	}
}
```

Create `pkg/buildercontroller/watch_test.go`:

```go
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package buildercontroller

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func overrideWatchSeams(t *testing.T, live bool) *atomic.Int32 {
	t.Helper()
	origLive, origPublish := liveRebuildEnabled, publishAppGoUpdated
	var published atomic.Int32
	liveRebuildEnabled = func() bool { return live }
	publishAppGoUpdated = func(appId string) { published.Add(1) }
	t.Cleanup(func() {
		liveRebuildEnabled, publishAppGoUpdated = origLive, origPublish
	})
	return &published
}

func TestHandleAppFilesChangedSuppressesEcho(t *testing.T) {
	home, _ := setupBuilderTest(t)
	appDir := makeTestApp(t, home, "echo")
	published := overrideWatchSeams(t, true)
	bc := makeBuilderController("test-echo")
	var builds atomic.Int32
	bc.runBuildFn = func(ctx context.Context, appId string, builderEnv map[string]string) { builds.Add(1) }

	hash, err := ComputeAppInputHash(appDir)
	if err != nil {
		t.Fatal(err)
	}
	bc.setLastBuildInputHash(hash)
	bc.handleAppFilesChanged("draft/echo")
	if published.Load() != 0 || builds.Load() != 0 {
		t.Fatalf("echo of our own build: %d events, %d builds; want 0 and 0", published.Load(), builds.Load())
	}

	if err := os.WriteFile(filepath.Join(appDir, "app.go"), []byte("package main // outside\n"), 0644); err != nil {
		t.Fatal(err)
	}
	bc.handleAppFilesChanged("draft/echo")
	if published.Load() != 1 {
		t.Fatalf("%d appgoupdated events for an outside change, want 1", published.Load())
	}
	waitUntil(t, 2*time.Second, func() bool { return builds.Load() == 1 }, "the live rebuild")
}

func TestHandleAppFilesChangedWithLiveRebuildOff(t *testing.T) {
	home, _ := setupBuilderTest(t)
	appDir := makeTestApp(t, home, "manual")
	published := overrideWatchSeams(t, false)
	bc := makeBuilderController("test-manual")
	var builds atomic.Int32
	bc.runBuildFn = func(ctx context.Context, appId string, builderEnv map[string]string) { builds.Add(1) }
	if err := os.WriteFile(filepath.Join(appDir, "app.go"), []byte("package main // outside\n"), 0644); err != nil {
		t.Fatal(err)
	}
	bc.handleAppFilesChanged("draft/manual")
	time.Sleep(200 * time.Millisecond)
	if published.Load() != 1 || builds.Load() != 0 {
		t.Fatalf("live rebuild off: %d events, %d builds; want 1 and 0", published.Load(), builds.Load())
	}
}

func TestStartWatchingReportsUnavailableOverDirCap(t *testing.T) {
	home, _ := setupBuilderTest(t)
	appDir := makeTestApp(t, home, "toomany")
	for _, sub := range []string{"a", "b", "c"} {
		if err := os.MkdirAll(filepath.Join(appDir, "static", sub), 0755); err != nil {
			t.Fatal(err)
		}
	}
	origMax := maxWatchedDirs
	maxWatchedDirs = 3
	t.Cleanup(func() { maxWatchedDirs = origMax })
	bc := makeBuilderController("test-dircap")
	status := bc.StartWatching("draft/toomany")
	if status.Status != WatchStatus_Unavailable || !strings.Contains(status.Reason, "more than 3") {
		t.Fatalf("status = %+v", status)
	}
	if n := activeWatcherCount(); n != 0 {
		t.Fatalf("a failed watcher kept its slot (%d active)", n)
	}
}

func TestDeleteControllerStopsWatcher(t *testing.T) {
	home, _ := setupBuilderTest(t)
	makeTestApp(t, home, "teardown")
	bc := GetOrCreateController("test-teardown")
	if status := bc.StartWatching("draft/teardown"); status.Status != WatchStatus_Active {
		t.Fatalf("status = %+v", status)
	}
	if n := activeWatcherCount(); n != 1 {
		t.Fatalf("%d watchers active, want 1", n)
	}
	DeleteController("test-teardown")
	if n := activeWatcherCount(); n != 0 {
		t.Fatalf("%d watchers active after DeleteController, want 0", n)
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./pkg/buildercontroller/... -run 'Watcher|HandleAppFiles|StartWatching|DeleteControllerStops' -v`
Expected: FAIL to compile: `undefined: MakeAppWatcher`, `undefined: WatchStatus_Unavailable`, `undefined: liveRebuildEnabled`.

- [ ] **Step 3: Implement the watcher**

Create `pkg/buildercontroller/appwatcher.go`:

```go
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package buildercontroller

import (
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/LannCo/remoteterm/pkg/panichandler"
	"github.com/LannCo/remoteterm/pkg/remotetermappstore"
	"github.com/fsnotify/fsnotify"
)

const (
	WatchStatus_Active      = "active"
	WatchStatus_Unavailable = "unavailable"
	MaxAppWatchers          = 8
)

var (
	watchDebounce        = 300 * time.Millisecond
	rootPollInterval     = 1 * time.Second
	rootUnavailableAfter = 10 * time.Second
	maxWatchedDirs       = 1000
)

var (
	watcherSlotLock sync.Mutex
	watcherSlots    int
)

type AppWatcher struct {
	lock        sync.Mutex
	appDir      string
	fsw         *fsnotify.Watcher
	watchedDirs map[string]bool
	onChange    func()
	onStatus    func(status string, reason string)
	debounce    *time.Timer
	closed      bool
	rootMissing bool
	closeCh     chan struct{}
	wg          sync.WaitGroup
}

// The global cap bounds inotify usage across builder windows; each watcher holds one
// inotify instance plus one watch per directory.
func acquireWatcherSlot() bool {
	watcherSlotLock.Lock()
	defer watcherSlotLock.Unlock()
	if watcherSlots >= MaxAppWatchers {
		return false
	}
	watcherSlots++
	return true
}

func releaseWatcherSlot() {
	watcherSlotLock.Lock()
	defer watcherSlotLock.Unlock()
	watcherSlots--
}

func activeWatcherCount() int {
	watcherSlotLock.Lock()
	defer watcherSlotLock.Unlock()
	return watcherSlots
}

func MakeAppWatcher(appDir string, onChange func(), onStatus func(status string, reason string)) (*AppWatcher, error) {
	if !acquireWatcherSlot() {
		return nil, fmt.Errorf("too many app folders are being watched (limit %d)", MaxAppWatchers)
	}
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		releaseWatcherSlot()
		return nil, fmt.Errorf("cannot start a file watcher: %w", err)
	}
	w := &AppWatcher{
		appDir:      filepath.Clean(appDir),
		fsw:         fsw,
		watchedDirs: make(map[string]bool),
		onChange:    onChange,
		onStatus:    onStatus,
		closeCh:     make(chan struct{}),
	}
	if err := w.addTree(); err != nil {
		fsw.Close()
		releaseWatcherSlot()
		return nil, err
	}
	w.wg.Add(1)
	go w.run()
	return w, nil
}

func (w *AppWatcher) Close() {
	if !w.markClosed() {
		return
	}
	close(w.closeCh)
	w.fsw.Close()
	w.wg.Wait()
	releaseWatcherSlot()
}

func (w *AppWatcher) markClosed() bool {
	w.lock.Lock()
	defer w.lock.Unlock()
	if w.closed {
		return false
	}
	w.closed = true
	if w.debounce != nil {
		w.debounce.Stop()
	}
	return true
}

func (w *AppWatcher) isClosed() bool {
	w.lock.Lock()
	defer w.lock.Unlock()
	return w.closed
}

func (w *AppWatcher) addTree() error {
	if err := remotetermappstore.CheckNoSymlinks(w.appDir); err != nil {
		return err
	}
	info, err := os.Lstat(w.appDir)
	if err != nil {
		return fmt.Errorf("app folder %s is not available: %w", w.appDir, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("app folder %s is not a directory", w.appDir)
	}
	if err := w.addWatch(w.appDir); err != nil {
		return err
	}
	staticDir := filepath.Join(w.appDir, "static")
	if info, err := os.Lstat(staticDir); err == nil && info.IsDir() {
		return w.addDirTree(staticDir)
	}
	return nil
}

// WalkDir never follows symlinked directories, so a link planted under static/
// cannot pull an outside tree into the watch set.
func (w *AppWatcher) addDirTree(root string) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if path == root {
				return walkErr
			}
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		if path != root && isSkippedWatchDir(d.Name()) {
			return fs.SkipDir
		}
		return w.addWatch(path)
	})
}

func isSkippedWatchDir(name string) bool {
	return strings.HasPrefix(name, ".") || name == "node_modules"
}

func isWatchableNewDir(rel string) bool {
	parts := strings.Split(rel, "/")
	if parts[0] != "static" {
		return false
	}
	for _, part := range parts {
		if isSkippedWatchDir(part) {
			return false
		}
	}
	return true
}

func (w *AppWatcher) addWatch(dir string) error {
	w.lock.Lock()
	defer w.lock.Unlock()
	if w.closed || w.watchedDirs[dir] {
		return nil
	}
	if len(w.watchedDirs) >= maxWatchedDirs {
		return fmt.Errorf("more than %d folders to watch in %s", maxWatchedDirs, w.appDir)
	}
	if err := w.fsw.Add(dir); err != nil {
		return fmt.Errorf("cannot watch %s: %w", dir, err)
	}
	w.watchedDirs[dir] = true
	return nil
}

func (w *AppWatcher) forgetWatches(dir string) {
	w.lock.Lock()
	defer w.lock.Unlock()
	prefix := dir + string(filepath.Separator)
	for watched := range w.watchedDirs {
		if watched == dir || strings.HasPrefix(watched, prefix) {
			w.fsw.Remove(watched)
			delete(w.watchedDirs, watched)
		}
	}
}

func (w *AppWatcher) run() {
	defer w.wg.Done()
	defer func() {
		panichandler.PanicHandler("AppWatcher.run", recover())
	}()
	for {
		select {
		case <-w.closeCh:
			return
		case event, ok := <-w.fsw.Events:
			if !ok {
				return
			}
			w.handleEvent(event)
		case err, ok := <-w.fsw.Errors:
			if !ok {
				return
			}
			log.Printf("app watcher %s: %v", w.appDir, err)
			if errors.Is(err, fsnotify.ErrEventOverflow) {
				w.scheduleChange()
			}
		}
	}
}

func (w *AppWatcher) handleEvent(event fsnotify.Event) {
	if event.Op == fsnotify.Chmod {
		return
	}
	name := filepath.Clean(event.Name)
	if name == w.appDir {
		if event.Op&(fsnotify.Remove|fsnotify.Rename) != 0 {
			w.handleRootLost()
		}
		return
	}
	if event.Op&(fsnotify.Remove|fsnotify.Rename) != 0 {
		w.forgetWatches(name)
	}
	rel, err := filepath.Rel(w.appDir, name)
	if err != nil {
		return
	}
	rel = filepath.ToSlash(rel)
	if event.Op&fsnotify.Create != 0 && isWatchableNewDir(rel) {
		if info, err := os.Lstat(name); err == nil && info.IsDir() {
			if err := w.addDirTree(name); err != nil {
				w.onStatus(WatchStatus_Unavailable, err.Error())
			}
			w.scheduleChange()
			return
		}
	}
	if IsRelevantAppPath(rel) {
		w.scheduleChange()
	}
}

func (w *AppWatcher) handleRootLost() {
	if !w.markRootMissing() {
		return
	}
	w.forgetWatches(w.appDir)
	w.wg.Add(1)
	go w.pollRoot()
}

func (w *AppWatcher) markRootMissing() bool {
	w.lock.Lock()
	defer w.lock.Unlock()
	if w.closed || w.rootMissing {
		return false
	}
	w.rootMissing = true
	return true
}

func (w *AppWatcher) setRootPresent() {
	w.lock.Lock()
	defer w.lock.Unlock()
	w.rootMissing = false
}

// Deleting or renaming the app folder drops every inotify watch; polling is the only
// way to notice it coming back (git checkout, an agent recreating the folder).
func (w *AppWatcher) pollRoot() {
	defer w.wg.Done()
	defer func() {
		panichandler.PanicHandler("AppWatcher.pollRoot", recover())
	}()
	start := time.Now()
	reported := false
	ticker := time.NewTicker(rootPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-w.closeCh:
			return
		case <-ticker.C:
		}
		if info, err := os.Lstat(w.appDir); err == nil && info.IsDir() {
			w.setRootPresent()
			if err := w.addTree(); err != nil {
				w.onStatus(WatchStatus_Unavailable, err.Error())
				return
			}
			w.onStatus(WatchStatus_Active, "")
			w.scheduleChange()
			return
		}
		if !reported && time.Since(start) >= rootUnavailableAfter {
			reported = true
			w.onStatus(WatchStatus_Unavailable, "the app folder is missing")
		}
	}
}

func (w *AppWatcher) scheduleChange() {
	w.lock.Lock()
	defer w.lock.Unlock()
	if w.closed {
		return
	}
	if w.debounce != nil {
		w.debounce.Stop()
	}
	w.debounce = time.AfterFunc(watchDebounce, w.fireChange)
}

func (w *AppWatcher) fireChange() {
	defer func() {
		panichandler.PanicHandler("AppWatcher.fireChange", recover())
	}()
	if w.isClosed() {
		return
	}
	w.onChange()
}
```

- [ ] **Step 4: Wire it into the controller**

Create `pkg/buildercontroller/watch.go`:

```go
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package buildercontroller

import (
	"github.com/LannCo/remoteterm/pkg/remotetermappstore"
	"github.com/LannCo/remoteterm/pkg/remotetermobj"
	"github.com/LannCo/remoteterm/pkg/rtconfig"
	"github.com/LannCo/remoteterm/pkg/rtstore"
	"github.com/LannCo/remoteterm/pkg/wps"
	"github.com/LannCo/remoteterm/pkg/wshrpc"
)

var liveRebuildEnabled = func() bool {
	return rtconfig.GetWatcher().GetFullConfig().Settings.BuilderLiveRebuild
}

var publishAppGoUpdated = func(appId string) {
	wps.Broker.Publish(wps.WaveEvent{
		Event:  wps.Event_WaveAppAppGoUpdated,
		Scopes: []string{appId},
	})
}

func (bc *BuilderController) StartWatching(appId string) wshrpc.BuilderWatchStatusData {
	appDir, err := remotetermappstore.GetAppDir(appId)
	if err != nil {
		return wshrpc.BuilderWatchStatusData{Status: WatchStatus_Unavailable, Reason: err.Error()}
	}
	bc.lock.Lock()
	defer bc.lock.Unlock()
	bc.stopWatcher_nolock()
	watcher, err := MakeAppWatcher(appDir, func() {
		bc.handleAppFilesChanged(appId)
	}, bc.publishWatchStatus)
	if err != nil {
		return wshrpc.BuilderWatchStatusData{Status: WatchStatus_Unavailable, Reason: err.Error()}
	}
	bc.watcher = watcher
	return wshrpc.BuilderWatchStatusData{Status: WatchStatus_Active}
}

func (bc *BuilderController) StopWatching() {
	bc.lock.Lock()
	defer bc.lock.Unlock()
	bc.stopWatcher_nolock()
}

func (bc *BuilderController) stopWatcher_nolock() {
	if bc.watcher == nil {
		return
	}
	bc.watcher.Close()
	bc.watcher = nil
}

// Off by default: an agent sandboxed to the app folder must not be able to run code
// outside the sandbox just by writing a file. The frontend offers a Rebuild button.
func (bc *BuilderController) handleAppFilesChanged(appId string) {
	if bc.isClosed() {
		return
	}
	appDir, err := remotetermappstore.GetAppDir(appId)
	if err != nil {
		return
	}
	hash, err := ComputeAppInputHash(appDir)
	if err == nil && hash == bc.getLastBuildInputHash() {
		return
	}
	publishAppGoUpdated(appId)
	if !liveRebuildEnabled() {
		return
	}
	var builderEnv map[string]string
	if rtInfo := rtstore.GetRTInfo(remotetermobj.MakeORef(remotetermobj.OType_Builder, bc.builderId)); rtInfo != nil {
		builderEnv = rtInfo.BuilderEnv
	}
	bc.RequestRebuild(appId, builderEnv)
}

func (bc *BuilderController) publishWatchStatus(status string, reason string) {
	wps.Broker.Publish(wps.WaveEvent{
		Event:  wps.Event_BuilderWatchStatus,
		Scopes: []string{remotetermobj.MakeORef(remotetermobj.OType_Builder, bc.builderId).String()},
		Data:   wshrpc.BuilderWatchStatusData{Status: status, Reason: reason},
	})
}
```

In `buildercontroller.go`, add `watcher *AppWatcher` to the struct. In `DeleteController` and `Shutdown`, call `bc.StopWatching()` between `bc.markClosed()` (Task 9) and `bc.Stop()`:

```go
	if bc != nil {
		bc.markClosed()
		bc.StopWatching()
		bc.Stop()
	}
```

```go
	for _, bc := range controllers {
		bc.markClosed()
		bc.StopWatching()
		bc.Stop()
	}
```

`handleAppFilesChanged` checks `isClosed()` first: a debounce timer can fire after `Close()` has returned and would otherwise queue a build on a controller being torn down.

- [ ] **Step 5: Add the setting, the event and the RPC**

`pkg/rtconfig/settingsconfig.go`, insert before `TsunamiClear` (line 139):

```go
	BuilderClear       bool `json:"builder:*,omitempty"`
	BuilderLiveRebuild bool `json:"builder:liverebuild,omitempty"`

```

No default is added to `pkg/rtconfig/defaultconfig/settings.json`: `false` is the default.

`pkg/wps/wpstypes.go`: add to the const block after `Event_WaveAppAppGoUpdated` (line 31):

```go
	Event_BuilderWatchStatus  = "rtapp:watchstatus"  // type: wshrpc.BuilderWatchStatusData
```

and add `Event_BuilderWatchStatus,` to `AllEvents` after `Event_WaveAppAppGoUpdated,` (line 51).

`pkg/tsgen/tsgenevent.go`: add after the `wps.Event_WaveAppAppGoUpdated: nil,` entry (line 37):

```go
	wps.Event_BuilderWatchStatus:  reflect.TypeOf(wshrpc.BuilderWatchStatusData{}),
```

`pkg/wshrpc/wshrpctypes_builder.go`: add to the interface after `RequestBuilderRebuildCommand`:

```go
	WatchBuilderAppCommand(ctx context.Context, data CommandWatchBuilderAppData) (*BuilderWatchStatusData, error)
```

and append:

```go
type CommandWatchBuilderAppData struct {
	BuilderId string `json:"builderid"`
}

type BuilderWatchStatusData struct {
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
}
```

`pkg/wshrpc/wshserver/wshserver.go`, after `RequestBuilderRebuildCommand`:

```go
func (ws *WshServer) WatchBuilderAppCommand(ctx context.Context, data wshrpc.CommandWatchBuilderAppData) (*wshrpc.BuilderWatchStatusData, error) {
	if data.BuilderId == "" {
		return nil, fmt.Errorf("must provide a builderId to WatchBuilderAppCommand")
	}
	rtInfo := rtstore.GetRTInfo(remotetermobj.MakeORef("builder", data.BuilderId))
	if rtInfo == nil {
		return nil, fmt.Errorf("builder rtinfo not found for builderid: %s", data.BuilderId)
	}
	if rtInfo.BuilderAppId == "" {
		return nil, fmt.Errorf("builder appid not set for builderid: %s", data.BuilderId)
	}
	status := buildercontroller.GetOrCreateController(data.BuilderId).StartWatching(rtInfo.BuilderAppId)
	return &status, nil
}
```

`docs/docs/config.mdx`: add after the `window:dimensions` row (line 106), padded to the table's column widths:

```markdown
| builder:liverebuild                                          | bool     | when `true`, the app builder rebuilds and restarts the app whenever files in its folder change outside the builder; when `false` (the default) it shows "Changed on disk" and waits for you to click Rebuild, so code written by another tool never runs without a click |
```

Run: `npx task generate`
Expected: exits 0. `git status --short` shows the six generated files from the Files list changed and no other generated file.

- [ ] **Step 6: Run the tests and vet**

Run: `go test ./pkg/buildercontroller/... ./pkg/tsgen/... ./pkg/wps/... ./pkg/rtconfig/... -v && go vet ./pkg/buildercontroller/... ./pkg/wshrpc/... ./pkg/wps/... ./pkg/tsgen/... ./pkg/rtconfig/...`
Expected: PASS; vet silent. The watcher tests use real inotify in temp dirs and take a few seconds.

Run: `go test -race -count=3 ./pkg/buildercontroller/...`
Expected: `ok` every time, no `DATA RACE` report. A failure that only appears on some runs is a real timing bug in the watcher or the test; report it with the output rather than widening a timeout.

Run: `npx tsc --noEmit`
Expected: exits 0.

- [ ] **Step 7: Commit**

```bash
git add pkg/buildercontroller/appwatcher.go pkg/buildercontroller/appwatcher_test.go pkg/buildercontroller/watch.go pkg/buildercontroller/watch_test.go pkg/buildercontroller/buildercontroller.go pkg/rtconfig/settingsconfig.go pkg/rtconfig/metaconsts.go pkg/wps/wpstypes.go pkg/tsgen/tsgenevent.go pkg/wshrpc/wshrpctypes_builder.go pkg/wshrpc/wshserver/wshserver.go pkg/wshrpc/wshclient/wshclient.go docs/docs/config.mdx schema/settings.json frontend/types/gotypes.d.ts frontend/types/remotetermevent.d.ts frontend/app/store/wshclientapi.ts
git commit -m "feat(builder): watch the app folder and report outside changes

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---
### Task 11: Frontend reload (`decideReload`, change handler, disk-changed bar, missing state, save path, live toggle, Rebuild, watch notice)

**Files:**
- Create: `frontend/builder/store/decide-reload.ts`
- Create: `frontend/builder/store/decide-reload.test.ts`
- Create: `frontend/builder/builder-appheader.tsx`
- Modify: `frontend/builder/store/builder-apppanel-model.ts` (imports lines 4-13, fields, constructor lines 45-52, `initialize`, `saveEnvVars`, `startBuilder`, `loadAppFile`, `saveAppFile`, `dispose`)
- Modify: `frontend/builder/builder-apppanel.tsx:366` (render `BuilderAppHeader`)
- Modify: `frontend/builder/tabs/builder-codetab.tsx:79-101` (disk-changed bar and layout)
- Modify: `frontend/builder/tabs/builder-previewtab.tsx` (missing view)

**Interfaces:**
- Consumes: `RpcApi.RequestBuilderRebuildCommand` (Task 9); `RpcApi.WatchBuilderAppCommand`, event `rtapp:watchstatus`, TS type `BuilderWatchStatusData`, setting key `builder:liverebuild` (Task 10); `BuilderAppPanelModel.seedStarterApp` (Task 8).
- Produces:
  - `decideReload(editor: string, original: string, disk: string, lastWritten: string): ReloadDecision` with `ReloadDecision` kinds `missing`, `sync-original` (`content`), `none`, `replace` (`content`), `conflict` (`disk`); `disk == null` means app.go is missing
  - model atoms `appGoMissingAtom: PrimitiveAtom<boolean>`, `diskChangedAtom: PrimitiveAtom<string>` (null when no conflict), `externalChangeAtom: PrimitiveAtom<boolean>`, `watchStatusAtom: PrimitiveAtom<BuilderWatchStatusData>`; field `lastWrittenContent: string`
  - model methods `requestRebuild()`, `handleAppGoUpdated(appId)`, `applyReloadDecision(decision)`, `loadDiskVersion()`, `keepMyEdits()`; `startBuilder()` now delegates to `requestRebuild()`; `debouncedRestart` is removed
  - the model subscribes to the `config` event and keeps `atoms.fullConfigAtom` current (field `configUnsubFn`), which the builder window otherwise never does
  - `BuilderAppHeader` component (named export) in `frontend/builder/builder-appheader.tsx`, containing `LiveRebuildToggle`, `ExternalChangeStrip`, `WatchStatusStrip`; Task 12 extends it

Rule order inside `decideReload` matters: a clean editor always follows the disk (so a stale `lastWritten` can never pin old content), and only a dirty editor can produce `none` or `conflict`.

- [ ] **Step 1: Write the failing vitest**

Create `frontend/builder/store/decide-reload.test.ts`:

```ts
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { describe, expect, it } from "vitest";
import { decideReload } from "./decide-reload";

describe("decideReload", () => {
    const cases: [string, string, string, string, string, ReturnType<typeof decideReload>][] = [
        ["app.go deleted", "A", "A", null, null, { kind: "missing" }],
        ["echo of our own save", "A", "A", "A", "A", { kind: "sync-original", content: "A" }],
        ["outside write identical to unsaved edits", "B", "A", "B", null, { kind: "sync-original", content: "B" }],
        ["clean editor, disk changed", "A", "A", "C", null, { kind: "replace", content: "C" }],
        ["clean editor ignores a stale lastWritten", "X", "X", "A", "A", { kind: "replace", content: "A" }],
        ["dirty editor, app.go unchanged (another file changed)", "B", "A", "A", null, { kind: "none" }],
        ["dirty editor, disk is what we just wrote", "B2", "A", "F", "F", { kind: "none" }],
        ["dirty editor, disk changed", "B", "A", "C", "A", { kind: "conflict", disk: "C" }],
        ["dirty editor, disk changed to empty", "B", "A", "", null, { kind: "conflict", disk: "" }],
    ];
    for (const [name, editor, original, disk, lastWritten, want] of cases) {
        it(name, () => {
            expect(decideReload(editor, original, disk, lastWritten)).toEqual(want);
        });
    }
});
```

- [ ] **Step 2: Run it to verify it fails**

Run: `npx vitest run frontend/builder/store/decide-reload.test.ts`
Expected: FAIL, cannot resolve `./decide-reload`.

- [ ] **Step 3: Implement `decideReload`**

Create `frontend/builder/store/decide-reload.ts`:

```ts
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

export type ReloadDecision =
    | { kind: "missing" }
    | { kind: "sync-original"; content: string }
    | { kind: "none" }
    | { kind: "replace"; content: string }
    | { kind: "conflict"; disk: string };

// Kept pure so the rules that protect unsaved edits are tested without an editor.
// A clean editor is checked before lastWritten so a stale lastWritten never pins
// the editor to content that is no longer on disk.
export function decideReload(editor: string, original: string, disk: string, lastWritten: string): ReloadDecision {
    if (disk == null) {
        return { kind: "missing" };
    }
    if (disk === editor) {
        return { kind: "sync-original", content: disk };
    }
    if (editor === original) {
        return { kind: "replace", content: disk };
    }
    if (disk === original || disk === lastWritten) {
        return { kind: "none" };
    }
    return { kind: "conflict", disk };
}
```

- [ ] **Step 4: Run the vitest**

Run: `npx vitest run frontend/builder/store/decide-reload.test.ts`
Expected: PASS (9 tests).

- [ ] **Step 5: Rework the model**

In `frontend/builder/store/builder-apppanel-model.ts`:

Replace the imports (lines 4-13) with:

```ts
import { globalStore } from "@/app/store/jotaiStore";
import { waveEventSubscribeSingle } from "@/app/store/wps";
import { RpcApi } from "@/app/store/wshclientapi";
import { TabRpcClient } from "@/app/store/wshrpcutil";
import { atoms, getApi, getSettingsKeyAtom, WOS } from "@/store/global";
import { base64ToString, stringToBase64 } from "@/util/util";
import type { WebviewTag } from "electron";
import { atom, type Atom, type PrimitiveAtom } from "jotai";
import type * as MonacoTypes from "monaco-editor";
import { decideReload, type ReloadDecision } from "./decide-reload";
```

Replace the field `debouncedRestart: (() => void) & { cancel: () => void };` (line 42) with:

```ts
    appGoMissingAtom: PrimitiveAtom<boolean> = atom<boolean>(false);
    diskChangedAtom = atom<string>(null) as PrimitiveAtom<string>;
    externalChangeAtom: PrimitiveAtom<boolean> = atom<boolean>(false);
    watchStatusAtom = atom<BuilderWatchStatusData>(null) as PrimitiveAtom<BuilderWatchStatusData>;
    watchStatusUnsubFn: (() => void) | null = null;
    configUnsubFn: (() => void) | null = null;
    lastWrittenContent: string = null;
```

In the constructor, delete the `this.debouncedRestart = debounce(800, () => { this.restartBuilder(); });` statement (lines 46-48) and keep the `saveNeededAtom` assignment.

In `initialize()`:

- In the `builderstatus` handler, after `this.updateSecretsLatch(status);` add:

```ts
                    if (status.status === "building") {
                        globalStore.set(this.externalChangeAtom, false);
                    }
```

- Replace the `rtapp:appgoupdated` subscription (lines 110-116) with:

```ts
        this.appGoUpdateUnsubFn = waveEventSubscribeSingle({
            eventType: "rtapp:appgoupdated",
            scope: appId,
            handler: () => {
                this.handleAppGoUpdated(appId);
            },
        });

        this.watchStatusUnsubFn = waveEventSubscribeSingle({
            eventType: "rtapp:watchstatus",
            scope: WOS.makeORef("builder", builderId),
            handler: (event) => {
                globalStore.set(this.watchStatusAtom, event.data);
            },
        });
        try {
            const watchStatus = await RpcApi.WatchBuilderAppCommand(TabRpcClient, { builderid: builderId });
            globalStore.set(this.watchStatusAtom, watchStatus);
        } catch (err) {
            console.error("Failed to watch the app folder:", err);
            globalStore.set(this.watchStatusAtom, { status: "unavailable", reason: err.message || "unknown error" });
        }

        // The builder window loads the config once at startup (initBuilder in
        // frontend/remoteterm.ts) and, unlike main windows, never runs
        // initGlobalWaveEventSubs; without this the live-rebuild toggle could not change.
        this.configUnsubFn = waveEventSubscribeSingle({
            eventType: "config",
            handler: (event) => {
                globalStore.set(atoms.fullConfigAtom, event.data.fullconfig);
            },
        });
```

In `saveEnvVars`, replace `this.debouncedRestart();` with `this.requestRebuild();`.

Replace the body of `startBuilder()` (keep `restartBuilder()` and its comment unchanged) and add `requestRebuild()` above it:

```ts
    async requestRebuild() {
        const builderId = globalStore.get(atoms.builderId);
        globalStore.set(this.externalChangeAtom, false);
        try {
            await RpcApi.RequestBuilderRebuildCommand(TabRpcClient, { builderid: builderId });
        } catch (err) {
            console.error("Failed to request a rebuild:", err);
            globalStore.set(this.errorAtom, `Failed to rebuild: ${err.message || "Unknown error"}`);
        }
    }

    async startBuilder() {
        return this.requestRebuild();
    }
```

In `loadAppFile`, in the `if (result.notfound)` branch add `globalStore.set(this.appGoMissingAtom, true);`; at the start of the `else` branch add `globalStore.set(this.appGoMissingAtom, false);`; after the `if/else` add `globalStore.set(this.diskChangedAtom, null);`. The existing `await this.startBuilder();` call stays: opening an app still starts it, now through `RequestBuilderRebuildCommand`.

In `saveAppFile`, replace the lines after `const formattedContent = base64ToString(result.data64);` through `this.debouncedRestart();` with:

```ts
            globalStore.set(this.codeContentAtom, formattedContent);
            globalStore.set(this.originalContentAtom, formattedContent);
            globalStore.set(this.appGoMissingAtom, false);
            globalStore.set(this.diskChangedAtom, null);
            globalStore.set(this.errorAtom, "");
            this.lastWrittenContent = formattedContent;
            this.requestRebuild();
```

Add after `saveAppFile`:

```ts
    async handleAppGoUpdated(appId: string) {
        const liveRebuild = globalStore.get(getSettingsKeyAtom("builder:liverebuild")) ?? false;
        if (!liveRebuild) {
            globalStore.set(this.externalChangeAtom, true);
        }
        let disk: string = null;
        try {
            const result = await RpcApi.ReadAppFileCommand(TabRpcClient, { appid: appId, filename: "app.go" });
            disk = result.notfound ? null : base64ToString(result.data64);
        } catch (err) {
            console.error("Failed to read app.go after an outside change:", err);
            return;
        }
        const decision = decideReload(
            globalStore.get(this.codeContentAtom),
            globalStore.get(this.originalContentAtom),
            disk,
            this.lastWrittenContent
        );
        this.applyReloadDecision(decision);
    }

    applyReloadDecision(decision: ReloadDecision) {
        if (decision.kind === "missing") {
            globalStore.set(this.appGoMissingAtom, true);
            return;
        }
        globalStore.set(this.appGoMissingAtom, false);
        if (decision.kind === "none") {
            return;
        }
        this.lastWrittenContent = null;
        if (decision.kind === "sync-original") {
            globalStore.set(this.originalContentAtom, decision.content);
            globalStore.set(this.diskChangedAtom, null);
            return;
        }
        if (decision.kind === "replace") {
            globalStore.set(this.codeContentAtom, decision.content);
            globalStore.set(this.originalContentAtom, decision.content);
            globalStore.set(this.diskChangedAtom, null);
            return;
        }
        globalStore.set(this.diskChangedAtom, decision.disk);
    }

    loadDiskVersion() {
        const disk = globalStore.get(this.diskChangedAtom);
        if (disk == null) {
            return;
        }
        globalStore.set(this.codeContentAtom, disk);
        globalStore.set(this.originalContentAtom, disk);
        globalStore.set(this.diskChangedAtom, null);
    }

    keepMyEdits() {
        const disk = globalStore.get(this.diskChangedAtom);
        if (disk == null) {
            return;
        }
        globalStore.set(this.originalContentAtom, disk);
        globalStore.set(this.diskChangedAtom, null);
    }
```

In `dispose()`, delete `this.debouncedRestart.cancel();` and add:

```ts
        if (this.watchStatusUnsubFn) {
            this.watchStatusUnsubFn();
            this.watchStatusUnsubFn = null;
        }
        if (this.configUnsubFn) {
            this.configUnsubFn();
            this.configUnsubFn = null;
        }
```

`keepMyEdits` sets the original to the disk content so the Save button stays enabled; saving then overwrites the outside change because the user chose to.

- [ ] **Step 6: Add the header strips and the live toggle**

Create `frontend/builder/builder-appheader.tsx`:

```tsx
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { RpcApi } from "@/app/store/wshclientapi";
import { TabRpcClient } from "@/app/store/wshrpcutil";
import { BuilderAppPanelModel } from "@/builder/store/builder-apppanel-model";
import { getSettingsKeyAtom } from "@/store/global";
import { useAtomValue } from "jotai";
import { memo } from "react";

const LiveRebuildToggle = memo(() => {
    const liveRebuild = useAtomValue(getSettingsKeyAtom("builder:liverebuild")) ?? false;
    const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
        RpcApi.SetConfigCommand(TabRpcClient, { "builder:liverebuild": e.target.checked }).catch((err) => {
            console.error("Failed to update builder:liverebuild:", err);
        });
    };
    return (
        <label className="shrink-0 flex items-center gap-2 text-xs text-secondary cursor-pointer select-none">
            <input type="checkbox" className="cursor-pointer" checked={liveRebuild} onChange={handleChange} />
            Rebuild on external changes
        </label>
    );
});

LiveRebuildToggle.displayName = "LiveRebuildToggle";

const ExternalChangeStrip = memo(() => {
    const model = BuilderAppPanelModel.getInstance();
    const externalChange = useAtomValue(model.externalChangeAtom);
    if (!externalChange) {
        return null;
    }
    return (
        <div className="shrink-0 flex items-center gap-3 px-4 py-1.5 bg-warning/10 border-b border-warning/30 text-sm">
            <i className="fa fa-arrows-rotate text-warning" />
            <span className="flex-1">Changed on disk</span>
            <button
                className="px-3 py-0.5 text-sm font-medium bg-accent/80 text-onaccent rounded hover:bg-accent transition-colors cursor-pointer"
                onClick={() => model.requestRebuild()}
            >
                Rebuild
            </button>
        </div>
    );
});

ExternalChangeStrip.displayName = "ExternalChangeStrip";

const WatchStatusStrip = memo(() => {
    const model = BuilderAppPanelModel.getInstance();
    const watchStatus = useAtomValue(model.watchStatusAtom);
    if (watchStatus?.status !== "unavailable") {
        return null;
    }
    const reason = watchStatus.reason ? `: ${watchStatus.reason}` : "";
    return (
        <div className="shrink-0 flex items-center gap-3 px-4 py-1.5 bg-panel border-b border-border text-sm text-secondary">
            <i className="fa fa-eye-slash" />
            <span className="flex-1 min-w-0 truncate" title={watchStatus.reason}>
                Live reload unavailable{reason}. Saves in the Code tab still rebuild.
            </span>
        </div>
    );
});

WatchStatusStrip.displayName = "WatchStatusStrip";

const BuilderAppHeader = memo(() => {
    return (
        <>
            <div className="shrink-0 flex items-center justify-end gap-3 px-3 py-1 border-b border-border">
                <LiveRebuildToggle />
            </div>
            <ExternalChangeStrip />
            <WatchStatusStrip />
        </>
    );
});

BuilderAppHeader.displayName = "BuilderAppHeader";

export { BuilderAppHeader };
```

In `frontend/builder/builder-apppanel.tsx`, add `import { BuilderAppHeader } from "@/builder/builder-appheader";` and render `<BuilderAppHeader />` on the line before `<ErrorStrip />` (line 366).

- [ ] **Step 7: Add the disk-changed bar to the Code tab**

In `frontend/builder/tabs/builder-codetab.tsx`, add above `BuilderCodeTab`:

```tsx
const DiskChangedBar = memo(() => {
    const model = BuilderAppPanelModel.getInstance();
    const diskChanged = useAtomValue(model.diskChangedAtom);
    if (diskChanged == null) {
        return null;
    }
    return (
        <div className="shrink-0 flex items-center gap-3 px-3 py-1.5 bg-warning/10 border-b border-warning/30 text-sm">
            <i className="fa fa-triangle-exclamation text-warning" />
            <span className="flex-1">app.go changed on disk.</span>
            <button
                className="px-2 py-0.5 text-sm bg-accent/80 text-onaccent rounded hover:bg-accent transition-colors cursor-pointer"
                onClick={() => model.loadDiskVersion()}
            >
                Load disk version
            </button>
            <button
                className="px-2 py-0.5 text-sm rounded hover:bg-secondary/10 transition-colors cursor-pointer"
                onClick={() => model.keepMyEdits()}
            >
                Keep my edits
            </button>
        </div>
    );
});

DiskChangedBar.displayName = "DiskChangedBar";
```

and replace the final `return (...)` of `BuilderCodeTab` (lines 79-101) with:

```tsx
    return (
        <div className="w-full h-full flex flex-col" onKeyDown={handleKeyDown}>
            <DiskChangedBar />
            <div className="flex-1 min-h-0 relative">
                <button
                    className={cn(
                        "absolute top-1 right-4 z-50 px-3 py-1 text-sm font-medium rounded transition-colors shadow-lg",
                        saveNeeded
                            ? "bg-accent/80 text-onaccent hover:bg-accent cursor-pointer"
                            : "bg-gray-600 text-gray-400 cursor-default"
                    )}
                    onClick={saveNeeded ? handleSave : undefined}
                >
                    Save
                </button>
                <CodeEditor
                    blockId={builderAppId}
                    text={codeContent}
                    readonly={false}
                    language="go"
                    fileName="app.go"
                    onChange={handleCodeChange}
                    onMount={handleEditorMount}
                />
            </div>
        </div>
    );
```

- [ ] **Step 8: Add the missing-app.go view to Preview**

In `frontend/builder/tabs/builder-previewtab.tsx`, add above `BuilderPreviewTab`:

```tsx
const MissingAppGoView = memo(() => {
    const model = BuilderAppPanelModel.getInstance();
    return (
        <div className="w-full h-full flex items-center justify-center bg-background">
            <div className="flex flex-col items-center gap-6 max-w-[500px] text-center px-8">
                <div className="flex flex-col gap-3">
                    <h2 className="text-2xl font-semibold text-primary">app.go is missing</h2>
                    <p className="text-base text-secondary leading-relaxed">
                        The app folder has no <span className="font-mono">app.go</span>. Create the starter app, or
                        add one from your editor.
                    </p>
                </div>
                <button
                    onClick={() => model.seedStarterApp()}
                    className="px-6 py-2 font-semibold bg-accent/80 text-onaccent rounded hover:bg-accent transition-colors cursor-pointer"
                >
                    Create starter app
                </button>
            </div>
        </div>
    );
});

MissingAppGoView.displayName = "MissingAppGoView";
```

In `BuilderPreviewTab`, add `const appGoMissing = useAtomValue(model.appGoMissingAtom);` with the other hooks, and change the overlay selection so the missing view wins:

```tsx
    let overlay = null;
    if (appGoMissing) {
        overlay = <MissingAppGoView />;
    } else if (!isLoading && !isWebViewActive) {
```

(the body of the existing `if (!isLoading && !isWebViewActive) { ... }` block is unchanged).

- [ ] **Step 9: Type-check and run the builder vitests**

Run: `npx tsc --noEmit`
Expected: exits 0. If `event.data` in the `rtapp:watchstatus` handler is not typed as `BuilderWatchStatusData`, check that Task 10's `npx task generate` updated `frontend/types/remotetermevent.d.ts`; do not cast around it.

Run: `npx vitest run frontend/builder`
Expected: PASS (`decide-reload.test.ts`, `tsunamisdk-packaging.test.ts`).

Run: `grep -n 'debouncedRestart' frontend/builder -r`
Expected: no output.

Check that the builder window now receives config updates; it had no `config` subscription before (`grep -rn 'eventType: "config"' frontend` lists only `frontend/app/store/global.ts`, which only main windows run):

Run: `grep -n 'eventType: "config"' frontend/builder/store/builder-apppanel-model.ts && grep -n 'configUnsubFn()' frontend/builder/store/builder-apppanel-model.ts`
Expected: one match each. A unit test is not practical here: the model imports the live RPC client. The orchestrator's isolated end-to-end run checks the toggle visibly flips after a click.

- [ ] **Step 10: Commit**

```bash
git add frontend/builder/store/decide-reload.ts frontend/builder/store/decide-reload.test.ts frontend/builder/builder-appheader.tsx frontend/builder/store/builder-apppanel-model.ts frontend/builder/builder-apppanel.tsx frontend/builder/tabs/builder-codetab.tsx frontend/builder/tabs/builder-previewtab.tsx
git commit -m "feat(builder): reload outside edits without losing unsaved work

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 12: Remove the placeholder; app folder header; Open terminal and Open folder (D5)

**Files:**
- Modify: `frontend/builder/builder-workspace.tsx:16-20`, `:73-80`, `:95-132`
- Modify: `frontend/builder/builder-appheader.tsx` (path, copy, buttons, notice)
- Modify: `frontend/builder/store/builder-apppanel-model.ts` (`appDirAtom`, `noticeAtom`, `loadAppDir`, `openTerminal`, `openFolder`, `clearNotice`; call `loadAppDir` in `initialize`)
- Create: `emain/emain-builder-select.ts`
- Create: `emain/emain-builder-select.test.ts`
- Modify: `emain/emain-window.ts:1037-1087` (extract `showQuakeWindow`, export `revealQuakeWindow`)
- Modify: `emain/emain-ipc.ts:18-25` (imports), after `close-builder-window` handler (ends line 513)
- Modify: `emain/preload.ts:67`, `frontend/types/custom.d.ts:125`, `frontend/preview/mock/preview-electron-api.ts:54`
- Create: `pkg/buildercontroller/appdir.go`
- Create: `pkg/buildercontroller/appdir_test.go`
- Modify: `pkg/wshrpc/wshrpctypes_builder.go`, `pkg/wshrpc/wshserver/wshserver.go`
- Generated: `frontend/types/gotypes.d.ts`, `frontend/app/store/wshclientapi.ts`, `pkg/wshrpc/wshclient/wshclient.go`

**Interfaces:**
- Consumes: `CheckNoSymlinks` (Task 5); `setupBuilderTest`, `makeTestApp` (Task 4); `BuilderAppHeader`, `LiveRebuildToggle`, `ExternalChangeStrip`, `WatchStatusStrip` (Task 11); `WshServer.CreateBlockCommand` (`wshserver.go:218`); `focusedRemoteTermWindow`, `getAllRemoteTermWindows`, `RemoteTermBrowserWindow` (`emain/emain-window.ts:108,722,153`); `getBuilderWindowByWebContentsId` (`emain/emain-builder.ts:28`).
- Produces:
  - `buildercontroller.ResolveBuilderAppDir(builderId string) (string, error)`: app dir from the builder's rtInfo, D7-checked, must be an existing directory
  - `buildercontroller.MakeBuilderTerminalBlockDef(appDir string) *remotetermobj.BlockDef`: meta `view: term`, `controller: shell`, `connection: local`, `cmd:cwd: <appDir>`
  - RPCs `OpenBuilderTerminalCommand(ctx, CommandOpenBuilderTerminalData{BuilderId, TabId}) error` and `GetBuilderAppDirCommand(ctx, CommandGetBuilderAppDirData{BuilderId}) (string, error)`
  - `pickTerminalWindow<T extends { isDestroyed(): boolean }>(lastFocused: T, all: T[]): T` in `emain/emain-builder-select.ts` (Task 13 adds to this file)
  - Electron API `openBuilderTerminal: () => Promise<string>` (IPC `open-builder-terminal`) and `openBuilderFolder: () => Promise<string>` (IPC `open-builder-folder`); each resolves to `""` on success or a message to show
  - model `appDirAtom`, `noticeAtom`, `openTerminal()`, `openFolder()`, `clearNotice()`

The renderer never passes a path: both IPCs are argument-free, the main process identifies the calling builder window and the backend resolves the folder from rtInfo. Read `.kilocode/skills/electron-api/SKILL.md` and `.kilocode/skills/add-rpc/SKILL.md` first.

- [ ] **Step 1: Write the failing tests**

Create `pkg/buildercontroller/appdir_test.go`:

```go
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package buildercontroller

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/LannCo/remoteterm/pkg/remotetermobj"
	"github.com/LannCo/remoteterm/pkg/rtstore"
)

func setBuilderAppId(t *testing.T, builderId string, appId string) {
	t.Helper()
	oref := remotetermobj.MakeORef(remotetermobj.OType_Builder, builderId)
	rtstore.SetRTInfo(oref, map[string]any{"builder:appid": appId})
	t.Cleanup(func() { rtstore.DeleteRTInfo(oref) })
}

func TestResolveBuilderAppDir(t *testing.T) {
	home, _ := setupBuilderTest(t)
	appDir := makeTestApp(t, home, "demo")
	setBuilderAppId(t, "test-appdir", "draft/demo")
	got, err := ResolveBuilderAppDir("test-appdir")
	if err != nil || got != appDir {
		t.Fatalf("ResolveBuilderAppDir = %q, %v; want %q", got, err, appDir)
	}

	if _, err := ResolveBuilderAppDir("test-appdir-unknown"); err == nil {
		t.Error("expected an error for a builder without rtinfo")
	}

	setBuilderAppId(t, "test-appdir-missing", "draft/nothere")
	if _, err := ResolveBuilderAppDir("test-appdir-missing"); err == nil {
		t.Error("expected an error for a missing app folder")
	}
}

func TestResolveBuilderAppDirRejectsSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink fixture")
	}
	home, _ := setupBuilderTest(t)
	if err := os.MkdirAll(filepath.Join(home, "waveapps", "draft"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(home, "waveapps", "draft", "linked")); err != nil {
		t.Fatal(err)
	}
	setBuilderAppId(t, "test-appdir-link", "draft/linked")
	if _, err := ResolveBuilderAppDir("test-appdir-link"); err == nil {
		t.Fatal("a symlinked app folder was accepted")
	}
}

func TestMakeBuilderTerminalBlockDef(t *testing.T) {
	def := MakeBuilderTerminalBlockDef("/home/u/waveapps/draft/demo")
	want := map[string]string{
		"view":       "term",
		"controller": "shell",
		"connection": "local",
		"cmd:cwd":    "/home/u/waveapps/draft/demo",
	}
	for key, value := range want {
		if def.Meta[key] != value {
			t.Errorf("meta[%q] = %v, want %q", key, def.Meta[key], value)
		}
	}
}
```

Create `emain/emain-builder-select.test.ts`:

```ts
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { describe, expect, it } from "vitest";
import { pickTerminalWindow } from "./emain-builder-select";

function makeWin(id: string, destroyed = false) {
    return { id, isDestroyed: () => destroyed };
}

describe("pickTerminalWindow", () => {
    it("prefers the last focused window", () => {
        const a = makeWin("a");
        const b = makeWin("b");
        expect(pickTerminalWindow(b, [a, b])).toBe(b);
    });

    it("falls back to the first live window when the last focused one is gone", () => {
        const gone = makeWin("gone", true);
        const live = makeWin("live");
        expect(pickTerminalWindow(gone, [gone, live])).toBe(live);
        expect(pickTerminalWindow(null, [live])).toBe(live);
    });

    it("returns null when no window is open", () => {
        expect(pickTerminalWindow(null, [])).toBeNull();
        expect(pickTerminalWindow(null, [makeWin("x", true)])).toBeNull();
    });
});
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./pkg/buildercontroller/... -run 'ResolveBuilderAppDir|MakeBuilderTerminalBlockDef' -v`
Expected: FAIL to compile, `undefined: ResolveBuilderAppDir`.

Run: `npx vitest run emain/emain-builder-select.test.ts`
Expected: FAIL, cannot resolve `./emain-builder-select`.

- [ ] **Step 3: Implement the backend side**

Create `pkg/buildercontroller/appdir.go`:

```go
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package buildercontroller

import (
	"fmt"
	"os"

	"github.com/LannCo/remoteterm/pkg/remotetermappstore"
	"github.com/LannCo/remoteterm/pkg/remotetermobj"
	"github.com/LannCo/remoteterm/pkg/rtstore"
)

// The folder always comes from the builder's rtInfo, never from the renderer, so a
// compromised page cannot point a terminal or a file manager somewhere else.
func ResolveBuilderAppDir(builderId string) (string, error) {
	rtInfo := rtstore.GetRTInfo(remotetermobj.MakeORef(remotetermobj.OType_Builder, builderId))
	if rtInfo == nil {
		return "", fmt.Errorf("builder rtinfo not found for builderid: %s", builderId)
	}
	if rtInfo.BuilderAppId == "" {
		return "", fmt.Errorf("builder appid not set for builderid: %s", builderId)
	}
	appDir, err := remotetermappstore.GetAppDir(rtInfo.BuilderAppId)
	if err != nil {
		return "", err
	}
	if err := remotetermappstore.CheckNoSymlinks(appDir); err != nil {
		return "", err
	}
	info, err := os.Lstat(appDir)
	if err != nil {
		return "", fmt.Errorf("app folder %s is not available: %w", appDir, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("app folder %s is not a directory", appDir)
	}
	return appDir, nil
}

func MakeBuilderTerminalBlockDef(appDir string) *remotetermobj.BlockDef {
	return &remotetermobj.BlockDef{
		Meta: remotetermobj.MetaMapType{
			remotetermobj.MetaKey_View:       "term",
			remotetermobj.MetaKey_Controller: "shell",
			remotetermobj.MetaKey_Connection: "local",
			remotetermobj.MetaKey_CmdCwd:     appDir,
		},
	}
}
```

In `pkg/wshrpc/wshrpctypes_builder.go`, add to the interface (after `WatchBuilderAppCommand`):

```go
	OpenBuilderTerminalCommand(ctx context.Context, data CommandOpenBuilderTerminalData) error
	GetBuilderAppDirCommand(ctx context.Context, data CommandGetBuilderAppDirData) (string, error)
```

and append:

```go
type CommandOpenBuilderTerminalData struct {
	BuilderId string `json:"builderid"`
	TabId     string `json:"tabid"`
}

type CommandGetBuilderAppDirData struct {
	BuilderId string `json:"builderid"`
}
```

In `pkg/wshrpc/wshserver/wshserver.go`, after `WatchBuilderAppCommand`:

```go
func (ws *WshServer) OpenBuilderTerminalCommand(ctx context.Context, data wshrpc.CommandOpenBuilderTerminalData) error {
	if data.BuilderId == "" || data.TabId == "" {
		return fmt.Errorf("must provide a builderId and a tabId to OpenBuilderTerminalCommand")
	}
	appDir, err := buildercontroller.ResolveBuilderAppDir(data.BuilderId)
	if err != nil {
		return err
	}
	_, err = ws.CreateBlockCommand(ctx, wshrpc.CommandCreateBlockData{
		TabId:    data.TabId,
		BlockDef: buildercontroller.MakeBuilderTerminalBlockDef(appDir),
		Focused:  true,
	})
	return err
}

func (ws *WshServer) GetBuilderAppDirCommand(ctx context.Context, data wshrpc.CommandGetBuilderAppDirData) (string, error) {
	if data.BuilderId == "" {
		return "", fmt.Errorf("must provide a builderId to GetBuilderAppDirCommand")
	}
	return buildercontroller.ResolveBuilderAppDir(data.BuilderId)
}
```

Run: `npx task generate`
Expected: exits 0; only the three generated files in the Files list change.

- [ ] **Step 4: Implement the Electron side**

Create `emain/emain-builder-select.ts`:

```ts
// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

type DestroyableWindow = { isDestroyed(): boolean };

// The quake window is the primary main window in this app (emain-window.ts), so it is
// a valid target; the caller reveals a hidden one through the quake show path.
export function pickTerminalWindow<T extends DestroyableWindow>(lastFocused: T, all: T[]): T {
    if (lastFocused != null && !lastFocused.isDestroyed()) {
        return lastFocused;
    }
    return all.find((win) => !win.isDestroyed()) ?? null;
}
```

In `emain/emain-window.ts`, the quake hotkey's show logic is inline in the `else` branch of `quakeToggle` (lines 1069-1087). Move it into a function and export a guarded entry point next to it. Add above `async function quakeToggle()` (line 1037):

```ts
async function showQuakeWindow(window: RemoteTermBrowserWindow) {
    const targetDisplay = getDisplayForQuakeToggle();
    moveWindowToDisplay(window, targetDisplay);
    window.show();
    if (quakeRestoreFullscreenOnShow) {
        const enterPromise = waitForFullscreenEnter(window);
        window.setFullScreen(true);
        try {
            await enterPromise;
        } catch {
            // timeout: proceed anyway
        }
    }
    quakeRestoreFullscreenOnShow = false;
    window.focus();
    if (window.activeTabView?.webContents) {
        window.activeTabView.webContents.focus();
    }
}

// Same path as the quake hotkey, so a hidden quake window comes back on the cursor's
// display and restores fullscreen exactly as the hotkey would.
export async function revealQuakeWindow() {
    if (quakeToggleInProgress) {
        return;
    }
    quakeToggleInProgress = true;
    try {
        const window = quakeWindow;
        if (window == null || window.isDestroyed() || window.isVisible()) {
            return;
        }
        await showQuakeWindow(window);
    } finally {
        quakeToggleInProgress = false;
    }
}
```

and replace that `else` branch (lines 1069-1087) with:

```ts
        } else {
            await showQuakeWindow(window);
        }
```

The moved comment's em-dash becomes a colon (`// timeout: proceed anyway`), per the repo's no-em-dash rule; the comment is otherwise kept. Run `grep -n 'showQuakeWindow\|revealQuakeWindow' emain/emain-window.ts` to confirm one definition each and one call from `quakeToggle`.

In `emain/emain-ipc.ts`, add to the imports: `pickTerminalWindow` from `./emain-builder-select`, and `focusedRemoteTermWindow`, `getAllRemoteTermWindows`, `getQuakeWindow`, `revealQuakeWindow` to the existing `./emain-window` import (line 24). Inside `initIpcHandlers()`, after the `close-builder-window` handler (ends line 513), add:

```ts
    electron.ipcMain.handle("open-builder-terminal", async (event): Promise<string> => {
        const bw = getBuilderWindowByWebContentsId(event.sender.id);
        if (bw == null) {
            return "This action is only available in a builder window.";
        }
        const ww = pickTerminalWindow(focusedRemoteTermWindow, getAllRemoteTermWindows());
        if (ww == null) {
            return "No RemoteTerm window is open. Open one, then try again.";
        }
        const tabId = ww.activeTabView?.remoteTermTabId;
        if (!tabId) {
            return "The RemoteTerm window has no active tab.";
        }
        try {
            await RpcApi.OpenBuilderTerminalCommand(ElectronWshClient, { builderid: bw.builderId, tabid: tabId });
        } catch (e) {
            return `Could not open a terminal: ${e instanceof Error ? e.message : String(e)}`;
        }
        if (ww === getQuakeWindow() && !ww.isVisible()) {
            await revealQuakeWindow();
            return "";
        }
        if (!ww.isVisible()) {
            ww.show();
        }
        ww.focus();
        return "";
    });

    electron.ipcMain.handle("open-builder-folder", async (event): Promise<string> => {
        const bw = getBuilderWindowByWebContentsId(event.sender.id);
        if (bw == null) {
            return "This action is only available in a builder window.";
        }
        let appDir: string;
        try {
            appDir = await RpcApi.GetBuilderAppDirCommand(ElectronWshClient, { builderid: bw.builderId });
        } catch (e) {
            return `Could not find the app folder: ${e instanceof Error ? e.message : String(e)}`;
        }
        try {
            if (!fs.lstatSync(appDir).isDirectory()) {
                return "The app folder is not a directory.";
            }
        } catch {
            return "The app folder does not exist.";
        }
        const err = await electron.shell.openPath(appDir);
        return err ?? "";
    });
```

`emain/preload.ts`, after `setBuilderWindowAppId` (line 67):

```ts
    openBuilderTerminal: () => ipcRenderer.invoke("open-builder-terminal"),
    openBuilderFolder: () => ipcRenderer.invoke("open-builder-folder"),
```

`frontend/types/custom.d.ts`, after `setBuilderWindowAppId` (line 125):

```ts
        openBuilderTerminal: () => Promise<string>; // open-builder-terminal
        openBuilderFolder: () => Promise<string>; // open-builder-folder
```

`frontend/preview/mock/preview-electron-api.ts`, after `setBuilderWindowAppId` (line 54):

```ts
    openBuilderTerminal: () => Promise.resolve(""),
    openBuilderFolder: () => Promise.resolve(""),
```

- [ ] **Step 5: Remove the placeholder panel**

In `frontend/builder/builder-workspace.tsx`:

- `DefaultLayoutPercentages` (lines 16-20) becomes `{ app: 80, build: 20 }` (drop `chat`).
- Delete `handleHorizontalLayout` (lines 73-80).
- Replace the returned JSX (lines 95-132) with:

```tsx
    return (
        <div className="flex-1 overflow-hidden">
            <div
                className={cn(
                    "flex flex-col relative h-full",
                    isAppFocused ? "border-2 border-accent" : "border-2 border-transparent"
                )}
                style={{
                    borderBottomRightRadius: 8,
                }}
            >
                <PanelGroup direction="vertical" onLayout={handleVerticalLayout}>
                    <Panel defaultSize={layout.app} minSize={20}>
                        <BuilderAppPanel />
                    </Panel>
                    <PanelResizeHandle className="h-0.5 bg-transparent hover:bg-gray-500/20 transition-colors" />
                    <Panel defaultSize={layout.build} minSize={20} maxSize={50} style={{ borderBottomRightRadius: 8 }}>
                        <BuilderBuildPanel />
                    </Panel>
                </PanelGroup>
            </div>
        </div>
    );
```

Saved layouts in rtInfo may still carry a `chat` key; it is ignored.

- [ ] **Step 6: Add the path, copy and action buttons to the header**

In `frontend/builder/store/builder-apppanel-model.ts`, add fields:

```ts
    appDirAtom = atom<string>(null) as PrimitiveAtom<string>;
    noticeAtom: PrimitiveAtom<string> = atom<string>("");
```

add methods:

```ts
    async loadAppDir() {
        const builderId = globalStore.get(atoms.builderId);
        try {
            const appDir = await RpcApi.GetBuilderAppDirCommand(TabRpcClient, { builderid: builderId });
            globalStore.set(this.appDirAtom, appDir);
        } catch (err) {
            console.error("Failed to resolve the app folder:", err);
        }
    }

    async openTerminal() {
        const err = await getApi().openBuilderTerminal();
        globalStore.set(this.noticeAtom, err ?? "");
    }

    async openFolder() {
        const err = await getApi().openBuilderFolder();
        globalStore.set(this.noticeAtom, err ?? "");
    }

    clearNotice() {
        globalStore.set(this.noticeAtom, "");
    }
```

and in `initialize()` call `await this.loadAppDir();` directly after `await this.loadEnvVars(builderId);`. Failures go to `noticeAtom`, not `errorAtom`, because the Code tab replaces the editor with `errorAtom`'s text.

In `frontend/builder/builder-appheader.tsx`, change the React import to `import { memo, useState } from "react";`, add these components above `BuilderAppHeader`:

```tsx
const AppFolderPath = memo(() => {
    const model = BuilderAppPanelModel.getInstance();
    const appDir = useAtomValue(model.appDirAtom);
    const [copied, setCopied] = useState(false);
    if (!appDir) {
        return <div className="flex-1 min-w-0" />;
    }
    const handleCopy = () => {
        navigator.clipboard
            .writeText(appDir)
            .then(() => {
                setCopied(true);
                setTimeout(() => setCopied(false), 1500);
            })
            .catch((err) => console.error("Failed to copy the app folder path:", err));
    };
    return (
        <div className="flex-1 min-w-0 flex items-center gap-1">
            <span className="min-w-0 truncate font-mono text-xs text-secondary text-left [direction:rtl]" title={appDir}>
                <bdi>{appDir}</bdi>
            </span>
            <button
                className="shrink-0 px-1 text-secondary hover:text-primary transition-colors cursor-pointer"
                onClick={handleCopy}
                aria-label="Copy app folder path"
                title="Copy path"
            >
                <i className={copied ? "fa fa-check" : "fa fa-copy"} />
            </button>
        </div>
    );
});

AppFolderPath.displayName = "AppFolderPath";

const NoticeStrip = memo(() => {
    const model = BuilderAppPanelModel.getInstance();
    const notice = useAtomValue(model.noticeAtom);
    if (!notice) {
        return null;
    }
    return (
        <div className="shrink-0 flex items-center gap-3 px-4 py-1.5 bg-warning/10 border-b border-warning/30 text-sm">
            <i className="fa fa-circle-info text-warning" />
            <span className="flex-1 min-w-0 truncate" title={notice}>
                {notice}
            </span>
            <button
                className="shrink-0 text-secondary hover:text-primary transition-colors cursor-pointer"
                onClick={() => model.clearNotice()}
                aria-label="Dismiss notice"
            >
                <i className="fa fa-xmark" />
            </button>
        </div>
    );
});

NoticeStrip.displayName = "NoticeStrip";
```

and replace `BuilderAppHeader` with:

```tsx
const BuilderAppHeader = memo(() => {
    const model = BuilderAppPanelModel.getInstance();
    return (
        <>
            <div className="shrink-0 flex items-center gap-3 px-3 py-1 border-b border-border">
                <AppFolderPath />
                <button
                    className="shrink-0 flex items-center gap-1.5 px-2 py-0.5 text-xs rounded hover:bg-secondary/10 transition-colors cursor-pointer"
                    onClick={() => model.openTerminal()}
                >
                    <i className="fa fa-terminal" />
                    Open terminal
                </button>
                <button
                    className="shrink-0 flex items-center gap-1.5 px-2 py-0.5 text-xs rounded hover:bg-secondary/10 transition-colors cursor-pointer"
                    onClick={() => model.openFolder()}
                >
                    <i className="fa fa-folder-open" />
                    Open folder
                </button>
                <LiveRebuildToggle />
            </div>
            <NoticeStrip />
            <ExternalChangeStrip />
            <WatchStatusStrip />
        </>
    );
});
```

- [ ] **Step 7: Run the tests, vet and type-check**

Run: `go test ./pkg/buildercontroller/... -v && go vet ./pkg/buildercontroller/... ./pkg/wshrpc/...`
Expected: PASS; vet silent.

Run: `npx vitest run emain/emain-builder-select.test.ts frontend/builder`
Expected: PASS.

Run: `npx tsc --noEmit`
Expected: exits 0.

Run: `grep -n 'AI features are disabled\|chat:' frontend/builder/builder-workspace.tsx`
Expected: no output.

- [ ] **Step 8: Commit**

```bash
git add pkg/buildercontroller/appdir.go pkg/buildercontroller/appdir_test.go pkg/wshrpc/wshrpctypes_builder.go pkg/wshrpc/wshserver/wshserver.go pkg/wshrpc/wshclient/wshclient.go frontend/types/gotypes.d.ts frontend/app/store/wshclientapi.ts emain/emain-builder-select.ts emain/emain-builder-select.test.ts emain/emain-window.ts emain/emain-ipc.ts emain/preload.ts frontend/types/custom.d.ts frontend/preview/mock/preview-electron-api.ts frontend/builder/builder-workspace.tsx frontend/builder/builder-appheader.tsx frontend/builder/store/builder-apppanel-model.ts
git commit -m "feat(builder): open a terminal or file manager at the app folder

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 13: One builder window per app (D9)

**Files:**
- Modify: `emain/emain-builder-select.ts` (add `findBuilderWindowForApp`)
- Modify: `emain/emain-builder-select.test.ts`
- Modify: `emain/emain-ipc.ts:473-480` (`set-builder-window-appid` becomes `handle`), `:484-513` (extract `destroyBuilderWindow`)
- Modify: `emain/preload.ts:67`, `frontend/types/custom.d.ts:125`, `frontend/preview/mock/preview-electron-api.ts:54`
- Modify: `frontend/builder/app-selection-modal.tsx` (`handleSelectApp`, `handleCreateNew`)

**Interfaces:**
- Consumes: `emain/emain-builder-select.ts` (Task 12); `getAllBuilderWindows`, `getBuilderWindowByWebContentsId`, `BuilderWindowType` (`emain/emain-builder.ts:15-34`).
- Produces:
  - `findBuilderWindowForApp<T extends { builderId: string; builderAppId?: string }>(windows: T[], appId: string, excludeBuilderId: string): T` (null for an empty `appId`)
  - Electron API `setBuilderWindowAppId: (appId: string) => Promise<boolean>`: `true` when this window keeps the app; `false` when another builder window already has it, in which case that window is focused and this one is destroyed

`openBuilderWindow` (`emain/emain-ipc.ts:48-56`) already focuses an existing builder window when opening from a main window; this task covers selecting an app inside a builder window.

- [ ] **Step 1: Write the failing test**

Append to `emain/emain-builder-select.test.ts` (and add `findBuilderWindowForApp` to its import):

```ts
describe("findBuilderWindowForApp", () => {
    const windows = [
        { builderId: "b1", builderAppId: "draft/one" },
        { builderId: "b2", builderAppId: "draft/two" },
        { builderId: "b3", builderAppId: "" },
    ];

    it("finds another window that already has the app", () => {
        expect(findBuilderWindowForApp(windows, "draft/two", "b1")).toBe(windows[1]);
    });

    it("ignores the asking window itself", () => {
        expect(findBuilderWindowForApp(windows, "draft/one", "b1")).toBeNull();
    });

    it("never matches an empty app id", () => {
        expect(findBuilderWindowForApp(windows, "", "b1")).toBeNull();
        expect(findBuilderWindowForApp(windows, null, "b1")).toBeNull();
    });
});
```

- [ ] **Step 2: Run it to verify it fails**

Run: `npx vitest run emain/emain-builder-select.test.ts`
Expected: FAIL, `findBuilderWindowForApp` is not exported.

- [ ] **Step 3: Implement the lookup**

Append to `emain/emain-builder-select.ts`:

```ts
type BuilderWindowLike = { builderId: string; builderAppId?: string };

// One window per app keeps one controller, one watcher and one build per app folder.
export function findBuilderWindowForApp<T extends BuilderWindowLike>(
    windows: T[],
    appId: string,
    excludeBuilderId: string
): T {
    if (!appId) {
        return null;
    }
    return windows.find((win) => win.builderId !== excludeBuilderId && win.builderAppId === appId) ?? null;
}
```

- [ ] **Step 4: Switch the IPC to request/response and redirect duplicates**

Task 12 inserted handlers and imports into `emain/emain-ipc.ts`, so the line numbers below are from the original file and will be off. Locate each edit by its content (the handler's channel name, the import's module path) rather than by line number.

In `emain/emain-ipc.ts`:

- Add `findBuilderWindowForApp` to the `./emain-builder-select` import and `type BuilderWindowType` to the `./emain-builder` import (line 18).
- Add this module-level function above `initIpcHandlers` (it is the body of today's `close-builder-window` handler, lines 489-512, moved unchanged):

```ts
async function destroyBuilderWindow(bw: BuilderWindowType) {
    const builderId = bw.builderId;
    if (builderId) {
        try {
            await RpcApi.SetRTInfoCommand(ElectronWshClient, {
                oref: `builder:${builderId}`,
                data: {} as ObjRTInfo,
                delete: true,
            });
        } catch (e) {
            console.error("Error deleting builder rtinfo:", e);
        }
    }
    const wc = bw.webContents;
    if (wc.isDevToolsOpened()) {
        wc.closeDevTools();
    }
    for (const guest of electron.webContents.getAllWebContents()) {
        if (guest.getType() === "webview" && guest.hostWebContents?.id === wc.id) {
            if (guest.isDevToolsOpened()) {
                guest.closeDevTools();
            }
        }
    }
    bw.destroy();
}
```

- Replace the `close-builder-window` handler (lines 484-513) with:

```ts
    electron.ipcMain.on("close-builder-window", async (event) => {
        const bw = getBuilderWindowByWebContentsId(event.sender.id);
        if (bw == null) {
            return;
        }
        await destroyBuilderWindow(bw);
    });
```

- Replace the `set-builder-window-appid` handler (lines 473-480) with:

```ts
    electron.ipcMain.handle("set-builder-window-appid", async (event, appId: string): Promise<boolean> => {
        const bw = getBuilderWindowByWebContentsId(event.sender.id);
        if (bw == null) {
            return false;
        }
        const other = findBuilderWindowForApp(getAllBuilderWindows(), appId, bw.builderId);
        if (other != null) {
            if (other.isMinimized()) {
                other.restore();
            }
            other.focus();
            await destroyBuilderWindow(bw);
            return false;
        }
        bw.builderAppId = appId;
        console.log("set-builder-window-appid", bw.builderId, appId);
        return true;
    });
```

`emain/preload.ts` line 67 becomes:

```ts
    setBuilderWindowAppId: (appId: string) => ipcRenderer.invoke("set-builder-window-appid", appId),
```

`frontend/types/custom.d.ts` line 125 becomes:

```ts
        setBuilderWindowAppId: (appId: string) => Promise<boolean>; // set-builder-window-appid
```

`frontend/preview/mock/preview-electron-api.ts` line 54 becomes:

```ts
    setBuilderWindowAppId: (_appId: string) => Promise.resolve(true),
```

- [ ] **Step 5: Claim the app before using it in the selection modal**

In `frontend/builder/app-selection-modal.tsx`, in `handleSelectApp`, insert after the `local/` conversion block and before `const builderId = ...`:

```tsx
        const kept = await getApi().setBuilderWindowAppId(appIdToUse);
        if (!kept) {
            return;
        }
```

and delete the trailing `getApi().setBuilderWindowAppId(appIdToUse);` line.

Replace `handleCreateNew` (the Task 8 version) with:

```tsx
    const handleCreateNew = async (appName: string) => {
        const draftAppId = `draft/${appName}`;
        const kept = await getApi().setBuilderWindowAppId(draftAppId);
        if (!kept) {
            return;
        }
        try {
            await RpcApi.SeedBuilderAppCommand(TabRpcClient, { appid: draftAppId });
        } catch (err) {
            console.error("Failed to create starter files:", err);
            setError(`Failed to create ${appName}: ${err.message || String(err)}`);
            await getApi().setBuilderWindowAppId(null);
            return;
        }
        const builderId = globalStore.get(atoms.builderId);
        const oref = WOS.makeORef("builder", builderId);
        await RpcApi.SetRTInfoCommand(TabRpcClient, {
            oref,
            data: { "builder:appid": draftAppId },
        });
        globalStore.set(atoms.builderAppId, draftAppId);
        document.title = `RTApp Builder (${draftAppId})`;
    };
```

The `getApi().setBuilderWindowAppId(null)` call in `switchBuilderApp` (`builder-apppanel-model.ts`) needs no change: a null app never matches another window.

- [ ] **Step 6: Run the tests and type-check**

Run: `npx vitest run emain/emain-builder-select.test.ts`
Expected: PASS (6 tests).

Run: `npx tsc --noEmit`
Expected: exits 0.

- [ ] **Step 7: Commit**

```bash
git add emain/emain-builder-select.ts emain/emain-builder-select.test.ts emain/emain-ipc.ts emain/preload.ts frontend/types/custom.d.ts frontend/preview/mock/preview-electron-api.ts frontend/builder/app-selection-modal.tsx
git commit -m "feat(builder): keep one builder window per app

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 14: Final verification

**Files:** none changed unless a check fails.

**Interfaces:** Consumes everything above. Produces a pass/fail report; it does not run the app. The end-to-end GUI run (spec "End-to-end, isolated") is done separately by the orchestrator under nested Xvfb with scratch HOME and XDG dirs.

- [ ] **Step 1: Go suites**

Run: `go test ./pkg/... 2>&1 | tail -60`
Expected: every package `ok` or `[no test files]`. If a package this branch did not touch fails, report it with its output as possibly pre-existing; do not change it.

Run: `cd tsunami && go test ./... 2>&1 | tail -30`
Expected: every package `ok` or `[no test files]`.

Run: `go vet ./pkg/buildercontroller/... ./pkg/remotetermappstore/... ./pkg/remotetermapputil/... ./pkg/blockcontroller/... ./pkg/wshrpc/... ./pkg/wps/... ./pkg/tsgen/... ./pkg/rtconfig/... && (cd tsunami && go vet ./build/... ./cmd/...)`
Expected: no output.

Run: `go test -race ./pkg/buildercontroller/... ./pkg/remotetermappstore/... && (cd tsunami && go test -race ./build/...)`
Expected: `ok` for each, no `DATA RACE` report.

Run: `go test ./pkg/remotetermappstore/starter/... -run StarterAppCompiles -v 2>&1 | tail -5`
Expected: `PASS`. A `SKIP` whose reason starts with `go not found` counts as a verification failure (the test's own toolchain should always be reachable); any other skip (Go older than the SDK, cold module cache) is reported with its reason.

- [ ] **Step 2: Frontend suites**

Run: `npx vitest run 2>&1 | tail -30`
Expected: all test files pass.

Run: `npx tsc --noEmit`
Expected: exits 0.

- [ ] **Step 3: Generated files are in sync**

Run: `npx task generate && git status --short`
Expected: no modified tracked files. Any diff means a generated file was hand-edited or not regenerated; report it.

- [ ] **Step 4: Packaging check**

Run: `npx task build:tsunamisdk && test -f dist/tsunamisdk/go.mod && test -f dist/tsunamisdk/app/defaultclient.go && test ! -e dist/tsunamisdk/build && echo BUNDLE-OK`
Expected: `BUNDLE-OK`.

- [ ] **Step 5: House rules on the branch diff**

Run: `git diff 3e126798..HEAD -- . ':!.planning' | grep '^+' | grep -nP '\x{2014}'`
Expected: no output (no em-dash in any added line; some touched files, such as `wshserver.go`, already contain em-dashes in lines this branch does not change).

Run: `git status --short | grep -E 'golang-|zig-'`
Expected: the two untracked directories are listed as `??` and nothing from them is staged or committed (`git log --stat 3e126798..HEAD | grep -E 'golang-|zig-'` prints nothing).

Run: `git diff 3e126798..HEAD -- '*.go' | grep -nE '^\+.*\bfunc New[A-Z]'`
Expected: no output (constructors use `Make`).

- [ ] **Step 6: Report**

Report each step's result, every skip with its reason, and any failure with its output. Do not commit unless a step required a fix; if it did, commit that fix on its own with a message naming the check.

---

## Spec deviations

Where the code disagrees with the spec, the plan follows the code. Each item names what differs and why.

1. **Quake window is the primary main window (D5).** The spec excludes the quake window when picking where "Open terminal" goes. In the code the quake window is simply the first main window (`emain/emain-window.ts:880-882`, `:896-898`, `:940-943`), so excluding it would make Open terminal fail in the common single-window case. The plan uses `focusedRemoteTermWindow` (last focused, survives blur, `emain-window.ts:106-108`), falls back to the first live window, and, when that window is the hidden quake window, reveals it through the quake hotkey's own show path (`revealQuakeWindow`, extracted from `quakeToggle`) so it returns on the cursor's display with fullscreen restored (Task 12).
2. **New RPC `GetBuilderAppDirCommand` (D5).** "Open folder" must resolve the folder server-side, but the Electron main process cannot call Go functions directly. The plan adds `GetBuilderAppDirCommand{builderid}`, used by the `open-builder-folder` IPC and by the header to display the path. The path still never travels from renderer to main.
3. **Input hash also recorded at request time (D4).** The spec records `lastBuildInputHash` at the start of each build. A save made while a build is running is only built after that build ends, so the watcher (300 ms) would see an unknown hash and report our own save as an outside change. `RequestRebuild` records the hash when the request arrives as well as when the build starts (Task 9).
4. **`decideReload` gains a `none` case (D4).** The spec's signature includes `lastWritten` but no rule uses it. The plan adds: a dirty editor with `disk == original` (another file changed) or `disk == lastWritten` (our save, response not yet applied) changes nothing. A clean editor is checked first, so a stale `lastWritten` cannot pin old content (Task 11).
5. **Secret bindings follow the app on draft, revert, rename and delete (D8).** The spec only names publish. Before this change the file travelled inside the app folder through `copyDir`, `os.Rename` and `os.RemoveAll`; the plan copies, moves and deletes it alongside to keep that behaviour (Task 6).
6. **D7 also guards `DeleteAppFile` and `RenameAppFile`.** The spec lists reads and writes only. A delete or rename through a symlinked parent reaches outside the app folder the same way, so the parent-directory check is applied there too (Task 5).
7. **Pre-build checks also apply to Tsunami app blocks.** `pkg/blockcontroller/tsunamicontroller.go:118-185` builds published apps with the same missing-module problem. The plan routes it through `PrepareTsunamiBuild` (Task 4, Step 9). Drop that step if published-app blocks are out of scope; the builder does not depend on it.
8. **`GOTOOLCHAIN=local` lives in the shared `tsunami/build` package.** It therefore also applies to the standalone `tsunami` CLI, not only builder builds.
9. **go.mod `go` line is raised to the SDK floor.** The spec is silent. An app's written-back `go.mod` can carry an older `go` line (today's builds write the local minor, e.g. `go 1.22`); the plan raises it to the SDK floor and never lowers it (Task 4).
10. **Starter `app.go` is embedded as `files/app.go.tmpl`.** A file named `app.go` in the starter package directory would be compiled into that package.
11. **D6 messages reach the Build panel as an output line.** The Build panel shows output lines, not the status `errormsg`, so `handleBuildError` appends `[error] <message>` to the output (Task 4).
12. **`setBuilderWindowAppId` becomes request/response (D9).** It was fire-and-forget; the renderer must learn that its window is being closed before it writes rtInfo for the app. D9 itself is feasible: `openBuilderWindow` (`emain-ipc.ts:48-56`) already focuses an existing builder window. On Linux, window-manager focus-stealing prevention may only flash the taskbar entry; that is an existing limitation.
13. **A set but missing `tsunami:gopath` reports "not found".** `CheckGoVersion` used to report a run error for it; the plan maps it to the D6 "Go toolchain not found" message, which names the setting.
14. **`WatchBuilderAppCommand` returns the initial status.** The spec defines the event only; returning the status gives the frontend its starting state and exposes the type to TypeScript generation.
15. **Reading the GOROOT from a shim is feasible.** mise and asdf shims and `/snap/bin/go` all run the real toolchain, so `<found> env GOROOT` works; if it fails, the plan falls back to the found path and a `gofmt` beside it (Task 3).
16. **Pre-existing data race fixed.** `GetStatus` read `bc.appId` without the lock that writers hold. Task 9's tests expose it under `-race`, so Task 9 snapshots the id through `bc.lock` first.
17. **Decision: symlink checks run from `~/waveapps/<ns>` down.** D7 says every component from `~/waveapps` down; that would lock out every app for a user whose `~/waveapps` is a symlink to another disk. `~/waveapps` itself is the user's choice and out of reach of anything confined to an app folder, so `CheckNoSymlinks` starts at the namespace directory (Task 5; tests `TestSymlinkedWaveappsRootAllowed`, `TestSymlinkedNamespaceRejected`, and `TestSeedAppThroughSymlinkedRootButNotNamespace` in Task 8).
18. **Probe uses `Setsid`, not `Setpgid` (D2).** A new session detaches the probe from any controlling tty, so an interactive login shell cannot stop on SIGTTIN; the group id still equals the shell's pid, so the timeout's group kill is unchanged (Task 3).
19. **Discovery checks each candidate against the floor (D2).** The spec takes the first Go found. A too-old Go early in PATH would then hide a newer one elsewhere, so every candidate is version-checked and skipped if older; the "too old" message appears only when none qualifies and names the newest one found (Task 3).
20. **Static files are hashed by metadata (D4).** The spec hashes path and content of every relevant file. Root `*.go` files keep content hashing; files under `static/` (possibly large media) contribute path, size and mtime only (Task 9).
21. **Save echo recorded on the server.** Extending item 3: `WriteAppGoFileCommand` records the input hash itself after writing, so recognising our own save no longer depends on which RPC arrives first (Task 9).
22. **`RestartBuilderAndWaitCommand` and `RestartAndWaitForBuild` are deleted.** Nothing outside generated code calls them, and they bypassed the coalescing and leaked the previous process (Task 9).
23. **Teardown is final.** `DeleteController` and `Shutdown` mark the controller closed, so neither a queued rebuild nor a late watcher callback can start an app after teardown (Tasks 9 and 10).
24. **`SeedApp` fills an empty `app.go` and clears stale bindings for a new app (D3, D8).** An empty regular `app.go` counts as missing; when the app folder did not exist before seeding, secret bindings left under that app id by a deleted app are removed (Task 8).

## Audit disposition

| # | Finding | Disposition | Where |
|---|---|---|---|
| 1 | Builder window never receives `config` events, so the live-rebuild toggle cannot show "on" | Accepted: model subscribes to `config` and sets `atoms.fullConfigAtom`; unsubscribed in `dispose`; explicit grep check (a unit test would need the live RPC client) | Task 11 Steps 5, 9 |
| 2 | Orphan process after teardown (queued build after `Stop`; late `fireChange`) | Accepted: controller `closed` flag set by `DeleteController`/`Shutdown`; `queueBuild`, `beginBuild`, `endBuild`, `handleAppFilesChanged` honour it; `Stop` waits on `!isBuilding()`; `TestDeleteControllerDuringBuildLeavesNoProcess` (verified to fail with the checks removed) | Tasks 9, 10 |
| 3 | Atomic-rename test did not prove the temp file is ignored | Accepted: temp write, wait 3x debounce, assert 0; rename, assert exactly 1 | Task 10 |
| 4 | Re-created app inherits stale secret bindings | Accepted: `SeedApp` deletes bindings when the folder did not exist; two tests | Task 8 |
| 5 | Interactive probe shell can SIGTTIN on an inherited tty | Accepted: `Setsid` replaces `Setpgid`; group kill unchanged | Task 3; Spec deviation 18 |
| 6 | Save echo depends on RPC arrival order | Accepted: `RecordAppInputHash` called from `WriteAppGoFileCommand`; still recorded in `RequestRebuild`; test | Task 9 |
| 7 | Hashing reads large static files | Accepted: `static/` hashed by path, size, mtime_ns; tests for unreadable media and same-size rewrite | Task 9 |
| 8 | Starter compile test skips when `go` is not on PATH | Accepted: falls back to `runtime.GOROOT()/bin/go`; a "go not found" skip fails Task 14 | Tasks 7, 14 |
| 9 | Hidden quake window shown with plain `show()` | Accepted: `showQuakeWindow` extracted from `quakeToggle`, `revealQuakeWindow` exported and used | Task 12 |
| 10 | `RestartAndWaitForBuild` bypasses coalescing | Accepted: deleted with its RPC and types (no non-generated callers) | Task 9 |
| 11 | Too-old Go first in PATH hides a newer one | Accepted: per-candidate floor check, `GoTooOldError` names the newest; two tests | Task 3 |
| 12 | Save button colour; stale line numbers in Task 13 | Accepted: `text-onaccent`; Task 13 matches `emain-ipc.ts` edits by content | Tasks 11, 13 |
| 13 | Symlinked `~/waveapps` locks users out | Accepted: checks start at `<ns>`; root-allowed and namespace-rejected tests | Tasks 5, 8; Spec deviation 17 |
| 14 | "Create starter app" useless for an empty `app.go` | Accepted: empty regular `app.go` filled via `O_WRONLY\|O_TRUNC` after `Lstat`; symlinked empty file left alone; tests | Task 8 |
