// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { openBuilderTerminal } from "@/app/store/builder-terminal";
import { getTabModelByTabId, TabModelContext } from "@/app/store/tab-model";
import { makeBuilderTileContents } from "@/builder/builder-termcontents";
import { BuilderFocusManager } from "@/builder/store/builder-focusmanager";
import { BuilderTermMismatchMessage, BuilderTermModel } from "@/builder/store/builder-term-model";
import { getLayoutModelForStaticTab, TileLayout } from "@/layout/index";
import { atoms, getApi, WOS } from "@/store/global";
import { cn, fireAndForget } from "@/util/util";
import { atom, useAtomValue } from "jotai";
import { memo, useCallback, useEffect, useMemo } from "react";

const ZeroAtom = atom(0);
const TileGapSizeAtom = atom((get) => get(atoms.settingsAtom)?.["window:tilegapsize"]);

type PanelMessageProps = {
    message: string;
    actionLabel?: string;
    onAction?: () => void;
};

const PanelMessage = memo(({ message, actionLabel, onAction }: PanelMessageProps) => {
    return (
        <div className="flex h-full w-full flex-col items-center justify-center gap-3 p-4 text-center text-sm text-secondary">
            <div>{message}</div>
            {actionLabel && (
                <button
                    className="px-3 py-1 bg-accent/80 text-onaccent rounded hover:bg-accent transition-colors cursor-pointer"
                    onClick={onAction}
                >
                    {actionLabel}
                </button>
            )}
        </div>
    );
});

PanelMessage.displayName = "PanelMessage";

// Rendered only once the model is ready, i.e. after the tab and its LayoutState are pinned and staticTabId
// is set, so the layout model created here subscribes to the right LayoutState.
const BuilderTermLayout = memo(({ tabId }: { tabId: string }) => {
    const tabAtom = useMemo(() => WOS.getWaveObjectAtom<Tab>(WOS.makeORef("tab", tabId)), [tabId]);
    const tileGapSize = useAtomValue(TileGapSizeAtom);
    const contents = useMemo(() => makeBuilderTileContents(tabId, tileGapSize), [tabId, tileGapSize]);
    const layoutModel = getLayoutModelForStaticTab();
    const numLeafs = useAtomValue(layoutModel?.numLeafs ?? ZeroAtom);
    const tab = useAtomValue(tabAtom);
    // numLeafs stays 0 until TileLayout's mount effect has run; blockids is already filled on first render.
    const isEmpty = numLeafs === 0 && (tab?.blockids?.length ?? 0) === 0;

    useEffect(() => {
        if (numLeafs === 0 && !isEmpty) {
            return;
        }
        BuilderTermModel.getInstance().handlePaneCount(numLeafs);
    }, [numLeafs, isEmpty]);

    // The count is only reported while mounted; without this, leaving "ready" (vanished, switching) would keep
    // terminal focus and a stale pane count.
    useEffect(() => {
        return () => BuilderTermModel.getInstance().handlePaneCount(0);
    }, []);

    return (
        <TabModelContext.Provider value={getTabModelByTabId(tabId)}>
            <div className="relative h-full w-full">
                <TileLayout
                    key={tabId}
                    contents={contents}
                    tabAtom={tabAtom}
                    getCursorPoint={getApi().getCursorPoint}
                />
                {isEmpty && (
                    <div className="absolute inset-0 bg-main-bg">
                        <PanelMessage
                            message="No terminals"
                            actionLabel="Open terminal"
                            onAction={() => fireAndForget(() => openBuilderTerminal("", null))}
                        />
                    </div>
                )}
            </div>
        </TabModelContext.Provider>
    );
});

BuilderTermLayout.displayName = "BuilderTermLayout";

export const BuilderTermPanel = memo(() => {
    const model = BuilderTermModel.getInstance();
    const state = useAtomValue(model.stateAtom);
    const errorMsg = useAtomValue(model.errorAtom);
    const tabId = useAtomValue(model.tabIdAtom);
    const focusType = useAtomValue(BuilderFocusManager.getInstance().focusType);

    useEffect(() => {
        fireAndForget(() => model.bootstrap());
    }, []);

    // With no panes the terminal side has nothing to act on, so a click there leaves Cmd:w closing the window.
    const handleFocusCapture = useCallback(() => {
        if (model.hasPanes()) {
            BuilderFocusManager.getInstance().setTerminalFocused();
        }
    }, [model]);

    let content: React.ReactNode;
    if (state === "ready" && tabId != null) {
        content = <BuilderTermLayout tabId={tabId} />;
    } else if (state === "error") {
        content = <PanelMessage message={errorMsg} actionLabel="Retry" onAction={() => model.retry()} />;
    } else if (state === "vanished") {
        content = (
            <PanelMessage message="The terminal tab is gone." actionLabel="Retry" onAction={() => model.retry()} />
        );
    } else if (state === "mismatch") {
        content = <PanelMessage message={BuilderTermMismatchMessage} />;
    } else if (state === "switching") {
        content = <PanelMessage message="Switching app…" />;
    } else {
        content = <PanelMessage message="Starting terminals…" />;
    }

    return (
        <div
            data-builder-term-panel
            className={cn(
                "h-full w-full overflow-hidden border-2",
                focusType === "terminal" ? "border-accent" : "border-transparent"
            )}
            onFocusCapture={handleFocusCapture}
            onMouseDownCapture={handleFocusCapture}
        >
            {content}
        </div>
    );
});

BuilderTermPanel.displayName = "BuilderTermPanel";
