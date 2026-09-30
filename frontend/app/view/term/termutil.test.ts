import { afterAll, beforeEach, describe, expect, it, vi } from "vitest";

// ---------------------------------------------------------------------------
// Mocks for heavy dependencies
// ---------------------------------------------------------------------------

vi.mock("@xterm/xterm", () => ({ Terminal: class {} }));

vi.mock("@/app/store/wshclientapi", () => ({
    RpcApi: {
        WriteTempFileCommand: vi.fn(),
        RemoteWriteTempFileCommand: vi.fn(),
    },
}));

vi.mock("@/app/store/wshrpcutil", () => ({
    TabRpcClient: {},
}));

import { RpcApi } from "@/app/store/wshclientapi";
import { colord, extend } from "colord";
import a11yPlugin from "colord/plugins/a11y";
import termthemes from "../../../../pkg/rtconfig/defaultconfig/termthemes.json";
import {
    colourSchemeReport,
    computeMinimumContrastRatio,
    computeReportedColours,
    computeTheme,
    createRemoteTempFileFromBlob,
    DefaultTermTheme,
    DefaultTermThemeLight,
    formatOscColourReport,
    getDefaultTermThemeName,
    resolveTermThemeName,
} from "./termutil";

extend([a11yPlugin]);

const mockRemoteWrite = RpcApi.RemoteWriteTempFileCommand as unknown as ReturnType<typeof vi.fn>;

// Node has no FileReader; provide a minimal stub backed by Blob.arrayBuffer().
class MockFileReader {
    result: ArrayBuffer | null = null;
    onload: (() => void) | null = null;
    onerror: ((err: unknown) => void) | null = null;

    readAsArrayBuffer(blob: Blob): void {
        blob.arrayBuffer()
            .then((buf) => {
                this.result = buf;
                if (this.onload) this.onload();
            })
            .catch((err) => {
                if (this.onerror) this.onerror(err);
            });
    }
}

vi.stubGlobal("FileReader", MockFileReader);

describe("createRemoteTempFileFromBlob", () => {
    beforeEach(() => {
        mockRemoteWrite.mockReset();
        mockRemoteWrite.mockResolvedValue("/tmp/remoteterm-abc123/file");
    });

    afterAll(() => {
        vi.unstubAllGlobals();
    });

    it("passes the provided filename through to the RPC unchanged", async () => {
        const blob = new Blob(["hello"], { type: "application/pdf" });
        const path = await createRemoteTempFileFromBlob(blob, "report.pdf", "ssh:myhost");

        expect(mockRemoteWrite).toHaveBeenCalledTimes(1);
        const [client, data, opts] = mockRemoteWrite.mock.calls[0];
        expect(data.filename).toBe("report.pdf");
        expect(data.data64).toBeTruthy();
        expect(opts).toEqual({ route: "conn:ssh:myhost" });
        expect(path).toBe("/tmp/remoteterm-abc123/file");
    });

    it("generates a waveterm_paste name when no filename is given (clipboard)", async () => {
        const blob = new Blob(["img"], { type: "image/png" });
        await createRemoteTempFileFromBlob(blob, undefined, "ssh:myhost");

        const data = mockRemoteWrite.mock.calls[0][1];
        expect(data.filename).toMatch(/^waveterm_paste_\d+_[a-z0-9]{6}\.png$/);
    });

    it("falls back to a generated name for an empty filename", async () => {
        const blob = new Blob(["img"], { type: "image/png" });
        await createRemoteTempFileFromBlob(blob, "", "ssh:myhost");

        const data = mockRemoteWrite.mock.calls[0][1];
        expect(data.filename).toMatch(/^waveterm_paste_\d+_[a-z0-9]{6}\.png$/);
    });

    it("preserves filenames with spaces, quotes, and unicode", async () => {
        const blob = new Blob(["data"], { type: "text/plain" });
        const tricky = "my file's (final) 副本.txt";
        await createRemoteTempFileFromBlob(blob, tricky, "ssh:myhost");

        const data = mockRemoteWrite.mock.calls[0][1];
        expect(data.filename).toBe(tricky);
    });

    it("uses the .bin extension for unknown mime types when generating a name", async () => {
        const blob = new Blob(["data"], { type: "application/octet-stream" });
        await createRemoteTempFileFromBlob(blob, undefined, "ssh:myhost");

        const data = mockRemoteWrite.mock.calls[0][1];
        expect(data.filename).toMatch(/\.bin$/);
    });

    it("omits route opts when no connName is given", async () => {
        const blob = new Blob(["img"], { type: "image/png" });
        await createRemoteTempFileFromBlob(blob, "x.png");

        const [, , opts] = mockRemoteWrite.mock.calls[0];
        expect(opts).toBeUndefined();
    });

    it("encodes the blob content as base64 in data64", async () => {
        const blob = new Blob(["hello"], { type: "text/plain" });
        await createRemoteTempFileFromBlob(blob, "hello.txt", "ssh:myhost");

        const data = mockRemoteWrite.mock.calls[0][1];
        expect(Buffer.from(data.data64, "base64").toString("utf8")).toBe("hello");
    });

    it("rejects blobs larger than 50MB without calling the RPC", async () => {
        const big = new Blob([new Uint8Array(50 * 1024 * 1024 + 1)], { type: "application/octet-stream" });

        await expect(createRemoteTempFileFromBlob(big, "big.bin", "ssh:myhost")).rejects.toThrow(
            "File too large (>50MB)"
        );
        expect(mockRemoteWrite).not.toHaveBeenCalled();
    });
});

