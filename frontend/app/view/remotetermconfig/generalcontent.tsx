// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import type { RemoteTermConfigViewModel } from "@/app/view/remotetermconfig/remotetermconfig-model";
import { cn } from "@/util/util";
import { useAtomValue, useSetAtom } from "jotai";
import { memo, useRef, useState } from "react";

type ControlType = "toggle" | "segmented" | "select" | "text" | "number" | "slider";

interface FieldOption {
    value: string;
    label: string;
}

interface FieldSchema {
    key: keyof SettingsType;
    label: string;
    category: string;
    control: ControlType;
    description: string;
    nullable?: boolean;
    options?: FieldOption[];
    unit?: string;
    min?: number;
    max?: number;
    step?: number;
    // Static display string for a nullable field's real runtime default, e.g. "on" or "3px".
    // Omitted (not just falsy) when the real default isn't determinable without guessing --
    // those fields render a bare "Not set" instead of a parenthetical.
    defaultDisplay?: string;
    placeholder?: string;
    // Non-nullable numeric fields whose Go zero-value (0, shown when settings.json has no entry
    // AND defaultconfig/settings.json has no entry either) does not match the value the app
    // actually falls back to at runtime -- shown instead of a misleading "0" when unset. Confirmed
    // against each field's real JS-side fallback, not guessed (term:fontsize ?? 12 in
    // term-model.ts, editor:fontsize ?? 12 in preview-model.tsx, markdown font sizes from
    // theme.scss's --markdown-font-size/--markdown-fixed-font-size).
    zeroFallback?: number;
}

const Categories = ["Appearance", "Terminal", "Editor & Web", "Tabs & Widgets", "Connections", "Advanced"] as const;

