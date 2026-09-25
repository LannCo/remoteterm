// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import type { RemoteTermConfigViewModel } from "@/app/view/remotetermconfig/remotetermconfig-model";
import { cn, isBlank } from "@/util/util";
import { useAtomValue, useSetAtom } from "jotai";
import { memo, useEffect, useState } from "react";

const BlendModes = [
    "normal",
    "multiply",
    "screen",
    "overlay",
    "darken",
    "lighten",
    "color-dodge",
    "color-burn",
    "hard-light",
    "soft-light",
    "difference",
    "exclusion",
    "hue",
    "saturation",
    "color",
    "luminosity",
];

const SwatchPreviewOpacity = 0.85;

interface BackgroundTileProps {
    label: string;
    bg?: string;
    active: boolean;
    onClick: () => void;
}

const BackgroundTile = memo(({ label, bg, active, onClick }: BackgroundTileProps) => {
    return (
        <button
            type="button"
            onClick={onClick}
            aria-pressed={active}
            className={cn(
                "relative flex flex-col overflow-hidden rounded-md text-left cursor-pointer transition-colors",
                active ? "border-2 border-accent" : "border border-border/60 hover:border-border"
            )}
        >
            <div
                className="h-16 bg-panel"
                style={isBlank(bg) ? undefined : { background: bg, opacity: SwatchPreviewOpacity }}
            />
            <div className="px-2 py-1.5 bg-modalbg text-caption truncate">{label}</div>
            {active && (
                <i
                    aria-hidden="true"
                    className="fa-sharp fa-solid fa-check absolute top-1 right-1 text-xxs bg-accent text-background rounded-full p-1"
                />
            )}
        </button>
    );
});
BackgroundTile.displayName = "BackgroundTile";

interface AddBackgroundFormProps {
    model: RemoteTermConfigViewModel;
}

const AddBackgroundForm = memo(({ model }: AddBackgroundFormProps) => {
    const name = useAtomValue(model.backgroundsAddNameAtom);
    const bg = useAtomValue(model.backgroundsAddBgAtom);
    const error = useAtomValue(model.backgroundsAddErrorAtom);
    const setName = useSetAtom(model.backgroundsAddNameAtom);
    const setBg = useSetAtom(model.backgroundsAddBgAtom);

    return (
        <div className="flex flex-col gap-2 rounded-md border border-dashed border-accent/40 bg-panel p-3">
            <div className="flex items-center gap-2">
                <input
                    type="text"
                    autoFocus
                    aria-label="Background name"
                    className="flex-1 max-w-[180px] bg-black/20 border border-border rounded px-2 py-1 text-xs focus:outline-none focus:border-accent"
                    placeholder="Name"
                    value={name}
                    onChange={(e) => setName(e.target.value)}
                    onKeyDown={(e) => e.key === "Escape" && model.closeBackgroundAdd()}
                />
                <input
                    type="text"
                    aria-label="Background CSS value"
                    className="flex-1 bg-black/20 border border-border rounded px-2 py-1 text-xs font-mono focus:outline-none focus:border-accent"
                    placeholder="CSS background value, e.g. linear-gradient(135deg, purple, blue)"
                    value={bg}
                    onChange={(e) => setBg(e.target.value)}
                    onKeyDown={(e) => {
                        if (e.key === "Enter") model.submitBackgroundAdd();
                        if (e.key === "Escape") model.closeBackgroundAdd();
                    }}
                />
                <button
                    className="px-3 py-1.5 text-xs rounded bg-accent/80 text-background hover:bg-accent transition-colors cursor-pointer shrink-0"
                    onClick={() => model.submitBackgroundAdd()}
                >
                    Add
                </button>
                <button
                    className="px-3 py-1.5 text-xs rounded border border-border text-secondary hover:text-primary transition-colors cursor-pointer shrink-0"
                    onClick={() => model.closeBackgroundAdd()}
                >
                    Cancel
                </button>
            </div>
            {error && (
                <div role="alert" className="flex items-center gap-1.5 text-xs">
                    <i aria-hidden="true" className="fa-sharp fa-solid fa-circle-exclamation text-error" />
                    <span className="text-primary">{error}</span>
                </div>
            )}
        </div>
    );
});
AddBackgroundForm.displayName = "AddBackgroundForm";

