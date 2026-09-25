// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { SecretNameRegex, type RemoteTermConfigViewModel } from "@/app/view/remotetermconfig/remotetermconfig-model";
import { cn } from "@/util/util";
import { useAtomValue, useSetAtom } from "jotai";
import { memo, useCallback, useId, useMemo } from "react";

interface ErrorDisplayProps {
    message: string;
    variant?: "error" | "warning";
}

const ErrorDisplay = memo(({ message, variant = "error" }: ErrorDisplayProps) => {
    const icon = variant === "error" ? "fa-circle-exclamation" : "fa-triangle-exclamation";
    const variantClasses =
        variant === "error" ? "bg-error/10 border-error/20 text-error" : "bg-warning/10 border-warning/20 text-warning";

    return (
        <div className={cn("flex items-center gap-2 px-3 py-2.5 border rounded-md text-xs", variantClasses)}>
            <i aria-hidden="true" className={`fa-sharp fa-solid ${icon}`} />
            <span className={variant === "error" ? "text-primary" : undefined}>{message}</span>
        </div>
    );
});
ErrorDisplay.displayName = "ErrorDisplay";

const LoadingSpinner = memo(({ message }: { message: string }) => {
    return (
        <div className="flex flex-col items-center justify-center gap-3 py-12">
            <i aria-hidden="true" className="fa-sharp fa-solid fa-spinner fa-spin text-2xl text-muted" />
            <span className="text-muted text-sm">{message}</span>
        </div>
    );
});
LoadingSpinner.displayName = "LoadingSpinner";

const EmptyState = memo(({ onAddSecret }: { onAddSecret: () => void }) => {
    return (
        <div className="flex flex-col items-center justify-center gap-3 h-full text-center">
            <i aria-hidden="true" className="fa-sharp fa-solid fa-key text-4xl text-muted" />
            <h3 className="text-sm font-semibold text-secondary">No Secrets</h3>
            <p className="text-xs text-muted">Add a secret to get started</p>
            <button
                className="flex items-center gap-2 mt-1 px-3 py-1.5 text-xs rounded bg-accent/80 text-background hover:bg-accent transition-colors cursor-pointer"
                onClick={onAddSecret}
            >
                <i aria-hidden="true" className="fa-sharp fa-solid fa-plus" />
                <span className="font-medium">Add New Secret</span>
            </button>
        </div>
    );
});
EmptyState.displayName = "EmptyState";

interface SecretListPanelProps {
    secretNames: string[];
    selectedSecret: string | null;
    onSelectSecret: (name: string) => void;
    onAddSecret: () => void;
}

const SecretListPanel = memo(({ secretNames, selectedSecret, onSelectSecret, onAddSecret }: SecretListPanelProps) => {
    return (
        <div className="w-[220px] @max-w450:w-[170px] shrink-0 flex flex-col gap-1.5 overflow-y-auto">
            <div className="text-caption font-semibold uppercase tracking-wide text-muted px-1 pb-0.5">
                Stored secrets
            </div>
            {secretNames.map((name) => (
                <button
                    key={name}
                    type="button"
                    className={cn(
                        "flex items-center justify-between gap-2 px-2.5 py-2 rounded-md border font-mono text-xs text-left cursor-pointer transition-colors",
                        name === selectedSecret
                            ? "bg-accentbg border-transparent text-primary"
                            : "bg-panel border-transparent text-secondary hover:border-border/60"
                    )}
                    onClick={() => onSelectSecret(name)}
                >
                    <span className="truncate">{name}</span>
                    <i aria-hidden="true" className="fa-sharp fa-solid fa-chevron-right text-xxs text-muted shrink-0" />
                </button>
            ))}
            <button
                type="button"
                className="flex items-center gap-2 mt-0.5 px-2.5 py-2 text-xs text-muted border border-dashed border-border rounded-md hover:text-secondary transition-colors cursor-pointer"
                onClick={onAddSecret}
            >
                <i aria-hidden="true" className="fa-sharp fa-solid fa-plus" />
                New secret
            </button>
        </div>
    );
});
SecretListPanel.displayName = "SecretListPanel";