describe("getDefaultTermThemeName", () => {
    it("maps dark to default-dark and light to default-light", () => {
        expect(getDefaultTermThemeName("dark")).toBe(DefaultTermTheme);
        expect(getDefaultTermThemeName("light")).toBe(DefaultTermThemeLight);
    });
});

describe("resolveTermThemeName", () => {
    it("honours an explicit override in both modes", () => {
        expect(resolveTermThemeName("dracula", "light")).toBe("dracula");
        expect(resolveTermThemeName("default-dark", "light")).toBe("default-dark");
        expect(resolveTermThemeName("default-light", "dark")).toBe("default-light");
    });

    it("follows the appearance mode when no override is set", () => {
        expect(resolveTermThemeName(null, "light")).toBe("default-light");
        expect(resolveTermThemeName(undefined, "dark")).toBe("default-dark");
    });
});

describe("computeTheme fallback", () => {
    const fullConfig = {
        termthemes: {
            "default-dark": { background: "#000000", foreground: "#c1c1c1" },
            "default-light": { background: "#ffffff", foreground: "#1a1a1a" },
        },
    } as unknown as FullConfigType;

    it("uses the named theme when present", () => {
        const [theme, bg] = computeTheme(fullConfig, "default-dark", 0, "default-light");
        expect(bg).toBe("#000000");
        expect(theme.foreground).toBe("#c1c1c1");
        expect(theme.background).toBe("#00000000");
    });

    it("falls back to the supplied fallback when the named theme is missing", () => {
        const [theme, bg] = computeTheme(fullConfig, "not-a-theme", 0, "default-light");
        expect(bg).toBe("#ffffff");
        expect(theme.foreground).toBe("#1a1a1a");
    });

    it("returns an empty theme with transparent background when both are missing", () => {
        const [theme, bg] = computeTheme({ termthemes: {} } as unknown as FullConfigType, "x", 0, "y");
        expect(bg).toBeUndefined();
        expect(theme.background).toBe("#00000000");
    });
});

describe("computeTheme reverse video and block cursor text", () => {
    const fullConfig = { termthemes } as unknown as FullConfigType;

    // xterm.js paints reversed default-colour text with opaque(theme.background) on theme.foreground,
    // and the character under a block cursor with theme.cursorAccent on theme.cursor.
    it.each(["default-dark", "default-light"])("%s reversed text reaches 4.5:1 on the reversed cell", (name) => {
        const [theme] = computeTheme(fullConfig, name, 0, name);
        const reversedText = colord(theme.background).alpha(1);
        const ratio = reversedText.contrast(theme.foreground);
        expect(
            ratio,
            `${name}: ${reversedText.toHex()} on ${theme.foreground} is ${ratio.toFixed(2)}:1`
        ).toBeGreaterThanOrEqual(4.5);
    });

    it.each(["default-dark", "default-light"])("%s text under the block cursor reaches 4.5:1", (name) => {
        const [theme] = computeTheme(fullConfig, name, 0, name);
        const cursorAccent = (theme as { cursorAccent?: string }).cursorAccent;
        const ratio = colord(cursorAccent).contrast(theme.cursor);
        expect(ratio, `${name}: ${cursorAccent} on ${theme.cursor} is ${ratio.toFixed(2)}:1`).toBeGreaterThanOrEqual(
            4.5
        );
    });

    it("keeps the canvas fully transparent in every theme", () => {
        for (const name of ["default-dark", "default-light"]) {
            const [theme] = computeTheme(fullConfig, name, 0.3, name);
            expect(colord(theme.background).alpha()).toBe(0);
        }
    });

    it("leaves the dark theme's colours exactly as before", () => {
        const [theme] = computeTheme(fullConfig, "default-dark", 0, "default-dark");
        expect(theme.background).toBe("#00000000");
        expect((theme as { cursorAccent?: string }).cursorAccent).toBe("#000000");
    });
});

