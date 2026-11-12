# PR #65: Kitty decoder reuse fix (audit item 1)

## Change
- `_processChunk` padded-m=1 branch: `decoder.init()` after `data8.slice()` instead of `release()` + `target.decoder = null`. The pending entry keeps one decoder for the whole transmission.
- Final-chunk path: skip `decoder.end()` when `decoder.loadedBytes === 0`. After a re-init, a payload-less `m=0` closer leaves the decoder empty, and ending an empty stream reports an error (this broke 4 existing tests before the guard).
- Applied to `src/kitty/KittyGraphicsHandler.ts` and `lib/addon-image.mjs`; patch regenerated with `npx patch-package @xterm/addon-image`.

## Environment finding
The installed `node_modules/@xterm/addon-image/lib/addon-image.mjs` in this worktree was stale: it did not match the committed patch (it still had `if(false)return this._processChunk(t)`, i.e. pre-550cd971). Running `patch-package` against it would have silently reverted the earlier payload-less-chunk fixes. It was rebuilt from the pristine npm tarball + committed patch before applying this change. Verified afterwards: pristine tarball + new patch reproduces `node_modules` byte-for-byte.

## Evidence
New test counts `WebAssembly.Memory` constructions (one per Base64Decoder first `init()`):
- Before (committed patch): 2000 allocations for 2000 padded chunks.
- After: 1 allocation; the pending entry's decoder object is identical across all 2000 chunks; image bytes match.
- Two unrelated transmissions: 2 allocations, distinct decoder objects, both images correct.

## Residual
Audit item 5 (aborted sequence releases the pending decoder but leaves it referenced) is unchanged. With reuse, if that decoder had grown past `keepSize` (4 MiB), `release()` drops its wasm instance and the next `put()` returns an error, where previously a fresh decoder was created. Both outcomes already lost the aborted chunk's data.
