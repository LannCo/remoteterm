// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import backgroundsJson from "../../../pkg/rtconfig/defaultconfig/backgrounds.json";
import mimetypesJson from "../../../pkg/rtconfig/defaultconfig/mimetypes.json";
import presetsJson from "../../../pkg/rtconfig/defaultconfig/presets.json";
import settingsJson from "../../../pkg/rtconfig/defaultconfig/settings.json";
import termthemesJson from "../../../pkg/rtconfig/defaultconfig/termthemes.json";
import widgetsJson from "../../../pkg/rtconfig/defaultconfig/widgets.json";

export const DefaultFullConfig: FullConfigType = {
    settings: settingsJson as SettingsType,
    mimetypes: mimetypesJson as unknown as { [key: string]: MimeTypeConfigType },
    defaultwidgets: widgetsJson as unknown as { [key: string]: WidgetConfigType },
    widgets: {},
    presets: presetsJson as unknown as { [key: string]: MetaType },
    termthemes: termthemesJson as unknown as { [key: string]: TermThemeType },
    connections: {},
    bookmarks: {},
    backgrounds: backgroundsJson as { [key: string]: BackgroundConfigType },
    configerrors: [],
};
