// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { globalStore } from "@/app/store/jotaiStore";
import { BuilderFocusManager } from "@/builder/store/builder-focusmanager";
import { deleteLayoutModelForTab } from "@/layout/index";
import { atoms, getApi, WOS } from "@/store/global";
import { fireAndForget } from "@/util/util";
import { atom, type PrimitiveAtom } from "jotai";

export type BuilderTermState = "idle" | "loading" | "ready" | "error" | "mismatch" | "vanished" | "switching";

export const BuilderTermMismatchMessage = "Terminal app and builder app differ; reopen the builder";

export class BuilderTermModel {
    private static instance: BuilderTermModel | null = null;

    stateAtom: PrimitiveAtom<BuilderTermState> = atom<BuilderTermState>("idle");
    errorAtom: PrimitiveAtom<string> = atom<string>("");
    tabIdAtom = atom<string>(null) as PrimitiveAtom<string>;
    ensureOkAtom: PrimitiveAtom<boolean> = atom<boolean>(false);
    bootstrapping = false;
    paneCount = 0;
    tabUnsubFn: (() => void) | null = null;

    private constructor() {}

    static getInstance(): BuilderTermModel {
        if (!BuilderTermModel.instance) {
            BuilderTermModel.instance = new BuilderTermModel();
        }
        return BuilderTermModel.instance;
    }

    static resetInstance(): void {
        BuilderTermModel.instance?.tabUnsubFn?.();
        BuilderTermModel.instance = null;
    }

    async bootstrap(): Promise<void> {
        const state = globalStore.get(this.stateAtom);
        if (this.bootstrapping || (state !== "idle" && state !== "error")) {
            return;
        }
        this.bootstrapping = true;
        try {
            await this.runBootstrap();
        } finally {
            this.bootstrapping = false;
        }
    }

    // The tab and its LayoutState are pinned before staticTabId is set: the first layout model for the
    // static tab subscribes to that LayoutState when it is created (layoutModelHooks.ts), so nothing may
    // build one before both are loaded.
    async runBootstrap(): Promise<void> {
        this.setState("loading");
        const result = await getApi().ensureBuilderTab();
        if (!this.isLoading()) {
            return;
        }
        if (result?.error || !result?.tabid) {
            this.setError(result?.error || "Could not start the terminals.");
            return;
        }
        if (result.appid !== globalStore.get(atoms.builderAppId)) {
            this.setState("mismatch");
            return;
        }
        try {
            const tab = await WOS.loadAndPinWaveObject<Tab>(WOS.makeORef("tab", result.tabid));
            if (tab == null) {
                throw new Error("the terminal tab was not found");
            }
            await WOS.loadAndPinWaveObject<LayoutState>(WOS.makeORef("layout", tab.layoutstate));
        } catch (e) {
            if (this.isLoading()) {
                this.setError(`Could not load the terminals: ${e?.message ?? String(e)}`);
            }
            return;
        }
        if (!this.isLoading()) {
            return;
        }
        const staticTabId = globalStore.get(atoms.staticTabId);
        if (staticTabId != null && staticTabId !== result.tabid) {
            // Only an app switch changes the tab, and it reloads the renderer; reload rather than mix two tabs.
            getApi().doRefresh();
            return;
        }
        if (staticTabId == null) {
            globalStore.set(atoms.staticTabId, result.tabid);
        }
        globalStore.set(this.tabIdAtom, result.tabid);
        this.watchTab(result.tabid);
        if (globalStore.get(WOS.getWaveObjectAtom<Tab>(WOS.makeORef("tab", result.tabid))) == null) {
            this.setState("vanished");
            return;
        }
        this.setState("ready");
    }

    // Switch App can move the model on while a bootstrap await is still pending; the stale bootstrap must not
    // overwrite that state.
    isLoading(): boolean {
        return globalStore.get(this.stateAtom) === "loading";
    }

    // The header's Open terminal button follows ensureOkAtom, so it is true exactly while the panel is
    // ready: an Open in any other state would add a pane nobody can see.
    setState(state: BuilderTermState) {
        globalStore.set(this.stateAtom, state);
        globalStore.set(this.ensureOkAtom, state === "ready");
    }

    setError(message: string) {
        globalStore.set(this.errorAtom, message);
        this.setState("error");
    }

    // Before the layout exists a failed bootstrap can simply run again; once a tab was mounted, only a
    // fresh renderer gets a clean layout model.
    retry() {
        if (globalStore.get(this.stateAtom) === "error") {
            fireAndForget(() => this.bootstrap());
            return;
        }
        getApi().doRefresh();
    }

    markSwitching() {
        this.setState("switching");
    }

    watchTab(tabId: string) {
        this.tabUnsubFn?.();
        const tabAtom = WOS.getWaveObjectAtom<Tab>(WOS.makeORef("tab", tabId));
        this.tabUnsubFn = globalStore.sub(tabAtom, () => this.onTabValue(tabId, globalStore.get(tabAtom)));
    }

    onTabValue(tabId: string, tab: Tab) {
        if (tab != null) {
            return;
        }
        const state = globalStore.get(this.stateAtom);
        if (state !== "ready" && state !== "switching") {
            return;
        }
        deleteLayoutModelForTab(tabId);
        this.setState(state === "ready" ? "vanished" : "switching");
    }

    handlePaneCount(count: number) {
        const focusManager = BuilderFocusManager.getInstance();
        if (count === 0) {
            focusManager.setAppFocused();
        } else if (this.paneCount === 0) {
            focusManager.setTerminalFocused();
        }
        this.paneCount = count;
    }

    hasPanes(): boolean {
        return this.paneCount > 0;
    }
}
