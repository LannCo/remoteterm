import { beforeEach, describe, expect, it, vi } from "vitest";

const capturedCsiHandlers: Record<string, (...args: any[]) => any> = {};
const capturedOscHandlers: Record<number, (data: string) => any> = {};
let capturedTerminalOptions: any;

vi.mock("@xterm/xterm", () => ({
    Terminal: class MockTerminal {
        rows = 24;
        cols = 80;
        parser = {
            registerCsiHandler: vi.fn((id: { prefix?: string; final: string }, cb: (...args: any[]) => any) => {
                capturedCsiHandlers[(id.prefix ?? "") + id.final] = cb;
                return { dispose: vi.fn() };
            }),
            registerOscHandler: vi.fn((ident: number, cb: (data: string) => any) => {
                capturedOscHandlers[ident] = cb;
                return { dispose: vi.fn() };
            }),
        };
        constructor(options: any) {
            capturedTerminalOptions = options;
        }
        open = vi.fn();
        loadAddon = vi.fn();
        attachCustomKeyEventHandler = vi.fn();
        onBell = vi.fn(() => ({ dispose: vi.fn() }));
        onData = vi.fn(() => ({ dispose: vi.fn() }));
        onBinary = vi.fn(() => ({ dispose: vi.fn() }));
        onTitleChange = vi.fn(() => ({ dispose: vi.fn() }));
        onRender = vi.fn(() => ({ dispose: vi.fn() }));
        onResize = vi.fn(() => ({ dispose: vi.fn() }));
        onWriteParsed = vi.fn(() => ({ dispose: vi.fn() }));
        onSelectionChange = vi.fn(() => ({ dispose: vi.fn() }));
    },
}));

vi.mock("@xterm/addon-fit", () => ({
    FitAddon: class MockFitAddon {
        fit = vi.fn();
    },
}));
vi.mock("@xterm/addon-image", () => ({ ImageAddon: class MockImageAddon {} }));
vi.mock("@xterm/addon-search", () => ({ SearchAddon: class MockSearchAddon {} }));
vi.mock("@xterm/addon-serialize", () => ({ SerializeAddon: class MockSerializeAddon {} }));
vi.mock("@xterm/addon-web-links", () => ({ WebLinksAddon: class MockWebLinksAddon {} }));
vi.mock("@xterm/addon-webgl", () => ({ WebglAddon: class MockWebglAddon {} }));

vi.mock("@/store/global", () => ({
    globalStore: { get: vi.fn(() => undefined), set: vi.fn(), sub: vi.fn(() => () => {}) },
    getApi: vi.fn(() => ({})),
    getOverrideConfigAtom: vi.fn(() => vi.fn()),
    getSettingsKeyAtom: vi.fn(() => vi.fn()),
    isDev: false,
    openLink: vi.fn(),
    WOS: { makeORef: vi.fn(), getWaveObjectAtom: vi.fn() },
    fetchWaveFile: vi.fn(),
}));
vi.mock("@/store/services", () => ({ BlockService: { SaveTerminalState: vi.fn() } }));
vi.mock("@/app/store/badge", () => ({ setBadge: vi.fn() }));
vi.mock("@/app/store/wps", () => ({ getFileSubject: vi.fn(() => null) }));
vi.mock("@/app/store/wshclientapi", () => ({ RpcApi: {} }));
vi.mock("@/app/store/wshrpcutil", () => ({ TabRpcClient: {} }));
vi.mock("@/util/platformutil", () => ({ PLATFORM: "darwin", PlatformMacOS: true }));
vi.mock("@/util/util", () => ({
    base64ToArray: vi.fn(),
    fireAndForget: vi.fn((f) => f()),
    makeConnRoute: vi.fn(),
}));
vi.mock("debug", () => ({ default: () => vi.fn() }));
vi.mock("jotai", () => ({
    atom: vi.fn((init) => ({ init })),
    PrimitiveAtom: class MockPrimitiveAtom {},
}));
vi.mock("throttle-debounce", () => ({ debounce: vi.fn((_, fn) => fn) }));
vi.mock("./osc-handlers", () => ({
    handleOsc16162Command: vi.fn(),
    handleOsc52Command: vi.fn(),
    handleOsc7Command: vi.fn(),
    isClaudeCodeCommand: vi.fn(),
}));

