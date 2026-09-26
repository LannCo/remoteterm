// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import type { HandlerDetails, Rectangle } from "electron";
import { isAllowedPopupUrl } from "./emain-websecurity";

export type WindowOpenRoute = "popup" | "tab" | "deny";

export const MaxLivePopupsPerOpener = 4;
const DefaultPopupWidth = 520;
const DefaultPopupHeight = 640;
const MinPopupWidth = 200;
const MinPopupHeight = 150;
const FalseyFeatureValues = new Set(["0", "no", "false"]);

export function parseWindowFeatures(features: string | undefined): Record<string, string | boolean> {
    const rtn: Record<string, string | boolean> = {};
    if (!features) {
        return rtn;
    }
    for (const rawPart of features.split(",")) {
        const part = rawPart.trim();
        if (part === "") {
            continue;
        }
        const eq = part.indexOf("=");
        if (eq < 0) {
            rtn[part.toLowerCase()] = true;
            continue;
        }
        const key = part.slice(0, eq).trim().toLowerCase();
        const value = part.slice(eq + 1).trim();
        rtn[key] = FalseyFeatureValues.has(value.toLowerCase()) ? false : value;
    }
    return rtn;
}

function featureNumber(parsed: Record<string, string | boolean>, ...keys: string[]): number | undefined {
    for (const key of keys) {
        const v = parsed[key];
        if (typeof v !== "string") {
            continue;
        }
        const n = parseInt(v, 10);
        if (!Number.isNaN(n)) {
            return n;
        }
    }
    return undefined;
}

export function requestedPopupGeometry(features: string | undefined): {
    width?: number;
    height?: number;
    x?: number;
    y?: number;
} {
    const parsed = parseWindowFeatures(features);
    return {
        width: featureNumber(parsed, "width", "innerwidth"),
        height: featureNumber(parsed, "height", "innerheight"),
        x: featureNumber(parsed, "left", "screenx"),
        y: featureNumber(parsed, "top", "screeny"),
    };
}

// Chromium reports "new-window" only when window.open() was given a features string
// (its NEW_POPUP disposition); plain window.open(url) and target=_blank are tabs.
export function classifyWindowOpen(details: Pick<HandlerDetails, "url" | "disposition" | "features">): WindowOpenRoute {
    if (details.disposition !== "new-window") {
        return "tab";
    }
    const parsed = parseWindowFeatures(details.features);
    if (parsed["noopener"] === true || parsed["noreferrer"] === true) {
        return "tab";
    }
    if (!isAllowedPopupUrl(details.url)) {
        return "deny";
    }
    return "popup";
}

function clamp(n: number, lo: number, hi: number): number {
    return Math.min(Math.max(n, lo), hi);
}

export function computePopupBounds(
    requested: { width?: number; height?: number; x?: number; y?: number },
    parentBounds: Rectangle,
    workArea: Rectangle
): Rectangle {
    const width = clamp(requested.width ?? DefaultPopupWidth, MinPopupWidth, workArea.width);
    const height = clamp(requested.height ?? DefaultPopupHeight, MinPopupHeight, workArea.height);
    const defaultX = parentBounds.x + Math.round((parentBounds.width - width) / 2);
    const defaultY = parentBounds.y + Math.round((parentBounds.height - height) / 2);
    const x = clamp(requested.x ?? defaultX, workArea.x, workArea.x + workArea.width - width);
    const y = clamp(requested.y ?? defaultY, workArea.y, workArea.y + workArea.height - height);
    return { x, y, width, height };
}
