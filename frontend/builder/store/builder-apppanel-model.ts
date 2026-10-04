// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { globalStore } from "@/app/store/jotaiStore";
import { waveEventSubscribeSingle } from "@/app/store/wps";
import { RpcApi } from "@/app/store/wshclientapi";
import { TabRpcClient } from "@/app/store/wshrpcutil";
import { atoms, getApi, getSettingsKeyAtom, WOS } from "@/store/global";
import { base64ToString, stringToBase64 } from "@/util/util";
import type { WebviewTag } from "electron";
import { atom, type Atom, type PrimitiveAtom } from "jotai";
import type * as MonacoTypes from "monaco-editor";
import { decideReload, type ReloadDecision } from "./decide-reload";

export type TabType = "preview" | "files" | "code" | "secrets" | "configdata";

export type EnvVar = {
    name: string;
    value: string;
    visible?: boolean;
};

export class BuilderAppPanelModel {
    private static instance: BuilderAppPanelModel | null = null;

    activeTab: PrimitiveAtom<TabType> = atom<TabType>("preview");
    codeContentAtom: PrimitiveAtom<string> = atom<string>("");
    originalContentAtom: PrimitiveAtom<string> = atom<string>("");
    envVarsArrayAtom: PrimitiveAtom<EnvVar[]> = atom<EnvVar[]>([]);
    envVarIndexAtoms: Atom<EnvVar | null>[] = [];
    envVarsDirtyAtom: PrimitiveAtom<boolean> = atom<boolean>(false);
    isLoadingAtom: PrimitiveAtom<boolean> = atom<boolean>(false);
    errorAtom: PrimitiveAtom<string> = atom<string>("");
    isSeedingAtom: PrimitiveAtom<boolean> = atom<boolean>(false);
    appGoMissingAtom: PrimitiveAtom<boolean> = atom<boolean>(false);
    diskChangedAtom = atom<string>(null) as PrimitiveAtom<string>;
    externalChangeAtom: PrimitiveAtom<boolean> = atom<boolean>(false);
    watchStatusAtom = atom<BuilderWatchStatusData>(null) as PrimitiveAtom<BuilderWatchStatusData>;
    appDirAtom = atom<string>(null) as PrimitiveAtom<string>;
    noticeAtom: PrimitiveAtom<string> = atom<string>("");
    builderStatusAtom = atom<BuilderStatusData>(null) as PrimitiveAtom<BuilderStatusData>;
    hasSecretsAtom: PrimitiveAtom<boolean> = atom<boolean>(false);
    saveNeededAtom!: Atom<boolean>;
    focusElemRef: { current: HTMLInputElement | null } = { current: null };
    monacoEditorRef: { current: MonacoTypes.editor.IStandaloneCodeEditor | null } = { current: null };
    webviewRef: { current: WebviewTag | null } = { current: null };
    statusUnsubFn: (() => void) | null = null;
    appGoUpdateUnsubFn: (() => void) | null = null;
    watchStatusUnsubFn: (() => void) | null = null;
    configUnsubFn: (() => void) | null = null;
    appIdUnsubFn: (() => void) | null = null;
    lastWrittenContent: string = null;
    reconcileSeq = 0;
    initialized = false;

    private constructor() {
        this.saveNeededAtom = atom((get) => {
            return get(this.codeContentAtom) !== get(this.originalContentAtom);
        });
    }

    static getInstance(): BuilderAppPanelModel {
        if (!BuilderAppPanelModel.instance) {
            BuilderAppPanelModel.instance = new BuilderAppPanelModel();
        }
        return BuilderAppPanelModel.instance;
    }

    setActiveTab(tab: TabType) {
        globalStore.set(this.activeTab, tab);
    }

    getActiveTab(): TabType {
        return globalStore.get(this.activeTab);
    }

    setCodeContent(content: string) {
        globalStore.set(this.codeContentAtom, content);
    }

