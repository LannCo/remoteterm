// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import type { WaveConfigViewModel } from "@/app/view/waveconfig/waveconfig-model";
import { cn, isBlank, makeIconClass } from "@/util/util";
import { useAtomValue } from "jotai";
import { memo, useCallback, useEffect, useRef, useState, type CSSProperties } from "react";
import { useDrag, useDrop } from "react-dnd";

const WidgetDragType = "WAVECONFIG_WIDGET_ROW";

interface DragItem {
    key: string;
    index: number;
}

function widgetIconBoxStyle(color: string): CSSProperties | undefined {
    if (isBlank(color)) {
        return undefined;
    }
    return { backgroundColor: `color-mix(in srgb, ${color} 16%, transparent)` };
}

interface VisibilityToggleProps {
    hidden: boolean;
    onToggle: () => void;
}

const VisibilityToggle = memo(({ hidden, onToggle }: VisibilityToggleProps) => {
    return (
        <button
            type="button"
            role="switch"
            aria-checked={!hidden}
            aria-label={hidden ? "Show widget" : "Hide widget"}
            onClick={onToggle}
            className={cn(
                "relative w-[30px] h-[17px] shrink-0 rounded-full border-none cursor-pointer transition-colors",
                hidden ? "bg-border" : "bg-accent"
            )}
        >
            <span
                className={cn(
                    "absolute top-0.5 w-3 h-3 rounded-full transition-all",
                    hidden ? "left-0.5 bg-muted-foreground" : "right-0.5 bg-background"
                )}
            />
        </button>
    );
});
VisibilityToggle.displayName = "VisibilityToggle";

interface WidgetOrderRowProps {
    widgetKey: string;
    widget: WidgetConfigType;
    index: number;
    moveRow: (dragIndex: number, hoverIndex: number) => void;
    onDragFinished: (key: string) => void;
    onToggleHidden: (key: string) => void;
}

const WidgetOrderRow = memo(
    ({ widgetKey, widget, index, moveRow, onDragFinished, onToggleHidden }: WidgetOrderRowProps) => {
        const rowRef = useRef<HTMLDivElement>(null);

        const [, drop] = useDrop<DragItem>({
            accept: WidgetDragType,
            hover(item, monitor) {
                if (!rowRef.current || item.key === widgetKey) {
                    return;
                }
                const dragIndex = item.index;
                const hoverIndex = index;
                if (dragIndex === hoverIndex) {
                    return;
                }
                const hoverRect = rowRef.current.getBoundingClientRect();
                const hoverMiddleY = (hoverRect.bottom - hoverRect.top) / 2;
                const clientOffset = monitor.getClientOffset();
                const hoverClientY = clientOffset.y - hoverRect.top;
                if (dragIndex < hoverIndex && hoverClientY < hoverMiddleY) {
                    return;
                }
                if (dragIndex > hoverIndex && hoverClientY > hoverMiddleY) {
                    return;
                }
                moveRow(dragIndex, hoverIndex);
                item.index = hoverIndex;
            },
        });

        const [{ isDragging }, drag] = useDrag({
            type: WidgetDragType,
            item: (): DragItem => ({ key: widgetKey, index }),
            collect: (monitor) => ({ isDragging: monitor.isDragging() }),
            end: (item) => onDragFinished(item.key),
        });

        drag(drop(rowRef));

        return (
            <div
                ref={rowRef}
                className={cn(
                    "flex items-center gap-2.5 bg-panel border border-border/60 rounded-md px-2.5 py-2 cursor-grab",
                    isDragging && "opacity-40"
                )}
            >
                <i className="fa-sharp fa-solid fa-grip-dots-vertical text-xxs text-muted shrink-0" />
                <div
                    className="w-[26px] h-[26px] rounded-md flex items-center justify-center shrink-0"
                    style={widgetIconBoxStyle(widget.color)}
                >
                    <i
                        className={cn("text-xs", makeIconClass(widget.icon, true, { defaultIcon: "browser" }))}
                        style={isBlank(widget.color) ? undefined : { color: widget.color }}
                    />
                </div>
                <span className="flex-1 text-xs truncate">{isBlank(widget.label) ? widgetKey : widget.label}</span>
                <VisibilityToggle hidden={!!widget["display:hidden"]} onToggle={() => onToggleHidden(widgetKey)} />
            </div>
        );
    }
);
WidgetOrderRow.displayName = "WidgetOrderRow";

interface WidgetOrderPanelProps {
    model: WaveConfigViewModel;
}

