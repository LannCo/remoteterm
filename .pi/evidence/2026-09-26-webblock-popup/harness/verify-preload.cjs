// Task 7 verification harness preload. Deliberately does (almost) nothing except
// exist and load, so `getLastWebPreferences().preload` can be compared
// against a real file path, and so the popup's own page context can be
// checked for `typeof require/process/module === "undefined"` (this preload
// itself runs in the isolated preload context, which does have `process`
// scoped down under sandbox; that scoping is not what assertion (g) tests --
// assertion (g) checks the page's own main-world context, which this preload
// never touches with contextBridge).
//
// Re-run addition (assertion q): exposes a contextBridge value so the harness
// can positively confirm which preload ran for a given popup, replacing the
// inconclusive g1b/g1c console-message probe from the first run.
"use strict";
const { contextBridge } = require("electron");
console.log("[verify-preload] loaded");
if (contextBridge != null) {
    contextBridge.exposeInMainWorld("__verifyPreload", "ok");
}
