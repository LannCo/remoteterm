// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { MessageModal } from "@/app/modals/messagemodal";
import { DeleteFileModal, PublishAppModal, RenameFileModal } from "@/builder/builder-apppanel";
import { SetSecretDialog } from "@/builder/tabs/builder-secrettab";
import { lazy } from "react";
import { AboutModal } from "./about";
import { UserInputPrompt } from "./userinputprompt";

// The onboarding screen runs once per install but pulls in the markdown, syntax
// highlighting and editor libraries, so it loads on first use. Render it inside its own <Suspense>.
// RemoteTerm: upstream upgrade modals (UpgradeOnboardingModal / UpgradeOnboardingPatch)
// are intentionally not registered — they describe Wave's feature history, not the fork's.
export const NewInstallOnboardingModal = lazy(() =>
    import("@/app/onboarding/onboarding").then((m) => ({ default: m.NewInstallOnboardingModal }))
);

const modalRegistry: { [key: string]: React.ComponentType<any> } = {
    NewInstallOnboardingModal,
    [UserInputPrompt.displayName || "UserInputPrompt"]: UserInputPrompt,
    [AboutModal.displayName || "AboutModal"]: AboutModal,
    [MessageModal.displayName || "MessageModal"]: MessageModal,
    [PublishAppModal.displayName || "PublishAppModal"]: PublishAppModal,
    [RenameFileModal.displayName || "RenameFileModal"]: RenameFileModal,
    [DeleteFileModal.displayName || "DeleteFileModal"]: DeleteFileModal,
    [SetSecretDialog.displayName || "SetSecretDialog"]: SetSecretDialog,
};

export const getModalComponent = (key: string): React.ComponentType<any> | undefined => {
    return modalRegistry[key];
};
