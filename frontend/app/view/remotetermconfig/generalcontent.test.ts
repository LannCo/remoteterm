// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { assert, test } from "vitest";
import { FieldSchemas, filterFieldSchemas, matchesFieldSearch, parseNumberInput } from "./generalcontent";

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
