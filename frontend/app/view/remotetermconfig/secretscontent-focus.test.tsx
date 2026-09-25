// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

// @vitest-environment happy-dom

import { SecretsContent } from "@/app/view/remotetermconfig/secretscontent";
import { act, cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { atom, createStore, Provider } from "jotai";
import { afterEach, expect, it, vi } from "vitest";

function makeModel(values: Record<string, any>): any {
    const cache: Record<string, any> = {};
    return new Proxy(
        {},
        {
            get(_target, key: string) {
                if (key in values) return (cache[key] ??= atom(values[key]));
                if (key.endsWith("Atom")) return (cache[key] ??= atom(null));
                return (cache[key] ??= vi.fn());
            },
        }
    );
}

afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
});

it("secret value textarea takes focus once on mount, not again on re-render", async () => {
    const focusSpy = vi.spyOn(HTMLTextAreaElement.prototype, "focus");
    const store = createStore();
    const model = makeModel({
        secretNamesAtom: ["FOO"],
        selectedSecretAtom: "FOO",
        secretValueAtom: "",
        secretShownAtom: true,
        isLoadingAtom: false,
    });
    render(
        <Provider store={store}>
            <SecretsContent model={model} />
        </Provider>
    );
    const textarea = screen.getByRole("textbox", { name: "Value" });
    expect(document.activeElement).toBe(textarea);
    expect(focusSpy).toHaveBeenCalledTimes(1);

    const user = userEvent.setup();
    await user.type(textarea, "abc");
    const save = screen.getByRole("button", { name: /Save/ });
    act(() => save.focus());

    act(() => store.set(model.secretValueAtom, "abcd"));
    act(() => store.set(model.isLoadingAtom, true));
    act(() => store.set(model.isLoadingAtom, false));

    expect(focusSpy).toHaveBeenCalledTimes(1);
    expect(document.activeElement).toBe(save);
});
