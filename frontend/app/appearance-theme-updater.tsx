// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { getResolvedAppearanceModeAtom } from "@/app/store/appearance-atoms";
import { atoms } from "@/store/global";
import { useAtomValue } from "jotai";
import { useEffect } from "react";

export function AppThemeUpdater() {
    const tabId = useAtomValue(atoms.staticTabId);
    const resolvedMode = useAtomValue(getResolvedAppearanceModeAtom(tabId));

    useEffect(() => {
        document.documentElement.dataset.theme = resolvedMode;
    }, [resolvedMode]);

    return null;
}
