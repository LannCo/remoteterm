import { beforeEach, describe, expect, it } from "vitest";
import { Terminal } from "@xterm/xterm";
// Import the ESM build directly. electron-vite's renderer build resolves
// @xterm/addon-image's "module" field (this same .mjs) when bundling the
// real app; vitest's default resolution instead picks "main" (lib/addon-
// image.js), which - independent of this fix - fails to parse at all (a
// pre-existing defect from an earlier IIP/TIFF patch, unrelated to Kitty).
// Importing the exact file the shipped app uses keeps this test honest.
import { ImageAddon } from "@xterm/addon-image/lib/addon-image.mjs";

// These tests exercise the real (unmocked) @xterm/addon-image Kitty graphics
// handler directly against a real xterm.js Terminal core (no DOM attach, so
// no term.open() - the escape-sequence parser and APC handler dispatch work
// independently of rendering). This is deliberate: the bug lives in the
// vendored/patched addon package's protocol state machine, not in
// termwrap.ts's wiring, so a TermWrap-level test with the addon mocked out
// (see image-addon.test.ts) would never exercise it.

function toBase64(bytes: Uint8Array): string {
    let s = "";
    for (let i = 0; i < bytes.length; i++) s += String.fromCharCode(bytes[i]);
    return btoa(s);
}

function writeAsync(term: Terminal, data: string | Uint8Array): Promise<void> {
    return new Promise((resolve) => term.write(data, resolve));
}

function solidColorRgba(width: number, height: number, r: number, g: number, b: number): Uint8Array {
    const bytes = new Uint8Array(width * height * 4);
    for (let p = 0; p < width * height; p++) {
        bytes[p * 4] = r;
        bytes[p * 4 + 1] = g;
        bytes[p * 4 + 2] = b;
        bytes[p * 4 + 3] = 255;
    }
    return bytes;
}

// Splits raw bytes into fixed-size sub-blocks and base64-encodes each block
// independently, matching real-world clients such as `chafa --format=kitty`
// that stream pixel data without buffering the whole image first. When a
// block's length isn't a multiple of 3, this produces a mid-transmission
// base64 '=' padding character - the exact shape that broke the handler.
function independentlyPaddedChunks(bytes: Uint8Array, blockSize: number): string[] {
    const chunks: string[] = [];
    for (let offset = 0; offset < bytes.length; offset += blockSize) {
        chunks.push(toBase64(bytes.subarray(offset, offset + blockSize)));
    }
    return chunks;
}

