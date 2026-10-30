// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { RpcApi } from "@/app/store/wshclientapi";
import { TabRpcClient } from "@/app/store/wshrpcutil";
import { formatRemoteUri } from "@/util/remotetermutil";
import { autoUpdate, shift, useFloating } from "@floating-ui/react";
import { memo, useCallback, useEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import "./directorydropdown.scss";

type DirectoryDropdownProps = {
    currentPath: string;
    connection: string;
    onSelect: (path: string) => void;
    onClose: () => void;
    anchorRef: React.RefObject<HTMLElement>;
    dirsOnly?: boolean;
};

type DirEntry = {
    name: string;
    path: string;
    isdir: boolean;
};

export function moveDropdownIndex(
    prevIndex: number,
    count: number,
    key: "ArrowUp" | "ArrowDown" | "Home" | "End"
): number {
    if (count === 0) {
        return -1;
    }
    // clamp a stale out-of-range index (list shrank) into the valid range first
    const clamped = Math.min(Math.max(prevIndex, -1), count - 1);
    switch (key) {
        case "Home":
            return 0;
        case "End":
            return count - 1;
        case "ArrowUp":
            // From no selection, ArrowUp lands on the last entry;
            // otherwise move up without wrapping past index 0
            return clamped < 0 ? count - 1 : Math.max(clamped - 1, 0);
        case "ArrowDown":
            return clamped < 0 ? 0 : Math.min(clamped + 1, count - 1);
        default:
            return prevIndex;
    }
}

export const DirectoryDropdown = memo(function DirectoryDropdown({
    currentPath,
    connection,
    onSelect,
    onClose,
    anchorRef,
    dirsOnly = false,
}: DirectoryDropdownProps) {
    const [entries, setEntries] = useState<DirEntry[]>([]);
    const [loading, setLoading] = useState(true);
    const [dirError, setDirError] = useState<string | null>(null);
    const [browsePath, setBrowsePath] = useState(currentPath);
    const dropdownRef = useRef<HTMLDivElement>(null);
    const [keyIndex, setKeyIndex] = useState(-1);

    // Reset browsePath when the widget's currentPath changes (e.g. dropdown reopened)
    useEffect(() => {
        setBrowsePath(currentPath);
    }, [currentPath]);

    // Positioning via floating-ui: autoUpdate tracks anchor movement (pane
    // resizes, scrolling) that a one-shot captured rect misses, and shift()
    // keeps the dropdown inside the window.
    const { refs, floatingStyles } = useFloating({
        placement: "bottom-start",
        strategy: "fixed",
        open: true,
        whileElementsMounted: autoUpdate,
        middleware: [shift({ padding: 4 })],
        elements: { reference: anchorRef?.current ?? null },
    });

    const loadDirectories = useCallback(
        async (dirPath: string) => {
            setLoading(true);
            // entries going stale: the rows vanish (loading placeholder takes
            // the list), so any keyboard highlight would point at invisible
            // rows with no feedback — clear it at load start
            setKeyIndex(-1);
            setDirError(null);
            const dirs: DirEntry[] = [];

            if (dirPath !== "/") {
                const parentPath = dirPath.split("/").slice(0, -1).join("/") || "/";
                dirs.push({ name: "..", path: parentPath, isdir: true });
            }

            try {
                const remotePath = formatRemoteUri(dirPath, connection || "local");
                const result = await RpcApi.FileListCommand(
                    TabRpcClient,
                    { path: remotePath },
                    undefined
                );

                if (result) {
                    for (const item of result) {
                        if (item.name !== "." && item.name !== "..") {
                            if (dirsOnly && !item.isdir) continue;
                            dirs.push({
                                name: item.name,
                                path: item.path,
                                isdir: item.isdir,
                            });
                        }
                    }
                }
            } catch (e) {
                console.error("Failed to load directories:", e);
                setDirError("Error loading directories");
            } finally {
                setLoading(false);
            }

            dirs.sort((a, b) => {
                if (a.name === "..") return -1;
                if (b.name === "..") return 1;
                if (a.isdir !== b.isdir) return a.isdir ? -1 : 1;
                return a.name.localeCompare(b.name);
            });
            setEntries(dirs);
            setKeyIndex(-1);
        },
        [connection, dirsOnly]
    );

    // Load entries from browsePath (local dropdown state), not currentPath
    useEffect(() => {
        loadDirectories(browsePath);
    }, [browsePath, loadDirectories]);

    useEffect(() => {
        function handleClickOutside(e: MouseEvent) {
            const anchor = anchorRef?.current;
            if (anchor && anchor.contains(e.target as Node)) return;
            if (dropdownRef.current && !dropdownRef.current.contains(e.target as Node)) {
                onClose();
            }
        }
        // Capture phase: see connectiondropdown — bubble-phase stopPropagation
        // elsewhere can starve this handler
        document.addEventListener("mousedown", handleClickOutside, true);
        return () => document.removeEventListener("mousedown", handleClickOutside, true);
    }, [onClose, anchorRef]);

    // Esc closes and refocuses the anchor where the anchor is focusable (the +
    // button). For directorydropdown the anchor is a plain span, so the
    // focus() call is a documented no-op there; the close still happens.
    useEffect(() => {
        function handleEscape(e: KeyboardEvent) {
            if (e.key === "Escape" && !e.defaultPrevented) {
                anchorRef?.current?.focus();
                onClose();
            }
        }
        document.addEventListener("keydown", handleEscape);
        return () => document.removeEventListener("keydown", handleEscape);
    }, [onClose, anchorRef]);

    const handleItemClick = useCallback(
        (e: React.MouseEvent, entry: DirEntry) => {
            e.stopPropagation();
            setBrowsePath(entry.path);
        },
        []
    );

    // Keyboard navigation: the dropdown itself is focused on open (tabIndex 0),
    // arrows/Home/End move the keyIndex over the rendered rows, Enter drills
    // into the entry (same as clicking it). Esc is handled by the document
    // keydown above. stopPropagation keeps arrows/Enter from reaching app-level
    // key handlers while the dropdown is open.
    const handleDropdownKeyDown = useCallback(
        (e: React.KeyboardEvent<HTMLDivElement>) => {
            const key = e.key;
            // Nav keys are intercepted wherever focus sits inside the dropdown
            // (e.g. the OK button) — otherwise they bubble to the preview file
            // list behind the open dropdown and move it. Selection only moves
            // when the container is focused AND entries are live — during the
            // loading/error window the row list is a placeholder, so a
            // highlight would act on invisible stale entries with no feedback.
            if (key === "ArrowUp" || key === "ArrowDown" || key === "Home" || key === "End") {
                e.preventDefault();
                e.stopPropagation();
                if (!loading && !dirError && e.target === e.currentTarget) {
                    setKeyIndex((idx) => moveDropdownIndex(idx, entries.length, key));
                }
                return;
            }
            if (key === "Enter") {
                // only hijack Enter when the container is focused; Enter on the
                // OK button (or any focused child) keeps its native action
                if (e.target !== e.currentTarget) {
                    return;
                }
                e.preventDefault();
                e.stopPropagation();
                const rowCount = entries.length;
                // nothing highlighted (or entries still stale from a previous
                // load): do nothing, consistent with the new-tab dropdown's
                // guarded Enter
                if (!loading && !dirError && keyIndex >= 0 && keyIndex < rowCount) {
                    setBrowsePath(entries[keyIndex].path);
                }
                return;
            }
            // swallow unhandled printable keys so they cannot reach the
            // focused block's terminal behind the portal
            if (!e.ctrlKey && !e.metaKey && !e.altKey && e.key.length === 1) {
                e.stopPropagation();
            }
        },
        [entries, keyIndex, loading, dirError]
    );

    // Keep the keyboard-selected row in view
    useEffect(() => {
        if (keyIndex < 0 || !dropdownRef.current) return;
        const list = dropdownRef.current.querySelector(".directory-dropdown-list");
        const row = list?.children[keyIndex] as HTMLElement | undefined;
        row?.scrollIntoView({ block: "nearest" });
    }, [keyIndex, entries]);

    // Focus the dropdown on open so keys land here without a click
    useEffect(() => {
        dropdownRef.current?.focus();
    }, []);

    return createPortal(
        <div
            ref={(node) => {
                dropdownRef.current = node;
                refs.setFloating(node);
            }}
            className="directory-dropdown"
            style={floatingStyles}
            tabIndex={0}
            onKeyDown={handleDropdownKeyDown}
        >
            <div className="directory-dropdown-breadcrumb">
                {browsePath !== "/" && (
                    <i
                        className="fa-solid fa-arrow-up directory-dropdown-breadcrumb-up"
                        onClick={() => setBrowsePath(browsePath.split("/").slice(0, -1).join("/") || "/")}
                        title="Up to parent directory"
                    />
                )}
                <span className="directory-dropdown-breadcrumb-path" title={browsePath}>{browsePath}</span>
            </div>
            <div className="directory-dropdown-list">
                {loading ? (
                    <div className="directory-dropdown-item">
                        <span className="directory-item-name">Loading...</span>
                    </div>
                ) : dirError ? (
                    <div className="directory-dropdown-item">
                        <span className="directory-item-name">Error loading directories</span>
                    </div>
                ) : entries.length === 0 ? (
                    <div className="directory-dropdown-item">
                        <span className="directory-item-name">No directories</span>
                    </div>
                ) : (
                    entries.map((entry, idx) => (
                        <div
                            key={entry.path}
                            className={`directory-dropdown-item${entry.path === browsePath ? " selected" : ""}${
                                idx === keyIndex ? " keyboard-active" : ""
                            }`}
                            onClick={(e) => handleItemClick(e, entry)}
                        >
                            <span className="directory-item-name">
                                <i className={`fa-solid ${entry.name === ".." ? "fa-arrow-up" : entry.isdir ? "fa-folder" : "fa-file"}`} />
                                {entry.name === ".." ? " Parent directory" : ` ${entry.name}`}
                            </span>
                        </div>
                    ))
                )}
            </div>
            <button
                type="button"
                className="directory-dropdown-ok"
                onClick={() => {
                    onSelect(browsePath);
                    onClose();
                }}
            >
                OK
            </button>
        </div>,
        document.body
    );
});

DirectoryDropdown.displayName = "DirectoryDropdown";
