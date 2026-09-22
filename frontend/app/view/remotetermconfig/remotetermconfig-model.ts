// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { BlockNodeModel } from "@/app/block/blocktypes";
import { globalStore } from "@/app/store/jotaiStore";
import type { TabModel } from "@/app/store/tab-model";
import { makeORef } from "@/app/store/wos";
import { TabRpcClient } from "@/app/store/wshrpcutil";
import { BackgroundsContent } from "@/app/view/remotetermconfig/backgroundscontent";
import { ConnectionsContent } from "@/app/view/remotetermconfig/connectionscontent";
import { GeneralContent } from "@/app/view/remotetermconfig/generalcontent";
import { SecretsContent } from "@/app/view/remotetermconfig/secretscontent";
import { RemoteTermConfigView } from "@/app/view/remotetermconfig/remotetermconfig";
import type { RemoteTermConfigEnv } from "@/app/view/remotetermconfig/remotetermconfigenv";
import { WidgetsContent } from "@/app/view/remotetermconfig/widgetscontent";
import { shouldIncludeWidgetForWorkspace, sortByDisplayOrder } from "@/app/workspace/widgetfilter";
import { base64ToString, stringToBase64 } from "@/util/util";
import { atom, type Atom, type PrimitiveAtom } from "jotai";
import type * as MonacoTypes from "monaco-editor";
import * as React from "react";

type ValidationResult = { success: true } | { error: string };
type ConfigValidator = (parsed: any) => ValidationResult;

export type ConfigFile = {
    name: string;
    path: string;
    language?: string;
    deprecated?: boolean;
    description?: string;
    docsUrl?: string;
    validator?: ConfigValidator;
    isSecrets?: boolean;
    hasJsonView?: boolean;
    visualComponent?: React.ComponentType<{ model: RemoteTermConfigViewModel }>;
};

export const SecretNameRegex = /^[A-Za-z][A-Za-z0-9_]*$/;

// Mirrors the Go userHostRe in pkg/remote/connutil.go ParseOpts() — keep in sync.
export const ConnectionQuickAddRegex = /^([a-zA-Z0-9][a-zA-Z0-9._@-]*@)?([a-zA-Z0-9][a-zA-Z0-9.-]*)(?::([0-9]+))?$/;

function makeConfigFiles(isWindows: boolean): ConfigFile[] {
    return [
        {
            name: "General",
            path: "settings.json",
            language: "json",
            docsUrl: "https://docs.rterm.dev/config",
            hasJsonView: true,
            visualComponent: GeneralContent,
        },
        {
            name: "Connections",
            path: "connections.json",
            language: "json",
            docsUrl: "https://docs.rterm.dev/connections",
            description: isWindows ? "SSH hosts and WSL distros" : "SSH hosts",
            hasJsonView: true,
            visualComponent: ConnectionsContent,
        },
        {
            name: "Sidebar Widgets",
            path: "widgets.json",
            language: "json",
            docsUrl: "https://docs.rterm.dev/customwidgets",
            hasJsonView: true,
            visualComponent: WidgetsContent,
        },
        {
            name: "Tab Backgrounds",
            path: "backgrounds.json",
            language: "json",
            docsUrl: "https://docs.rterm.dev/tab-backgrounds",
            hasJsonView: true,
            visualComponent: BackgroundsContent,
        },
        {
            name: "Secrets",
            path: "secrets",
            isSecrets: true,
            hasJsonView: false,
            visualComponent: SecretsContent,
        },
    ];
}

const deprecatedConfigFiles: ConfigFile[] = [
    {
        name: "Presets",
        path: "presets.json",
        language: "json",
        deprecated: true,
        hasJsonView: true,
    },
];

export class RemoteTermConfigViewModel implements ViewModel {
    blockId: string;
    viewType = "remotetermconfig";
    viewIcon = atom("gear");
    viewName = atom("RemoteTerm Config");
    viewComponent = RemoteTermConfigView;
    noPadding = atom(true);
    nodeModel: BlockNodeModel;
    tabModel: TabModel;
    env: RemoteTermConfigEnv;

    selectedFileAtom: PrimitiveAtom<ConfigFile>;
    fileContentAtom: PrimitiveAtom<string>;
    originalContentAtom: PrimitiveAtom<string>;
    hasEditedAtom: PrimitiveAtom<boolean>;
    isLoadingAtom: PrimitiveAtom<boolean>;
    isSavingAtom: PrimitiveAtom<boolean>;
    errorMessageAtom: PrimitiveAtom<string>;
    validationErrorAtom: PrimitiveAtom<string>;
    isMenuOpenAtom: PrimitiveAtom<boolean>;
    presetsJsonExistsAtom: PrimitiveAtom<boolean>;
    activeTabAtom: PrimitiveAtom<"visual" | "json">;
    configErrorFilesAtom: Atom<Set<string>>;
    configDir: string;
    saveShortcut: string;
    editorRef: React.RefObject<MonacoTypes.editor.IStandaloneCodeEditor>;

