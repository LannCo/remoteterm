import { beforeEach, describe, expect, it, vi } from "vitest";

const capturedCsiHandlers: Record<string, (...args: any[]) => any> = {};
const capturedOscHandlers: Record<number, (data: string) => any> = {};
let capturedRawSgrHandler: ((params: any) => boolean) | undefined;
let rawCsiFinals: string[] = [];
let lastTerminal: any;

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
        element: any = undefined;
        _core = {
            registerCsiHandler: vi.fn((id: { prefix?: string; final: string }, cb: (params: any) => boolean) => {
                rawCsiFinals.push((id.prefix ?? "") + id.final);
                if (id.final === "m") {
                    capturedRawSgrHandler = cb;
                }
                return { dispose: vi.fn() };
            }),
        };
        constructor(options: any) {
            lastTerminal = this;
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

function sgr(...values: number[]) {
    const params = Int32Array.from(values);
    return { length: values.length, params, hasSubParams: () => false };
}

function screenWith(gl: any) {
    return { querySelectorAll: vi.fn(() => [{ getContext: vi.fn(() => gl) }]) };
}

function makeGl() {
    return { SRC_ALPHA: 0x302, ONE_MINUS_SRC_ALPHA: 0x303, ONE: 1, blendFunc: vi.fn(), blendFuncSeparate: vi.fn() };
}

function makeTerm(options: any = {}) {
    const mockElem = {
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
        style: {},
    } as unknown as HTMLDivElement;
    return new TermWrap("tab-1", "block-1", mockElem, {}, options);
}

describe("TermWrap light theme text fixes", () => {
    beforeEach(() => {
        capturedRawSgrHandler = undefined;
        rawCsiFinals = [];
    });

    it("watches SGR on the raw parameter list, which the public parser API only hands over as a copy", () => {
        makeTerm();
        expect(rawCsiFinals).toEqual(["m"]);
    });

    it("turns faint into a no-op while the light fixes are on, and never consumes the sequence", () => {
        const term = makeTerm({ lightTextFixes: true });
        const p = sgr(2, 31);
        expect(capturedRawSgrHandler!(p)).toBe(false);
        expect(Array.from(p.params)).toEqual([26, 31]);
        term.setLightTextFixes(false);
        const q = sgr(2, 31);
        expect(capturedRawSgrHandler!(q)).toBe(false);
        expect(Array.from(q.params)).toEqual([2, 31]);
    });

    it("leaves faint alone when no light fixes were asked for", () => {
        makeTerm();
        const p = sgr(2);
        capturedRawSgrHandler!(p);
        expect(Array.from(p.params)).toEqual([2]);
    });

    it("corrects the glyph blend of an open terminal, and restores it for a dark theme", () => {
        const term = makeTerm();
        const gl = makeGl();
        lastTerminal.element = screenWith(gl);
        term.setLightTextFixes(true);
        expect(gl.blendFuncSeparate).toHaveBeenCalledWith(0x302, 0x303, 1, 0x303);
        term.setLightTextFixes(false);
        expect(gl.blendFunc).toHaveBeenCalledWith(0x302, 0x303);
    });

    it("re-applies the glyph blend when the renderer is rebuilt", () => {
        const term = makeTerm({ lightTextFixes: true });
        const gl = makeGl();
        lastTerminal.element = screenWith(gl);
        term.applyGlyphBlend();
        expect(gl.blendFuncSeparate).toHaveBeenCalledTimes(1);
    });
});
