// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { RpcApi } from "@/app/store/wshclientapi";
import { TabRpcClient } from "@/app/store/wshrpcutil";
import { BuilderAppPanelModel } from "@/builder/store/builder-apppanel-model";
import { getSettingsKeyAtom } from "@/store/global";
import { useAtomValue } from "jotai";
import { memo } from "react";

const LiveRebuildToggle = memo(() => {
    const liveRebuild = useAtomValue(getSettingsKeyAtom("builder:liverebuild")) ?? false;
    const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
        RpcApi.SetConfigCommand(TabRpcClient, { "builder:liverebuild": e.target.checked }).catch((err) => {
            console.error("Failed to update builder:liverebuild:", err);
        });
    };
    return (
        <label className="shrink-0 flex items-center gap-2 text-xs text-secondary cursor-pointer select-none">
            <input type="checkbox" className="cursor-pointer" checked={liveRebuild} onChange={handleChange} />
            Rebuild on external changes
        </label>
    );
});

LiveRebuildToggle.displayName = "LiveRebuildToggle";

const ExternalChangeStrip = memo(() => {
    const model = BuilderAppPanelModel.getInstance();
    const externalChange = useAtomValue(model.externalChangeAtom);
    if (!externalChange) {
        return null;
    }
    return (
        <div className="shrink-0 flex items-center gap-3 px-4 py-1.5 bg-warning/10 border-b border-warning/30 text-sm">
            <i className="fa fa-arrows-rotate text-warning" />
            <span className="flex-1">Changed on disk</span>
            <button
                className="px-3 py-0.5 text-sm font-medium bg-accent/80 text-onaccent rounded hover:bg-accent transition-colors cursor-pointer"
                onClick={() => model.requestRebuild()}
            >
                Rebuild
            </button>
        </div>
    );
});

ExternalChangeStrip.displayName = "ExternalChangeStrip";

const WatchStatusStrip = memo(() => {
    const model = BuilderAppPanelModel.getInstance();
    const watchStatus = useAtomValue(model.watchStatusAtom);
    if (watchStatus?.status !== "unavailable") {
        return null;
    }
    const reason = watchStatus.reason ? `: ${watchStatus.reason}` : "";
    return (
        <div className="shrink-0 flex items-center gap-3 px-4 py-1.5 bg-panel border-b border-border text-sm text-secondary">
            <i className="fa fa-eye-slash" />
            <span className="flex-1 min-w-0 truncate" title={watchStatus.reason}>
                Live reload unavailable{reason}. Saves in the Code tab still rebuild.
            </span>
        </div>
    );
});

WatchStatusStrip.displayName = "WatchStatusStrip";

const BuilderAppHeader = memo(() => {
    return (
        <>
            <div className="shrink-0 flex items-center justify-end gap-3 px-3 py-1 border-b border-border">
                <LiveRebuildToggle />
            </div>
            <ExternalChangeStrip />
            <WatchStatusStrip />
        </>
    );
});

BuilderAppHeader.displayName = "BuilderAppHeader";

export { BuilderAppHeader };