    secretNamesAtom: PrimitiveAtom<string[]>;
    selectedSecretAtom: PrimitiveAtom<string | null>;
    secretValueAtom: PrimitiveAtom<string>;
    secretShownAtom: PrimitiveAtom<boolean>;
    isAddingNewAtom: PrimitiveAtom<boolean>;
    newSecretNameAtom: PrimitiveAtom<string>;
    newSecretValueAtom: PrimitiveAtom<string>;
    storageBackendErrorAtom: PrimitiveAtom<string | null>;
    secretValueRef: HTMLTextAreaElement | null = null;

    connectionsViewAtom: PrimitiveAtom<"hosts" | "keychain">;
    connectionsSearchAtom: PrimitiveAtom<string>;
    connectionsQuickAddOpenAtom: PrimitiveAtom<boolean>;
    connectionsQuickAddValueAtom: PrimitiveAtom<string>;
    connectionsQuickAddErrorAtom: PrimitiveAtom<string | null>;
    connectionNamesAtom: Atom<string[]>;
    connStatusMapAtom: Atom<Map<string, ConnStatus>>;

    widgetsMapAtom: Atom<{ [key: string]: WidgetConfigType }>;
    widgetsOrderedAtom: Atom<[string, WidgetConfigType][]>;
    widgetsPreviewAtom: Atom<WidgetConfigType[]>;
    // Plain instance field, not a Jotai atom: Jotai's async-atom/thenable detection
    // intercepts any atom whose stored value is a Promise, which breaks manual .then()
    // chaining on the value returned by globalStore.get() -- the continuation silently
    // never fires. Nothing reads this reactively (no useAtomValue anywhere), so it never
    // needed to be an atom; it's pure internal write-serialization state.
    widgetsWriteQueue: Promise<void> = Promise.resolve();

    backgroundsMapAtom: Atom<{ [key: string]: BackgroundConfigType }>;
    backgroundsOrderedAtom: Atom<[string, BackgroundConfigType][]>;
    activeTabBackgroundKeyAtom: Atom<string>;
    // Same reasoning as widgetsWriteQueue above -- not a Jotai atom on purpose.
    backgroundsWriteQueue: Promise<void> = Promise.resolve();
    backgroundsAddOpenAtom: PrimitiveAtom<boolean>;
    backgroundsAddNameAtom: PrimitiveAtom<string>;
    backgroundsAddBgAtom: PrimitiveAtom<string>;
    backgroundsAddErrorAtom: PrimitiveAtom<string | null>;

    settingsAtom: Atom<SettingsType>;
    // Parsed from the raw settings.json content already loaded into originalContentAtom for
    // the Raw JSON tab -- this is the *unmerged* on-disk file, needed to tell "user explicitly
    // set this key" apart from "showing the merged-in default" (fullConfigAtom.settings always
    // has defaults merged in, so it can't answer that question by itself).
    generalRawSettingsAtom: Atom<SettingsType>;
    generalSearchAtom: PrimitiveAtom<string>;