// Grouping and control-type mapping follow WAVE_CONFIG_RESKIN_PLAN.md's Task 5 section exactly,
// derived from SettingsType in pkg/rtconfig/settingsconfig.go. `feature:rtappbuilder` has no
// category assignment in the plan's grouping list (only in its raw field dump) -- it's an
// app-level feature toggle with no better home, so it's grouped into Appearance here.
export const FieldSchemas: FieldSchema[] = [
    // --- Appearance ---
    {
        key: "app:tabbar",
        label: "Tab bar position",
        category: "Appearance",
        control: "segmented",
        description: "Shows the tab bar horizontally at the top of the window, or vertically on the left.",
        options: [
            { value: "top", label: "Top" },
            { value: "left", label: "Left" },
        ],
    },
    {
        key: "app:confirmquit",
        label: "Confirm before quitting",
        category: "Appearance",
        control: "toggle",
        description: "Shows a confirmation dialog before quitting RemoteTerm. Requires an app restart.",
        nullable: true,
        defaultDisplay: "on",
    },
    {
        key: "app:globalhotkey",
        label: "Global hotkey",
        category: "Appearance",
        control: "text",
        description: "A systemwide key combination (e.g. Ctrl:Option:e) that opens your most recent RemoteTerm window.",
        placeholder: "e.g. Cmd+Shift+Space",
    },
    {
        key: "app:ctrlvpaste",
        label: "Paste on Ctrl+V select",
        category: "Appearance",
        control: "toggle",
        description:
            "On Windows/Linux, forces Ctrl+V to paste in the terminal. macOS always uses Cmd+V regardless of this setting.",
        nullable: true,
    },
    {
        key: "app:disablectrlshiftarrows",
        label: "Disable Ctrl+Shift+Arrow shortcuts",
        category: "Appearance",
        control: "toggle",
        description: "Disables the Ctrl+Shift block-navigation keybindings (arrow keys and h/j/k/l).",
    },
    {
        key: "app:disablectrlshiftdisplay",
        label: "Disable Ctrl+Shift+Display shortcuts",
        category: "Appearance",
        control: "toggle",
        description: "Disables the visual indicator shown while a Ctrl+Shift block-navigation shortcut is active.",
    },
    {
        key: "app:focusfollowscursor",
        label: "Focus follows cursor",
        category: "Appearance",
        control: "segmented",
        description:
            "Controls whether block focus follows cursor movement: off, all blocks (on), or terminal blocks only (term).",
        options: [
            { value: "off", label: "Off" },
            { value: "on", label: "On" },
            { value: "term", label: "Term" },
        ],
    },
    {
        key: "app:dismissarchitecturewarning",
        label: "Dismiss architecture warning",
        category: "Appearance",
        control: "toggle",
        description:
            "Suppresses the startup warning shown when RemoteTerm is running under architecture translation (e.g. ARM64 emulation).",
    },
    {
        key: "app:showoverlayblocknums",
        label: "Show overlay block numbers",
        category: "Appearance",
        control: "toggle",
        description: "Shows the block-number overlay that appears while holding Ctrl+Shift.",
        nullable: true,
        defaultDisplay: "on",
    },
    {
        key: "app:defaultnewblock",
        label: "Default new block",
        category: "Appearance",
        control: "select",
        description: "Sets which block type Cmd:n / Cmd:d creates by default: terminal or launcher.",
        options: [
            { value: "term", label: "Terminal" },
            { value: "launcher", label: "Launcher" },
        ],
    },
    {
        key: "feature:rtappbuilder",
        label: "RTApp Builder feature",
        category: "Appearance",
        control: "toggle",
        description:
            "Shows the RTApp Builder entry points in the widget bar and app menu. Always shown in dev builds regardless of this setting.",
    },
    {
        key: "window:transparent",
        label: "Window transparency enabled",
        category: "Appearance",
        control: "toggle",
        description:
            "Enables window transparency. Cannot be combined with window background blur. macOS/Windows only, requires app restart.",
    },
    {
        key: "window:blur",
        label: "Window background blur",
        category: "Appearance",
        control: "toggle",
        description:
            "Enables window background blur. Cannot be combined with window transparency. macOS/Windows only, requires app restart.",
    },
    {
        key: "window:opacity",
        label: "Window opacity",
        category: "Appearance",
        control: "slider",
        description: "Sets window opacity when window transparency or window background blur is enabled.",
        nullable: true,
        defaultDisplay: "80%",
    },
    {
        key: "window:bgcolor",
        label: "Window background color",
        category: "Appearance",
        control: "text",
        description: "Sets the window background color as a hex value.",
        placeholder: "e.g. #1e1e1e",
    },
    {
        key: "window:reducedmotion",
        label: "Reduced motion",
        category: "Appearance",
        control: "toggle",
        description: "Disables most UI animations.",
    },
    {
        key: "window:nativetitlebar",
        label: "Use native title bar",
        category: "Appearance",
        control: "toggle",
        description:
            "Uses the OS-native title bar instead of RemoteTerm's overlay. Windows and Linux only, requires app restart.",
    },
    {
        key: "window:showmenubar",
        label: "Show menu bar",
        category: "Appearance",
        control: "toggle",
        description: "Uses the OS-native menu bar. Windows and Linux only, requires app restart.",
    },
    {
        key: "window:zoom",
        label: "Window zoom",
        category: "Appearance",
        control: "number",
        description: "Declared in settings but not currently read anywhere in the app -- has no effect yet.",
        nullable: true,
        unit: "×",
        min: 0.25,
        max: 3,
        step: 0.05,
    },
    {
        key: "window:dimensions",
        label: "Window dimensions on launch",
        category: "Appearance",
        control: "text",
        description: "Sets the default WIDTHxHEIGHT dimensions applied to newly created windows, e.g. 1920x1080.",
        placeholder: "e.g. 1280x800",
    },
    {
        key: "window:fullscreenonlaunch",
        label: "Fullscreen on launch",
        category: "Appearance",
        control: "toggle",
        description: "Launches the foreground window in fullscreen mode.",
    },
    {
        key: "window:savelastwindow",
        label: "Save last window position",
        category: "Appearance",
        control: "toggle",
        description: "Reopens the last-closed window automatically the next time the app launches.",
    },
    {
        key: "window:tilegapsize",
        label: "Split-window gap size",
        category: "Appearance",
        control: "number",
        description: "Sets the gap, in CSS pixels, between split blocks.",
        nullable: true,
        unit: "px",
        min: 0,
        max: 40,
        step: 1,
        defaultDisplay: "3px",
    },
    {
        key: "window:magnifiedblockopacity",
        label: "Magnified block opacity",
        category: "Appearance",
        control: "slider",
        description: "Sets the background opacity behind a magnified block.",
        nullable: true,
        defaultDisplay: "60%",
    },
    {
        key: "window:magnifiedblocksize",
        label: "Magnified block size",
        category: "Appearance",
        control: "slider",
        description: "Sets the size of a magnified block as a percentage of its parent layout's dimensions.",
        nullable: true,
        defaultDisplay: "95%",
    },
    {
        key: "window:magnifiedblockblurprimarypx",
        label: "Magnified block blur (primary)",
        category: "Appearance",
        control: "number",
        description: "Sets the backdrop blur, in CSS pixels, applied directly behind a magnified block.",
        nullable: true,
        unit: "px",
        min: 0,
        max: 50,
        step: 1,
        defaultDisplay: "10px",
    },
    {
        key: "window:magnifiedblockblursecondarypx",
        label: "Magnified block blur (secondary)",
        category: "Appearance",
        control: "number",
        description:
            "Sets the backdrop blur, in CSS pixels, applied to the visible portions of non-magnified blocks while one block is magnified.",
        nullable: true,
        unit: "px",
        min: 0,
        max: 50,
        step: 1,
        defaultDisplay: "2px",
    },
    {
        key: "window:confirmclose",
        label: "Confirm before closing window",
        category: "Appearance",
        control: "toggle",
        description:
            "Shows a confirmation prompt before closing a window that has an unsaved workspace with more than one tab.",
    },
    {
        key: "window:maxtabcachesize",
        label: "Max tab cache size",
        category: "Appearance",
        control: "number",
        description: "Sets the number of tabs kept cached in memory for fast switching between them.",
        unit: "tabs",
        min: 1,
        max: 50,
        step: 1,
    },
    {
        key: "window:disablehardwareacceleration",
        label: "Disable hardware acceleration",
        category: "Appearance",
        control: "toggle",
        description:
            "Disables Chromium hardware acceleration, useful for resolving graphical bugs. Requires app restart.",
    },

    // --- Terminal ---
    {
        key: "term:fontsize",
        label: "Terminal font size",
        category: "Terminal",
        control: "number",
        description: "Sets the terminal block font size.",
        unit: "px",
        min: 6,
        max: 48,
        step: 1,
        zeroFallback: 12,
    },
    {
        key: "term:fontfamily",
        label: "Terminal font family",
        category: "Terminal",
        control: "text",
        description: "Sets the font family used in terminal blocks.",
        placeholder: "e.g. Hack",
    },
    {
        key: "term:theme",
        label: "Terminal theme",
        category: "Terminal",
        control: "text",
        description: "Sets the name of the preset terminal theme applied by default.",
        placeholder: "theme name",
    },
    {
        key: "term:disablewebgl",
        label: "Disable WebGL renderer",
        category: "Terminal",
        control: "toggle",
        description: "Disables the WebGL-accelerated terminal renderer.",
    },
    {
        key: "term:localshellpath",
        label: "Local shell path",
        category: "Terminal",
        control: "text",
        description: "Overrides the default shell path used for local terminals.",
        placeholder: "e.g. /bin/zsh",
    },
    {
        key: "term:gitbashpath",
        label: "Git Bash path (Windows)",
        category: "Terminal",
        control: "text",
        description: "Overrides the auto-detected Git Bash executable path used for local terminals on Windows.",
        placeholder: "e.g. C:\\Program Files\\Git\\bin\\bash.exe",
    },
    {
        key: "term:scrollback",
        label: "Scrollback buffer size",
        category: "Terminal",
        control: "number",
        description: "Sets the terminal scrollback buffer size, in lines.",
        nullable: true,
        unit: "lines",
        min: 0,
        max: 50000,
        step: 100,
        defaultDisplay: "2000 lines",
    },
    {
        key: "term:copyonselect",
        label: "Copy on select",
        category: "Terminal",
        control: "toggle",
        description: "Copies selected terminal text to the clipboard automatically.",
        nullable: true,
        defaultDisplay: "on",
    },
    {
        key: "term:transparency",
        label: "Terminal transparency",
        category: "Terminal",
        control: "slider",
        description: "Sets the terminal background transparency (0 = opaque, 1 = fully transparent).",
        nullable: true,
        defaultDisplay: "50%",
    },
    {
        key: "term:allowbracketedpaste",
        label: "Allow bracketed paste",
        category: "Terminal",
        control: "toggle",
        description: "Enables bracketed paste mode in the terminal.",
        nullable: true,
        defaultDisplay: "on",
    },
    {
        key: "term:shiftenternewline",
        label: "Shift+Enter sends newline",
        category: "Terminal",
        control: "toggle",
        description:
            "Makes Shift+Enter send an escape-sequence newline instead of a carriage return, for AI coding tools like Claude Code.",
        nullable: true,
        defaultDisplay: "on",
    },
    {
        key: "term:macoptionismeta",
        label: "macOS Option key as Meta",
        category: "Terminal",
        control: "toggle",
        description: "On macOS, treats the Option key as Meta for terminal keybindings.",
        nullable: true,
        defaultDisplay: "off",
    },
    {
        key: "term:cursor",
        label: "Cursor style",
        category: "Terminal",
        control: "select",
        description: "Sets the terminal cursor style.",
        options: [
            { value: "block", label: "Block" },
            { value: "bar", label: "Bar" },
            { value: "underline", label: "Underline" },
        ],
    },
    {
        key: "term:cursorblink",
        label: "Cursor blink",
        category: "Terminal",
        control: "toggle",
        description: "Makes the terminal cursor blink.",
        nullable: true,
        defaultDisplay: "off",
    },
    {
        key: "term:bellsound",
        label: "Bell sound",
        category: "Terminal",
        control: "toggle",
        description: "Plays the system beep sound when the terminal receives a bell (BEL) character.",
        nullable: true,
        defaultDisplay: "off",
    },
    {
        key: "term:bellindicator",
        label: "Bell indicator",
        category: "Terminal",
        control: "toggle",
        description: "Shows a visual indicator in the tab when the terminal bell is received.",
        nullable: true,
        defaultDisplay: "on",
    },
    {
        key: "term:osc52",
        label: "OSC52 clipboard access",
        category: "Terminal",
        control: "segmented",
        description:
            "Controls when OSC 52 clipboard writes from the terminal are allowed: always, or only when the window and block are focused.",
        options: [
            { value: "focus", label: "Focus" },
            { value: "always", label: "Always" },
        ],
    },
    {
        key: "term:durable",
        label: "Durable (persistent) sessions",
        category: "Terminal",
        control: "toggle",
        description: "Keeps remote terminal sessions alive across network disconnects.",
        nullable: true,
        defaultDisplay: "off",
    },
    {
        key: "term:showsplitbuttons",
        label: "Show split buttons",
        category: "Terminal",
        control: "toggle",
        description: "Shows split-horizontal and split-vertical buttons in the terminal block header.",
    },
    {
        key: "term:trimtrailingwhitespace",
        label: "Trim trailing whitespace on copy",
        category: "Terminal",
        control: "toggle",
        description: "Trims trailing whitespace from each line when copying terminal text.",
        nullable: true,
        defaultDisplay: "on",
    },

    // --- Editor & Web ---
    {
        key: "editor:minimapenabled",
        label: "Show minimap",
        category: "Editor & Web",
        control: "toggle",
        description: "Shows the code minimap in the editor.",
    },
    {
        key: "editor:stickyscrollenabled",
        label: "Sticky scroll",
        category: "Editor & Web",
        control: "toggle",
        description:
            "Enables Monaco's sticky scroll, pinning the current context's header (e.g. class or method name) at the top.",
    },
    {
        key: "editor:wordwrap",
        label: "Word wrap",
        category: "Editor & Web",
        control: "toggle",
        description: "Enables word wrapping in the editor.",
    },
    {
        key: "editor:fontsize",
        label: "Editor font size",
        category: "Editor & Web",
        control: "number",
        description: "Sets the editor font size.",
        unit: "px",
        min: 6,
        max: 48,
        step: 1,
        zeroFallback: 12,
    },
    {
        key: "editor:inlinediff",
        label: "Inline diff view",
        category: "Editor & Web",
        control: "toggle",
        description: "Shows diffs inline instead of side-by-side.",
    },
    {
        key: "web:openlinksinternally",
        label: "Open links in-app",
        category: "Editor & Web",
        control: "toggle",
        description: "Opens web links inside RemoteTerm's web widget instead of the external browser.",
    },
    {
        key: "web:defaulturl",
        label: "Default web URL",
        category: "Editor & Web",
        control: "text",
        description: "Sets the default homepage loaded in the web widget when no URL is given.",
        placeholder: "https://…",
    },
    {
        key: "web:defaultsearch",
        label: "Default search engine URL",
        category: "Editor & Web",
        control: "text",
        description:
            "Sets the search-engine URL template used for web widget searches; {query} is replaced with the search term.",
        placeholder: "https://…{query}…",
    },
    {
        key: "markdown:fontsize",
        label: "Markdown font size",
        category: "Editor & Web",
        control: "number",
        description: "Sets the body text font size when rendering markdown in preview.",
        unit: "px",
        min: 6,
        max: 48,
        step: 1,
        zeroFallback: 14,
    },
    {
        key: "markdown:fixedfontsize",
        label: "Markdown monospace font size",
        category: "Editor & Web",
        control: "number",
        description: "Sets the code-block font size when rendering markdown in preview.",
        unit: "px",
        min: 6,
        max: 48,
        step: 1,
        zeroFallback: 12,
    },
    {
        key: "preview:showhiddenfiles",
        label: "Show hidden files",
        category: "Editor & Web",
        control: "toggle",
        description: "Shows hidden files in the directory preview.",
        nullable: true,
        defaultDisplay: "on",
    },
    {
        key: "preview:defaultsort",
        label: "Default file sort",
        category: "Editor & Web",
        control: "segmented",
        description:
            "Sets the default sort column for directory preview: name (alphabetical) or modtime (most recently modified first).",
        options: [
            { value: "name", label: "Name" },
            { value: "modtime", label: "Modified" },
        ],
    },

    // --- Tabs & Widgets ---
    {
        key: "tab:preset",
        label: "Default tab preset",
        category: "Tabs & Widgets",
        control: "text",
        description:
            "Deprecated: a bg@ background preset applied automatically to new tabs. Superseded by tab:background.",
        placeholder: "preset name",
    },
    {
        key: "tab:confirmclose",
        label: "Confirm before closing tab",
        category: "Tabs & Widgets",
        control: "toggle",
        description: "Shows a confirmation dialog before closing a tab.",
    },
    {
        key: "tab:background",
        label: "Default tab background",
        category: "Tabs & Widgets",
        control: "text",
        description: "Sets the bg@ background preset applied automatically to new tabs.",
        placeholder: "background key",
    },
    {
        key: "widget:showhelp",
        label: "Show help widget",
        category: "Tabs & Widgets",
        control: "toggle",
        description: "Declared in settings but not currently read by any UI code -- has no effect yet.",
        nullable: true,
    },

    // --- Connections ---
    {
        key: "conn:askbeforewshinstall",
        label: "Ask before installing wsh",
        category: "Connections",
        control: "toggle",
        description: "Shows a confirmation prompt before installing the wsh helper on a newly connected machine.",
        nullable: true,
        defaultDisplay: "on",
    },
    {
        key: "conn:wshenabled",
        label: "wsh helper enabled",
        category: "Connections",
        control: "toggle",
        description:
            "Controls whether the wsh helper is installed and used on a connection; per-connection overrides live in connections.json.",
    },
    {
        key: "conn:localhostdisplayname",
        label: "Local host display name",
        category: "Connections",
        control: "text",
        description: "Overrides the display name shown for localhost in the UI. Set to empty to hide the name.",
        nullable: true,
        defaultDisplay: "OS username@hostname",
        placeholder: "e.g. my-machine",
    },

    // --- Advanced (auto-update, debug, tsunami-dev) ---
    {
        key: "autoupdate:enabled",
        label: "Auto-update enabled",
        category: "Advanced",
        control: "toggle",
        description: "Enables checking for app updates. Requires app restart.",
    },
    {
        key: "autoupdate:intervalms",
        label: "Auto-update check interval",
        category: "Advanced",
        control: "number",
        description: "Sets the time between update checks. Requires app restart.",
        unit: "ms",
        min: 60000,
        max: 86400000,
        step: 60000,
    },
    {
        key: "autoupdate:installonquit",
        label: "Install update on quit",
        category: "Advanced",
        control: "toggle",
        description: "Automatically installs a downloaded update when the app quits. Requires app restart.",
    },
    {
        key: "autoupdate:channel",
        label: "Update channel",
        category: "Advanced",
        control: "text",
        description:
            'Sets the update channel: "latest" for stable builds, or "beta" for more frequent updates. Requires app restart.',
        placeholder: "e.g. latest",
    },
    {
        key: "debug:pprofport",
        label: "pprof port",
        category: "Advanced",
        control: "number",
        description: "Starts a Go pprof profiling server on this port when the app launches.",
        nullable: true,
        unit: "port",
        min: 0,
        max: 65535,
        step: 1,
    },
    {
        key: "debug:pprofmemprofilerate",
        label: "pprof mem profile rate",
        category: "Advanced",
        control: "number",
        description: "Sets Go's runtime.MemProfileRate for memory profiling.",
        nullable: true,
        min: 0,
        max: 1000000,
        step: 1,
    },
    {
        key: "debug:webglstatus",
        label: "Show WebGL status",
        category: "Advanced",
        control: "toggle",
        description: "Shows a WebGL status indicator button in the terminal block header.",
    },
    {
        key: "tsunami:scaffoldpath",
        label: "Tsunami scaffold path",
        category: "Advanced",
        control: "text",
        description: "Overrides the path to the local Tsunami app scaffold used when building RTApps.",
        placeholder: "path",
    },
    {
        key: "tsunami:sdkreplacepath",
        label: "Tsunami SDK replace path",
        category: "Advanced",
        control: "text",
        description:
            "Sets a local filesystem path used as a Go module replace directive for the Tsunami SDK when building RTApps.",
        placeholder: "path",
    },
    {
        key: "tsunami:sdkversion",
        label: "Tsunami SDK version",
        category: "Advanced",
        control: "text",
        description: "Overrides the Tsunami SDK version used when building RTApps.",
        placeholder: "e.g. v0.1.0",
    },
    {
        key: "tsunami:gopath",
        label: "Tsunami Go path",
        category: "Advanced",
        control: "text",
        description: "Overrides the Go path used when building Tsunami-based RTApps.",
        placeholder: "path",
    },
];

