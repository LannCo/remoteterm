// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import * as jotai from "jotai";
import { memo } from "react";

interface ActionErrorBannerProps {
    errorAtom: jotai.Atom<string | null>;
    onDismiss: () => void;
}

export const ActionErrorBanner = memo(({ errorAtom, onDismiss }: ActionErrorBannerProps) => {
    const error = jotai.useAtomValue(errorAtom);
    if (!error) {
        return null;
    }
    return (
        <div
            role="alert"
            className="mx-3 mt-2 flex items-start gap-2 p-2 bg-red-500/20 border border-red-500/30 rounded text-xs text-red-400"
        >
            <span className="flex-1 break-words">{error}</span>
            <button
                type="button"
                aria-label="Dismiss error"
                className="shrink-0 cursor-pointer text-red-400 hover:text-white"
                onClick={onDismiss}
            >
                <i aria-hidden="true" className="fa-solid fa-times" />
            </button>
        </div>
    );
});
ActionErrorBanner.displayName = "ActionErrorBanner";