    constructor({ blockId, nodeModel, tabModel, waveEnv }: ViewModelInitType) {
        this.blockId = blockId;
        this.nodeModel = nodeModel;
        this.tabModel = tabModel;
        this.env = waveEnv as RemoteTermConfigEnv;
        this.configDir = this.env.electron.getConfigDir();
        const platform = this.env.electron.getPlatform();
        this.saveShortcut = platform === "darwin" ? "Cmd+S" : "Alt+S";

        this.selectedFileAtom = atom(null) as PrimitiveAtom<ConfigFile>;
        this.fileContentAtom = atom("");
        this.originalContentAtom = atom("");
        this.hasEditedAtom = atom(false);
        this.isLoadingAtom = atom(false);
        this.isSavingAtom = atom(false);
        this.errorMessageAtom = atom(null) as PrimitiveAtom<string>;
        this.validationErrorAtom = atom(null) as PrimitiveAtom<string>;
        this.isMenuOpenAtom = atom(false);
        this.presetsJsonExistsAtom = atom(false);
        this.activeTabAtom = atom<"visual" | "json">("visual");
        this.configErrorFilesAtom = atom((get) => {
            const fullConfig = get(this.env.atoms.fullConfigAtom);
            const errorSet = new Set<string>();
            for (const cerr of fullConfig?.configerrors ?? []) {
                errorSet.add(cerr.file);
            }
            return errorSet;
        });
        this.editorRef = React.createRef();

        this.secretNamesAtom = atom<string[]>([]);
        this.selectedSecretAtom = atom<string | null>(null) as PrimitiveAtom<string | null>;
        this.secretValueAtom = atom<string>("");
        this.secretShownAtom = atom<boolean>(false);
        this.isAddingNewAtom = atom<boolean>(false);
        this.newSecretNameAtom = atom<string>("");
        this.newSecretValueAtom = atom<string>("");
        this.storageBackendErrorAtom = atom<string | null>(null) as PrimitiveAtom<string | null>;

        this.connectionsViewAtom = atom<"hosts" | "keychain">("hosts");
        this.connectionsSearchAtom = atom<string>("");
        this.connectionsQuickAddOpenAtom = atom<boolean>(false);
        this.connectionsQuickAddValueAtom = atom<string>("");
        this.connectionsQuickAddErrorAtom = atom<string | null>(null) as PrimitiveAtom<string | null>;
        this.connectionNamesAtom = atom((get) => {
            const fullConfig = get(this.env.atoms.fullConfigAtom);
            return Object.keys(fullConfig?.connections ?? {}).filter((name) => name !== "");
        });
        this.connStatusMapAtom = atom((get) => {
            const statuses = get(this.env.atoms.allConnStatus);
            const map = new Map<string, ConnStatus>();
            for (const status of statuses ?? []) {
                map.set(status.connection, status);
            }
            return map;
        });

        this.widgetsMapAtom = atom((get) => {
            const fullConfig = get(this.env.atoms.fullConfigAtom);
            return fullConfig?.widgets ?? {};
        });
        this.widgetsOrderedAtom = atom((get) => {
            const widgetsMap = get(this.widgetsMapAtom);
            const sorted = sortByDisplayOrder(widgetsMap);
            const keyByWidget = new Map(Object.entries(widgetsMap).map(([key, widget]) => [widget, key] as const));
            return sorted.map((widget) => [keyByWidget.get(widget), widget] as [string, WidgetConfigType]);
        });
        this.widgetsPreviewAtom = atom((get) => {
            const widgetsMap = get(this.widgetsMapAtom);
            const workspaceId = get(this.env.atoms.workspaceId);
            const filtered = Object.fromEntries(
                Object.entries(widgetsMap).filter(([, widget]) => shouldIncludeWidgetForWorkspace(widget, workspaceId))
            );
            return sortByDisplayOrder(filtered);
        });

        this.backgroundsMapAtom = atom((get) => {
            const fullConfig = get(this.env.atoms.fullConfigAtom);
            return fullConfig?.backgrounds ?? {};
        });
        this.backgroundsOrderedAtom = atom((get) => {
            const backgroundsMap = get(this.backgroundsMapAtom);
            const entries = Object.entries(backgroundsMap);
            entries.sort((a, b) => (a[1]["display:order"] ?? 0) - (b[1]["display:order"] ?? 0));
            return entries;
        });
        this.activeTabBackgroundKeyAtom = atom((get) =>
            get(this.env.getTabMetaKeyAtom(this.tabModel.tabId, "tab:background"))
        );
        this.backgroundsAddOpenAtom = atom<boolean>(false);
        this.backgroundsAddNameAtom = atom<string>("");
        this.backgroundsAddBgAtom = atom<string>("");
        this.backgroundsAddErrorAtom = atom<string | null>(null) as PrimitiveAtom<string | null>;

        this.settingsAtom = atom((get) => get(this.env.atoms.fullConfigAtom)?.settings ?? {});
        this.generalRawSettingsAtom = atom((get) => {
            const selectedFile = get(this.selectedFileAtom);
            if (selectedFile?.path !== "settings.json") {
                return {};
            }
            const content = get(this.originalContentAtom);
            try {
                const parsed = JSON.parse(content);
                if (typeof parsed === "object" && parsed != null && !Array.isArray(parsed)) {
                    return parsed as SettingsType;
                }
            } catch {
                // fall through to empty -- an unparsable raw file just means we can't tell
                // which keys are user-set, so every nullable field renders as "not set"
            }
            return {};
        });
        this.generalSearchAtom = atom<string>("");

        this.checkPresetsJsonExists();
        this.initialize();
    }

    async checkPresetsJsonExists() {
        try {
            const fullPath = `${this.configDir}/presets.json`;
            const fileInfo = await this.env.rpc.FileInfoCommand(TabRpcClient, {
                info: { path: fullPath },
            });
            if (!fileInfo.notfound) {
                globalStore.set(this.presetsJsonExistsAtom, true);
            }
        } catch {
            // File doesn't exist
        }
    }

    initialize() {
        const selectedFile = globalStore.get(this.selectedFileAtom);
        if (!selectedFile) {
            const metaFileAtom = this.env.getBlockMetaKeyAtom(this.blockId, "file");
            const savedFilePath = globalStore.get(metaFileAtom);
            const configFiles = this.getConfigFiles();
            const deprecatedConfigFiles = this.getDeprecatedConfigFiles();

            let fileToLoad: ConfigFile | null = null;
            if (savedFilePath) {
                fileToLoad =
                    configFiles.find((f) => f.path === savedFilePath) ||
                    deprecatedConfigFiles.find((f) => f.path === savedFilePath) ||
                    null;
            }

            if (!fileToLoad) {
                fileToLoad = configFiles[0];
            }

            if (fileToLoad) {
                this.loadFile(fileToLoad);
            }
        }
    }

    getConfigFiles(): ConfigFile[] {
        return makeConfigFiles(this.env.isWindows());
    }

    getDeprecatedConfigFiles(): ConfigFile[] {
        const presetsJsonExists = globalStore.get(this.presetsJsonExistsAtom);
        return deprecatedConfigFiles.filter((f) => {
            if (f.path === "presets.json") {
                return presetsJsonExists;
            }
            return true;
        });
    }

