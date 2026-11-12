// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

// Utility to abstract the fetch function so the Electron net module can be used when available.

let net: Pick<Electron.Net, "fetch">;

// The main process registers Electron's net module here. Importing "electron" from this file would pull the
// package, which is not a browser module, into the renderer bundle.
export function setElectronNet(electronNet: Pick<Electron.Net, "fetch">) {
    net = electronNet;
}

export function fetch(input: string | GlobalRequest | URL, init?: RequestInit): Promise<Response> {
    if (net) {
        return net.fetch(input.toString(), init);
    } else {
        return globalThis.fetch(input, init);
    }
}