const SelectSecretPlaceholder = memo(() => {
    return (
        <div className="flex flex-col items-center justify-center gap-2 h-full text-center">
            <i aria-hidden="true" className="fa-sharp fa-solid fa-key text-3xl text-muted" />
            <div className="text-secondary text-sm">Select a secret to view its details</div>
        </div>
    );
});
SelectSecretPlaceholder.displayName = "SelectSecretPlaceholder";

interface AddSecretFormProps {
    newSecretName: string;
    newSecretValue: string;
    isLoading: boolean;
    onNameChange: (name: string) => void;
    onValueChange: (value: string) => void;
    onCancel: () => void;
    onSubmit: () => void;
}

const AddSecretForm = memo(
    ({
        newSecretName,
        newSecretValue,
        isLoading,
        onNameChange,
        onValueChange,
        onCancel,
        onSubmit,
    }: AddSecretFormProps) => {
        const isNameInvalid = newSecretName !== "" && !SecretNameRegex.test(newSecretName);
        const nameId = useId();
        const nameHintId = useId();
        const valueId = useId();

        return (
            <div className="flex flex-col gap-3.5 h-full min-h-0">
                <h3 className="text-sm font-semibold">Add New Secret</h3>
                <div className="flex flex-col gap-1.5">
                    <label htmlFor={nameId} className="text-caption text-muted">
                        Name
                    </label>
                    <input
                        id={nameId}
                        type="text"
                        autoFocus
                        aria-invalid={isNameInvalid || undefined}
                        aria-describedby={nameHintId}
                        className={cn(
                            "px-2.5 py-1.5 bg-black/20 border rounded-md focus:outline-none font-mono text-xs",
                            isNameInvalid ? "border-error focus:border-error" : "border-border focus:border-accent"
                        )}
                        value={newSecretName}
                        onChange={(e) => onNameChange(e.target.value)}
                        placeholder="MY_SECRET_NAME"
                        disabled={isLoading}
                    />
                    <div id={nameHintId} className={`text-caption ${isNameInvalid ? "text-primary" : "text-muted"}`}>
                        {isNameInvalid && "Invalid name. "}
                        Must start with a letter and contain only letters, numbers, and underscores
                    </div>
                </div>
                <div className="flex flex-col gap-1.5">
                    <label htmlFor={valueId} className="text-caption text-muted">
                        Value
                    </label>
                    <textarea
                        id={valueId}
                        className="w-full px-2.5 py-1.5 bg-black/20 border border-border rounded-md focus:outline-none focus:border-accent font-mono text-xs"
                        value={newSecretValue}
                        onChange={(e) => onValueChange(e.target.value)}
                        placeholder="Enter secret value..."
                        disabled={isLoading}
                        rows={4}
                    />
                </div>
                <div className="flex-1" />
                <div className="flex justify-end gap-2 border-t border-border/60 pt-3">
                    <button
                        className="px-3 py-1.5 text-xs border border-border rounded-md text-secondary hover:text-primary transition-colors cursor-pointer disabled:opacity-50 disabled:cursor-default"
                        onClick={onCancel}
                        disabled={isLoading}
                    >
                        Cancel
                    </button>
                    <button
                        className="flex items-center gap-2 px-3.5 py-1.5 text-xs font-medium rounded bg-accent/80 text-background hover:bg-accent transition-colors cursor-pointer disabled:opacity-50 disabled:cursor-default"
                        onClick={onSubmit}
                        disabled={isLoading || isNameInvalid || newSecretName.trim() === ""}
                    >
                        {isLoading ? (
                            <>
                                <i aria-hidden="true" className="fa-sharp fa-solid fa-spinner fa-spin" />
                                Adding...
                            </>
                        ) : (
                            "Add Secret"
                        )}
                    </button>
                </div>
            </div>
        );
    }
);
AddSecretForm.displayName = "AddSecretForm";

