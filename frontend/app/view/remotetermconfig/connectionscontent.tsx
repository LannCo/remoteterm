// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { TabRpcClient } from "@/app/store/wshrpcutil";
import type { RemoteTermConfigViewModel } from "@/app/view/remotetermconfig/remotetermconfig-model";
import { cn, formatRelativeTime } from "@/util/util";
import { useAtomValue, useSetAtom } from "jotai";
import { memo, useMemo, useState } from "react";

type ConnectionsView = "hosts" | "keychain";

interface ViewToggleProps {
    view: ConnectionsView;
    onChange: (view: ConnectionsView) => void;
}

const ViewToggle = memo(({ view, onChange }: ViewToggleProps) => {
    return (
        <div
            role="group"
            aria-label="Connections view"
            className="flex items-center gap-1 bg-panel border border-border rounded-md p-0.5 shrink-0"
        >
            <button
                type="button"
                aria-pressed={view === "hosts"}
                className={cn(
                    "px-3 py-1 text-xs rounded cursor-pointer transition-colors",
                    view === "hosts" ? "bg-accentbg text-primary" : "text-secondary hover:text-primary"
                )}
                onClick={() => onChange("hosts")}
            >
                Hosts
            </button>
            <button
                type="button"
                aria-pressed={view === "keychain"}
                className={cn(
                    "flex items-center gap-1.5 px-3 py-1 text-xs rounded cursor-pointer transition-colors",
                    view === "keychain" ? "bg-accentbg text-primary" : "text-secondary hover:text-primary"
                )}
                onClick={() => onChange("keychain")}
            >
                Keychain
                <span className="text-xxs px-1.5 py-0.5 rounded-full bg-warning/15 text-warning">concept</span>
            </button>
        </div>
    );
});
ViewToggle.displayName = "ViewToggle";

function statusDotClass(status: ConnStatus | undefined): string {
    if (!status) {
        return "bg-muted-foreground/60";
    }
    return status.connected ? "bg-success" : "bg-muted-foreground";
}

function statusLabel(status: ConnStatus | undefined): string {
    if (!status) {
        return "Unknown";
    }
    return status.connected ? "Connected" : "Disconnected";
}

interface QuickAddRowProps {
    model: RemoteTermConfigViewModel;
}

