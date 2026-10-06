// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { BuilderNoticeAtom } from "@/app/store/builder-terminal";
import { globalStore } from "@/app/store/jotaiStore";
import { stringToBase64 } from "@/util/util";
import { beforeEach, describe, expect, it, vi } from "vitest";

const rpc = vi.hoisted(() => ({
    ReadAppFileCommand: vi.fn(),
    SeedBuilderAppCommand: vi.fn(),
    WriteAppGoFileCommand: vi.fn(),
    RequestBuilderRebuildCommand: vi.fn(),
}));

vi.mock("@/app/store/wshclientapi", () => ({ RpcApi: rpc }));
vi.mock("@/app/store/wshrpcutil", () => ({ TabRpcClient: {} }));
vi.mock("@/app/store/wps", () => ({ waveEventSubscribeSingle: vi.fn(() => () => {}) }));
vi.mock("@/builder/store/builder-term-model", () => ({
    BuilderTermModel: { getInstance: () => ({ markSwitching: vi.fn() }) },
}));
vi.mock("@/store/global", async () => {
    const { atom } = await import("jotai");
    return {
        atoms: { builderId: atom("builder-1"), builderAppId: atom("draft/app") },
        getApi: vi.fn(),
        getSettingsKeyAtom: vi.fn(() => atom(false)),
        WOS: { makeORef: (otype: string, oid: string) => `${otype}:${oid}` },
    };
});

import { BuilderAppPanelModel, getBuilderPreviewPartition } from "./builder-apppanel-model";

function diskFile(content: string | null) {
    return content == null ? { notfound: true } : { data64: stringToBase64(content) };
}

