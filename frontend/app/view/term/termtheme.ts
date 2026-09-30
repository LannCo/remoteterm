// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { resolvedAppearanceModeAtom } from "@/app/store/appearance-atoms";
import type { TermViewModel } from "@/app/view/term/term-model";
import {
    computeMinimumContrastRatio,
    computeReportedColours,
    computeTheme,
    getDefaultTermThemeName,
} from "@/app/view/term/termutil";
import { TermWrap } from "@/app/view/term/termwrap";
import { atoms } from "@/store/global";
import { useAtomValue } from "jotai";
import { useEffect } from "react";

interface TermThemeProps {
    blockId: string;
    termRef: React.RefObject<TermWrap>;
    model: TermViewModel;
}

const TermThemeUpdater = ({ blockId, model, termRef }: TermThemeProps) => {
    const fullConfig = useAtomValue(atoms.fullConfigAtom);
    const blockTermTheme = useAtomValue(model.termThemeNameAtom);
    const transparency = useAtomValue(model.termTransparencyAtom);
    const appearanceMode = useAtomValue(resolvedAppearanceModeAtom);
    const [theme, bgcolor] = computeTheme(
        fullConfig,
        blockTermTheme,
        transparency,
        getDefaultTermThemeName(appearanceMode)
    );
    useEffect(() => {
        if (termRef.current?.terminal) {
            termRef.current.terminal.options.theme = theme;
            termRef.current.terminal.options.minimumContrastRatio = computeMinimumContrastRatio(theme, bgcolor);
            termRef.current.setReportedColours(computeReportedColours(theme, bgcolor));
        }
    }, [theme]);
    return null;
};

export { TermThemeUpdater };