    hasChanges(): boolean {
        return globalStore.get(this.hasEditedAtom);
    }

    confirmDiscardChanges(): boolean {
        if (!this.hasChanges()) {
            return true;
        }
        return window.confirm("You have unsaved changes. Discard and continue?");
    }

    discardChanges() {
        const originalContent = globalStore.get(this.originalContentAtom);
        globalStore.set(this.fileContentAtom, originalContent);
        globalStore.set(this.hasEditedAtom, false);
        globalStore.set(this.validationErrorAtom, null);
        globalStore.set(this.errorMessageAtom, null);
    }

    markAsEdited() {
        globalStore.set(this.hasEditedAtom, true);
    }

    async loadFile(file: ConfigFile) {
        globalStore.set(this.isLoadingAtom, true);
        globalStore.set(this.errorMessageAtom, null);
        globalStore.set(this.hasEditedAtom, false);

        if (file.isSecrets) {
            globalStore.set(this.selectedFileAtom, file);
            this.env.rpc.SetMetaCommand(TabRpcClient, {
                oref: makeORef("block", this.blockId),
                meta: { file: file.path },
            });
            globalStore.set(this.isLoadingAtom, false);
            this.checkStorageBackend();
            this.refreshSecrets();
            return;
        }

        try {
            const fullPath = `${this.configDir}/${file.path}`;
            const fileData = await this.env.rpc.FileReadCommand(TabRpcClient, {
                info: { path: fullPath },
            });
            const content = fileData?.data64 ? base64ToString(fileData.data64) : "";
            globalStore.set(this.originalContentAtom, content);
            if (content.trim() === "") {
                globalStore.set(this.fileContentAtom, "{\n\n}");
            } else {
                globalStore.set(this.fileContentAtom, content);
            }
            globalStore.set(this.selectedFileAtom, file);
            this.env.rpc.SetMetaCommand(TabRpcClient, {
                oref: makeORef("block", this.blockId),
                meta: { file: file.path },
            });
        } catch (err) {
            globalStore.set(this.errorMessageAtom, `Failed to load ${file.name}: ${err.message || String(err)}`);
            globalStore.set(this.fileContentAtom, "");
            globalStore.set(this.originalContentAtom, "");
        } finally {
            globalStore.set(this.isLoadingAtom, false);
        }
    }

    async saveFile() {
        const selectedFile = globalStore.get(this.selectedFileAtom);
        if (!selectedFile) return;

        const fileContent = globalStore.get(this.fileContentAtom);

        if (fileContent.trim() === "") {
            globalStore.set(this.isSavingAtom, true);
            globalStore.set(this.errorMessageAtom, null);
            globalStore.set(this.validationErrorAtom, null);

            try {
                const fullPath = `${this.configDir}/${selectedFile.path}`;
                await this.env.rpc.FileWriteCommand(TabRpcClient, {
                    info: { path: fullPath },
                    data64: stringToBase64(""),
                });
                globalStore.set(this.fileContentAtom, "");
                globalStore.set(this.originalContentAtom, "");
                globalStore.set(this.hasEditedAtom, false);
            } catch (err) {
                globalStore.set(
                    this.errorMessageAtom,
                    `Failed to save ${selectedFile.name}: ${err.message || String(err)}`
                );
            } finally {
                globalStore.set(this.isSavingAtom, false);
            }
            return;
        }

        try {
            const parsed = JSON.parse(fileContent);

            if (typeof parsed !== "object" || parsed == null || Array.isArray(parsed)) {
                globalStore.set(this.validationErrorAtom, "JSON must be an object, not an array, primitive, or null");
                return;
            }

            if (selectedFile.validator) {
                const validationResult = selectedFile.validator(parsed);
                if ("error" in validationResult) {
                    globalStore.set(this.validationErrorAtom, validationResult.error);
                    return;
                }
            }

            const formatted = JSON.stringify(parsed, null, 2);

            globalStore.set(this.isSavingAtom, true);
            globalStore.set(this.errorMessageAtom, null);
            globalStore.set(this.validationErrorAtom, null);

            try {
                const fullPath = `${this.configDir}/${selectedFile.path}`;
                await this.env.rpc.FileWriteCommand(TabRpcClient, {
                    info: { path: fullPath },
                    data64: stringToBase64(formatted),
                });
                globalStore.set(this.fileContentAtom, formatted);
                globalStore.set(this.originalContentAtom, formatted);
                globalStore.set(this.hasEditedAtom, false);
            } catch (err) {
                globalStore.set(
                    this.errorMessageAtom,
                    `Failed to save ${selectedFile.name}: ${err.message || String(err)}`
                );
            } finally {
                globalStore.set(this.isSavingAtom, false);
            }
        } catch (err) {
            globalStore.set(this.validationErrorAtom, `Invalid JSON: ${err.message || String(err)}`);
        }
    }

    clearError() {
        globalStore.set(this.errorMessageAtom, null);
    }

    clearValidationError() {
        globalStore.set(this.validationErrorAtom, null);
    }