const AdvancedCategoryHint =
    "Auto-update, debug, and tsunami-dev options — power-user territory, not shown by default.";

function fieldsByCategory(category: string): FieldSchema[] {
    return FieldSchemas.filter((f) => f.category === category);
}

export function matchesFieldSearch(schema: FieldSchema, query: string): boolean {
    const q = query.trim().toLowerCase();
    if (!q) {
        return true;
    }
    return (
        schema.label.toLowerCase().includes(q) ||
        String(schema.key).toLowerCase().includes(q) ||
        schema.description.toLowerCase().includes(q)
    );
}

export function filterFieldSchemas(schemas: FieldSchema[], query: string): FieldSchema[] {
    if (!query.trim()) {
        return schemas;
    }
    return schemas.filter((schema) => matchesFieldSearch(schema, query));
}

// Groups filtered fields back under their category headings, in Categories order, so search
// results still show which section each match belongs to.
function groupFieldsByCategory(schemas: FieldSchema[]): [string, FieldSchema[]][] {
    const byCategory = new Map<string, FieldSchema[]>();
    for (const schema of schemas) {
        const existing = byCategory.get(schema.category);
        if (existing) {
            existing.push(schema);
        } else {
            byCategory.set(schema.category, [schema]);
        }
    }
    return Categories.filter((category) => byCategory.has(category)).map(
        (category) => [category, byCategory.get(category)] as [string, FieldSchema[]]
    );
}