    async initialize() {
        if (this.initialized) return;
        this.initialized = true;

        // builderId is set in initialization so is always available
        const builderId = globalStore.get(atoms.builderId);

        if (this.statusUnsubFn) {
            this.statusUnsubFn();
        }

        this.statusUnsubFn = waveEventSubscribeSingle({
            eventType: "builderstatus",
            scope: WOS.makeORef("builder", builderId),
            handler: (event) => {
                const status = event.data;
                const currentStatus = globalStore.get(this.builderStatusAtom);
                if (!currentStatus || !currentStatus.version || status.version > currentStatus.version) {
                    globalStore.set(this.builderStatusAtom, status);
                    this.updateSecretsLatch(status);
                    if (status.status === "building") {
                        globalStore.set(this.externalChangeAtom, false);
                    }
                }
            },
        });

        try {
            const status = await RpcApi.GetBuilderStatusCommand(TabRpcClient, builderId);
            globalStore.set(this.builderStatusAtom, status);
            this.updateSecretsLatch(status);
        } catch (err) {
            console.error("Failed to load builder status:", err);
        }

        // the apppanel does not render until appId is set, so this will never be null during initialization
        const appId = globalStore.get(atoms.builderAppId);
        await this.loadAppFile(appId);
        await this.loadEnvVars(builderId);
        await this.loadAppDir();

        this.watchStatusUnsubFn = waveEventSubscribeSingle({
            eventType: "rtapp:watchstatus",
            scope: WOS.makeORef("builder", builderId),
            handler: (event) => {
                globalStore.set(this.watchStatusAtom, event.data);
            },
        });

        // The builder window loads the config once at startup (initBuilder in
        // frontend/remoteterm.ts) and, unlike main windows, never runs
        // initGlobalWaveEventSubs; without this the live-rebuild toggle could not change.
        this.configUnsubFn = waveEventSubscribeSingle({
            eventType: "config",
            handler: (event) => {
                globalStore.set(atoms.fullConfigAtom, event.data.fullconfig);
            },
        });

        await this.watchApp(builderId, appId);
        // The server drops watch status and rebuilds when the watcher's app id is not the
        // window's current one, so a changed app id needs a fresh watch.
        this.appIdUnsubFn = globalStore.sub(atoms.builderAppId, () => {
            const newAppId = globalStore.get(atoms.builderAppId);
            if (newAppId == null) {
                return;
            }
            this.watchApp(builderId, newAppId);
        });
    }

    async watchApp(builderId: string, appId: string) {
        if (this.appGoUpdateUnsubFn) {
            this.appGoUpdateUnsubFn();
        }
        this.appGoUpdateUnsubFn = waveEventSubscribeSingle({
            eventType: "rtapp:appgoupdated",
            scope: appId,
            handler: () => {
                this.handleAppGoUpdated(appId);
            },
        });
        try {
            const watchStatus = await RpcApi.WatchBuilderAppCommand(TabRpcClient, { builderid: builderId });
            globalStore.set(this.watchStatusAtom, watchStatus);
        } catch (err) {
            console.error("Failed to watch the app folder:", err);
            globalStore.set(this.watchStatusAtom, { status: "unavailable", reason: err.message || "unknown error" });
        }
    }

    updateSecretsLatch(status: BuilderStatusData) {
        if (!status?.manifest?.secrets) return;
        const secrets = status.manifest.secrets;
        if (Object.keys(secrets).length > 0) {
            globalStore.set(this.hasSecretsAtom, true);
        }
    }

    updateSecretBindings(newBindings: { [key: string]: string }) {
        const currentStatus = globalStore.get(this.builderStatusAtom);
        if (currentStatus) {
            globalStore.set(this.builderStatusAtom, {
                ...currentStatus,
                secretbindings: newBindings,
            });
        }
    }

    async loadAppDir() {
        const builderId = globalStore.get(atoms.builderId);
        try {
            const appDir = await RpcApi.GetBuilderAppDirCommand(TabRpcClient, { builderid: builderId });
            globalStore.set(this.appDirAtom, appDir);
        } catch (err) {
            console.error("Failed to resolve the app folder:", err);
        }
    }

    async openTerminal() {
        const err = await getApi().openBuilderTerminal();
        globalStore.set(this.noticeAtom, err ?? "");
    }

    async openFolder() {
        const err = await getApi().openBuilderFolder();
        globalStore.set(this.noticeAtom, err ?? "");
    }