interface SecretDetailViewProps {
    model: RemoteTermConfigViewModel;
}

const SecretDetailView = memo(({ model }: SecretDetailViewProps) => {
    const secretName = useAtomValue(model.selectedSecretAtom);
    const secretValue = useAtomValue(model.secretValueAtom);
    const secretShown = useAtomValue(model.secretShownAtom);
    const isLoading = useAtomValue(model.isLoadingAtom);
    const setSecretValue = useSetAtom(model.secretValueAtom);
    const valueId = useId();
    // A stable callback ref runs once per mount; an inline ref gets a new identity on every
    // render, so React re-invokes it and focus() would yank focus back from Reveal/Save/Delete.
    const valueRef = useCallback(
        (ref: HTMLTextAreaElement | null) => {
            model.secretValueRef = ref;
            ref?.focus();
        },
        [model]
    );

    if (!secretName) {
        return null;
    }

    const revealAnnouncement = secretShown ? `${secretName} value revealed` : "";

    return (
        <div className="flex flex-col gap-3.5 h-full min-h-0">
            <div>
                <label className="text-caption text-muted block mb-1">Name</label>
                <div className="font-mono text-sm font-semibold">{secretName}</div>
            </div>
            <div className="flex flex-col gap-1.5">
                <label htmlFor={valueId} className="text-caption text-muted">
                    Value
                </label>
                <textarea
                    id={valueId}
                    ref={valueRef}
                    className="w-full px-2.5 py-1.5 bg-black/20 border border-border rounded-md focus:outline-none focus:border-accent font-mono text-xs"
                    value={secretValue}
                    onChange={(e) => setSecretValue(e.target.value)}
                    onKeyDown={(e) => {
                        if (e.key === "Escape") {
                            model.closeSecretView();
                        }
                    }}
                    disabled={isLoading}
                    rows={6}
                    placeholder={!secretShown ? "Enter new secret value..." : ""}
                />
                <div aria-live="polite" className="sr-only">
                    {revealAnnouncement}
                </div>
                {!secretShown &&
                    (isLoading ? (
                        <div className="text-caption text-muted">
                            <i aria-hidden="true" className="fa-sharp fa-solid fa-spinner fa-spin" /> Loading...
                        </div>
                    ) : (
                        <button
                            className="flex items-center gap-1.5 self-start px-2.5 py-1 text-xs border border-border rounded-md text-secondary hover:text-primary transition-colors cursor-pointer disabled:opacity-50 disabled:cursor-default"
                            onClick={() => model.showSecret()}
                            disabled={isLoading}
                        >
                            <i aria-hidden="true" className="fa-sharp fa-solid fa-eye" />
                            Reveal
                        </button>
                    ))}
            </div>
            <div className="flex items-center gap-2 px-3 py-2 bg-accent/5 border border-accent/25 rounded-md max-w-[420px]">
                <i aria-hidden="true" className="fa-sharp fa-solid fa-lock text-accent" />
                <span className="text-caption text-secondary">
                    Stored in your OS keychain — CLI access via{" "}
                    <code className="font-mono text-primary">wsh secret get {secretName}</code>
                </span>
            </div>
            <div className="flex-1" />
            <div className="flex justify-between border-t border-border/60 pt-3">
                <button
                    className="flex items-center gap-2 px-3 py-1.5 text-xs border border-error/40 rounded-md text-error hover:bg-error/10 transition-colors cursor-pointer disabled:opacity-50 disabled:cursor-default"
                    onClick={() => model.deleteSecret()}
                    disabled={isLoading}
                    title="Delete this secret"
                >
                    {isLoading ? (
                        <>
                            <i aria-hidden="true" className="fa-sharp fa-solid fa-spinner fa-spin" />
                            Deleting...
                        </>
                    ) : (
                        <>
                            <i aria-hidden="true" className="fa-sharp fa-solid fa-trash" />
                            Delete secret
                        </>
                    )}
                </button>
                <div className="flex gap-2">
                    <button
                        className="px-3 py-1.5 text-xs border border-border rounded-md text-secondary hover:text-primary transition-colors cursor-pointer disabled:opacity-50 disabled:cursor-default"
                        onClick={() => model.closeSecretView()}
                        disabled={isLoading}
                    >
                        Cancel
                    </button>
                    <button
                        className="flex items-center gap-2 px-3.5 py-1.5 text-xs font-medium rounded bg-accent/80 text-background hover:bg-accent transition-colors cursor-pointer disabled:opacity-50 disabled:cursor-default"
                        onClick={() => model.saveSecret()}
                        disabled={isLoading}
                    >
                        {isLoading ? (
                            <>
                                <i aria-hidden="true" className="fa-sharp fa-solid fa-spinner fa-spin" />
                                Saving...
                            </>
                        ) : (
                            "Save"
                        )}
                    </button>
                </div>
            </div>
        </div>
    );
});
SecretDetailView.displayName = "SecretDetailView";