    async checkStorageBackend() {
        try {
            const backend = await this.env.rpc.GetSecretsLinuxStorageBackendCommand(TabRpcClient);
            if (backend === "basic_text" || backend === "unknown") {
                globalStore.set(
                    this.storageBackendErrorAtom,
                    "No appropriate secret manager found. Cannot manage secrets securely."
                );
            } else {
                globalStore.set(this.storageBackendErrorAtom, null);
            }
        } catch (error) {
            globalStore.set(this.storageBackendErrorAtom, `Error checking storage backend: ${error.message}`);
        }
    }

    async refreshSecrets() {
        globalStore.set(this.isLoadingAtom, true);
        globalStore.set(this.errorMessageAtom, null);

        try {
            const names = await this.env.rpc.GetSecretsNamesCommand(TabRpcClient);
            globalStore.set(this.secretNamesAtom, names || []);
        } catch (error) {
            globalStore.set(this.errorMessageAtom, `Failed to load secrets: ${error.message}`);
        } finally {
            globalStore.set(this.isLoadingAtom, false);
        }
    }

    async viewSecret(name: string) {
        globalStore.set(this.errorMessageAtom, null);
        globalStore.set(this.selectedSecretAtom, name);
        globalStore.set(this.secretShownAtom, false);
        globalStore.set(this.secretValueAtom, "");
    }

    closeSecretView() {
        globalStore.set(this.selectedSecretAtom, null);
        globalStore.set(this.secretValueAtom, "");
        globalStore.set(this.errorMessageAtom, null);
    }

    async showSecret() {
        const selectedSecret = globalStore.get(this.selectedSecretAtom);
        if (!selectedSecret) {
            return;
        }

        globalStore.set(this.isLoadingAtom, true);
        globalStore.set(this.errorMessageAtom, null);

        try {
            const secrets = await this.env.rpc.GetSecretsCommand(TabRpcClient, [selectedSecret]);
            const value = secrets[selectedSecret];
            if (value !== undefined) {
                globalStore.set(this.secretValueAtom, value);
                globalStore.set(this.secretShownAtom, true);
            } else {
                globalStore.set(this.errorMessageAtom, `Secret not found: ${selectedSecret}`);
            }
        } catch (error) {
            globalStore.set(this.errorMessageAtom, `Failed to load secret: ${error.message}`);
        } finally {
            globalStore.set(this.isLoadingAtom, false);
        }
    }

    async saveSecret() {
        const selectedSecret = globalStore.get(this.selectedSecretAtom);
        const secretValue = globalStore.get(this.secretValueAtom);

        if (!selectedSecret) {
            return;
        }

        globalStore.set(this.isLoadingAtom, true);
        globalStore.set(this.errorMessageAtom, null);

        try {
            await this.env.rpc.SetSecretsCommand(TabRpcClient, { [selectedSecret]: secretValue });
            this.closeSecretView();
        } catch (error) {
            globalStore.set(this.errorMessageAtom, `Failed to save secret: ${error.message}`);
        } finally {
            globalStore.set(this.isLoadingAtom, false);
        }
    }

    async deleteSecret() {
        const selectedSecret = globalStore.get(this.selectedSecretAtom);

        if (!selectedSecret) {
            return;
        }

        globalStore.set(this.isLoadingAtom, true);
        globalStore.set(this.errorMessageAtom, null);

        try {
            await this.env.rpc.SetSecretsCommand(TabRpcClient, { [selectedSecret]: null });
            this.closeSecretView();
            await this.refreshSecrets();
        } catch (error) {
            globalStore.set(this.errorMessageAtom, `Failed to delete secret: ${error.message}`);
        } finally {
            globalStore.set(this.isLoadingAtom, false);
        }
    }

    startAddingSecret() {
        globalStore.set(this.isAddingNewAtom, true);
        globalStore.set(this.newSecretNameAtom, "");
        globalStore.set(this.newSecretValueAtom, "");
        globalStore.set(this.errorMessageAtom, null);
    }

    cancelAddingSecret() {
        globalStore.set(this.isAddingNewAtom, false);
        globalStore.set(this.newSecretNameAtom, "");
        globalStore.set(this.newSecretValueAtom, "");
        globalStore.set(this.errorMessageAtom, null);
    }

    async addNewSecret() {
        const name = globalStore.get(this.newSecretNameAtom).trim();
        const value = globalStore.get(this.newSecretValueAtom);

        if (!name) {
            globalStore.set(this.errorMessageAtom, "Secret name cannot be empty");
            return;
        }

        if (!SecretNameRegex.test(name)) {
            globalStore.set(
                this.errorMessageAtom,
                "Invalid secret name: must start with a letter and contain only letters, numbers, and underscores"
            );
            return;
        }

        const existingNames = globalStore.get(this.secretNamesAtom);
        if (existingNames.includes(name)) {
            globalStore.set(this.errorMessageAtom, `Secret "${name}" already exists`);
            return;
        }

        globalStore.set(this.isLoadingAtom, true);
        globalStore.set(this.errorMessageAtom, null);

        try {
            await this.env.rpc.SetSecretsCommand(TabRpcClient, { [name]: value });
            globalStore.set(this.isAddingNewAtom, false);
            globalStore.set(this.newSecretNameAtom, "");
            globalStore.set(this.newSecretValueAtom, "");
            await this.refreshSecrets();
        } catch (error) {
            globalStore.set(this.errorMessageAtom, `Failed to add secret: ${error.message}`);
        } finally {
            globalStore.set(this.isLoadingAtom, false);
        }
    }

