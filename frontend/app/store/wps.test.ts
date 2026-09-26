// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { describe, expect, it } from "vitest";
import { getFileSubject, peekFileSubject } from "./wps";

function makeEventData(overrides: Partial<WSFileEventData> = {}): WSFileEventData {
    return {
        zoneid: "zone",
        filename: "file",
        fileop: "append",
        data64: "",
        ...overrides,
    };
}

describe("wps file subjects", () => {
    it("peekFileSubject returns the same instance as getFileSubject without bumping refCount", () => {
        const subject = getFileSubject("zone1", "file1");
        const peeked = peekFileSubject("zone1", "file1");
        expect(peeked).toBe(subject);

        // Only one owning getFileSubject call was made above, so a single release
        // must be enough to bring refCount to zero. If peekFileSubject had incremented
        // refCount, the entry would survive this release.
        subject.release();
        expect(peekFileSubject("zone1", "file1")).toBeNull();
    });

    it("peekFileSubject returns null for a key nobody has created and does not create an entry", () => {
        expect(peekFileSubject("nozone", "nofile")).toBeNull();

        // A fresh owning call afterwards should start refCount at 1, i.e. the
        // preceding peek did not leave behind a phantom entry.
        const subject = getFileSubject("nozone", "nofile");
        subject.release();
        expect(peekFileSubject("nozone", "nofile")).toBeNull();
    });

    it("keeps the subject alive until every owning caller has released", () => {
        const owner1 = getFileSubject("zoneA", "fileA");
        const owner2 = getFileSubject("zoneA", "fileA");
        expect(owner2).toBe(owner1);

        owner1.release();
        expect(peekFileSubject("zoneA", "fileA")).toBe(owner1);

        owner2.release();
        expect(peekFileSubject("zoneA", "fileA")).toBeNull();
    });

    it("completes the subject and removes it from the map once refCount reaches zero", () => {
        const subject = getFileSubject("zoneB", "fileB");
        let completed = false;
        subject.subscribe({ complete: () => (completed = true) });

        subject.release();

        expect(completed).toBe(true);
        expect(peekFileSubject("zoneB", "fileB")).toBeNull();
    });

    it("stops delivering to an unsubscribed observer even though the shared subject survives via another owner", () => {
        // Mirrors termwrap.ts: initTerminal() subscribes once and dispose() unsubscribes + releases.
        const owner1 = getFileSubject("zoneC", "fileC");
        const received: WSFileEventData[] = [];
        const subscription = owner1.subscribe((data) => received.push(data));

        // A second owning caller (e.g. another live TermWrap) keeps the subject alive.
        const owner2 = getFileSubject("zoneC", "fileC");

        // Equivalent of dispose(): unsubscribe the observer, then release the owning ref.
        subscription.unsubscribe();
        owner1.release();

        // Equivalent of the "blockfile" event handler firing after dispose: a pure
        // lookup-and-publish via peekFileSubject must not resurrect delivery to the
        // disposed observer.
        const stillLive = peekFileSubject("zoneC", "fileC");
        expect(stillLive).not.toBeNull();
        stillLive.next(makeEventData({ zoneid: "zoneC", filename: "fileC" }));

        expect(received).toHaveLength(0);

        owner2.release();
        expect(peekFileSubject("zoneC", "fileC")).toBeNull();
    });
});