describe("computeMinimumContrastRatio", () => {
    const fullConfig = { termthemes } as unknown as FullConfigType;

    it("asks xterm.js for 4.5:1 on the light theme, whatever the transparency", () => {
        for (const transparency of [0, 0.4]) {
            const [theme, bg] = computeTheme(fullConfig, "default-light", transparency, "default-light");
            expect(computeMinimumContrastRatio(theme, bg)).toBe(4.5);
        }
    });

    it("leaves every dark built-in theme at xterm.js's default of 1", () => {
        const dark = Object.keys(termthemes).filter(
            (name) => colord(termthemes[name].background).luminance() < colord(termthemes[name].foreground).luminance()
        );
        expect(dark).toContain("default-dark");
        for (const name of dark) {
            const [theme, bg] = computeTheme(fullConfig, name, 0, name);
            expect(computeMinimumContrastRatio(theme, bg), name).toBe(1);
        }
    });

    it("stays at 1 when the theme has no colours", () => {
        const [theme, bg] = computeTheme({ termthemes: {} } as unknown as FullConfigType, "x", 0, "y");
        expect(computeMinimumContrastRatio(theme, bg)).toBe(1);
    });
});

describe("reported terminal colours", () => {
    const fullConfig = {
        termthemes: {
            "default-dark": { background: "#000000", foreground: "#c1c1c1" },
            "default-light": { background: "#ffffff", foreground: "#1a1a1a" },
        },
    } as unknown as FullConfigType;

    it("reports the opaque theme colours in light mode, not the transparent canvas colour", () => {
        const [theme, bg] = computeTheme(fullConfig, "default-light", 0, "default-light");
        expect(computeReportedColours(theme, bg)).toEqual({ foreground: "#1a1a1a", background: "#ffffff" });
    });

    it("reports the opaque theme colours in dark mode", () => {
        const [theme, bg] = computeTheme(fullConfig, "default-dark", 0, "default-dark");
        expect(computeReportedColours(theme, bg)).toEqual({ foreground: "#c1c1c1", background: "#000000" });
    });

    it("ignores the transparency alpha applied to the background", () => {
        const [theme, bg] = computeTheme(fullConfig, "default-light", 0.4, "default-light");
        expect(bg).not.toBe("#ffffff");
        expect(computeReportedColours(theme, bg).background).toBe("#ffffff");
    });

    it("leaves a colour undefined when the theme does not define it", () => {
        const [theme, bg] = computeTheme({ termthemes: {} } as unknown as FullConfigType, "x", 0, "y");
        expect(computeReportedColours(theme, bg)).toEqual({ foreground: undefined, background: undefined });
    });

    it("formats OSC 10 and OSC 11 reports as 16-bit rgb, matching xterm.js", () => {
        expect(formatOscColourReport(10, "#1a1a1a")).toBe("\x1b]10;rgb:1a1a/1a1a/1a1a\x1b\\");
        expect(formatOscColourReport(11, "#ffffff")).toBe("\x1b]11;rgb:ffff/ffff/ffff\x1b\\");
    });

    it("reports the colour scheme as light (2) when the background is lighter than the foreground", () => {
        expect(colourSchemeReport({ foreground: "#1a1a1a", background: "#ffffff" })).toBe("\x1b[?997;2n");
    });

    it("reports the colour scheme as dark (1) when the background is darker than the foreground", () => {
        expect(colourSchemeReport({ foreground: "#c1c1c1", background: "#000000" })).toBe("\x1b[?997;1n");
    });

    it("returns null for the colour scheme when either colour is missing", () => {
        expect(colourSchemeReport({ foreground: "#1a1a1a", background: undefined })).toBeNull();
    });
});
