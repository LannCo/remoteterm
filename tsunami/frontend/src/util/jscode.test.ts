// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { afterEach, describe, expect, it, vi } from "vitest";
import { runVDomJsCode } from "./jscode";

describe("runVDomJsCode", () => {
    afterEach(() => {
        vi.restoreAllMocks();
    });

    it("calls the function expression with the handler args and returns its value", () => {
        const event = { target: { value: "hi" } };
        const extra = { id: 7 };
        expect(runVDomJsCode("(e, elem) => e.target.value + ':' + elem.id", [event, extra])).toBe("hi:7");
    });

    it("runs block bodies for their side effects", () => {
        const event = { clicked: false };
        runVDomJsCode("(e) => { e.clicked = true; }", [event]);
        expect(event.clicked).toBe(true);
    });

    it("accepts a trailing semicolon and a trailing line comment", () => {
        expect(runVDomJsCode("() => 1;", [])).toBe(1);
        expect(runVDomJsCode("() => 2 // note", [])).toBe(2);
    });

    it("catches a thrown error, logs it and returns undefined", () => {
        const errSpy = vi.spyOn(console, "error").mockImplementation(() => {});
        const boom = new Error("boom");
        expect(runVDomJsCode("(err) => { throw err; }", [boom])).toBeUndefined();
        expect(errSpy).toHaveBeenCalledWith("vdom jscode error:", boom);
    });

    it("logs a syntax error instead of throwing", () => {
        const errSpy = vi.spyOn(console, "error").mockImplementation(() => {});
        expect(runVDomJsCode("(e) => {", [])).toBeUndefined();
        expect(errSpy).toHaveBeenCalledWith("vdom jscode error:", expect.any(SyntaxError));
    });

    it("ignores code that does not evaluate to a function", () => {
        const errSpy = vi.spyOn(console, "error").mockImplementation(() => {});
        expect(runVDomJsCode("42", [])).toBeUndefined();
        expect(errSpy).not.toHaveBeenCalled();
    });

    it("does not expose module-scope identifiers of the runner", () => {
        expect(runVDomJsCode("() => typeof compileJsCode", [])).toBe("undefined");
        expect(runVDomJsCode("() => typeof runVDomJsCode", [])).toBe("undefined");
        expect(runVDomJsCode("() => typeof jscode", [])).toBe("undefined");
        expect(runVDomJsCode("() => typeof expr", [])).toBe("undefined");
    });

    it("still reaches globals and runs in strict mode", () => {
        expect(runVDomJsCode("() => typeof globalThis.Math", [])).toBe("object");
        const errSpy = vi.spyOn(console, "error").mockImplementation(() => {});
        runVDomJsCode("() => { undeclaredAssignment = 1; }", []);
        expect(errSpy).toHaveBeenCalledWith("vdom jscode error:", expect.any(ReferenceError));
    });
});
