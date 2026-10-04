// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { RpcApi } from "@/app/store/wshclientapi";
import { TabRpcClient } from "@/app/store/wshrpcutil";
import { BuilderAppPanel } from "@/builder/builder-apppanel";
import { BuilderBuildPanel } from "@/builder/builder-buildpanel";
import { BuilderTermPanel } from "@/builder/builder-termpanel";
import { BuilderAppPanelModel } from "@/builder/store/builder-apppanel-model";
import { BuilderFocusManager } from "@/builder/store/builder-focusmanager";
import { type BuilderLayout, mergeBuilderLayout, MinTerminalPercent } from "@/builder/store/builder-layout";
import { atoms } from "@/store/global";
import { cn } from "@/util/util";
import { useAtomValue, useSetAtom } from "jotai";
import { memo, useCallback, useEffect, useRef, useState } from "react";
import { Panel, PanelGroup, PanelResizeHandle } from "react-resizable-panels";
import { debounce } from "throttle-debounce";

const BuilderWorkspace = memo(() => {
    const builderId = useAtomValue(atoms.builderId);
    const [initialLayout, setInitialLayout] = useState<BuilderLayout>(null);
    const layoutRef = useRef<BuilderLayout>(null);
    const focusType = useAtomValue(BuilderFocusManager.getInstance().focusType);
    const isAppFocused = focusType === "app";
    const setResizeDragging = useSetAtom(BuilderAppPanelModel.getInstance().resizeDraggingAtom);

    useEffect(() => {
        const loadLayout = async () => {
            let saved: Record<string, number> = null;
            if (builderId) {
                try {
                    const rtInfo = await RpcApi.GetRTInfoCommand(TabRpcClient, {
                        oref: `builder:${builderId}`,
                    });
                    saved = rtInfo?.["builder:layout"] as Record<string, number>;
                } catch (error) {
                    console.error("Failed to load builder layout:", error);
                }
            }
            const layout = mergeBuilderLayout(saved);
            layoutRef.current = layout;
            setInitialLayout(layout);
        };

        loadLayout();
    }, [builderId]);

    const saveLayout = useCallback(
        debounce(500, (newLayout: BuilderLayout) => {
            if (!builderId) return;

            RpcApi.SetRTInfoCommand(TabRpcClient, {
                oref: `builder:${builderId}`,
                data: {
                    "builder:layout": newLayout,
                },
            }).catch((error) => {
                console.error("Failed to save builder layout:", error);
            });
        }),
        [builderId]
    );

    // Both panel groups report their sizes on mount; merging into a ref stops one report overwriting the
    // other with a stale copy of the layout.
    const updateLayout = useCallback(
        (patch: Partial<BuilderLayout>) => {
            layoutRef.current = { ...layoutRef.current, ...patch };
            saveLayout(layoutRef.current);
        },
        [saveLayout]
    );

    const handleHorizontalLayout = useCallback(
        (sizes: number[]) => updateLayout({ terminal: sizes[0] }),
        [updateLayout]
    );

    const handleVerticalLayout = useCallback(
        (sizes: number[]) => updateLayout({ app: sizes[0], build: sizes[1] }),
        [updateLayout]
    );

    // The app panel sets app focus itself; this also covers the build panel below it.
    const handleAppColumnFocus = useCallback(() => {
        BuilderFocusManager.getInstance().setAppFocused();
    }, []);

    if (initialLayout == null) {
        return null;
    }

    return (
        <div className="flex-1 overflow-hidden" data-builder-focus={focusType}>
            <PanelGroup direction="horizontal" onLayout={handleHorizontalLayout}>
                <Panel defaultSize={initialLayout.terminal} minSize={MinTerminalPercent}>
                    <BuilderTermPanel />
                </Panel>
                <PanelResizeHandle
                    className="w-0.5 bg-transparent hover:bg-gray-500/20 transition-colors"
                    onDragging={setResizeDragging}
                />
                <Panel defaultSize={100 - initialLayout.terminal} minSize={20}>
                    <div
                        className={cn(
                            "flex flex-col relative h-full",
                            isAppFocused ? "border-2 border-accent" : "border-2 border-transparent"
                        )}
                        style={{
                            borderBottomRightRadius: 8,
                        }}
                        onFocusCapture={handleAppColumnFocus}
                        onMouseDownCapture={handleAppColumnFocus}
                    >
                        <PanelGroup direction="vertical" onLayout={handleVerticalLayout}>
                            <Panel defaultSize={initialLayout.app} minSize={20}>
                                <BuilderAppPanel />
                            </Panel>
                            <PanelResizeHandle
                                className="h-0.5 bg-transparent hover:bg-gray-500/20 transition-colors"
                                onDragging={setResizeDragging}
                            />
                            <Panel
                                defaultSize={initialLayout.build}
                                minSize={20}
                                maxSize={50}
                                style={{ borderBottomRightRadius: 8 }}
                            >
                                <BuilderBuildPanel />
                            </Panel>
                        </PanelGroup>
                    </div>
                </Panel>
            </PanelGroup>
        </div>
    );
});

BuilderWorkspace.displayName = "BuilderWorkspace";

export { BuilderWorkspace };
