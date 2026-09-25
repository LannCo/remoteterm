// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import * as electron from "electron";
import * as child_process from "node:child_process";
import * as readline from "readline";
import { WebServerEndpointVarName, WSServerEndpointVarName } from "../frontend/util/endpoints";
import { AuthKey, RemoteTermAuthKeyEnv } from "./authkey";
import { setForceQuit, setUserConfirmedQuit } from "./emain-activity";
import {
    getElectronAppResourcesPath,
    getElectronAppUnpackedBasePath,
    getRemoteTermConfigDir,
    getRemoteTermDataDir,
    getRemoteTermSrvCwd,
    getRemoteTermSrvPath,
    getXdgCurrentDesktop,
    RemoteTermConfigHomeVarName,
    RemoteTermDataHomeVarName,
} from "./emain-platform";
import {
    getElectronExecPath,
    RemoteTermAppElectronExecPath,
    RemoteTermAppPathVarName,
    RemoteTermAppResourcesPathVarName,
} from "./emain-util";


let isRemoteTermSrvDead = false;
let remoteTermSrvProc: child_process.ChildProcessWithoutNullStreams | null = null;
let RemoteTermVersion = "unknown"; // set by WAVESRV-ESTART
let RemoteTermBuildTime = 0; // set by WAVESRV-ESTART

export function getRemoteTermVersion(): { version: string; buildTime: number } {
    return { version: RemoteTermVersion, buildTime: RemoteTermBuildTime };
}

let remoteTermSrvReadyResolve = (value: boolean) => {};
const remoteTermSrvReady: Promise<boolean> = new Promise((resolve, _) => {
    remoteTermSrvReadyResolve = resolve;
});

export function getRemoteTermSrvReady(): Promise<boolean> {
    return remoteTermSrvReady;
}

export function getRemoteTermSrvProc(): child_process.ChildProcessWithoutNullStreams | null {
    return remoteTermSrvProc;
}

export function getIsRemoteTermSrvDead(): boolean {
    return isRemoteTermSrvDead;
}

export function runRemoteTermSrv(handleWSEvent: (evtMsg: WSEventType) => void): Promise<boolean> {
    let pResolve: (value: boolean) => void;
    let pReject: (reason?: any) => void;
    const rtnPromise = new Promise<boolean>((argResolve, argReject) => {
        pResolve = argResolve;
        pReject = argReject;
    });
    const envCopy = { ...process.env };
    const xdgCurrentDesktop = getXdgCurrentDesktop();
    if (xdgCurrentDesktop != null) {
        envCopy["XDG_CURRENT_DESKTOP"] = xdgCurrentDesktop;
    }
    envCopy[RemoteTermAppPathVarName] = getElectronAppUnpackedBasePath();
    envCopy[RemoteTermAppResourcesPathVarName] = getElectronAppResourcesPath();
    envCopy[RemoteTermAppElectronExecPath] = getElectronExecPath();
    envCopy[RemoteTermAuthKeyEnv] = AuthKey;
    envCopy[RemoteTermDataHomeVarName] = getRemoteTermDataDir();
    envCopy[RemoteTermConfigHomeVarName] = getRemoteTermConfigDir();
    const remoteTermSrvCmd = getRemoteTermSrvPath();
    console.log("trying to run local server", remoteTermSrvCmd);
    const proc = child_process.spawn(getRemoteTermSrvPath(), {
        cwd: getRemoteTermSrvCwd(),
        env: envCopy,
    });
    proc.on("exit", (e) => {
        console.log("remotetermsrv exited, shutting down");
        setForceQuit(true);
        isRemoteTermSrvDead = true;
        electron.app.quit();
    });
    proc.on("spawn", (e) => {
        console.log("spawned remotetermsrv");
        remoteTermSrvProc = proc;
        pResolve(true);
    });
    proc.on("error", (e) => {
        console.log("error running remotetermsrv", e);
        pReject(e);
    });
    const rlStdout = readline.createInterface({
        input: proc.stdout,
        terminal: false,
    });
    rlStdout.on("line", (line) => {
        console.log(line);
    });
    const rlStderr = readline.createInterface({
        input: proc.stderr,
        terminal: false,
    });
    rlStderr.on("line", (line) => {
        if (line.includes("WAVESRV-ESTART")) {
            const startParams = /ws:([a-z0-9.:]+) web:([a-z0-9.:]+) version:([a-z0-9.-]+) buildtime:(\d+)/gm.exec(
                line
            );
            if (startParams == null) {
                console.log("error parsing WAVESRV-ESTART line", line);
                setUserConfirmedQuit(true);
                electron.app.quit();
                return;
            }
            process.env[WSServerEndpointVarName] = startParams[1];
            process.env[WebServerEndpointVarName] = startParams[2];
            RemoteTermVersion = startParams[3];
            RemoteTermBuildTime = parseInt(startParams[4]);
            remoteTermSrvReadyResolve(true);
            return;
        }
        if (line.startsWith("WAVESRV-EVENT:")) {
            const evtJson = line.slice("WAVESRV-EVENT:".length);
            try {
                const evtMsg: WSEventType = JSON.parse(evtJson);
                handleWSEvent(evtMsg);
            } catch (e) {
                console.log("error handling WAVESRV-EVENT", e);
            }
            return;
        }
        console.log(line);
    });
    return rtnPromise;
}
