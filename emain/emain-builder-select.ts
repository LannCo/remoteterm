// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

type BuilderWindowLike = { builderId: string; builderAppId?: string };

// One window per app keeps one controller, one watcher and one build per app folder.
export function findBuilderWindowForApp<T extends BuilderWindowLike>(
    windows: T[],
    appId: string,
    excludeBuilderId: string
): T {
    if (!appId) {
        return null;
    }
    return windows.find((win) => win.builderId !== excludeBuilderId && win.builderAppId === appId) ?? null;
}

export const OpenPathGraceMs = 250;

// shell.openPath settles only when the opener process exits, and a file manager keeps running,
// so awaiting it left the renderer's promise pending for as long as the window stayed open. A
// failure that arrives inside the grace period is returned to the caller; a later one is logged.
export async function openPathDetached(
    openPath: (target: string) => Promise<string>,
    target: string,
    logError: (message: string) => void,
    graceMs: number = OpenPathGraceMs
): Promise<string> {
    let settled: Promise<string>;
    try {
        settled = openPath(target).then(
            (err) => err ?? "",
            (e) => `Could not open the folder: ${e instanceof Error ? e.message : String(e)}`
        );
    } catch (e) {
        return `Could not open the folder: ${e instanceof Error ? e.message : String(e)}`;
    }
    let timer: ReturnType<typeof setTimeout>;
    const grace = new Promise<null>((resolve) => {
        timer = setTimeout(() => resolve(null), graceMs);
    });
    const early = await Promise.race([settled, grace]);
    clearTimeout(timer);
    if (early != null) {
        return early;
    }
    void settled.then((err) => {
        if (err) {
            logError(`Failed to open ${target}: ${err}`);
        }
    });
    return "";
}
