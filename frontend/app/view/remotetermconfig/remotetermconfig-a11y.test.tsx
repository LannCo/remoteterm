// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { atom, createStore, Provider } from "jotai";
import type { ComponentType } from "react";
import { DndProvider } from "react-dnd";
import { HTML5Backend } from "react-dnd-html5-backend";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import { BackgroundsContent } from "./backgroundscontent";
import { ConnectionsContent } from "./connectionscontent";
import { SecretsContent } from "./secretscontent";
import { WidgetsContent } from "./widgetscontent";

vi.mock("@/app/view/remotetermconfig/remotetermconfig-model", () => ({
    SecretNameRegex: /^[A-Za-z][A-Za-z0-9_]*$/,
}));

// Any *Atom the test doesn't name resolves to atom(null); any other property is a no-op
// method, so each test only spells out the state its markup depends on.
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

function render(Component: ComponentType<{ model: any }>, model: any): string {
    return renderToStaticMarkup(
        <Provider store={createStore()}>
            <Component model={model} />
        </Provider>
    );
}

function escapeRegex(s: string): string {
    return s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

function labelTarget(html: string, labelText: string): string {
    const match = html.match(new RegExp(`<label[^>]*for="([^"]+)"[^>]*>${escapeRegex(labelText)}</label>`));
    expect(match, `no <label for> with text "${labelText}"`).not.toBeNull();
    return match[1];
}

function openingTagWithId(html: string, id: string): string {
    const match = html.match(new RegExp(`<[a-z]+[^>]*\\sid="${escapeRegex(id)}"[^>]*>`));
    expect(match, `no element with id "${id}"`).not.toBeNull();
    return match[0];
}

// #E54D2E (text-error) on the panel's rgb(34,34,34) is 4.12:1, under AA's 4.5:1 for text,
// so error text must be a readable colour; text-error stays on icons and borders.
function expectReadableErrorText(html: string, text: string) {
    const match = html.match(new RegExp(`<[a-z]+([^>]*)>${escapeRegex(text)}<`));
    expect(match, `no element with text "${text}"`).not.toBeNull();
    const cls = match[1].match(/class="([^"]*)"/)?.[1].split(/\s+/) ?? [];
    expect(cls).not.toContain("text-error");
    expect(cls).toContain("text-primary");
}

function elementTextById(html: string, id: string): string {
    const match = html.match(new RegExp(`\\sid="${escapeRegex(id)}"[^>]*>([^<]*)<`));
    return match?.[1] ?? "";
}

const SecretsBase = {
    secretNamesAtom: ["FOO"],
    selectedSecretAtom: null,
    isLoadingAtom: false,
    errorMessageAtom: null,
    storageBackendErrorAtom: null,
    isAddingNewAtom: false,
    newSecretNameAtom: "",
    newSecretValueAtom: "",
    secretValueAtom: "",
    secretShownAtom: false,
};

describe("SecretsContent labels and validation", () => {
    it("associates the add form's Name and Value labels with their fields", () => {
        const html = render(SecretsContent, makeModel({ ...SecretsBase, isAddingNewAtom: true }));
        expect(openingTagWithId(html, labelTarget(html, "Name"))).toMatch(/^<input/);
        expect(openingTagWithId(html, labelTarget(html, "Value"))).toMatch(/^<textarea/);
    });

    it("associates the detail view's Value label with the value textarea", () => {
        const html = render(SecretsContent, makeModel({ ...SecretsBase, selectedSecretAtom: "FOO" }));
        expect(openingTagWithId(html, labelTarget(html, "Value"))).toMatch(/^<textarea/);
    });

    it("marks an invalid secret name with aria-invalid and a linked text explanation", () => {
        const html = render(
            SecretsContent,
            makeModel({ ...SecretsBase, isAddingNewAtom: true, newSecretNameAtom: "1bad-name" })
        );
        const input = openingTagWithId(html, labelTarget(html, "Name"));
        expect(input).toContain('aria-invalid="true"');
        const describedBy = input.match(/aria-describedby="([^"]+)"/)?.[1];
        expect(describedBy).toBeTruthy();
        expect(elementTextById(html, describedBy)).toMatch(/^Invalid name/);
        expect(openingTagWithId(html, describedBy)).not.toMatch(/text-error/);
        expect(openingTagWithId(html, describedBy)).toMatch(/text-primary/);
        expect(openingTagWithId(html, describedBy)).toMatch(/text-caption/);
    });

    it("does not mark a valid or empty secret name as invalid", () => {
        for (const name of ["", "GOOD_NAME"]) {
            const html = render(
                SecretsContent,
                makeModel({ ...SecretsBase, isAddingNewAtom: true, newSecretNameAtom: name })
            );
            const input = openingTagWithId(html, labelTarget(html, "Name"));
            expect(input).not.toContain('aria-invalid="true"');
            const describedBy = input.match(/aria-describedby="([^"]+)"/)?.[1];
            expect(elementTextById(html, describedBy)).toMatch(/^Must start with a letter/);
        }
    });
});

