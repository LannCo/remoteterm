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
        document.addEventListener("mousedown", handleClickOutside);
        return () => document.removeEventListener("mousedown", handleClickOutside);
    }, [onClose, anchorRef]);

    // Esc closes and returns focus to the anchor (component has no filter input
    // of its own, so there is no in-element Esc handler)
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

    return createPortal(
        <div
            ref={(node) => {
                dropdownRef.current = node;
                refs.setFloating(node);
            }}
            className="directory-dropdown"
            style={floatingStyles}
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
                    entries.map((entry) => (
                        <div
                            key={entry.path}
                            className={`directory-dropdown-item${entry.path === browsePath ? " selected" : ""}`}
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
