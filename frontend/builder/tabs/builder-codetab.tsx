// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { CodeEditor } from "@/app/view/codeeditor/codeeditor";
import { BuilderAppPanelModel } from "@/builder/store/builder-apppanel-model";
import { atoms } from "@/store/global";
import * as keyutil from "@/util/keyutil";
import { cn } from "@/util/util";
import { useAtomValue } from "jotai";
import type * as MonacoTypes from "monaco-editor";
import { memo, useEffect } from "react";

const DiskChangedBar = memo(() => {
    const model = BuilderAppPanelModel.getInstance();
    const diskChanged = useAtomValue(model.diskChangedAtom);
    if (diskChanged == null) {
        return null;
    }
    return (
        <div className="shrink-0 flex items-center gap-3 px-3 py-1.5 bg-warning/10 border-b border-warning/30 text-sm">
            <i className="fa fa-triangle-exclamation text-warning" />
            <span className="flex-1">app.go changed on disk.</span>
            <button
                className="px-2 py-0.5 text-sm bg-accent/80 text-onaccent rounded hover:bg-accent transition-colors cursor-pointer"
                onClick={() => model.loadDiskVersion()}
            >
                Load disk version
            </button>
            <button
                className="px-2 py-0.5 text-sm rounded hover:bg-secondary/10 transition-colors cursor-pointer"
                onClick={() => model.keepMyEdits()}
            >
                Keep my edits
            </button>
        </div>
    );
});

DiskChangedBar.displayName = "DiskChangedBar";

const BuilderCodeTab = memo(() => {
    const model = BuilderAppPanelModel.getInstance();
    const builderAppId = useAtomValue(atoms.builderAppId);
    const codeContent = useAtomValue(model.codeContentAtom);
    const isLoading = useAtomValue(model.isLoadingAtom);
    const error = useAtomValue(model.errorAtom);
    const saveNeeded = useAtomValue(model.saveNeededAtom);
    const activeTab = useAtomValue(model.activeTab);

    useEffect(() => {
        if (activeTab === "code" && model.monacoEditorRef.current) {
            setTimeout(() => {
                model.monacoEditorRef.current?.layout();
            }, 0);
        }
    }, [activeTab, model.monacoEditorRef]);

    const handleCodeChange = (newText: string) => {
        model.setCodeContent(newText);
    };

    const handleEditorMount = (editor: MonacoTypes.editor.IStandaloneCodeEditor, monaco: typeof MonacoTypes) => {
        model.setMonacoEditorRef(editor);
        return () => {
            model.setMonacoEditorRef(null);
        };
    };

    const handleSave = () => {
        if (builderAppId) {
            model.saveAppFile(builderAppId);
        }
    };

    const handleKeyDown = keyutil.keydownWrapper((waveEvent: WaveKeyboardEvent) => {
        if (keyutil.checkKeyPressed(waveEvent, "Cmd:s")) {
            handleSave();
            return true;
        }
        return false;
    });

    if (!builderAppId) {
        return (
            <div className="w-full h-full flex items-center justify-center">
                <div className="text-secondary">No builder app selected</div>
            </div>
        );
    }

    if (isLoading) {
        return (
            <div className="w-full h-full flex items-center justify-center">
                <div className="text-secondary">Loading app.go...</div>
            </div>
        );
    }

    if (error) {
        return (
            <div className="w-full h-full flex items-center justify-center">
                <div className="text-red-500">{error}</div>
            </div>
        );
    }

    return (
        <div className="w-full h-full flex flex-col" onKeyDown={handleKeyDown}>
            <DiskChangedBar />
            <div className="shrink-0 flex justify-end px-3 py-1">
                <button
                    className={cn(
                        "px-3 py-1 text-sm font-medium rounded transition-colors",
                        saveNeeded
                            ? "bg-accent/80 text-onaccent hover:bg-accent cursor-pointer"
                            : "bg-gray-600 text-gray-400 cursor-default"
                    )}
                    onClick={saveNeeded ? handleSave : undefined}
                >
                    Save
                </button>
            </div>
            <div className="flex-1 min-h-0">
                <CodeEditor
                    blockId={builderAppId}
                    text={codeContent}
                    readonly={false}
                    language="go"
                    fileName="app.go"
                    onChange={handleCodeChange}
                    onMount={handleEditorMount}
                />
            </div>
        </div>
    );
});

BuilderCodeTab.displayName = "BuilderCodeTab";

export { BuilderCodeTab };