// Two fields have a real but non-static default (platform-dependent or computed at runtime) --
// resolved here instead of in the schema table, which only holds fixed display strings.
function resolveDefaultDisplay(schema: FieldSchema, model: RemoteTermConfigViewModel): string | undefined {
    if (schema.key === "app:ctrlvpaste") {
        return model.env.isWindows() ? "on (Windows)" : "off (this platform)";
    }
    return schema.defaultDisplay;
}

function percentString(value: number): string {
    return `${Math.round(value * 100)}%`;
}

interface ResetButtonProps {
    onClick: () => void;
    label: string;
}

const ResetButton = memo(({ onClick, label }: ResetButtonProps) => (
    <button
        type="button"
        onClick={onClick}
        aria-label={label}
        title={label}
        className="w-6 h-6 rounded-md border-none bg-transparent text-muted hover:text-secondary flex items-center justify-center cursor-pointer shrink-0"
    >
        <i aria-hidden="true" className="fa-sharp fa-solid fa-arrow-rotate-left text-caption" />
    </button>
));
ResetButton.displayName = "ResetButton";

interface FieldRowProps {
    schema: FieldSchema;
    isSet: boolean;
    defaultDisplay?: string;
    labelId: string;
    hintId: string;
    onReset?: () => void;
    children: React.ReactNode;
}