const QuickAddRow = memo(({ model }: QuickAddRowProps) => {
    const value = useAtomValue(model.connectionsQuickAddValueAtom);
    const error = useAtomValue(model.connectionsQuickAddErrorAtom);
    const setValue = useSetAtom(model.connectionsQuickAddValueAtom);
    const setError = useSetAtom(model.connectionsQuickAddErrorAtom);

    return (
        <div className="flex flex-col gap-1">
            <div className="flex items-center gap-2">
                <input
                    type="text"
                    autoFocus
                    aria-label="New connection, user@host:port"
                    className="flex-1 max-w-[260px] bg-black/20 border border-dashed border-accent/40 rounded-md px-2.5 py-1.5 text-xs font-mono text-accent focus:outline-none focus:border-accent"
                    value={value}
                    onChange={(e) => {
                        setValue(e.target.value);
                        setError(null);
                    }}
                    onKeyDown={(e) => {
                        if (e.key === "Enter") {
                            model.submitConnectionQuickAdd();
                        } else if (e.key === "Escape") {
                            model.closeConnectionQuickAdd();
                        }
                    }}
                    placeholder="user@host:port"
                />
                <button
                    className="px-3 py-1.5 text-xs rounded bg-accent/80 text-background hover:bg-accent transition-colors cursor-pointer"
                    onClick={() => model.submitConnectionQuickAdd()}
                >
                    Add
                </button>
                <button
                    className="px-3 py-1.5 text-xs rounded border border-border text-secondary hover:text-primary transition-colors cursor-pointer"
                    onClick={() => model.closeConnectionQuickAdd()}
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
QuickAddRow.displayName = "QuickAddRow";

interface HostsHeaderProps {
    model: RemoteTermConfigViewModel;
    view: ConnectionsView;
    quickAddOpen: boolean;
}

const HostsHeader = memo(({ model, view, quickAddOpen }: HostsHeaderProps) => {
    const search = useAtomValue(model.connectionsSearchAtom);
    const setSearch = useSetAtom(model.connectionsSearchAtom);
    const setView = useSetAtom(model.connectionsViewAtom);

    return (
        <div className="flex flex-col gap-2">
            <div className="flex items-center gap-2.5">
                <ViewToggle view={view} onChange={setView} />
                <input
                    type="search"
                    aria-label="Search connections"
                    className="max-w-[260px] flex-1 bg-black/20 border border-border rounded-md px-2.5 py-1.5 text-xs focus:outline-none focus:border-accent"
                    value={search}
                    onChange={(e) => setSearch(e.target.value)}
                    placeholder="Search connections..."
                />
                <div className="flex-1" />
                <button
                    className="flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium rounded bg-accent/80 text-background hover:bg-accent transition-colors cursor-pointer"
                    onClick={() => model.openConnectionQuickAdd()}
                >
                    <i aria-hidden="true" className="fa-sharp fa-solid fa-plus" />
                    New Connection
                </button>
            </div>
            {quickAddOpen && <QuickAddRow model={model} />}
        </div>
    );
});
HostsHeader.displayName = "HostsHeader";

interface HostsListProps {
    names: string[];
    connStatusMap: Map<string, ConnStatus>;
    connKeywordsMap: Map<string, ConnKeywords>;
    onSetHeavyInterval: (name: string, seconds: number) => void;
}

const HostsList = memo(({ names, connStatusMap, connKeywordsMap, onSetHeavyInterval }: HostsListProps) => {
    const [expandedName, setExpandedName] = useState<string | null>(null);

    if (names.length === 0) {
        return (
            <div className="flex flex-col items-center justify-center gap-2 py-12 text-center">
                <i aria-hidden="true" className="fa-sharp fa-solid fa-server text-3xl text-muted" />
                <div className="text-secondary">No connections found</div>
            </div>
        );
    }

    return (
        <div className="flex-1 min-h-0 flex flex-col gap-1.5 overflow-y-auto">
            <div className="grid grid-cols-[14px_20px_1fr_90px] gap-2.5 px-2 text-caption font-semibold uppercase tracking-wide text-muted">
                <span />
                <span />
                <span>Connection</span>
                <span>Last used</span>
            </div>
            {names.map((name) => {
                const status = connStatusMap.get(name);
                const expanded = expandedName === name;
                const currentInterval = connKeywordsMap.get(name)?.["sysinfo:heavyinterval"] ?? 5;
                return (
                    <div key={name} className="flex flex-col gap-1.5">
                        <button
                            type="button"
                            aria-expanded={expanded}
                            onClick={() => setExpandedName(expanded ? null : name)}
                            title={statusLabel(status)}
                            className="grid grid-cols-[14px_20px_1fr_90px] items-center gap-2.5 bg-panel border border-border/60 rounded-md px-2 py-2 text-left cursor-pointer hover:border-border"
                        >
                            <span
                                aria-hidden="true"
                                className={cn("w-1.5 h-1.5 rounded-full justify-self-center", statusDotClass(status))}
                            />
                            <span className="w-5 h-5 rounded flex items-center justify-center bg-surface text-secondary">
                                <i aria-hidden="true" className="fa-sharp fa-solid fa-server text-xxs" />
                            </span>
                            <span className="font-mono text-xs truncate">
                                {name}
                                <span className="sr-only"> — {statusLabel(status)}</span>
                            </span>
                            <span className="text-xs text-muted-foreground">
                                {formatRelativeTime(status?.lastconnecttime ?? 0)}
                            </span>
                        </button>
                        {expanded && (
                            <div className="flex items-center gap-2 px-3 py-2 bg-panel/60 border border-border/40 rounded-md text-caption">
                                <label htmlFor={`heavy-interval-${name}`} className="text-muted">
                                    GPU/temp poll interval (seconds)
                                </label>
                                <input
                                    id={`heavy-interval-${name}`}
                                    type="number"
                                    min={1}
                                    max={60}
                                    value={currentInterval}
                                    onChange={(e) => onSetHeavyInterval(name, Number(e.target.value))}
                                    className="w-16 bg-black/20 border border-border rounded px-1.5 py-0.5 text-xs"
                                />
                            </div>
                        )}
                    </div>
                );
            })}
        </div>
    );
});
HostsList.displayName = "HostsList";

const KeychainBanner = memo(() => {
    return (
        <div className="flex items-center gap-2 px-3 py-2 bg-warning/10 border border-warning/30 rounded-md">
            <i aria-hidden="true" className="fa-sharp fa-solid fa-triangle-exclamation text-warning" />
            <span className="text-xs text-secondary">
                Concept only — no key vault exists in <code className="font-mono text-primary">connections.json</code>{" "}
                today. Shown to scope a possible future addition, not wired to anything real yet.
            </span>
        </div>
    );
});
KeychainBanner.displayName = "KeychainBanner";

interface KeychainRowProps {
    icon: string;
    name: string;
    subtitle: string;
    type: string;
    fingerprint: string;
    usedBy: string;
}

const KeychainRow = memo(({ icon, name, subtitle, type, fingerprint, usedBy }: KeychainRowProps) => {
    return (
        <div className="grid grid-cols-[22px_1.4fr_80px_1fr_80px_16px] items-center gap-2.5 bg-panel border border-border/60 rounded-md px-2.5 py-2">
            <i aria-hidden="true" className={cn("fa-sharp fa-solid text-secondary text-sm opacity-70", icon)} />
            <span>
                <div className="text-sm">{name}</div>
                <div className="text-xxs text-muted">{subtitle}</div>
            </span>
            <span className="text-xxs text-secondary bg-surface rounded-full px-1.5 py-0.5 w-fit">{type}</span>
            <span className="font-mono text-xxs text-muted">{fingerprint}</span>
            <span className="text-xs text-secondary">{usedBy}</span>
            <i aria-hidden="true" className="fa-sharp fa-solid fa-ellipsis text-muted justify-self-end opacity-70" />
        </div>
    );
});
KeychainRow.displayName = "KeychainRow";

const KeychainView = memo(() => {
    return (
        <div className="flex flex-col gap-2.5">
            <KeychainBanner />
            <div className="grid grid-cols-[22px_1.4fr_80px_1fr_80px_16px] gap-2.5 px-2.5 text-caption font-semibold uppercase tracking-wide text-muted">
                <span />
                <span>Key</span>
                <span>Type</span>
                <span>Fingerprint</span>
                <span>Used by</span>
                <span />
            </div>
            <KeychainRow
                icon="fa-lock"
                name="prod-ed25519"
                subtitle="Generated · 2026-03-11"
                type="ed25519"
                fingerprint="SHA256:k92j…a71f"
                usedBy="4 hosts"
            />
            <KeychainRow
                icon="fa-shield-halved"
                name="yubikey-5c"
                subtitle="FIDO2 hardware key"
                type="sk-ecdsa"
                fingerprint="SHA256:9f0c…22b0"
                usedBy="1 host"
            />
            <button
                disabled
                className="flex items-center gap-2 px-2.5 py-2 text-xs text-muted border border-dashed border-border rounded-md opacity-70"
            >
                <i aria-hidden="true" className="fa-sharp fa-solid fa-plus" />
                Generate / import key — not implemented
            </button>
        </div>
    );
});
KeychainView.displayName = "KeychainView";

interface ConnectionsContentProps {
    model: RemoteTermConfigViewModel;
}

export const ConnectionsContent = memo(({ model }: ConnectionsContentProps) => {
    const view = useAtomValue(model.connectionsViewAtom);
    const quickAddOpen = useAtomValue(model.connectionsQuickAddOpenAtom);
    const search = useAtomValue(model.connectionsSearchAtom);
    const connectionNames = useAtomValue(model.connectionNamesAtom);
    const connStatusMap = useAtomValue(model.connStatusMapAtom);
    const connKeywordsMap = useAtomValue(model.connKeywordsMapAtom);
    const setView = useSetAtom(model.connectionsViewAtom);

    const handleSetHeavyInterval = async (name: string, seconds: number) => {
        if (!Number.isFinite(seconds) || seconds < 1) {
            return;
        }
        const metamaptype: unknown = { "sysinfo:heavyinterval": seconds };
        const data: ConnConfigRequest = { host: name, metamaptype };
        try {
            await model.env.rpc.SetConnectionsConfigCommand(TabRpcClient, data);
        } catch (e) {
            console.log("problem setting connection config: ", e);
        }
    };

    const filteredNames = useMemo(() => {
        const lowerSearch = search.trim().toLowerCase();
        const filtered = lowerSearch
            ? connectionNames.filter((name) => name.toLowerCase().includes(lowerSearch))
            : connectionNames;
        return [...filtered].sort((a, b) => a.toLowerCase().localeCompare(b.toLowerCase()));
    }, [connectionNames, search]);

    if (view === "keychain") {
        return (
            <div className="flex flex-col gap-4 w-full h-full p-4">
                <div className="flex items-center gap-2.5">
                    <ViewToggle view={view} onChange={setView} />
                </div>
                <KeychainView />
            </div>
        );
    }

    return (
        <div className="flex flex-col gap-4 w-full h-full p-4 min-h-0">
            <HostsHeader model={model} view={view} quickAddOpen={quickAddOpen} />
            <HostsList
                names={filteredNames}
                connStatusMap={connStatusMap}
                connKeywordsMap={connKeywordsMap}
                onSetHeavyInterval={handleSetHeavyInterval}
            />
        </div>
    );
});

ConnectionsContent.displayName = "ConnectionsContent";
