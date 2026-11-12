// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { ipcMain } from "electron";
import { getWebServerEndpoint, getWSServerEndpoint } from "../frontend/util/endpoints";
import { isAppWebContentsId } from "./emain-websecurity";

const AuthKeyHeader = "X-AuthKey";
export const RemoteTermAuthKeyEnv = "REMOTETERM_AUTH_KEY";
export const AuthKey = crypto.randomUUID();

ipcMain.on("get-auth-key", (event) => {
    event.returnValue = isAppWebContentsId(event.sender.id) ? AuthKey : null;
});

// Only requests initiated by app-owned webContents get the key; the default session can also
// carry requests from other initiators, and anything that holds the key has full RPC access.
// A null/undefined webContentsId means the request came from the main process itself (e.g.
// electron.net.request/global fetch used by our own RPC client), never from web content - any
// request a web page can trigger always carries that page's own (non-app) webContents id.
export function configureAuthKeyRequestInjection(session: Electron.Session) {
    const filter: Electron.WebRequestFilter = {
        urls: [`${getWebServerEndpoint()}/*`, `${getWSServerEndpoint()}/*`],
    };
    session.webRequest.onBeforeSendHeaders(filter, (details, callback) => {
        if (details.webContentsId == null || isAppWebContentsId(details.webContentsId)) {
            details.requestHeaders[AuthKeyHeader] = AuthKey;
        }
        callback({ requestHeaders: details.requestHeaders });
    });
}