describe("quick-add error messages are announced", () => {
    it("connections quick-add error sits in role=alert", () => {
        const html = render(
            ConnectionsContent,
            makeModel({
                connectionsViewAtom: "hosts",
                connectionsQuickAddOpenAtom: true,
                connectionsSearchAtom: "",
                connectionNamesAtom: [],
                connStatusMapAtom: new Map(),
                connectionsQuickAddValueAtom: "",
                connectionsQuickAddErrorAtom: "CONN-ERR",
            })
        );
        expect(html).toMatch(/<div[^>]*role="alert"[^>]*>(?:(?!<\/div>).)*CONN-ERR/);
        expectReadableErrorText(html, "CONN-ERR");
    });

    it("background add-form error sits in role=alert", () => {
        const html = render(
            BackgroundsContent,
            makeModel({
                backgroundsOrderedAtom: [],
                backgroundsAddOpenAtom: true,
                backgroundsAddNameAtom: "",
                backgroundsAddBgAtom: "",
                backgroundsAddErrorAtom: "BG-ERR",
            })
        );
        expect(html).toMatch(/<div[^>]*role="alert"[^>]*>(?:(?!<\/div>).)*BG-ERR/);
        expectReadableErrorText(html, "BG-ERR");
    });
});

describe("Keychain rows", () => {
    it("do not dim their text with a row-level opacity", () => {
        const html = render(
            ConnectionsContent,
            makeModel({
                connectionsViewAtom: "keychain",
                connectionsSearchAtom: "",
                connectionNamesAtom: [],
                connStatusMapAtom: new Map(),
            })
        );
        const rows = html.match(/<div class="grid grid-cols-\[22px_1\.4fr_80px_1fr_80px_16px\] items-center[^"]*"/g);
        expect(rows).toHaveLength(2);
        for (const row of rows) {
            expect(row).not.toMatch(/opacity-/);
        }
    });
});

describe("Widget order move buttons", () => {
    it("are at least 24x24px (WCAG 2.5.8)", () => {
        const widgets: [string, WidgetConfigType][] = [
            ["w@a", { "display:order": 1, label: "a", icon: "globe" } as WidgetConfigType],
            ["w@b", { "display:order": 2, label: "b", icon: "globe" } as WidgetConfigType],
        ];
        const model = makeModel({ widgetsOrderedAtom: widgets, widgetsPreviewAtom: widgets.map(([, w]) => w) });
        const html = renderToStaticMarkup(
            <DndProvider backend={HTML5Backend}>
                <Provider store={createStore()}>
                    <WidgetsContent model={model} />
                </Provider>
            </DndProvider>
        );
        const buttons = html.match(/<button[^>]*aria-label="Move [^"]+ (up|down)"[^>]*>/g);
        expect(buttons).toHaveLength(4);
        for (const button of buttons) {
            const cls = button.match(/class="([^"]*)"/)[1].split(/\s+/);
            expect(cls, button).toContain("w-6");
            expect(cls, button).toContain("h-6");
        }
    });
});
