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
