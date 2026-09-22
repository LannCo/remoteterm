// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

// @vitest-environment happy-dom

import { act, cleanup, render, screen } from "@testing-library/react";
import fs from "fs";
import path from "path";
import { afterEach, expect, it, vi } from "vitest";
import { ViewLogoCommand } from "./onboarding-command";

vi.mock("./onboarding-layout-term", () => ({ FakeTermBlock: () => null }));
vi.mock("@/app/element/streamdown", () => ({ WaveStreamdown: () => null }));
vi.mock("@/app/view/codeeditor/codeeditor", () => ({ CodeEditor: () => null }));

const PublicDir = path.resolve(import.meta.dirname, "../../../public");

function pngSize(file: string): [number, number] {
    const buf = fs.readFileSync(file);
    return [buf.readUInt32BE(16), buf.readUInt32BE(20)];
}

afterEach(() => {
    cleanup();
    vi.useRealTimers();
});

it("first-run logo demo types and shows the RemoteTerm logo, not Wave art", () => {
    vi.useFakeTimers();
    render(<ViewLogoCommand />);
    act(() => {
        vi.advanceTimersByTime(100 * 60);
    });

    expect(screen.getByText("wsh view public/remoteterm-logo.png")).toBeTruthy();
    const img = screen.getByRole("img", { name: "remoteterm-logo.png" }) as HTMLImageElement;
    const src = img.getAttribute("src");
    expect(src).not.toMatch(/wave/i);

    const file = path.join(PublicDir, src.replace(/^\//, ""));
    expect(fs.existsSync(file), file).toBe(true);
    expect(pngSize(file)).toEqual(pngSize(path.join(PublicDir, "logos/wave-logo.png")));
});
