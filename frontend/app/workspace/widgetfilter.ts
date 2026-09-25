// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

function shouldIncludeWidgetForWorkspace(widget: WidgetConfigType, workspaceId?: string): boolean {
    const workspaces = widget.workspaces;
    return !Array.isArray(workspaces) || workspaces.length === 0 || (workspaceId != null && workspaces.includes(workspaceId));
}

function sortByDisplayOrder(wmap: { [key: string]: WidgetConfigType }): WidgetConfigType[] {
    if (wmap == null) {
        return [];
    }
    const wlist = Object.values(wmap);
    wlist.sort((a, b) => {
        return (a["display:order"] ?? 0) - (b["display:order"] ?? 0);
    });
    return wlist;
}

export { shouldIncludeWidgetForWorkspace, sortByDisplayOrder };