    clearNotice() {
        globalStore.set(this.noticeAtom, "");
    }

    async loadEnvVars(builderId: string) {
        try {
            const rtInfo = await RpcApi.GetRTInfoCommand(TabRpcClient, {
                oref: WOS.makeORef("builder", builderId),
            });
            const envVars = rtInfo?.["builder:env"] || {};
            const envVarsArray = Object.entries(envVars).map(([name, value]) => ({ name, value, visible: false }));
            globalStore.set(this.envVarsArrayAtom, envVarsArray);
            globalStore.set(this.envVarsDirtyAtom, false);
        } catch (err) {
            console.error("Failed to load environment variables:", err);
        }
    }

    async saveEnvVars(builderId: string) {
        try {
            const envVarsArray = globalStore.get(this.envVarsArrayAtom);
            const envVars: Record<string, string> = {};
            envVarsArray.forEach((v) => {
                const trimmedName = v.name.trim();
                if (trimmedName) {
                    envVars[trimmedName] = v.value;
                }
            });
            const cleanedArray = Object.entries(envVars).map(([name, value]) => ({ name, value, visible: false }));
            await RpcApi.SetRTInfoCommand(TabRpcClient, {
                oref: WOS.makeORef("builder", builderId),
                data: {
                    "builder:env": envVars,
                },
            });
            globalStore.set(this.envVarsArrayAtom, cleanedArray);
            globalStore.set(this.envVarsDirtyAtom, false);
            globalStore.set(this.errorAtom, "");
            this.requestRebuild();
        } catch (err) {
            console.error("Failed to save environment variables:", err);
            globalStore.set(this.errorAtom, `Failed to save environment variables: ${err.message || "Unknown error"}`);
        }
    }

    getEnvVarIndexAtom(index: number): Atom<EnvVar | null> {
        if (!this.envVarIndexAtoms[index]) {
            this.envVarIndexAtoms[index] = atom((get) => {
                const array = get(this.envVarsArrayAtom);
                return array[index] ?? null;
            });
        }
        return this.envVarIndexAtoms[index];
    }

    addEnvVar() {
        const current = globalStore.get(this.envVarsArrayAtom);
        globalStore.set(this.envVarsArrayAtom, [...current, { name: "", value: "", visible: false }]);
        globalStore.set(this.envVarsDirtyAtom, true);
    }

    removeEnvVar(index: number) {
        const current = globalStore.get(this.envVarsArrayAtom);
        const newArray = current.filter((_, i) => i !== index);
        globalStore.set(this.envVarsArrayAtom, newArray);
        globalStore.set(this.envVarsDirtyAtom, true);
    }

    setEnvVarAtIndex(index: number, envVar: EnvVar, dirty: boolean) {
        const current = globalStore.get(this.envVarsArrayAtom);
        const newArray = [...current];
        newArray[index] = envVar;
        globalStore.set(this.envVarsArrayAtom, newArray);
        if (dirty) {
            globalStore.set(this.envVarsDirtyAtom, true);
        }
    }

    async requestRebuild() {
        const builderId = globalStore.get(atoms.builderId);
        globalStore.set(this.externalChangeAtom, false);
        try {
            await RpcApi.RequestBuilderRebuildCommand(TabRpcClient, { builderid: builderId });
        } catch (err) {
            console.error("Failed to request a rebuild:", err);
            globalStore.set(this.errorAtom, `Failed to rebuild: ${err.message || "Unknown error"}`);
        }
    }

    async startBuilder() {
        return this.requestRebuild();
    }

    async restartBuilder() {
        // the RPC call that starts the builder actually forces a restart, so this works
        return this.startBuilder();
    }

    async switchBuilderApp() {
        const builderId = globalStore.get(atoms.builderId);
        try {
            await RpcApi.DeleteBuilderCommand(TabRpcClient, builderId);
            await new Promise((resolve) => setTimeout(resolve, 500));
            await RpcApi.SetRTInfoCommand(TabRpcClient, {
                oref: WOS.makeORef("builder", builderId),
                data: { "builder:appid": null },
            });
            getApi().setBuilderWindowAppId(null);
            await new Promise((resolve) => setTimeout(resolve, 100));
            getApi().doRefresh();
        } catch (err) {
            console.error("Failed to switch builder app:", err);
            globalStore.set(this.errorAtom, `Failed to switch builder app: ${err.message || "Unknown error"}`);
        }
    }

