// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { assert, test } from "vitest";
import { FieldSchemas, filterFieldSchemas, matchesFieldSearch } from "./generalcontent";

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