import { TermWrap } from "./termwrap";

const LightColours = { foreground: "#1a1a1a", background: "#ffffff" };
const DarkColours = { foreground: "#c1c1c1", background: "#000000" };

describe("TermWrap colour reporting", () => {
    let term: TermWrap;
    let sent: string[];

    beforeEach(() => {
        Object.keys(capturedCsiHandlers).forEach((k) => delete capturedCsiHandlers[k]);
        Object.keys(capturedOscHandlers).forEach((k) => delete capturedOscHandlers[k as any]);
        sent = [];
        const mockElem = {
            addEventListener: vi.fn(),
            removeEventListener: vi.fn(),
            style: {},
        } as unknown as HTMLDivElement;
        term = new TermWrap("tab-1", "block-1", mockElem, {}, { sendDataHandler: (d) => sent.push(d) });
        term.loaded = true;
    });

    it("hands colour scheme queries to RemoteTerm instead of xterm.js's own theme-derived answer", () => {
        expect(capturedTerminalOptions.vtExtensions?.colorSchemeQuery).toBe(false);
    });

    describe("OSC 11 (background)", () => {
        it("answers a query with the opaque light background", () => {
            term.setReportedColours(LightColours);
            expect(capturedOscHandlers[11]("?")).toBe(true);
            expect(sent).toEqual(["\x1b]11;rgb:ffff/ffff/ffff\x1b\\"]);
        });

        it("answers with the new colour after the theme changes", () => {
            term.setReportedColours(LightColours);
            term.setReportedColours(DarkColours);
            capturedOscHandlers[11]("?");
            expect(sent).toEqual(["\x1b]11;rgb:0000/0000/0000\x1b\\"]);
        });

        it("falls through to xterm.js for a set request", () => {
            term.setReportedColours(LightColours);
            expect(capturedOscHandlers[11]("rgb:1111/2222/3333")).toBe(false);
            expect(sent).toEqual([]);
        });

        it("falls through to xterm.js when no colours are known", () => {
            expect(capturedOscHandlers[11]("?")).toBe(false);
            expect(sent).toEqual([]);
        });
    });

    describe("OSC 10 (foreground)", () => {
        it("answers a query with the theme foreground", () => {
            term.setReportedColours(LightColours);
            expect(capturedOscHandlers[10]("?")).toBe(true);
            expect(sent).toEqual(["\x1b]10;rgb:1a1a/1a1a/1a1a\x1b\\"]);
        });
    });

    describe("CSI ? 996 n (colour scheme query)", () => {
        it("reports light (2) in light mode", () => {
            term.setReportedColours(LightColours);
            expect(capturedCsiHandlers["?n"]([996])).toBe(true);
            expect(sent).toEqual(["\x1b[?997;2n"]);
        });

        it("reports dark (1) in dark mode", () => {
            term.setReportedColours(DarkColours);
            expect(capturedCsiHandlers["?n"]([996])).toBe(true);
            expect(sent).toEqual(["\x1b[?997;1n"]);
        });

        it("leaves other DSR queries to xterm.js", () => {
            term.setReportedColours(LightColours);
            expect(capturedCsiHandlers["?n"]([6])).toBe(false);
            expect(sent).toEqual([]);
        });
    });

    describe("DEC mode 2031 (colour scheme updates)", () => {
        it("sends an unsolicited report when the scheme changes and the program opted in", () => {
            term.setReportedColours(DarkColours);
            term.activeDecModes.add(2031);
            term.setReportedColours(LightColours);
            expect(sent).toEqual(["\x1b[?997;2n"]);
        });

        it("sends nothing when the program has not opted in", () => {
            term.setReportedColours(DarkColours);
            term.setReportedColours(LightColours);
            expect(sent).toEqual([]);
        });

        it("sends nothing when the scheme is unchanged", () => {
            term.setReportedColours(LightColours);
            term.activeDecModes.add(2031);
            term.setReportedColours({ foreground: "#222222", background: "#fafafa" });
            expect(sent).toEqual([]);
        });
    });
});
