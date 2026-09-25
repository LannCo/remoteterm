// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

// @vitest-environment happy-dom

import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { atom, createStore, Provider, type PrimitiveAtom } from "jotai";
import { afterEach, expect, it, vi } from "vitest";

vi.mock("@/app/remotetermenv/remotetermenv", () => {
    const env = { atoms: { fullConfigAtom: atom({}) } };
    return { useWaveEnv: () => env };
});
vi.mock("@/app/store/keymodel", () => ({ tryReinjectKey: () => false }));
vi.mock("@/app/view/codeeditor/codeeditor", () => ({ CodeEditor: () => null }));
vi.mock("@/app/view/remotetermconfig/remotetermconfig-model", () => ({}));

afterEach(() => {
    cleanup();
});

function setup() {
    const store = createStore();
    const errorMessageAtom = atom("Failed to save setting: boom") as PrimitiveAtom<string>;
    const validationErrorAtom = atom("Invalid JSON") as PrimitiveAtom<string>;
    const model: any = {
        selectedFileAtom: atom({ name: "General", path: "settings.json", language: "json", hasJsonView: true }),
        fileContentAtom: atom("{}"),
        isLoadingAtom: atom(false),
        isSavingAtom: atom(false),
        errorMessageAtom,
        validationErrorAtom,
        isMenuOpenAtom: atom(false),
        hasEditedAtom: atom(false),
        activeTabAtom: atom("json"),
        configErrorFilesAtom: atom(new Set()),
        nodeModel: { isFocused: atom(false) },
        saveShortcut: "Cmd:s",
        getConfigFiles: () => [],
        getDeprecatedConfigFiles: () => [],
        clearError: () => store.set(errorMessageAtom, null),
        clearValidationError: () => store.set(validationErrorAtom, null),
    };
    return { store, model };
}

it.each([["Dismiss error"], ["Dismiss validation error"]])(
    "%s from the keyboard leaves focus in the config view, not on <body>",
    async (name) => {
        const { RemoteTermConfigView } = await import("./remotetermconfig");
        const { store, model } = setup();
        render(
            <Provider store={store}>
                <RemoteTermConfigView blockId="b1" model={model} {...({} as any)} />
            </Provider>
        );
        const user = userEvent.setup();
        const button = screen.getByRole("button", { name });
        button.focus();
        await user.keyboard("{Enter}");

        expect(screen.queryByRole("button", { name })).toBeNull();
        expect(document.activeElement).not.toBe(document.body);
        expect(document.activeElement.contains(screen.getByText("General"))).toBe(true);
    }
);