const FieldRow = memo(({ schema, isSet, defaultDisplay, labelId, hintId, onReset, children }: FieldRowProps) => {
    const showHint = schema.nullable && !isSet;
    return (
        <div className="flex items-center gap-3 bg-modalbg px-3 py-2.5">
            <div className="flex-1 min-w-0">
                <div id={labelId} className="text-xs">
                    {schema.label}
                </div>
                <div className="text-caption text-muted font-mono">{schema.key}</div>
                <div className="text-caption text-muted">{schema.description}</div>
                {showHint && (
                    <div id={hintId} className="text-xxs text-muted mt-0.5">
                        {defaultDisplay ? `Not set — using default (${defaultDisplay})` : "Not set"}
                    </div>
                )}
            </div>
            {children}
            {onReset && <ResetButton onClick={onReset} label={`Reset ${schema.label} to default`} />}
        </div>
    );
});
FieldRow.displayName = "FieldRow";

interface ToggleControlProps {
    checked: boolean;
    dashed?: boolean;
    onToggle: () => void;
    label: string;
    describedBy?: string;
}

const ToggleControl = memo(({ checked, dashed, onToggle, label, describedBy }: ToggleControlProps) => (
    <button
        type="button"
        role="switch"
        aria-checked={checked}
        aria-label={label}
        aria-describedby={describedBy}
        onClick={onToggle}
        className="shrink-0 p-1.5 -m-1.5 flex items-center justify-center cursor-pointer"
    >
        <span
            className={cn(
                "relative w-[30px] h-[17px] rounded-full transition-colors",
                checked
                    ? "bg-accent border-none"
                    : dashed
                      ? "bg-border border border-dashed border-muted"
                      : "bg-border border-none"
            )}
        >
            <span
                className={cn(
                    "absolute top-0.5 w-3 h-3 rounded-full transition-all",
                    checked ? "right-0.5 bg-background" : "left-0.5 bg-muted"
                )}
            />
        </span>
    </button>
));
ToggleControl.displayName = "ToggleControl";

interface SegmentedControlProps {
    value: string;
    options: FieldOption[];
    onChange: (value: string) => void;
    labelledBy: string;
    describedBy?: string;
}

const SegmentedControl = memo(({ value, options, onChange, labelledBy, describedBy }: SegmentedControlProps) => (
    <div
        role="group"
        aria-labelledby={labelledBy}
        aria-describedby={describedBy}
        className="flex bg-black/25 border border-border rounded-md p-0.5 shrink-0"
    >
        {options.map((opt) => (
            <button
                key={opt.value}
                type="button"
                aria-pressed={value === opt.value}
                onClick={() => onChange(opt.value)}
                className={cn(
                    "px-3 py-1 text-caption rounded cursor-pointer transition-colors",
                    value === opt.value ? "bg-activebg text-primary" : "text-secondary hover:text-primary"
                )}
            >
                {opt.label}
            </button>
        ))}
    </div>
));
SegmentedControl.displayName = "SegmentedControl";

interface SelectControlProps {
    value: string;
    options: FieldOption[];
    onChange: (value: string) => void;
    labelledBy: string;
    describedBy?: string;
}

const SelectControl = memo(({ value, options, onChange, labelledBy, describedBy }: SelectControlProps) => (
    <select
        value={value}
        onChange={(e) => onChange(e.target.value)}
        aria-labelledby={labelledBy}
        aria-describedby={describedBy}
        className="shrink-0 bg-black/25 border border-border rounded-md px-2.5 py-1.5 text-xs text-primary cursor-pointer"
    >
        {options.map((opt) => (
            <option key={opt.value} value={opt.value}>
                {opt.label}
            </option>
        ))}
    </select>
));
SelectControl.displayName = "SelectControl";

