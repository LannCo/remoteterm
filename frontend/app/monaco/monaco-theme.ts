// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { resolvedAppearanceModeAtom } from "@/app/store/appearance-atoms";
import { useAtomValue } from "jotai";
import * as monaco from "monaco-editor";
import { useEffect } from "react";

export function monacoThemeForMode(mode: "light" | "dark"): "wave-theme-light" | "wave-theme-dark" {
    return mode === "light" ? "wave-theme-light" : "wave-theme-dark";
}

export function useMonacoAppearanceTheme(): void {
    const mode = useAtomValue(resolvedAppearanceModeAtom);
    useEffect(() => {
        monaco.editor.setTheme(monacoThemeForMode(mode));
    }, [mode]);
}
