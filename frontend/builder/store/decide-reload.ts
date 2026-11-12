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