    async seedStarterApp() {
        const appId = globalStore.get(atoms.builderAppId);
        if (!appId || globalStore.get(this.isSeedingAtom)) {
            return;
        }
        globalStore.set(this.isSeedingAtom, true);
        let seedError: string = null;
        try {
            await RpcApi.SeedBuilderAppCommand(TabRpcClient, { appid: appId });
        } catch (err) {
            console.error("Failed to create starter app:", err);
            seedError = `Failed to create starter app: ${err.message || "Unknown error"}`;
        }
        // Reload even after a failure so files written before it show up. loadAppFile clears
        // the error, so the seed error is set afterwards. A dirty editor must not be replaced
        // by the starter (or by "" after a failed seed), so it goes through decideReload and
        // ends up as a conflict the user resolves.
        if (globalStore.get(this.codeContentAtom) !== globalStore.get(this.originalContentAtom)) {
            globalStore.set(this.errorAtom, "");
            await this.reconcileWithDisk(appId);
        } else {
            await this.loadAppFile(appId);
        }
        if (seedError != null) {
            globalStore.set(this.errorAtom, seedError);
        }
        globalStore.set(this.isSeedingAtom, false);
    }

    async loadAppFile(appId: string) {
        // A reconcile read that started before this load holds older disk content; bumping
        // the sequence makes it drop its result instead of overwriting the loaded file.
        ++this.reconcileSeq;
        try {
            globalStore.set(this.isLoadingAtom, true);
            globalStore.set(this.errorAtom, "");

            const result = await RpcApi.ReadAppFileCommand(TabRpcClient, {
                appid: appId,
                filename: "app.go",
            });

            if (result.notfound) {
                globalStore.set(this.codeContentAtom, "");
                globalStore.set(this.originalContentAtom, "");
                globalStore.set(this.appGoMissingAtom, true);
            } else {
                globalStore.set(this.appGoMissingAtom, false);
                const decoded = base64ToString(result.data64);
                globalStore.set(this.codeContentAtom, decoded);
                globalStore.set(this.originalContentAtom, decoded);

                if (decoded.trim() !== "") {
                    const currentStatus = globalStore.get(this.builderStatusAtom);
                    if (currentStatus?.status !== "running" && currentStatus?.status !== "building") {
                        await this.startBuilder();
                    }
                }
            }
            globalStore.set(this.diskChangedAtom, null);
        } catch (err) {
            console.error("Failed to load app.go:", err);
            globalStore.set(this.errorAtom, `Failed to load app.go: ${err.message || "Unknown error"}`);
        } finally {
            globalStore.set(this.isLoadingAtom, false);
        }
    }

    async saveAppFile(appId: string) {
        try {
            const content = globalStore.get(this.codeContentAtom);
            const encoded = stringToBase64(content);
            const result = await RpcApi.WriteAppGoFileCommand(TabRpcClient, {
                appid: appId,
                data64: encoded,
                builderid: globalStore.get(atoms.builderId),
            });
            const formattedContent = base64ToString(result.data64);
            // Keystrokes typed while the save was in flight must survive; the formatted text
            // only replaces the editor when it still holds exactly what was sent.
            if (globalStore.get(this.codeContentAtom) === content) {
                globalStore.set(this.codeContentAtom, formattedContent);
            }
            globalStore.set(this.originalContentAtom, formattedContent);
            globalStore.set(this.appGoMissingAtom, false);
            globalStore.set(this.diskChangedAtom, null);
            globalStore.set(this.errorAtom, "");
            this.lastWrittenContent = formattedContent;
        } catch (err) {
            console.error("Failed to save app.go:", err);
            globalStore.set(this.errorAtom, `Failed to save app.go: ${err.message || "Unknown error"}`);
        }
    }