interface TextControlProps {
    value: string;
    placeholder?: string;
    onCommit: (value: string) => void;
    labelledBy: string;
    describedBy?: string;
}

const TextControl = memo(({ value, placeholder, onCommit, labelledBy, describedBy }: TextControlProps) => {
    const [local, setLocal] = useState(value);
    const [dirty, setDirty] = useState(false);

    if (!dirty && local !== value) {
        setLocal(value);
    }

    const commit = () => {
        setDirty(false);
        if (local !== value) {
            onCommit(local);
        }
    };

    return (
        <input
            type="text"
            value={local}
            placeholder={placeholder}
            aria-labelledby={labelledBy}
            aria-describedby={describedBy}
            onChange={(e) => {
                setDirty(true);
                setLocal(e.target.value);
            }}
            onBlur={commit}
            onKeyDown={(e) => {
                if (e.key === "Enter") {
                    (e.target as HTMLInputElement).blur();
                }
            }}
            className="w-[170px] shrink-0 bg-black/25 border border-border rounded-md px-2.5 py-1.5 text-xs font-mono text-right text-primary focus:outline-none focus:border-accent"
        />
    );
});
TextControl.displayName = "TextControl";

interface NumberControlProps {
    value: number;
    unit?: string;
    min?: number;
    max?: number;
    step?: number;
    // Resolves false when the write failed, so the control can drop back to the stored value.
    onChange: (value: number) => Promise<boolean>;
    fieldLabel: string;
    labelledBy: string;
    describedBy?: string;
}

// Number("") is 0, so an emptied field must be rejected explicitly rather than saved as 0.
// Out-of-range values are clamped because consumers index arrays by some of these settings
// (window:maxtabcachesize drives emain's tab-cache eviction).
export function parseNumberInput(raw: string, min?: number, max?: number): number | null {
    if (raw.trim() === "") {
        return null;
    }
    let next = Number(raw);
    if (!Number.isFinite(next)) {
        return null;
    }
    if (min != null) next = Math.max(min, next);
    if (max != null) next = Math.min(max, next);
    return next;
}

// Step from the typed draft, not `value`: `value` only catches up after the settings
// round-trip, so stepping from it would discard an uncommitted draft.
export function bumpNumberInput(draft: string, value: number, delta: number, min?: number, max?: number): number {
    const base = parseNumberInput(draft, min, max) ?? value;
    return parseNumberInput(String(base + delta), min, max);
}

const NumberControl = memo(
    ({ value, unit, min, max, step = 1, onChange, fieldLabel, labelledBy, describedBy }: NumberControlProps) => {
        const [local, setLocal] = useState(String(value));
        const [dirty, setDirty] = useState(false);
        // The last value this control wrote; `value` only catches up after the config watcher's
        // event, so until then the written value, not the stale prop, is what is shown and stepped.
        const [pending, setPending] = useState<number>(null);
        const [prevValue, setPrevValue] = useState(value);
        const writeQueueRef = useRef<Promise<unknown>>(Promise.resolve());
        const inFlightRef = useRef(0);
        // Values written since `pending` was last clear, so an echo of an earlier one is not
        // mistaken for another writer's change.
        const sentRef = useRef<number[]>([]);

        if (value !== prevValue) {
            setPrevValue(value);
            // With nothing of ours outstanding, a change that is not our own echo came from another
            // writer (Reset, another window, `wsh setconfig`), whose read may have skipped our echo.
            if (pending != null && value !== pending && inFlightRef.current === 0 && !sentRef.current.includes(value)) {
                setPending(null);
            }
        }
        if (pending != null && pending === value) {
            setPending(null);
        }
        if (!dirty && pending == null && local !== String(value)) {
            setLocal(String(value));
        }

        // Each SetConfigCommand runs on its own goroutine server-side, so two in-flight writes for
        // this key can be applied out of order; send the next only after the previous resolves.
        const write = (next: number) => {
            setLocal(String(next));
            setPending(next);
            sentRef.current = pending == null ? [next] : [...sentRef.current, next];
            inFlightRef.current++;
            const sent = writeQueueRef.current.then(() => onChange(next));
            writeQueueRef.current = sent.catch(() => {});
            sent.then(
                (ok) => {
                    inFlightRef.current--;
                    if (ok === false) setPending((p) => (p === next ? null : p));
                },
                () => {
                    inFlightRef.current--;
                    setPending((p) => (p === next ? null : p));
                }
            );
        };

        const commit = () => {
            if (!dirty) {
                return;
            }
            setDirty(false);
            const next = parseNumberInput(local, min, max);
            const current = pending ?? value;
            if (next == null || next === current) {
                setLocal(String(current));
                return;
            }
            write(next);
        };

        const bump = (delta: number) => {
            setDirty(false);
            write(bumpNumberInput(local, pending ?? value, delta, min, max));
        };

        // Commit only when focus leaves the whole control: moving from the input to its own spin
        // button must not commit the draft, or the click would send a second write stepped from it.
        const onControlBlur = (e: React.FocusEvent<HTMLDivElement>) => {
            if (e.currentTarget.contains(e.relatedTarget as Node)) {
                return;
            }
            commit();
        };

        return (
            <div
                onBlur={onControlBlur}
                className="flex items-center gap-1 shrink-0 bg-black/25 border border-border rounded-md pl-2.5 pr-1 py-0.5"
            >
                <input
                    type="number"
                    value={local}
                    min={min}
                    max={max}
                    step={step}
                    aria-labelledby={labelledBy}
                    aria-describedby={describedBy}
                    onChange={(e) => {
                        setDirty(true);
                        setLocal(e.target.value);
                    }}
                    onKeyDown={(e) => {
                        if (e.key === "Enter") {
                            (e.target as HTMLInputElement).blur();
                        }
                    }}
                    className="w-14 bg-transparent text-xs font-mono text-primary focus:outline-none"
                />
                {unit && <span className="text-caption text-muted mr-1">{unit}</span>}
                <div className="flex flex-col">
                    <button
                        type="button"
                        aria-label={`Increase ${fieldLabel}`}
                        onMouseDown={(e) => e.preventDefault()}
                        onClick={() => bump(step)}
                        className="relative w-4 h-[11px] flex items-center justify-center bg-hover rounded-t-sm text-secondary cursor-pointer before:absolute before:-inset-x-1.5 before:-top-2 before:-bottom-0.5 before:content-['']"
                    >
                        <i aria-hidden="true" className="fa-sharp fa-solid fa-caret-up text-xxs" />
                    </button>
                    <button
                        type="button"
                        aria-label={`Decrease ${fieldLabel}`}
                        onMouseDown={(e) => e.preventDefault()}
                        onClick={() => bump(-step)}
                        className="relative w-4 h-[11px] flex items-center justify-center bg-hover rounded-b-sm text-secondary cursor-pointer mt-px before:absolute before:-inset-x-1.5 before:-top-0.5 before:-bottom-2 before:content-['']"
                    >
                        <i aria-hidden="true" className="fa-sharp fa-solid fa-caret-down text-xxs" />
                    </button>
                </div>
            </div>
        );
    }
);
NumberControl.displayName = "NumberControl";