describe("BuilderAppPanelModel reload glue", () => {
    const model = BuilderAppPanelModel.getInstance();

    function setEditor(editor: string, original: string) {
        globalStore.set(model.codeContentAtom, editor);
        globalStore.set(model.originalContentAtom, original);
    }

    beforeEach(() => {
        vi.clearAllMocks();
        vi.spyOn(console, "error").mockImplementation(() => {});
        setEditor("", "");
        globalStore.set(model.appGoMissingAtom, false);
        globalStore.set(model.diskChangedAtom, null);
        globalStore.set(model.errorAtom, "");
        globalStore.set(model.isSeedingAtom, false);
        model.lastWrittenContent = null;
        rpc.SeedBuilderAppCommand.mockResolvedValue(undefined);
        rpc.RequestBuilderRebuildCommand.mockResolvedValue(undefined);
    });

    it("seeding with a dirty editor keeps the edits and shows the conflict", async () => {
        setEditor("my edits", "");
        rpc.ReadAppFileCommand.mockResolvedValue(diskFile("starter"));
        await model.seedStarterApp();
        expect(globalStore.get(model.codeContentAtom)).toBe("my edits");
        expect(globalStore.get(model.diskChangedAtom)).toBe("starter");
        expect(globalStore.get(model.isSeedingAtom)).toBe(false);
    });

    it("a failed seed with a dirty editor keeps the edits and reports the error", async () => {
        setEditor("my edits", "");
        rpc.SeedBuilderAppCommand.mockRejectedValue(new Error("disk full"));
        rpc.ReadAppFileCommand.mockResolvedValue(diskFile(null));
        await model.seedStarterApp();
        expect(globalStore.get(model.codeContentAtom)).toBe("my edits");
        expect(globalStore.get(model.appGoMissingAtom)).toBe(true);
        expect(globalStore.get(model.errorAtom)).toContain("disk full");
    });

    it("seeding with a clean editor loads the starter", async () => {
        rpc.ReadAppFileCommand.mockResolvedValue(diskFile("starter"));
        await model.seedStarterApp();
        expect(globalStore.get(model.codeContentAtom)).toBe("starter");
        expect(globalStore.get(model.originalContentAtom)).toBe("starter");
        expect(globalStore.get(model.diskChangedAtom)).toBeNull();
    });

    it("clears the conflict bar when the disk reverts to the original", async () => {
        setEditor("mine", "orig");
        rpc.ReadAppFileCommand.mockResolvedValueOnce(diskFile("outside"));
        await model.handleAppGoUpdated("draft/app");
        expect(globalStore.get(model.diskChangedAtom)).toBe("outside");
        rpc.ReadAppFileCommand.mockResolvedValueOnce(diskFile("orig"));
        await model.handleAppGoUpdated("draft/app");
        expect(globalStore.get(model.diskChangedAtom)).toBeNull();
        expect(globalStore.get(model.codeContentAtom)).toBe("mine");
    });

    it("clears the conflict bar and shows missing when the disk file is deleted", async () => {
        setEditor("mine", "orig");
        rpc.ReadAppFileCommand.mockResolvedValueOnce(diskFile("outside"));
        await model.handleAppGoUpdated("draft/app");
        rpc.ReadAppFileCommand.mockResolvedValueOnce(diskFile(null));
        await model.handleAppGoUpdated("draft/app");
        expect(globalStore.get(model.diskChangedAtom)).toBeNull();
        expect(globalStore.get(model.appGoMissingAtom)).toBe(true);
        expect(globalStore.get(model.codeContentAtom)).toBe("mine");
    });

    it("drops a disk read that finishes after a newer one", async () => {
        setEditor("mine", "orig");
        let releaseOld: (v: unknown) => void;
        rpc.ReadAppFileCommand.mockReturnValueOnce(new Promise((resolve) => (releaseOld = resolve)));
        const older = model.handleAppGoUpdated("draft/app");
        rpc.ReadAppFileCommand.mockResolvedValueOnce(diskFile("orig"));
        await model.handleAppGoUpdated("draft/app");
        releaseOld(diskFile("stale"));
        await older;
        expect(globalStore.get(model.diskChangedAtom)).toBeNull();
    });

    it("drops a disk read that started before a load", async () => {
        setEditor("mine", "orig");
        let releaseOld: (v: unknown) => void;
        rpc.ReadAppFileCommand.mockReturnValueOnce(new Promise((resolve) => (releaseOld = resolve)));
        const older = model.handleAppGoUpdated("draft/app");
        rpc.ReadAppFileCommand.mockResolvedValueOnce(diskFile("loaded"));
        await model.loadAppFile("draft/app");
        releaseOld(diskFile("older"));
        await older;
        expect(globalStore.get(model.codeContentAtom)).toBe("loaded");
        expect(globalStore.get(model.originalContentAtom)).toBe("loaded");
        expect(globalStore.get(model.diskChangedAtom)).toBeNull();
    });

    it("loadDiskVersion replaces the editor with the disk content", () => {
        setEditor("mine", "orig");
        globalStore.set(model.diskChangedAtom, "outside");
        model.loadDiskVersion();
        expect(globalStore.get(model.codeContentAtom)).toBe("outside");
        expect(globalStore.get(model.originalContentAtom)).toBe("outside");
        expect(globalStore.get(model.diskChangedAtom)).toBeNull();
    });

    it("keepMyEdits keeps the editor dirty so Save overwrites the disk", () => {
        setEditor("mine", "orig");
        globalStore.set(model.diskChangedAtom, "outside");
        model.keepMyEdits();
        expect(globalStore.get(model.codeContentAtom)).toBe("mine");
        expect(globalStore.get(model.originalContentAtom)).toBe("outside");
        expect(globalStore.get(model.diskChangedAtom)).toBeNull();
        expect(globalStore.get(model.saveNeededAtom)).toBe(true);
    });

    it("keeps keystrokes typed while a save is in flight", async () => {
        setEditor("v1", "orig");
        let finishSave: (v: unknown) => void;
        rpc.WriteAppGoFileCommand.mockReturnValueOnce(new Promise((resolve) => (finishSave = resolve)));
        const saving = model.saveAppFile("draft/app");
        globalStore.set(model.codeContentAtom, "v1 plus typing");
        finishSave({ data64: stringToBase64("v1 formatted") });
        await saving;
        expect(globalStore.get(model.codeContentAtom)).toBe("v1 plus typing");
        expect(globalStore.get(model.originalContentAtom)).toBe("v1 formatted");
        expect(globalStore.get(model.saveNeededAtom)).toBe(true);
    });

    it("applies the formatted text when nothing was typed during the save", async () => {
        setEditor("v1", "orig");
        rpc.WriteAppGoFileCommand.mockResolvedValueOnce({ data64: stringToBase64("v1 formatted") });
        await model.saveAppFile("draft/app");
        expect(globalStore.get(model.codeContentAtom)).toBe("v1 formatted");
        expect(globalStore.get(model.saveNeededAtom)).toBe(false);
    });
});