describe("Kitty graphics protocol chunked transmission (real addon-image handler)", () => {
    let term: Terminal;
    let imageAddon: ImageAddon;
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    let kittyHandler: any;
    let responses: string[];

    beforeEach(() => {
        term = new Terminal({ cols: 80, rows: 24, allowProposedApi: true });
        imageAddon = new ImageAddon({
            sixelSupport: true,
            kittySupport: true,
            iipSupport: true,
            enableSizeReports: true,
        });
        term.loadAddon(imageAddon);
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        kittyHandler = (imageAddon as any)._handlers.get("kitty");
        responses = [];
        term.onData((d) => responses.push(d));
    });

    it("responds OK to a capability query (a=q)", async () => {
        await writeAsync(term, "\x1b_Gi=1,a=q\x1b\\");
        expect(responses).toContain("\x1b_Gi=1;OK\x1b\\");
    });

    it("stores and clears pending state for a single-shot direct transmission", async () => {
        // Uses a=t (transmit only, no display) rather than a=T: display
        // requires createImageBitmap/ImageData, which don't exist in this
        // DOM-free Node test environment. a=t exercises the exact same
        // _processChunk/_streamPayload storage path this fix touches and
        // responds as soon as storage succeeds, without needing to render.
        const rgba = solidColorRgba(1, 1, 255, 0, 0);
        await writeAsync(term, `\x1b_Ga=t,f=32,s=1,v=1,i=10;${toBase64(rgba)}\x1b\\`);

        expect(kittyHandler.images.size).toBe(1);
        expect(kittyHandler.pendingTransmissions.size).toBe(0);
        expect(responses).toContain("\x1b_Gi=10;OK\x1b\\");
    });

    it("finalizes a transmission split across a metadata-only opener and a payload-less closer", async () => {
        // Real clients (chafa, and reportedly kitten icat) send:
        //   1. a=T,f=...,m=1              <- control data only, no payload
        //   2. m=1;<payload>              <- one or more payload chunks
        //   3. m=0                        <- control data only, no payload
        // Before the fix, steps 1 and 3 fell into the standalone-command
        // switch (which only understands delete/query/placement), silently
        // dropping the whole transfer and leaking a pending-transmission
        // entry forever.
        const width = 4;
        const height = 4;
        const rgba = solidColorRgba(width, height, 0, 255, 0);
        const b64 = toBase64(rgba);
        const mid = Math.floor(b64.length / 2);

        await writeAsync(term, `\x1b_Ga=T,f=32,s=${width},v=${height},m=1\x1b\\`);
        await writeAsync(term, `\x1b_Gm=1;${b64.slice(0, mid)}\x1b\\`);
        await writeAsync(term, `\x1b_Gm=1;${b64.slice(mid)}\x1b\\`);
        await writeAsync(term, "\x1b_Gm=0\x1b\\");

        expect(kittyHandler.pendingTransmissions.size).toBe(0);
        expect(kittyHandler.images.size).toBe(1);
        // No i= was sent, so the terminal auto-assigns the image id - fetch
        // the one stored image rather than assuming a specific id.
        const stored = [...kittyHandler.images.values()][0];
        expect(stored).toBeDefined();
        expect(stored.width).toBe(width);
        expect(stored.height).toBe(height);
        const storedBytes = new Uint8Array(await stored.data.arrayBuffer());
        expect(storedBytes).toEqual(rgba);
    });

    it("reassembles a transmission whose chunks are independently base64-padded (chafa's actual wire format)", async () => {
        // Verified against real `chafa --format=kitty` output: it base64-
        // encodes each 512-byte block of pixel data on its own, so most
        // chunks carry their own '=' padding mid-transmission. A streaming
        // base64 decoder fed straight through treats '=' as end-of-input and
        // rejects everything that follows - this is the second, independent
        // bug the fix addresses (KittyGraphicsHandler._streamPayload).
        const width = 8;
        const height = 8;
        const rgba = solidColorRgba(width, height, 10, 20, 30);
        const blockSize = 21; // deliberately not a multiple of 3 -> forces '=' padding on every block

        // a=t (transmit only) for the same DOM-free reason as above.
        await writeAsync(term, `\x1b_Ga=t,f=32,s=${width},v=${height},i=42,m=1\x1b\\`);
        for (const chunkB64 of independentlyPaddedChunks(rgba, blockSize)) {
            await writeAsync(term, `\x1b_Gm=1;${chunkB64}\x1b\\`);
        }
        await writeAsync(term, "\x1b_Gm=0\x1b\\");

        expect(kittyHandler.pendingTransmissions.size).toBe(0);
        expect(kittyHandler.images.size).toBe(1);
        const stored = kittyHandler.images.get(42);
        expect(stored).toBeDefined();
        const storedBytes = new Uint8Array(await stored.data.arrayBuffer());
        expect(storedBytes.length).toBe(rgba.length);
        expect(storedBytes).toEqual(rgba);
        expect(responses).toContain("\x1b_Gi=42;OK\x1b\\");
    });

    it("still handles a spec-conformant final chunk that carries both m=0 and payload", async () => {
        // Boundary check against the fix: a fully spec-conformant client
        // that puts the last bytes on the same escape sequence as m=0
        // (rather than a separate payload-less closer) must keep working.
        const width = 2;
        const height = 2;
        const rgba = solidColorRgba(width, height, 1, 2, 3);
        const b64 = toBase64(rgba);
        const mid = Math.floor(b64.length / 2);

        await writeAsync(term, `\x1b_Ga=T,f=32,s=${width},v=${height},i=7,m=1;${b64.slice(0, mid)}\x1b\\`);
        await writeAsync(term, `\x1b_Gm=0;${b64.slice(mid)}\x1b\\`);

        expect(kittyHandler.pendingTransmissions.size).toBe(0);
        const stored = kittyHandler.images.get(7);
        expect(stored).toBeDefined();
        const storedBytes = new Uint8Array(await stored.data.arrayBuffer());
        expect(storedBytes).toEqual(rgba);
    });
});