    async handleAppGoUpdated(appId: string) {
        const liveRebuild = globalStore.get(getSettingsKeyAtom("builder:liverebuild")) ?? false;
        if (!liveRebuild) {
            globalStore.set(this.externalChangeAtom, true);
        }
        await this.reconcileWithDisk(appId);
    }

    // A read that finishes after a newer one has started is dropped, so diskChangedAtom
    // always holds the latest disk read and "Load disk version" never loads older content.
    async reconcileWithDisk(appId: string) {
        const seq = ++this.reconcileSeq;
        let disk: string = null;
        try {
            const result = await RpcApi.ReadAppFileCommand(TabRpcClient, { appid: appId, filename: "app.go" });
            disk = result.notfound ? null : base64ToString(result.data64);
        } catch (err) {
            console.error("Failed to read app.go after an outside change:", err);
            return;
        }
        if (seq !== this.reconcileSeq) {
            return;
        }
        const decision = decideReload(
            globalStore.get(this.codeContentAtom),
            globalStore.get(this.originalContentAtom),
            disk,
            this.lastWrittenContent
        );
        this.applyReloadDecision(decision);
    }

    applyReloadDecision(decision: ReloadDecision) {
        if (decision.kind === "missing") {
            globalStore.set(this.appGoMissingAtom, true);
            globalStore.set(this.diskChangedAtom, null);
            return;
        }
        globalStore.set(this.appGoMissingAtom, false);
        if (decision.kind === "none") {
            globalStore.set(this.diskChangedAtom, null);
            return;
        }
        this.lastWrittenContent = null;
        if (decision.kind === "sync-original") {
            globalStore.set(this.originalContentAtom, decision.content);
            globalStore.set(this.diskChangedAtom, null);
            return;
        }
        if (decision.kind === "replace") {
            globalStore.set(this.codeContentAtom, decision.content);
            globalStore.set(this.originalContentAtom, decision.content);
            globalStore.set(this.diskChangedAtom, null);
            return;
        }
        globalStore.set(this.diskChangedAtom, decision.disk);
    }

    loadDiskVersion() {
        const disk = globalStore.get(this.diskChangedAtom);
        if (disk == null) {
            return;
        }
        globalStore.set(this.codeContentAtom, disk);
        globalStore.set(this.originalContentAtom, disk);
        globalStore.set(this.diskChangedAtom, null);
    }

    // The original becomes the disk content so Save stays enabled; saving then overwrites
    // the outside change because the user chose to.
    keepMyEdits() {
        const disk = globalStore.get(this.diskChangedAtom);
        if (disk == null) {
            return;
        }
        globalStore.set(this.originalContentAtom, disk);
        globalStore.set(this.diskChangedAtom, null);
    }

    clearError() {
        globalStore.set(this.errorAtom, "");
    }

    giveFocus() {
        const activeTab = globalStore.get(this.activeTab);
        if (activeTab === "code" && this.monacoEditorRef.current) {
            this.monacoEditorRef.current.focus();
        } else {
            this.focusElemRef.current?.focus();
        }
    }

    setFocusElemRef(ref: HTMLInputElement | null) {
        this.focusElemRef.current = ref;
    }

    setMonacoEditorRef(ref: MonacoTypes.editor.IStandaloneCodeEditor | null) {
        this.monacoEditorRef.current = ref;
    }

    openPreviewDevTools() {
        if (!this.webviewRef.current) return;
        if (this.webviewRef.current.isDevToolsOpened()) {
            this.webviewRef.current.closeDevTools();
        } else {
            this.webviewRef.current.openDevTools();
        }
    }

    dispose() {
        if (this.statusUnsubFn) {
            this.statusUnsubFn();
            this.statusUnsubFn = null;
        }
        if (this.appGoUpdateUnsubFn) {
            this.appGoUpdateUnsubFn();
            this.appGoUpdateUnsubFn = null;
        }
        if (this.watchStatusUnsubFn) {
            this.watchStatusUnsubFn();
            this.watchStatusUnsubFn = null;
        }
        if (this.configUnsubFn) {
            this.configUnsubFn();
            this.configUnsubFn = null;
        }
        if (this.appIdUnsubFn) {
            this.appIdUnsubFn();
            this.appIdUnsubFn = null;
        }
    }
}
