// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { beforeEach, describe, expect, it, vi } from "vitest";
import { classifyWindowOpen, computePopupBounds, parseWindowFeatures } from "./emain-popup";

vi.mock("electron", () => ({
    session: { defaultSession: { id: "default" } },
}));

beforeEach(() => {
    vi.spyOn(console, "log").mockImplementation(() => {});
});

const Popup = { url: "https://accounts.example.test/auth", disposition: "new-window" as const };

describe("parseWindowFeatures", () => {
    it("parses key=value pairs and bare tokens", () => {
        expect(parseWindowFeatures("width=480, height=600,noopener,popup=yes,menubar=0")).toEqual({
            width: "480",
            height: "600",
            noopener: true,
            popup: "yes",
            menubar: false,
        });
    });

    it("handles empty and undefined", () => {
        expect(parseWindowFeatures("")).toEqual({});
        expect(parseWindowFeatures(undefined)).toEqual({});
    });
});

describe("classifyWindowOpen", () => {
    it("routes a new-window disposition with an opener to a popup", () => {
        expect(classifyWindowOpen({ ...Popup, features: "width=480,height=600" })).toBe("popup");
        expect(classifyWindowOpen({ url: "about:blank", disposition: "new-window", features: "width=1" })).toBe("popup");
    });

    it("routes tab dispositions to the existing pane path", () => {
        for (const disposition of ["foreground-tab", "background-tab", "default", "other"] as const) {
            expect(classifyWindowOpen({ url: Popup.url, disposition, features: "" })).toBe("tab");
        }
    });

    it("routes noopener/noreferrer popups to the pane path", () => {
        expect(classifyWindowOpen({ ...Popup, features: "noopener,width=480" })).toBe("tab");
        expect(classifyWindowOpen({ ...Popup, features: "noreferrer" })).toBe("tab");
        expect(classifyWindowOpen({ ...Popup, features: "noopener=0,width=480" })).toBe("popup");
    });

    it("denies popups to non-web schemes", () => {
        for (const url of ["file:///tmp/x.html", "javascript:alert(1)", "chrome://gpu", "data:text/html,x"]) {
            expect(classifyWindowOpen({ url, disposition: "new-window", features: "width=1" })).toBe("deny");
        }
    });
});

describe("computePopupBounds", () => {
    const parent = { x: 100, y: 100, width: 1200, height: 800 };
    const work = { x: 0, y: 0, width: 1920, height: 1080 };

    it("uses defaults centred over the parent when nothing is requested", () => {
        expect(computePopupBounds({}, parent, work)).toEqual({ x: 440, y: 180, width: 520, height: 640 });
    });

    it("honours requested size and position", () => {
        expect(computePopupBounds({ width: 400, height: 300, x: 50, y: 60 }, parent, work)).toEqual({
            x: 50,
            y: 60,
            width: 400,
            height: 300,
        });
    });

    it("clamps size and position into the work area", () => {
        const r = computePopupBounds({ width: 5000, height: 5000, x: 5000, y: -50 }, parent, work);
        expect(r.width).toBe(work.width);
        expect(r.height).toBe(work.height);
        expect(r.x + r.width).toBeLessThanOrEqual(work.x + work.width);
        expect(r.y).toBeGreaterThanOrEqual(work.y);
    });

    it("enforces a minimum size", () => {
        const r = computePopupBounds({ width: 1, height: 1 }, parent, work);
        expect(r.width).toBe(200);
        expect(r.height).toBe(150);
    });
});
