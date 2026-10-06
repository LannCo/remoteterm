// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import tailwindcss from "@tailwindcss/vite";
import react from "@vitejs/plugin-react-swc";
import { defineConfig } from "vite";

const ReactPackages = new Set(["react", "react-dom", "scheduler"]);
const ChartPackages = new Set([
    "recharts",
    "recharts-scale",
    "victory-vendor",
    "internmap",
    "react-redux",
    "redux",
    "@reduxjs/toolkit",
    "reselect",
    "immer",
    "es-toolkit",
    "decimal.js-light",
    "eventemitter3",
    "tiny-invariant",
    "use-sync-external-store",
    "fast-equals",
]);
const MarkdownPackagePattern =
    /^(react-markdown|unified|remark-.+|rehype-.+|micromark.*|mdast-.+|hast-.+|unist-.+|vfile.*|bail|trough|devlop|decode-named-character-reference|character-entities.*|property-information|space-separated-tokens|comma-separated-tokens|html-url-attributes|ccount|markdown-table|trim-lines|zwitch|longest-streak|is-plain-obj|style-to-.+|inline-style-parser|estree-util-.+)$/;

// The bundle is split by dependency family so no chunk reaches the 500 kB warning size.
function manualChunks(id: string): string | undefined {
    const parts = id.split("/node_modules/");
    if (parts.length < 2) {
        return undefined;
    }
    const segments = parts[parts.length - 1].split("/");
    const pkg = segments[0].startsWith("@") ? segments[0] + "/" + segments[1] : segments[0];
    if (ReactPackages.has(pkg)) {
        return "react";
    }
    if (ChartPackages.has(pkg) || pkg.startsWith("d3-")) {
        return "charts";
    }
    if (MarkdownPackagePattern.test(pkg)) {
        return "markdown";
    }
    return "vendor";
}

export default defineConfig({
    plugins: [react(), tailwindcss()],
    resolve: {
        alias: {
            "@": "/src",
        },
    },
    server: {
        port: 12025,
        open: true,
        proxy: {
            "/api": {
                target: "http://localhost:12026",
                changeOrigin: true,
            },
            "/assets": {
                target: "http://localhost:12026",
                changeOrigin: true,
            },
        },
    },
    build: {
        outDir: "dist",
        minify: process.env.NODE_ENV === "development" ? false : "esbuild",
        sourcemap: process.env.NODE_ENV === "development" ? true : false,
        rollupOptions: {
            output: { manualChunks },
        },
    },
});
