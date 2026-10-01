// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { getResolvedAppearanceModeAtom } from "@/app/store/appearance-atoms";
import { atoms } from "@/store/global";
import { useAtomValue } from "jotai";
import { useLayoutEffect } from "react";

export function AppThemeUpdater() {
    const tabId = useAtomValue(atoms.staticTabId);
    const resolvedMode = useAtomValue(getResolvedAppearanceModeAtom(tabId));

    // Layout effect so the palette switches before paint rather than one frame late.
    useLayoutEffect(() => {
        document.documentElement.dataset.theme = resolvedMode;
    }, [resolvedMode]);

    return null;
}