    openConnectionQuickAdd() {
        globalStore.set(this.connectionsQuickAddOpenAtom, true);
        globalStore.set(this.connectionsQuickAddValueAtom, "");
        globalStore.set(this.connectionsQuickAddErrorAtom, null);
    }

    closeConnectionQuickAdd() {
        globalStore.set(this.connectionsQuickAddOpenAtom, false);
        globalStore.set(this.connectionsQuickAddValueAtom, "");
        globalStore.set(this.connectionsQuickAddErrorAtom, null);
    }

    async submitConnectionQuickAdd() {
        const value = globalStore.get(this.connectionsQuickAddValueAtom).trim();
        if (!value) {
            return;
        }
        if (!ConnectionQuickAddRegex.test(value)) {
            globalStore.set(this.connectionsQuickAddErrorAtom, "Invalid format: expected user@host or user@host:port");
            return;
        }
        globalStore.set(this.connectionsQuickAddErrorAtom, null);
        try {
            await this.env.rpc.SetConnectionsConfigCommand(TabRpcClient, { host: value, metamaptype: {} });
            globalStore.set(this.connectionsQuickAddValueAtom, "");
        } catch (error) {
            globalStore.set(this.connectionsQuickAddErrorAtom, `Failed to add connection: ${error.message}`);
        }
    }

    // settings.json merges per-top-level-key server-side (SetBaseConfigValue), so a single-key
    // write here is already minimal-diff -- no read-modify-write queue needed the way widgets/
    // backgrounds need one. `null` clears a key back to its default (MetaMapType merge semantics).
    async setGeneralSetting(patch: SettingsType) {
        globalStore.set(this.errorMessageAtom, null);
        try {
            await this.env.rpc.SetConfigCommand(TabRpcClient, patch);
            await this.refreshGeneralRawContent();
        } catch (err) {
            globalStore.set(this.errorMessageAtom, `Failed to save setting: ${err.message || String(err)}`);
        }
    }

    // Keeps originalContentAtom/fileContentAtom (and therefore generalRawSettingsAtom, and the
    // Raw JSON tab) in sync after a visual-tab write, the same way writeWidgetPatch/
    // writeBackgroundPatch do for their own files. Best-effort: setGeneralSetting's RPC already
    // succeeded by the time this runs, so a refresh failure here isn't surfaced as an error.
    async refreshGeneralRawContent() {
        const selectedFile = globalStore.get(this.selectedFileAtom);
        if (selectedFile?.path !== "settings.json") {
            return;
        }
        try {
            const fullPath = `${this.configDir}/settings.json`;
            const fileData = await this.env.rpc.FileReadCommand(TabRpcClient, {
                info: { path: fullPath },
            });
            const content = fileData?.data64 ? base64ToString(fileData.data64) : "";
            const formatted = content.trim() === "" ? "{\n\n}" : content;
            globalStore.set(this.originalContentAtom, formatted);
            globalStore.set(this.fileContentAtom, formatted);
        } catch {
            // ignore -- best-effort refresh only
        }
    }

    // Reads the user's own widgets.json exactly as the Raw JSON tab does (unmerged —
    // no built-in defaultconfig/widgets.json entries). Writes must be computed against
    // this, not against fullConfig.widgets, or every touched edit would silently fork
    // every currently-effective default widget into the user's file.
    //
    // Returns {} for a genuinely-missing file (first-ever widget edit — empty base is
    // correct), or null if the check/read/parse failed for any other reason (RPC
    // hiccup, permissions, malformed existing JSON) — callers must treat null as
    // "abort the write", never as "empty file", or a transient failure on a populated
    // file would silently wipe every other customized widget.
    async readRawWidgetsFile(): Promise<{ [key: string]: WidgetConfigType } | null> {
        const fullPath = `${this.configDir}/widgets.json`;
        try {
            const fileInfo = await this.env.rpc.FileInfoCommand(TabRpcClient, {
                info: { path: fullPath },
            });
            if (fileInfo.notfound) {
                return {};
            }
        } catch (err) {
            globalStore.set(this.errorMessageAtom, `Failed to check widgets.json: ${err.message || String(err)}`);
            return null;
        }

        try {
            const fileData = await this.env.rpc.FileReadCommand(TabRpcClient, {
                info: { path: fullPath },
            });
            const content = fileData?.data64 ? base64ToString(fileData.data64) : "";
            if (content.trim() === "") {
                return {};
            }
            const parsed = JSON.parse(content);
            if (typeof parsed !== "object" || parsed == null || Array.isArray(parsed)) {
                globalStore.set(this.errorMessageAtom, "widgets.json content is not a valid object");
                return null;
            }
            return parsed;
        } catch (err) {
            globalStore.set(this.errorMessageAtom, `Failed to read widgets.json: ${err.message || String(err)}`);
            return null;
        }
    }