describe("BuilderAppPanelModel auto-run on open", () => {
    const model = BuilderAppPanelModel.getInstance();

    beforeEach(() => {
        vi.clearAllMocks();
        vi.spyOn(console, "error").mockImplementation(() => {});
        globalStore.set(model.codeContentAtom, "");
        globalStore.set(model.originalContentAtom, "");
        globalStore.set(model.errorAtom, "");
        globalStore.set(model.builderStatusAtom, null);
        globalStore.set(model.autoRunDeclinedAtom, false);
        rpc.RequestBuilderRebuildCommand.mockResolvedValue(undefined);
        rpc.ReadAppFileCommand.mockResolvedValue(diskFile("package main"));
    });

    it("loading an app asks for an auto-run, never a plain rebuild", async () => {
        await model.loadAppFile("draft/app");
        expect(rpc.RequestBuilderRebuildCommand).toHaveBeenCalledTimes(1);
        expect(rpc.RequestBuilderRebuildCommand).toHaveBeenCalledWith(expect.anything(), {
            builderid: "builder-1",
            autorun: true,
        });
    });

    it("an empty app.go starts nothing", async () => {
        rpc.ReadAppFileCommand.mockResolvedValue(diskFile("  \n"));
        await model.loadAppFile("draft/app");
        expect(rpc.RequestBuilderRebuildCommand).not.toHaveBeenCalled();
    });

    it("an app that is already running or building is left alone", async () => {
        for (const status of ["running", "building"]) {
            globalStore.set(model.builderStatusAtom, { status } as BuilderStatusData);
            await model.loadAppFile("draft/app");
        }
        expect(rpc.RequestBuilderRebuildCommand).not.toHaveBeenCalled();
    });

    it("a declined auto-run offers the Start button instead of showing an error", async () => {
        rpc.RequestBuilderRebuildCommand.mockRejectedValue(
            new Error("ERR-AUTORUN-DECLINED: the app has changed since you last started it")
        );
        await model.loadAppFile("draft/app");
        expect(globalStore.get(model.autoRunDeclinedAtom)).toBe(true);
        expect(globalStore.get(model.errorAtom)).toBe("");
    });

    it("any other auto-run failure is an error, not a Start prompt", async () => {
        rpc.RequestBuilderRebuildCommand.mockRejectedValue(new Error("builder rtinfo not found"));
        await model.loadAppFile("draft/app");
        expect(globalStore.get(model.autoRunDeclinedAtom)).toBe(false);
        expect(globalStore.get(model.errorAtom)).toContain("builder rtinfo not found");
    });

    it("pressing Start is an explicit request without the auto-run flag, and clears the prompt", async () => {
        globalStore.set(model.autoRunDeclinedAtom, true);
        await model.startBuilder();
        expect(rpc.RequestBuilderRebuildCommand).toHaveBeenCalledWith(expect.anything(), { builderid: "builder-1" });
        expect(globalStore.get(model.autoRunDeclinedAtom)).toBe(false);
    });

    it("a rebuild and a restart are explicit too", async () => {
        await model.requestRebuild();
        await model.restartBuilder();
        for (const call of rpc.RequestBuilderRebuildCommand.mock.calls) {
            expect(call[1]).toEqual({ builderid: "builder-1" });
        }
        expect(rpc.RequestBuilderRebuildCommand).toHaveBeenCalledTimes(2);
    });
});

describe("BuilderAppPanelModel notice", () => {
    it("shares the builder notice atom, so Open terminal errors from any path show in the header", () => {
        expect(BuilderAppPanelModel.getInstance().noticeAtom).toBe(BuilderNoticeAtom);
    });
});

describe("getBuilderPreviewPartition", () => {
    it("gives each builder its own in-memory partition", () => {
        expect(getBuilderPreviewPartition("b1")).toBe("builder-preview-b1");
        expect(getBuilderPreviewPartition("b1").startsWith("persist:")).toBe(false);
        expect(getBuilderPreviewPartition("b2")).not.toBe(getBuilderPreviewPartition("b1"));
    });
});