interface SecretsContentProps {
    model: RemoteTermConfigViewModel;
}

export const SecretsContent = memo(({ model }: SecretsContentProps) => {
    const secretNames = useAtomValue(model.secretNamesAtom);
    const selectedSecret = useAtomValue(model.selectedSecretAtom);
    const isLoading = useAtomValue(model.isLoadingAtom);
    const errorMessage = useAtomValue(model.errorMessageAtom);
    const storageBackendError = useAtomValue(model.storageBackendErrorAtom);
    const isAddingNew = useAtomValue(model.isAddingNewAtom);
    const newSecretName = useAtomValue(model.newSecretNameAtom);
    const newSecretValue = useAtomValue(model.newSecretValueAtom);

    const setNewSecretName = useSetAtom(model.newSecretNameAtom);
    const setNewSecretValue = useSetAtom(model.newSecretValueAtom);

    const sortedSecretNames = useMemo(() => {
        return [...secretNames].sort((a, b) => a.toLowerCase().localeCompare(b.toLowerCase()));
    }, [secretNames]);

    if (storageBackendError) {
        return (
            <div className="w-full h-full p-4">
                <ErrorDisplay message={storageBackendError} variant="warning" />
            </div>
        );
    }

    if (isLoading && secretNames.length === 0 && !selectedSecret) {
        return (
            <div className="w-full h-full p-4">
                <LoadingSpinner message="Loading secrets..." />
            </div>
        );
    }

    if (secretNames.length === 0 && !isAddingNew) {
        return (
            <div className="w-full h-full p-4 flex flex-col gap-4">
                {errorMessage && <ErrorDisplay message={errorMessage} />}
                <div className="flex-1">
                    <EmptyState onAddSecret={() => model.startAddingSecret()} />
                </div>
            </div>
        );
    }

    const renderRightPane = () => {
        if (isAddingNew) {
            return (
                <AddSecretForm
                    newSecretName={newSecretName}
                    newSecretValue={newSecretValue}
                    isLoading={isLoading}
                    onNameChange={setNewSecretName}
                    onValueChange={setNewSecretValue}
                    onCancel={() => model.cancelAddingSecret()}
                    onSubmit={() => model.addNewSecret()}
                />
            );
        }

        if (selectedSecret) {
            return <SecretDetailView key={selectedSecret} model={model} />;
        }

        return <SelectSecretPlaceholder />;
    };

    return (
        <div className="flex flex-col gap-4 w-full h-full p-4 min-h-0">
            {errorMessage && <ErrorDisplay message={errorMessage} />}
            <div className="flex-1 flex gap-4 min-h-0">
                <SecretListPanel
                    secretNames={sortedSecretNames}
                    selectedSecret={selectedSecret}
                    onSelectSecret={(name) => model.viewSecret(name)}
                    onAddSecret={() => model.startAddingSecret()}
                />
                <div className="flex-1 min-h-0">{renderRightPane()}</div>
            </div>
        </div>
    );
});

SecretsContent.displayName = "SecretsContent";
