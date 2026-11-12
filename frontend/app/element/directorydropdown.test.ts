// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { expect, test } from "vitest";
import { moveDropdownIndex } from "./directorydropdown";

test("moveDropdownIndex: ArrowUp from no selection lands on the last entry", () => {
    expect(moveDropdownIndex(-1, 3, "ArrowUp")).toBe(2);
});

test("moveDropdownIndex: ArrowDown from no selection lands on the first entry", () => {
    expect(moveDropdownIndex(-1, 3, "ArrowDown")).toBe(0);
});

test("moveDropdownIndex: ArrowUp does not go below the first entry", () => {
    expect(moveDropdownIndex(0, 3, "ArrowUp")).toBe(0);
});

test("moveDropdownIndex: ArrowDown does not go past the last entry", () => {
    expect(moveDropdownIndex(2, 3, "ArrowDown")).toBe(2);
});

test("moveDropdownIndex: Home and End clamp to the valid range", () => {
    expect(moveDropdownIndex(1, 3, "Home")).toBe(0);
    expect(moveDropdownIndex(1, 3, "End")).toBe(2);
});

test("moveDropdownIndex: clamps stale out-of-range indices into the valid range first", () => {
    // list shrank while a highlight pointed past its end
    expect(moveDropdownIndex(5, 3, "ArrowUp")).toBe(1);
    expect(moveDropdownIndex(5, 3, "ArrowDown")).toBe(2);
    expect(moveDropdownIndex(7, 3, "Home")).toBe(0);
    expect(moveDropdownIndex(-9, 3, "ArrowDown")).toBe(0);
});

test("moveDropdownIndex: empty list always returns -1", () => {
    expect(moveDropdownIndex(0, 0, "ArrowDown")).toBe(-1);
    expect(moveDropdownIndex(0, 0, "End")).toBe(-1);
    expect(moveDropdownIndex(-1, 0, "ArrowUp")).toBe(-1);
});
