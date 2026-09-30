// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { resolvedAppearanceModeAtom } from "@/app/store/appearance-atoms";
import { useAtomValue } from "jotai";
import { mapStatusColour } from "./status-colour";

export function useStatusColour(color: string): string {
    const mode = useAtomValue(resolvedAppearanceModeAtom);
    return mapStatusColour(color, mode);
}