interface BackgroundDetailPanelProps {
    model: RemoteTermConfigViewModel;
    activeKey: string;
    background: BackgroundConfigType;
}

const BackgroundDetailPanel = memo(({ model, activeKey, background }: BackgroundDetailPanelProps) => {
    const [localOpacity, setLocalOpacity] = useState(Math.round((background["bg:opacity"] ?? 1) * 100));

    useEffect(() => {
        setLocalOpacity(Math.round((background["bg:opacity"] ?? 1) * 100));
    }, [background]);

    const commitOpacity = () => {
        model.updateBackgroundOpacity(activeKey, localOpacity / 100);
    };

    return (
        <div className="flex gap-5 items-center rounded-lg border border-border/60 bg-panel p-3">
            <div
                className="w-20 h-13 rounded shrink-0 bg-modalbg"
                style={{ background: background.bg, opacity: SwatchPreviewOpacity }}
            />
            <div className="flex-1 min-w-0">
                <div className="text-xs font-semibold truncate">
                    {background["display:name"]} &mdash; editing preset
                </div>
                <div className="flex gap-5 mt-2">
                    <label className="flex flex-col gap-1 text-caption text-muted">
                        Opacity
                        <input
                            type="range"
                            min={0}
                            max={100}
                            value={localOpacity}
                            onChange={(e) => setLocalOpacity(Number(e.target.value))}
                            onMouseUp={commitOpacity}
                            onTouchEnd={commitOpacity}
                            onKeyUp={commitOpacity}
                            className="w-36 accent-accent cursor-pointer"
                        />
                    </label>
                    <label className="flex flex-col gap-1 text-caption text-muted">
                        Blend mode
                        <select
                            value={background["bg:blendmode"] ?? "normal"}
                            onChange={(e) => model.updateBackgroundBlendMode(activeKey, e.target.value)}
                            className="rounded border border-border bg-background px-1.5 py-1 text-foreground text-caption cursor-pointer"
                        >
                            {BlendModes.map((mode) => (
                                <option key={mode} value={mode}>
                                    {mode}
                                </option>
                            ))}
                        </select>
                    </label>
                </div>
            </div>
        </div>
    );
});
BackgroundDetailPanel.displayName = "BackgroundDetailPanel";

interface BackgroundsContentProps {
    model: RemoteTermConfigViewModel;
}

export const BackgroundsContent = memo(({ model }: BackgroundsContentProps) => {
    const backgroundsOrdered = useAtomValue(model.backgroundsOrderedAtom);
    const activeKey = useAtomValue(model.activeTabBackgroundKeyAtom);
    const addOpen = useAtomValue(model.backgroundsAddOpenAtom);

    const backgroundsMap = Object.fromEntries(backgroundsOrdered);
    const activeBackground = activeKey ? backgroundsMap[activeKey] : null;

    return (
        <div className="flex flex-col gap-4 w-full h-full p-4 overflow-y-auto">
            <div className="grid grid-cols-3 @w450:grid-cols-4 @w600:grid-cols-5 @w900:grid-cols-6 gap-3">
                <BackgroundTile label="Default" active={!activeKey} onClick={() => model.applyBackgroundToTab(null)} />
                {backgroundsOrdered.map(([key, background]) => (
                    <BackgroundTile
                        key={key}
                        label={background["display:name"] ?? key}
                        bg={background.bg}
                        active={key === activeKey}
                        onClick={() => model.applyBackgroundToTab(key)}
                    />
                ))}
                <button
                    type="button"
                    onClick={() => model.openBackgroundAdd()}
                    className="flex flex-col items-center justify-center gap-1 h-[94px] rounded-md border border-dashed border-border text-muted text-caption cursor-pointer hover:border-accent/60 hover:text-secondary transition-colors"
                >
                    <i aria-hidden="true" className="fa-sharp fa-solid fa-plus" />
                    New background
                </button>
            </div>

            {addOpen && <AddBackgroundForm model={model} />}

            {activeKey && activeBackground && (
                <BackgroundDetailPanel model={model} activeKey={activeKey} background={activeBackground} />
            )}
        </div>
    );
});

BackgroundsContent.displayName = "BackgroundsContent";
