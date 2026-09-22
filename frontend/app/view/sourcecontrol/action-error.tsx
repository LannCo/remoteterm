// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import * as jotai from "jotai";
import { memo, type RefObject } from "react";

interface ActionErrorBannerProps {
    errorAtom: jotai.Atom<string | null>;
    onDismiss: () => void;
    // Dismissing unmounts the banner along with its focused button; focus moves here first so
    // it does not fall to <body>.
    focusAnchorRef: RefObject<HTMLElement>;
}

export const ActionErrorBanner = memo(({ errorAtom, onDismiss, focusAnchorRef }: ActionErrorBannerProps) => {
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
                className="shrink-0 min-w-6 min-h-6 -m-1 flex items-center justify-center rounded cursor-pointer text-red-400 hover:text-white"
                onClick={() => {
                    focusAnchorRef.current?.focus();
                    onDismiss();
                }}
            >
                <i aria-hidden="true" className="fa-solid fa-times" />
            </button>
        </div>
    );
});
ActionErrorBanner.displayName = "ActionErrorBanner";