    // Merges only the touched widget key(s) into the raw file (each written in full,
    // per Wave's whole-key-replace merge semantics for the widgets map) and leaves
    // every other key exactly as it already is on disk. Writes are serialized through
    // widgetsWriteQueue (a plain field, not an atom -- see its declaration) so a toggle
    // and a drag-drop landing close together can't race: each write's read-then-merge
    // waits for the previous write to finish first.
    async persistWidgetPatch(updates: { [key: string]: WidgetConfigType }) {
        if (Object.keys(updates).length === 0) {
            return;
        }
        const nextWrite = this.widgetsWriteQueue.then(
            () => this.writeWidgetPatch(updates),
            () => this.writeWidgetPatch(updates)
        );
        this.widgetsWriteQueue = nextWrite;
        await nextWrite;
    }

    async writeWidgetPatch(updates: { [key: string]: WidgetConfigType }) {
        globalStore.set(this.errorMessageAtom, null);
        const rawContent = await this.readRawWidgetsFile();
        if (rawContent == null) {
            return;
        }
        try {
            const merged = { ...rawContent, ...updates };
            const fullPath = `${this.configDir}/widgets.json`;
            const formatted = JSON.stringify(merged, null, 2);
            await this.env.rpc.FileWriteCommand(TabRpcClient, {
                info: { path: fullPath },
                data64: stringToBase64(formatted),
            });
            const selectedFile = globalStore.get(this.selectedFileAtom);
            if (selectedFile?.path === "widgets.json") {
                globalStore.set(this.originalContentAtom, formatted);
                globalStore.set(this.fileContentAtom, formatted);
            }
        } catch (err) {
            globalStore.set(this.errorMessageAtom, `Failed to save widgets.json: ${err.message || String(err)}`);
        }
    }

    // Assigns the moved widget a display:order strictly between its new neighbors'
    // existing values (fractional indexing) so a drag only ever touches the one
    // widget that moved — neighbors keep their current order untouched.
    async reorderWidget(movedKey: string, newIndex: number, orderedKeys: string[]) {
        const widgetsMap = globalStore.get(this.widgetsMapAtom);
        const widget = widgetsMap[movedKey];
        if (widget == null) return;

        const prevKey = orderedKeys[newIndex - 1];
        const nextKey = orderedKeys[newIndex + 1];
        const prevOrder = prevKey != null ? (widgetsMap[prevKey]?.["display:order"] ?? 0) : null;
        const nextOrder = nextKey != null ? (widgetsMap[nextKey]?.["display:order"] ?? 0) : null;

        let newOrder: number;
        if (prevOrder != null && nextOrder != null) {
            newOrder = (prevOrder + nextOrder) / 2;
        } else if (prevOrder != null) {
            newOrder = prevOrder + 1;
        } else if (nextOrder != null) {
            newOrder = nextOrder - 1;
        } else {
            newOrder = 0;
        }

        await this.persistWidgetPatch({ [movedKey]: { ...widget, "display:order": newOrder } });
    }

    async toggleWidgetHidden(key: string) {
        const widgetsMap = globalStore.get(this.widgetsMapAtom);
        const widget = widgetsMap[key];
        if (widget == null) return;
        await this.persistWidgetPatch({ [key]: { ...widget, "display:hidden": !widget["display:hidden"] } });
    }

    // Same unmerged-read rationale as readRawWidgetsFile: fullConfig.backgrounds includes
    // shipped defaultconfig presets, so writes must be computed against the user's own
    // (possibly absent) backgrounds.json, or editing a default preset would fork every
    // other default preset into the user's file too.
    async readRawBackgroundsFile(): Promise<{ [key: string]: BackgroundConfigType } | null> {
        const fullPath = `${this.configDir}/backgrounds.json`;
        try {
            const fileInfo = await this.env.rpc.FileInfoCommand(TabRpcClient, {
                info: { path: fullPath },
            });
            if (fileInfo.notfound) {
                return {};
            }
        } catch (err) {
            globalStore.set(this.errorMessageAtom, `Failed to check backgrounds.json: ${err.message || String(err)}`);
            return null;
        }

        try {
            const fileData = await this.env.rpc.FileReadCommand(TabRpcClient, {
                info: { path: fullPath },
            });
            const content = fileData?.data64 ? base64ToString(fileData.data64) : "";
            if (content.trim() === "") {
                return {};
            }
            const parsed = JSON.parse(content);
            if (typeof parsed !== "object" || parsed == null || Array.isArray(parsed)) {
                globalStore.set(this.errorMessageAtom, "backgrounds.json content is not a valid object");
                return null;
            }
            return parsed;
        } catch (err) {
            globalStore.set(this.errorMessageAtom, `Failed to read backgrounds.json: ${err.message || String(err)}`);
            return null;
        }
    }

