// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

// @vitest-environment happy-dom

import { GeneralContent } from "@/app/view/remotetermconfig/generalcontent";
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { atom, createStore, Provider, type PrimitiveAtom } from "jotai";
import { afterEach, describe, expect, it, vi } from "vitest";

const Key = "term:fontsize";
const Label = "Terminal font size";

// Mirrors RemoteTermConfigViewModel.setGeneralSetting: the RPC resolves after `delayMs`, and the
// config watcher's event (which is what moves `settings`) lands after that.
function setup({ delayMs = 200, initial = 12, fail = false } = {}) {
    const store = createStore();
    const settingsAtom = atom({ [Key]: initial }) as PrimitiveAtom<SettingsType>;
    let inFlight = 0;
    let maxInFlight = 0;
    const SetConfigCommand = vi.fn(async (patch: SettingsType) => {
        inFlight++;
        maxInFlight = Math.max(maxInFlight, inFlight);
        await new Promise((r) => setTimeout(r, delayMs));
        inFlight--;
        if (fail) {
            throw new Error("write rejected");
        }
        setTimeout(() => store.set(settingsAtom, { ...store.get(settingsAtom), ...patch }), 10);
    });
    const model: any = {
        settingsAtom,
        generalRawSettingsAtom: atom({ [Key]: initial }),
        generalSearchAtom: atom(Label),
        env: { isWindows: () => false },
        async setGeneralSetting(patch: SettingsType) {
            try {
                await SetConfigCommand(patch);
                return true;
            } catch {
                return false;
            }
        },
    };
    render(
        <Provider store={store}>
            <GeneralContent model={model} />
        </Provider>
    );
    const writes = () => SetConfigCommand.mock.calls.map((c) => c[0][Key]);
    const settled = () => waitFor(() => expect(inFlight).toBe(0), { timeout: 3000 });
    const eventsLanded = () =>
        waitFor(() => expect(store.get(settingsAtom)[Key]).toBe(writes().at(-1) ?? initial), { timeout: 3000 });
    return {
        writes,
        settled,
        eventsLanded,
        maxInFlight: () => maxInFlight,
        stored: () => store.get(settingsAtom)[Key],
        input: () => screen.getByRole("spinbutton") as HTMLInputElement,
        up: () => screen.getByRole("button", { name: `Increase ${Label}` }),
        down: () => screen.getByRole("button", { name: `Decrease ${Label}` }),
    };
}

afterEach(() => {
    cleanup();
});

describe("NumberControl interaction", () => {
    it("typed draft then spin click with a slow round-trip sends one write of draft + step", async () => {
        const user = userEvent.setup();
        const c = setup({ delayMs: 200 });
        await user.clear(c.input());
        await user.type(c.input(), "20");
        await user.click(c.up());
        await c.settled();
        await c.eventsLanded();
        expect(c.writes()).toEqual([21]);
        expect(c.input().value).toBe("21");
        expect(c.stored()).toBe(21);
    });

    it("typed draft then spin-down click with a slow round-trip sends one write of draft - step", async () => {
        const user = userEvent.setup();
        const c = setup({ delayMs: 200 });
        await user.clear(c.input());
        await user.type(c.input(), "20");
        await user.click(c.down());
        await c.settled();
        await c.eventsLanded();
        expect(c.writes()).toEqual([19]);
        expect(c.input().value).toBe("19");
    });

    it("keyboard: typed draft, Tab to the increase button, Enter sends one write of draft + step", async () => {
        const user = userEvent.setup();
        const c = setup({ delayMs: 200 });
        await user.clear(c.input());
        await user.type(c.input(), "20");
        await user.tab();
        expect(document.activeElement).toBe(c.up());
        await user.keyboard("{Enter}");
        await c.settled();
        await c.eventsLanded();
        expect(c.writes()).toEqual([21]);
        expect(c.input().value).toBe("21");
    });

    it("rapid plain spin clicks during a slow round-trip each step from the last shown value", async () => {
        const user = userEvent.setup();
        const c = setup({ delayMs: 200 });
        await user.click(c.up());
        await user.click(c.up());
        await user.click(c.up());
        expect(c.input().value).toBe("15");
        await c.settled();
        await c.eventsLanded();
        expect(c.writes()).toEqual([13, 14, 15]);
        expect(c.input().value).toBe("15");
        expect(c.stored()).toBe(15);
    });

    it("writes from one control are serialised: the next is not sent until the previous resolves", async () => {
        const user = userEvent.setup();
        const c = setup({ delayMs: 100 });
        await user.click(c.up());
        await user.click(c.up());
        await c.settled();
        await c.eventsLanded();
        expect(c.maxInFlight()).toBe(1);
        expect(c.stored()).toBe(14);
    });

    it("blur commits a typed draft once", async () => {
        const user = userEvent.setup();
        const c = setup({ delayMs: 200 });
        await user.clear(c.input());
        await user.type(c.input(), "20");
        act(() => c.input().blur());
        await c.settled();
        await c.eventsLanded();
        expect(c.writes()).toEqual([20]);
        expect(c.input().value).toBe("20");
    });

    it("Enter commits a typed draft once", async () => {
        const user = userEvent.setup();
        const c = setup({ delayMs: 200 });
        await user.clear(c.input());
        await user.type(c.input(), "20{Enter}");
        await c.settled();
        await c.eventsLanded();
        expect(c.writes()).toEqual([20]);
        expect(c.input().value).toBe("20");
    });

    it("a failed write puts the field back to the stored value", async () => {
        const user = userEvent.setup();
        const c = setup({ delayMs: 50, fail: true });
        await user.click(c.up());
        await c.settled();
        await waitFor(() => expect(c.input().value).toBe("12"));
        expect(c.writes()).toEqual([13]);
    });
});
