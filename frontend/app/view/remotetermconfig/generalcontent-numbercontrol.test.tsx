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
// config watcher's event (which is what moves `settings`) lands after that, re-reading the whole
// file as it is at that moment (pkg/rtconfig/filewatcher.go, ReadFullConfig).
function setup({
    delayMs = 200,
    initial = 12 as number,
    fail = false,
    key = Key as string,
    label = Label,
    eventLagMs = 10,
} = {}) {
    const store = createStore();
    const file: SettingsType = { [key]: initial };
    const settingsAtom = atom({ ...file }) as PrimitiveAtom<SettingsType>;
    const watcherEvent = () => setTimeout(() => store.set(settingsAtom, { ...file }), eventLagMs);
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
        Object.assign(file, patch);
        watcherEvent();
    });
    const model: any = {
        settingsAtom,
        generalRawSettingsAtom: atom((get) => get(settingsAtom)),
        generalSearchAtom: atom(label),
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
    const writes = () => SetConfigCommand.mock.calls.map((c) => c[0][key]);
    const settled = () => waitFor(() => expect(inFlight).toBe(0), { timeout: 3000 });
    const eventsLanded = () =>
        waitFor(() => expect(store.get(settingsAtom)[key]).toBe(writes().at(-1) ?? initial), { timeout: 3000 });
    return {
        writes,
        settled,
        eventsLanded,
        maxInFlight: () => maxInFlight,
        stored: () => store.get(settingsAtom)[key],
        // Resolves when the most recent SetConfigCommand RPC has, before its watcher event lands.
        lastRpc: () => act(() => SetConfigCommand.mock.results.at(-1).value),
        // Another writer (Reset elsewhere, another window, `wsh setconfig`) changing the file.
        external: (value: unknown) => {
            file[key] = value;
            watcherEvent();
        },
        // A watcher event whose read of the file happened earlier, delivered now.
        deliver: (value: unknown) => act(() => store.set(settingsAtom, { ...store.get(settingsAtom), [key]: value })),
        input: () => screen.getByRole("spinbutton") as HTMLInputElement,
        up: () => screen.getByRole("button", { name: `Increase ${label}` }),
        down: () => screen.getByRole("button", { name: `Decrease ${label}` }),
        reset: () => screen.getByRole("button", { name: `Reset ${label} to default` }),
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

    it("the spin buttons are wired to stepping: no draft, one click each way", async () => {
        const user = userEvent.setup();
        const c = setup({ delayMs: 0 });
        await user.click(c.up());
        await c.settled();
        await c.eventsLanded();
        expect(c.writes()).toEqual([13]);
        await user.click(c.down());
        await c.settled();
        await c.eventsLanded();
        expect(c.writes()).toEqual([13, 12]);
        expect(c.input().value).toBe("12");
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

    it("an external change landing after the write is acknowledged, but before its echo, is shown", async () => {
        const user = userEvent.setup();
        const c = setup({ delayMs: 30 });
        await user.click(c.up());
        await c.lastRpc();
        c.external(30);
        await waitFor(() => expect(c.stored()).toBe(30));
        await waitFor(() => expect(c.input().value).toBe("30"));
        expect(c.writes()).toEqual([13]);
        await user.click(c.up());
        await c.settled();
        expect(c.writes()).toEqual([13, 31]);
    });

    it("an external change landing while this control's write is still in flight does not replace the shown value", async () => {
        const user = userEvent.setup();
        const c = setup({ delayMs: 150 });
        await user.click(c.up());
        c.external(30);
        await waitFor(() => expect(c.stored()).toBe(30));
        expect(c.input().value).toBe("13");
        await c.settled();
        await c.eventsLanded();
        expect(c.input().value).toBe("13");
    });

    it("a late echo of an earlier write from this control does not replace the value it wrote since", async () => {
        const user = userEvent.setup();
        const c = setup({ delayMs: 20, eventLagMs: 200 });
        await user.click(c.up());
        await user.click(c.up());
        await c.settled();
        expect(c.writes()).toEqual([13, 14]);
        await c.deliver(13);
        expect(c.input().value).toBe("14");
        await c.eventsLanded();
        expect(c.input().value).toBe("14");
    });
});
