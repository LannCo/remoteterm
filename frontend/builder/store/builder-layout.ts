// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

export type BuilderLayout = {
    terminal: number;
    app: number;
    build: number;
};

export const DefaultBuilderLayout: BuilderLayout = { terminal: 40, app: 80, build: 20 };
export const MinTerminalPercent = 20;

// builder:layout lives in rtinfo, which older builds wrote without "terminal" and which any pane can
// rewrite, so each value is checked before it sizes a panel.
export function mergeBuilderLayout(saved: Record<string, number>): BuilderLayout {
    const rtn: BuilderLayout = { ...DefaultBuilderLayout };
    if (saved == null) {
        return rtn;
    }
    for (const key of Object.keys(DefaultBuilderLayout) as (keyof BuilderLayout)[]) {
        const val = saved[key];
        if (typeof val === "number" && Number.isFinite(val) && val > 0 && val < 100) {
            rtn[key] = val;
        }
    }
    return rtn;
}
