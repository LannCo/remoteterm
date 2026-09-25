// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import backgroundsSchema from "../../../schema/backgrounds.json";
import connectionsSchema from "../../../schema/connections.json";
import settingsSchema from "../../../schema/settings.json";
import widgetsSchema from "../../../schema/widgets.json";

type SchemaInfo = {
    uri: string;
    fileMatch: Array<string>;
    schema: object;
};

const MonacoSchemas: SchemaInfo[] = [
    {
        uri: "remoteterm://schema/settings.json",
        fileMatch: ["*/WAVECONFIGPATH/settings.json"],
        schema: settingsSchema,
    },
    {
        uri: "remoteterm://schema/connections.json",
        fileMatch: ["*/WAVECONFIGPATH/connections.json"],
        schema: connectionsSchema,
    },
    {
        uri: "remoteterm://schema/backgrounds.json",
        fileMatch: ["*/WAVECONFIGPATH/backgrounds.json"],
        schema: backgroundsSchema,
    },
    {
        uri: "remoteterm://schema/widgets.json",
        fileMatch: ["*/WAVECONFIGPATH/widgets.json"],
        schema: widgetsSchema,
    },
];

export { MonacoSchemas };