const WidgetOrderPanel = memo(({ model }: WidgetOrderPanelProps) => {
    const orderedEntries = useAtomValue(model.widgetsOrderedAtom);
    const [localKeys, setLocalKeys] = useState<string[]>(() => orderedEntries.map(([key]) => key));
    const isDraggingRef = useRef(false);
    const localKeysRef = useRef(localKeys);
    localKeysRef.current = localKeys;

    useEffect(() => {
        if (isDraggingRef.current) {
            return;
        }
        setLocalKeys(orderedEntries.map(([key]) => key));
    }, [orderedEntries]);

    const widgetByKey = new Map(orderedEntries);

    const moveRow = useCallback((dragIndex: number, hoverIndex: number) => {
        isDraggingRef.current = true;
        setLocalKeys((prev) => {
            const next = [...prev];
            const [moved] = next.splice(dragIndex, 1);
            next.splice(hoverIndex, 0, moved);
            return next;
        });
    }, []);

    const onDragFinished = useCallback(
        (movedKey: string) => {
            isDraggingRef.current = false;
            const finalKeys = localKeysRef.current;
            const finalIndex = finalKeys.indexOf(movedKey);
            if (finalIndex === -1) {
                return;
            }
            model.reorderWidget(movedKey, finalIndex, finalKeys);
        },
        [model]
    );

    const onToggleHidden = useCallback(
        (key: string) => {
            model.toggleWidgetHidden(key);
        },
        [model]
    );

    return (
        <div className="w-[340px] @max-w600:w-[260px] @max-w450:w-[200px] shrink-0 flex flex-col gap-1.5 overflow-y-auto">
            <div className="text-caption font-semibold uppercase tracking-wide text-muted px-1 pb-0.5">
                Widget order &middot; drag to reorder
            </div>
            {localKeys.map((key, idx) => {
                const widget = widgetByKey.get(key);
                if (widget == null) {
                    return null;
                }
                return (
                    <WidgetOrderRow
                        key={key}
                        widgetKey={key}
                        widget={widget}
                        index={idx}
                        moveRow={moveRow}
                        onDragFinished={onDragFinished}
                        onToggleHidden={onToggleHidden}
                    />
                );
            })}
            <button
                type="button"
                disabled
                title="Add a new widget entry via Raw JSON — authoring a widget's blockdef isn't supported here yet"
                className="flex items-center gap-2 mt-0.5 px-2.5 py-2 text-xs text-muted border border-dashed border-border rounded-md opacity-70"
            >
                <i className="fa-sharp fa-solid fa-plus" />
                Add widget
            </button>
        </div>
    );
});
WidgetOrderPanel.displayName = "WidgetOrderPanel";

interface LivePreviewRailProps {
    model: WaveConfigViewModel;
}

const LivePreviewRail = memo(({ model }: LivePreviewRailProps) => {
    const previewWidgets = useAtomValue(model.widgetsPreviewAtom);

    return (
        <div className="flex-1 min-w-0 flex flex-col items-center">
            <div className="w-full max-w-[220px] flex flex-col gap-2.5">
                <div className="text-caption font-semibold uppercase tracking-wide text-muted">Live preview</div>
                <div className="w-16 bg-panel border border-border rounded-lg py-2.5 flex flex-col items-center gap-3.5 self-center">
                    {previewWidgets.map((widget, idx) => (
                        <div
                            key={idx}
                            className={cn(
                                "w-[30px] h-[30px] rounded-lg flex items-center justify-center text-[15px]",
                                widget["display:hidden"] && "hidden"
                            )}
                            style={widgetIconBoxStyle(widget.color)}
                            title={isBlank(widget.label) ? undefined : widget.label}
                        >
                            <i
                                className={makeIconClass(widget.icon, true, { defaultIcon: "browser" })}
                                style={isBlank(widget.color) ? undefined : { color: widget.color }}
                            />
                        </div>
                    ))}
                </div>
                <div className="text-caption text-muted leading-relaxed">
                    Reflects the toggle at left instantly &mdash; this is the same order/filter logic that renders the
                    real sidebar rail for the current workspace, not a redrawn approximation.
                </div>
            </div>
        </div>
    );
});
LivePreviewRail.displayName = "LivePreviewRail";

interface WidgetsContentProps {
    model: WaveConfigViewModel;
}

export const WidgetsContent = memo(({ model }: WidgetsContentProps) => {
    return (
        <div className="flex gap-4 w-full h-full p-4 min-h-0 overflow-hidden">
            <WidgetOrderPanel model={model} />
            <LivePreviewRail model={model} />
        </div>
    );
});

WidgetsContent.displayName = "WidgetsContent";