    async persistBackgroundPatch(updates: { [key: string]: BackgroundConfigType }) {
        if (Object.keys(updates).length === 0) {
            return;
        }
        const nextWrite = this.backgroundsWriteQueue.then(
            () => this.writeBackgroundPatch(updates),
            () => this.writeBackgroundPatch(updates)
        );
        this.backgroundsWriteQueue = nextWrite;
        await nextWrite;
    }

    async writeBackgroundPatch(updates: { [key: string]: BackgroundConfigType }) {
        globalStore.set(this.errorMessageAtom, null);
        const rawContent = await this.readRawBackgroundsFile();
        if (rawContent == null) {
            return;
        }
        try {
            const merged = { ...rawContent, ...updates };
            const fullPath = `${this.configDir}/backgrounds.json`;
            const formatted = JSON.stringify(merged, null, 2);
            await this.env.rpc.FileWriteCommand(TabRpcClient, {
                info: { path: fullPath },
                data64: stringToBase64(formatted),
            });
            const selectedFile = globalStore.get(this.selectedFileAtom);
            if (selectedFile?.path === "backgrounds.json") {
                globalStore.set(this.originalContentAtom, formatted);
                globalStore.set(this.fileContentAtom, formatted);
            }
        } catch (err) {
            globalStore.set(this.errorMessageAtom, `Failed to save backgrounds.json: ${err.message || String(err)}`);
        }
    }

    async applyBackgroundToTab(key: string | null) {
        const oref = makeORef("tab", this.tabModel.tabId);
        await this.env.rpc.SetMetaCommand(TabRpcClient, {
            oref,
            meta: { "bg:*": true, "tab:background": key },
        });
    }

    async updateBackgroundOpacity(key: string, opacity: number) {
        const backgroundsMap = globalStore.get(this.backgroundsMapAtom);
        const background = backgroundsMap[key];
        if (background == null) return;
        await this.persistBackgroundPatch({ [key]: { ...background, "bg:opacity": opacity } });
    }

    async updateBackgroundBlendMode(key: string, blendMode: string) {
        const backgroundsMap = globalStore.get(this.backgroundsMapAtom);
        const background = backgroundsMap[key];
        if (background == null) return;
        await this.persistBackgroundPatch({ [key]: { ...background, "bg:blendmode": blendMode } });
    }

    async addBackground(displayName: string, bg: string) {
        const name = displayName.trim();
        if (!name) return;
        const backgroundsMap = globalStore.get(this.backgroundsMapAtom);
        const slug = name
            .toLowerCase()
            .replace(/[^a-z0-9]+/g, "-")
            .replace(/(^-|-$)/g, "");
        let key = `bg@${slug || "custom"}`;
        let suffix = 2;
        while (backgroundsMap[key] != null) {
            key = `bg@${slug || "custom"}-${suffix}`;
            suffix++;
        }
        const maxOrder = Object.values(backgroundsMap).reduce(
            (max, entry) => Math.max(max, entry["display:order"] ?? 0),
            0
        );
        // 0.3 matches the shipped defaultconfig presets that (like a hand-typed quick-add)
        // are a single flat CSS value rather than a multi-layer gradient — Rainbow, Green,
        // Blue, and Red in pkg/wconfig/defaultconfig/backgrounds.json all use it, and none
        // of those set bg:blendmode either.
        await this.persistBackgroundPatch({
            [key]: { "display:name": name, bg, "bg:opacity": 0.3, "display:order": maxOrder + 1 },
        });
        return key;
    }

    openBackgroundAdd() {
        globalStore.set(this.backgroundsAddOpenAtom, true);
        globalStore.set(this.backgroundsAddNameAtom, "");
        globalStore.set(this.backgroundsAddBgAtom, "");
        globalStore.set(this.backgroundsAddErrorAtom, null);
    }

    closeBackgroundAdd() {
        globalStore.set(this.backgroundsAddOpenAtom, false);
        globalStore.set(this.backgroundsAddNameAtom, "");
        globalStore.set(this.backgroundsAddBgAtom, "");
        globalStore.set(this.backgroundsAddErrorAtom, null);
    }

    async submitBackgroundAdd() {
        const name = globalStore.get(this.backgroundsAddNameAtom).trim();
        const bg = globalStore.get(this.backgroundsAddBgAtom).trim();
        if (!name) {
            globalStore.set(this.backgroundsAddErrorAtom, "Name cannot be empty");
            return;
        }
        if (!bg) {
            globalStore.set(this.backgroundsAddErrorAtom, "CSS background value cannot be empty");
            return;
        }
        globalStore.set(this.backgroundsAddErrorAtom, null);
        const key = await this.addBackground(name, bg);
        this.closeBackgroundAdd();
        if (key) {
            await this.applyBackgroundToTab(key);
        }
    }

    giveFocus(): boolean {
        const selectedFile = globalStore.get(this.selectedFileAtom);
        if (selectedFile?.isSecrets && this.secretValueRef) {
            this.secretValueRef.focus();
            return true;
        }
        if (this.editorRef?.current) {
            this.editorRef.current.focus();
            return true;
        }
        return false;
    }
}