interface SliderControlProps {
    value: number;
    onCommit: (value: number) => void;
    labelledBy: string;
    describedBy?: string;
}

const SliderControl = memo(({ value, onCommit, labelledBy, describedBy }: SliderControlProps) => {
    const [local, setLocal] = useState(Math.round(value * 100));
    const [dirty, setDirty] = useState(false);

    if (!dirty && local !== Math.round(value * 100)) {
        setLocal(Math.round(value * 100));
    }

    const commit = () => {
        setDirty(false);
        onCommit(local / 100);
    };

    return (
        <div className="flex items-center gap-2 shrink-0">
            <input
                type="range"
                min={0}
                max={100}
                value={local}
                aria-labelledby={labelledBy}
                aria-describedby={describedBy}
                onChange={(e) => {
                    setDirty(true);
                    setLocal(Number(e.target.value));
                }}
                onMouseUp={commit}
                onTouchEnd={commit}
                onKeyUp={commit}
                className="w-28 accent-accent cursor-pointer"
            />
            <span className="text-caption font-mono text-secondary w-9 text-right">{percentString(local / 100)}</span>
        </div>
    );
});
SliderControl.displayName = "SliderControl";

interface FieldControlProps {
    schema: FieldSchema;
    model: RemoteTermConfigViewModel;
    settings: SettingsType;
    isSet: boolean;
}

// Renders the control for one field and wires its onChange to a single-key SetConfigCommand
// write (never a read-modify-write patch -- settings.json merges per top-level key already).
const FieldControl = memo(({ schema, model, settings, isSet }: FieldControlProps) => {
    const rawValue = settings[schema.key];
    const defaultDisplay = resolveDefaultDisplay(schema, model);
    const write = (value: unknown) => model.setGeneralSetting({ [schema.key]: value } as SettingsType);
    const reset = () => model.setGeneralSetting({ [schema.key]: null } as SettingsType);

    const labelId = `field-label-${String(schema.key)}`;
    const hintId = `field-hint-${String(schema.key)}`;
    const showHint = schema.nullable && !isSet;
    const describedBy = showHint ? hintId : undefined;

    if (schema.control === "toggle") {
        const checked = !!rawValue;
        return (
            <FieldRow
                schema={schema}
                isSet={isSet}
                defaultDisplay={defaultDisplay}
                labelId={labelId}
                hintId={hintId}
                onReset={schema.nullable && isSet ? reset : undefined}
            >
                <ToggleControl
                    checked={checked}
                    dashed={schema.nullable && !isSet}
                    onToggle={() => write(!checked)}
                    label={`Toggle ${schema.label}`}
                    describedBy={describedBy}
                />
            </FieldRow>
        );
    }

    if (schema.control === "segmented") {
        const value = (rawValue as string) ?? schema.options[0].value;
        return (
            <FieldRow
                schema={schema}
                isSet={isSet}
                defaultDisplay={defaultDisplay}
                labelId={labelId}
                hintId={hintId}
                onReset={schema.nullable && isSet ? reset : undefined}
            >
                <SegmentedControl
                    value={value}
                    options={schema.options}
                    onChange={write}
                    labelledBy={labelId}
                    describedBy={describedBy}
                />
            </FieldRow>
        );
    }

    if (schema.control === "select") {
        const value = (rawValue as string) ?? schema.options[0].value;
        return (
            <FieldRow
                schema={schema}
                isSet={isSet}
                defaultDisplay={defaultDisplay}
                labelId={labelId}
                hintId={hintId}
                onReset={schema.nullable && isSet ? reset : undefined}
            >
                <SelectControl
                    value={value}
                    options={schema.options}
                    onChange={write}
                    labelledBy={labelId}
                    describedBy={describedBy}
                />
            </FieldRow>
        );
    }

    if (schema.control === "text") {
        const value = (rawValue as string) ?? "";
        return (
            <FieldRow
                schema={schema}
                isSet={isSet}
                defaultDisplay={defaultDisplay}
                labelId={labelId}
                hintId={hintId}
                onReset={isSet && value !== "" ? reset : undefined}
            >
                <TextControl
                    value={value}
                    placeholder={schema.placeholder}
                    onCommit={(v) => (v === "" ? reset() : write(v))}
                    labelledBy={labelId}
                    describedBy={describedBy}
                />
            </FieldRow>
        );
    }

    if (schema.control === "number") {
        const value = (rawValue as number) ?? schema.zeroFallback ?? schema.min ?? 0;
        return (
            <FieldRow
                schema={schema}
                isSet={isSet}
                defaultDisplay={defaultDisplay}
                labelId={labelId}
                hintId={hintId}
                onReset={schema.nullable && isSet ? reset : undefined}
            >
                <NumberControl
                    value={value}
                    unit={schema.unit}
                    min={schema.min}
                    max={schema.max}
                    step={schema.step}
                    onChange={write}
                    fieldLabel={schema.label}
                    labelledBy={labelId}
                    describedBy={describedBy}
                />
            </FieldRow>
        );
    }

    // slider
    const value = (rawValue as number) ?? 0;
    return (
        <FieldRow
            schema={schema}
            isSet={isSet}
            defaultDisplay={defaultDisplay}
            labelId={labelId}
            hintId={hintId}
            onReset={schema.nullable && isSet ? reset : undefined}
        >
            <SliderControl value={value} onCommit={write} labelledBy={labelId} describedBy={describedBy} />
        </FieldRow>
    );
});
FieldControl.displayName = "FieldControl";

