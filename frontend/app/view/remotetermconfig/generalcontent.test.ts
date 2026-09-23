// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { assert, test } from "vitest";
import {
    bumpNumberInput,
    FieldSchemas,
    filterFieldSchemas,
    matchesFieldSearch,
    parseNumberInput,
} from "./generalcontent";

test("every FieldSchema has a non-empty description", () => {
    for (const schema of FieldSchemas) {
        assert(
            typeof schema.description === "string" && schema.description.trim().length > 0,
            `${String(schema.key)} is missing a description`
        );
    }
});

test("matchesFieldSearch is empty-query-passthrough", () => {
    const schema = FieldSchemas[0];
    assert(matchesFieldSearch(schema, ""));
    assert(matchesFieldSearch(schema, "   "));
});

test("matchesFieldSearch matches label case-insensitively", () => {
    const schema = FieldSchemas.find((f) => f.key === "term:fontsize");
    assert(matchesFieldSearch(schema, "TERMINAL FONT"));
});

test("matchesFieldSearch matches raw settings key case-insensitively", () => {
    const schema = FieldSchemas.find((f) => f.key === "term:fontsize");
    assert(matchesFieldSearch(schema, "term:fontsize"));
    assert(matchesFieldSearch(schema, "TERM:FONTSIZE"));
});

test("matchesFieldSearch matches description case-insensitively", () => {
    const schema = FieldSchemas.find((f) => f.key === "conn:askbeforewshinstall");
    assert(matchesFieldSearch(schema, "wsh helper"));
});

test("matchesFieldSearch rejects a query with no match", () => {
    const schema = FieldSchemas.find((f) => f.key === "term:fontsize");
    assert(!matchesFieldSearch(schema, "zzznomatch"));
});

test("filterFieldSchemas returns all schemas for an empty query", () => {
    assert.equal(filterFieldSchemas(FieldSchemas, "").length, FieldSchemas.length);
    assert.equal(filterFieldSchemas(FieldSchemas, "  ").length, FieldSchemas.length);
});

test("filterFieldSchemas narrows across categories by key substring", () => {
    const results = filterFieldSchemas(FieldSchemas, "fontsize");
    assert(results.length > 1);
    assert(results.every((schema) => String(schema.key).toLowerCase().includes("fontsize")));
});

test("parseNumberInput rejects empty and non-numeric input instead of reading it as 0", () => {
    assert.isNull(parseNumberInput("", 1, 50));
    assert.isNull(parseNumberInput("   ", 1, 50));
    assert.isNull(parseNumberInput("abc", 1, 50));
    assert.isNull(parseNumberInput("Infinity", 1, 50));
});

test("parseNumberInput clamps to the field's min and max", () => {
    assert.equal(parseNumberInput("0", 1, 50), 1);
    assert.equal(parseNumberInput("-5", 1, 50), 1);
    assert.equal(parseNumberInput("500", 1, 50), 50);
    assert.equal(parseNumberInput("12", 1, 50), 12);
    assert.equal(parseNumberInput("1.5", 0.25, 3), 1.5);
});

test("parseNumberInput leaves an unbounded side alone", () => {
    assert.equal(parseNumberInput("-5", undefined, undefined), -5);
    assert.equal(parseNumberInput("99999", 0, undefined), 99999);
});

test("no FieldSchema label or description uses the upstream Wave product name", () => {
    for (const schema of FieldSchemas) {
        const text = `${schema.label} ${schema.description}`;
        assert(!/\bWave\b/.test(text), `${String(schema.key)} still says "Wave": ${text}`);
    }
});

test("bumpNumberInput steps from the typed draft, not the stale committed value", () => {
    assert.strictEqual(bumpNumberInput("50", 10, 1), 51);
    assert.strictEqual(bumpNumberInput("50", 10, -1), 49);
});

test("bumpNumberInput falls back to the committed value for an empty or invalid draft", () => {
    assert.strictEqual(bumpNumberInput("", 10, 1), 11);
    assert.strictEqual(bumpNumberInput("abc", 10, -1), 9);
});

test("bumpNumberInput does not accumulate float error from a fractional step", () => {
    let v = 1;
    for (const expected of [1.05, 1.1, 1.15, 1.2]) {
        v = bumpNumberInput("", v, 0.05, 0.25, 3);
        assert.strictEqual(v, expected);
    }
    assert.strictEqual(bumpNumberInput("0.3", 1, -0.1), 0.2);
});

test("bumpNumberInput keeps a typed draft's own precision", () => {
    assert.strictEqual(bumpNumberInput("1.123", 1, 0.05), 1.173);
    assert.strictEqual(bumpNumberInput("12.5", 12, 1), 13.5);
    assert.strictEqual(bumpNumberInput("1e-7", 0, 1), 1.0000001);
});

test("bumpNumberInput clamps the stepped value", () => {
    assert.strictEqual(bumpNumberInput("20", 10, 1, 0, 20), 20);
    assert.strictEqual(bumpNumberInput("0", 10, -1, 0, 20), 0);
});
