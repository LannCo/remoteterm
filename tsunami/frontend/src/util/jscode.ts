// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

// The Go SDK documents jscode as a JS function expression, e.g. (e) => { ... }; the handler arguments are
// passed to the function it evaluates to. Direct eval would also expose this module's locals to that code,
// so the string is compiled by the Function constructor, which only sees the global scope.
function compileJsCode(jscode: string): unknown {
    // eval accepted a trailing semicolon after the expression; a "return (...)" wrapper does not. The newline
    // before the closing parenthesis keeps a trailing line comment in jscode from swallowing it.
    const expr = jscode.trim().replace(/;+$/, "");
    return new Function('"use strict"; return (' + expr + "\n);")();
}

export function runVDomJsCode(jscode: string, args: any[]): any {
    try {
        const fn = compileJsCode(jscode);
        if (typeof fn === "function") {
            return fn(...args);
        }
    } catch (err) {
        console.error("vdom jscode error:", err);
    }
    return undefined;
}