interface FieldsListProps {
    fields: FieldSchema[];
    model: RemoteTermConfigViewModel;
    settings: SettingsType;
    rawSettings: SettingsType;
}

const FieldsList = memo(({ fields, model, settings, rawSettings }: FieldsListProps) => (
    <div className="max-w-[640px] flex flex-col gap-px bg-border/30 border border-border/30 rounded-lg overflow-hidden">
        {fields.map((schema) => (
            <FieldControl
                key={schema.key}
                schema={schema}
                model={model}
                settings={settings}
                isSet={rawSettings[schema.key] != null}
            />
        ))}
    </div>
));
FieldsList.displayName = "FieldsList";

interface CategoryPanelProps {
    category: string;
    model: RemoteTermConfigViewModel;
    settings: SettingsType;
    rawSettings: SettingsType;
}

const CategoryPanel = memo(({ category, model, settings, rawSettings }: CategoryPanelProps) => {
    const fields = fieldsByCategory(category);
    return (
        <div className="flex-1 min-w-0 overflow-y-auto p-4">
            <h2 className="text-caption font-semibold uppercase tracking-wide text-muted pb-2 px-0.5">{category}</h2>
            {category === "Advanced" && (
                <div className="text-caption text-muted pb-3 max-w-[480px]">{AdvancedCategoryHint}</div>
            )}
            <FieldsList fields={fields} model={model} settings={settings} rawSettings={rawSettings} />
        </div>
    );
});
CategoryPanel.displayName = "CategoryPanel";

interface SearchResultsPanelProps {
    query: string;
    model: RemoteTermConfigViewModel;
    settings: SettingsType;
    rawSettings: SettingsType;
}

const SearchResultsPanel = memo(({ query, model, settings, rawSettings }: SearchResultsPanelProps) => {
    const grouped = groupFieldsByCategory(filterFieldSchemas(FieldSchemas, query));

    if (grouped.length === 0) {
        return (
            <div className="flex-1 min-w-0 overflow-y-auto p-4">
                <div className="text-caption text-muted">No settings match &quot;{query}&quot;</div>
            </div>
        );
    }

    return (
        <div className="flex-1 min-w-0 overflow-y-auto p-4 flex flex-col gap-4">
            {grouped.map(([category, fields]) => (
                <div key={category}>
                    <h2 className="text-caption font-semibold uppercase tracking-wide text-muted pb-2 px-0.5">
                        {category}
                    </h2>
                    <FieldsList fields={fields} model={model} settings={settings} rawSettings={rawSettings} />
                </div>
            ))}
        </div>
    );
});
SearchResultsPanel.displayName = "SearchResultsPanel";

interface CategoryRailProps {
    active: string;
    onSelect: (category: string) => void;
}

const CategoryRail = memo(({ active, onSelect }: CategoryRailProps) => (
    <div className="w-[168px] @max-w450:w-[130px] shrink-0 border-r border-border/60 p-2 flex flex-col gap-px">
        {Categories.map((category) => (
            <button
                key={category}
                type="button"
                onClick={() => onSelect(category)}
                aria-current={active === category ? "true" : undefined}
                className={cn(
                    "text-left text-xs px-2.5 py-1.5 rounded-md cursor-pointer transition-colors",
                    active === category ? "bg-activebg text-primary" : "text-secondary hover:bg-hover"
                )}
            >
                {category}
            </button>
        ))}
    </div>
));
CategoryRail.displayName = "CategoryRail";

interface GeneralContentProps {
    model: RemoteTermConfigViewModel;
}

export const GeneralContent = memo(({ model }: GeneralContentProps) => {
    const settings = useAtomValue(model.settingsAtom);
    const rawSettings = useAtomValue(model.generalRawSettingsAtom);
    const [activeCategory, setActiveCategory] = useState<string>(Categories[0]);
    const search = useAtomValue(model.generalSearchAtom);
    const setSearch = useSetAtom(model.generalSearchAtom);
    const isSearching = search.trim() !== "";

    return (
        <div className="flex flex-col w-full h-full min-h-0">
            <div className="p-3 border-b border-border/60 shrink-0">
                <input
                    type="search"
                    aria-label="Search settings"
                    className="w-full max-w-[300px] bg-black/20 border border-border rounded-md px-2.5 py-1.5 text-xs focus:outline-none focus:border-accent"
                    value={search}
                    onChange={(e) => setSearch(e.target.value)}
                    placeholder="Search settings..."
                />
            </div>
            <div className="flex flex-1 min-h-0">
                {!isSearching && <CategoryRail active={activeCategory} onSelect={setActiveCategory} />}
                {isSearching ? (
                    <SearchResultsPanel query={search} model={model} settings={settings} rawSettings={rawSettings} />
                ) : (
                    <CategoryPanel
                        category={activeCategory}
                        model={model}
                        settings={settings}
                        rawSettings={rawSettings}
                    />
                )}
            </div>
        </div>
    );
});

GeneralContent.displayName = "GeneralContent";
