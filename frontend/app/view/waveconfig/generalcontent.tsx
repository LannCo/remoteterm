// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import type { WaveConfigViewModel } from "@/app/view/waveconfig/waveconfig-model";
import { cn } from "@/util/util";
import { useAtomValue } from "jotai";
import { memo, useState } from "react";

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
// derived from SettingsType in pkg/wconfig/settingsconfig.go. `feature:waveappbuilder` has no
// category assignment in the plan's grouping list (only in its raw field dump) -- it's an
// app-level feature toggle with no better home, so it's grouped into Appearance here.
const FieldSchemas: FieldSchema[] = [
    // --- Appearance ---
    {
        key: "app:tabbar",
        label: "Tab bar position",
        category: "Appearance",
        control: "segmented",
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
        nullable: true,
        defaultDisplay: "on",
    },
    {
        key: "app:globalhotkey",
        label: "Global hotkey",
        category: "Appearance",
        control: "text",
        placeholder: "e.g. Cmd+Shift+Space",
    },
    {
        key: "app:ctrlvpaste",
        label: "Paste on Ctrl+V select",
        category: "Appearance",
        control: "toggle",
        nullable: true,
    },
    {
        key: "app:disablectrlshiftarrows",
        label: "Disable Ctrl+Shift+Arrow shortcuts",
        category: "Appearance",
        control: "toggle",
    },
    {
        key: "app:disablectrlshiftdisplay",
        label: "Disable Ctrl+Shift+Display shortcuts",
        category: "Appearance",
        control: "toggle",
    },
    {
        key: "app:focusfollowscursor",
        label: "Focus follows cursor",
        category: "Appearance",
        control: "segmented",
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
    },
    {
        key: "app:showoverlayblocknums",
        label: "Show overlay block numbers",
        category: "Appearance",
        control: "toggle",
        nullable: true,
        defaultDisplay: "on",
    },
    {
        key: "app:defaultnewblock",
        label: "Default new block",
        category: "Appearance",
        control: "select",
        options: [
            { value: "term", label: "Terminal" },
            { value: "launcher", label: "Launcher" },
        ],
    },
    { key: "feature:waveappbuilder", label: "Wave app builder feature", category: "Appearance", control: "toggle" },
    { key: "window:transparent", label: "Window transparency enabled", category: "Appearance", control: "toggle" },
    { key: "window:blur", label: "Window background blur", category: "Appearance", control: "toggle" },
    {
        key: "window:opacity",
        label: "Window opacity",
        category: "Appearance",
        control: "slider",
        nullable: true,
        defaultDisplay: "80%",
    },
    {
        key: "window:bgcolor",
        label: "Window background color",
        category: "Appearance",
        control: "text",
        placeholder: "e.g. #1e1e1e",
    },
    { key: "window:reducedmotion", label: "Reduced motion", category: "Appearance", control: "toggle" },
    { key: "window:nativetitlebar", label: "Use native title bar", category: "Appearance", control: "toggle" },
    { key: "window:showmenubar", label: "Show menu bar", category: "Appearance", control: "toggle" },
    {
        key: "window:zoom",
        label: "Window zoom",
        category: "Appearance",
        control: "number",
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
        placeholder: "e.g. 1280x800",
    },
    { key: "window:fullscreenonlaunch", label: "Fullscreen on launch", category: "Appearance", control: "toggle" },
    { key: "window:savelastwindow", label: "Save last window position", category: "Appearance", control: "toggle" },
    {
        key: "window:tilegapsize",
        label: "Split-window gap size",
        category: "Appearance",
        control: "number",
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
        nullable: true,
        defaultDisplay: "60%",
    },
    {
        key: "window:magnifiedblocksize",
        label: "Magnified block size",
        category: "Appearance",
        control: "slider",
        nullable: true,
        defaultDisplay: "95%",
    },
    {
        key: "window:magnifiedblockblurprimarypx",
        label: "Magnified block blur (primary)",
        category: "Appearance",
        control: "number",
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
        nullable: true,
        unit: "px",
        min: 0,
        max: 50,
        step: 1,
        defaultDisplay: "2px",
    },
    { key: "window:confirmclose", label: "Confirm before closing window", category: "Appearance", control: "toggle" },
    {
        key: "window:maxtabcachesize",
        label: "Max tab cache size",
        category: "Appearance",
        control: "number",
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
    },

    // --- Terminal ---
    {
        key: "term:fontsize",
        label: "Terminal font size",
        category: "Terminal",
        control: "number",
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
        placeholder: "e.g. Hack",
    },
    { key: "term:theme", label: "Terminal theme", category: "Terminal", control: "text", placeholder: "theme name" },
    { key: "term:disablewebgl", label: "Disable WebGL renderer", category: "Terminal", control: "toggle" },
    {
        key: "term:localshellpath",
        label: "Local shell path",
        category: "Terminal",
        control: "text",
        placeholder: "e.g. /bin/zsh",
    },
    {
        key: "term:gitbashpath",
        label: "Git Bash path (Windows)",
        category: "Terminal",
        control: "text",
        placeholder: "e.g. C:\\Program Files\\Git\\bin\\bash.exe",
    },
    {
        key: "term:scrollback",
        label: "Scrollback buffer size",
        category: "Terminal",
        control: "number",
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
        nullable: true,
        defaultDisplay: "on",
    },
    {
        key: "term:transparency",
        label: "Terminal transparency",
        category: "Terminal",
        control: "slider",
        nullable: true,
        defaultDisplay: "50%",
    },
    {
        key: "term:allowbracketedpaste",
        label: "Allow bracketed paste",
        category: "Terminal",
        control: "toggle",
        nullable: true,
        defaultDisplay: "on",
    },
    {
        key: "term:shiftenternewline",
        label: "Shift+Enter sends newline",
        category: "Terminal",
        control: "toggle",
        nullable: true,
        defaultDisplay: "on",
    },
    {
        key: "term:macoptionismeta",
        label: "macOS Option key as Meta",
        category: "Terminal",
        control: "toggle",
        nullable: true,
        defaultDisplay: "off",
    },
    {
        key: "term:cursor",
        label: "Cursor style",
        category: "Terminal",
        control: "select",
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
        nullable: true,
        defaultDisplay: "off",
    },
    {
        key: "term:bellsound",
        label: "Bell sound",
        category: "Terminal",
        control: "toggle",
        nullable: true,
        defaultDisplay: "off",
    },
    {
        key: "term:bellindicator",
        label: "Bell indicator",
        category: "Terminal",
        control: "toggle",
        nullable: true,
        defaultDisplay: "on",
    },
    {
        key: "term:osc52",
        label: "OSC52 clipboard access",
        category: "Terminal",
        control: "segmented",
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
        nullable: true,
        defaultDisplay: "off",
    },
    { key: "term:showsplitbuttons", label: "Show split buttons", category: "Terminal", control: "toggle" },
    {
        key: "term:trimtrailingwhitespace",
        label: "Trim trailing whitespace on copy",
        category: "Terminal",
        control: "toggle",
        nullable: true,
        defaultDisplay: "on",
    },

    // --- Editor & Web ---
    { key: "editor:minimapenabled", label: "Show minimap", category: "Editor & Web", control: "toggle" },
    { key: "editor:stickyscrollenabled", label: "Sticky scroll", category: "Editor & Web", control: "toggle" },
    { key: "editor:wordwrap", label: "Word wrap", category: "Editor & Web", control: "toggle" },
    {
        key: "editor:fontsize",
        label: "Editor font size",
        category: "Editor & Web",
        control: "number",
        unit: "px",
        min: 6,
        max: 48,
        step: 1,
        zeroFallback: 12,
    },
    { key: "editor:inlinediff", label: "Inline diff view", category: "Editor & Web", control: "toggle" },
    { key: "web:openlinksinternally", label: "Open links in-app", category: "Editor & Web", control: "toggle" },
    {
        key: "web:defaulturl",
        label: "Default web URL",
        category: "Editor & Web",
        control: "text",
        placeholder: "https://…",
    },
    {
        key: "web:defaultsearch",
        label: "Default search engine URL",
        category: "Editor & Web",
        control: "text",
        placeholder: "https://…{query}…",
    },
    {
        key: "markdown:fontsize",
        label: "Markdown font size",
        category: "Editor & Web",
        control: "number",
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
        nullable: true,
        defaultDisplay: "on",
    },
    {
        key: "preview:defaultsort",
        label: "Default file sort",
        category: "Editor & Web",
        control: "segmented",
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
        placeholder: "preset name",
    },
    { key: "tab:confirmclose", label: "Confirm before closing tab", category: "Tabs & Widgets", control: "toggle" },
    {
        key: "tab:background",
        label: "Default tab background",
        category: "Tabs & Widgets",
        control: "text",
        placeholder: "background key",
    },
    {
        key: "widget:showhelp",
        label: "Show help widget",
        category: "Tabs & Widgets",
        control: "toggle",
        nullable: true,
    },

    // --- Connections ---
    {
        key: "conn:askbeforewshinstall",
        label: "Ask before installing wsh",
        category: "Connections",
        control: "toggle",
        nullable: true,
        defaultDisplay: "on",
    },
    { key: "conn:wshenabled", label: "wsh helper enabled", category: "Connections", control: "toggle" },
    {
        key: "conn:localhostdisplayname",
        label: "Local host display name",
        category: "Connections",
        control: "text",
        nullable: true,
        defaultDisplay: "OS username@hostname",
        placeholder: "e.g. my-machine",
    },

    // --- Advanced (auto-update, debug, tsunami-dev) ---
    { key: "autoupdate:enabled", label: "Auto-update enabled", category: "Advanced", control: "toggle" },
    {
        key: "autoupdate:intervalms",
        label: "Auto-update check interval",
        category: "Advanced",
        control: "number",
        unit: "ms",
        min: 60000,
        max: 86400000,
        step: 60000,
    },
    { key: "autoupdate:installonquit", label: "Install update on quit", category: "Advanced", control: "toggle" },
    {
        key: "autoupdate:channel",
        label: "Update channel",
        category: "Advanced",
        control: "text",
        placeholder: "e.g. latest",
    },
    {
        key: "debug:pprofport",
        label: "pprof port",
        category: "Advanced",
        control: "number",
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
        nullable: true,
        min: 0,
        max: 1000000,
        step: 1,
    },
    { key: "debug:webglstatus", label: "Show WebGL status", category: "Advanced", control: "toggle" },
    {
        key: "tsunami:scaffoldpath",
        label: "Tsunami scaffold path",
        category: "Advanced",
        control: "text",
        placeholder: "path",
    },
    {
        key: "tsunami:sdkreplacepath",
        label: "Tsunami SDK replace path",
        category: "Advanced",
        control: "text",
        placeholder: "path",
    },
    {
        key: "tsunami:sdkversion",
        label: "Tsunami SDK version",
        category: "Advanced",
        control: "text",
        placeholder: "e.g. v0.1.0",
    },
    { key: "tsunami:gopath", label: "Tsunami Go path", category: "Advanced", control: "text", placeholder: "path" },
];

const AdvancedCategoryHint =
    "Auto-update, debug, and tsunami-dev options — power-user territory, not shown by default.";

function fieldsByCategory(category: string): FieldSchema[] {
    return FieldSchemas.filter((f) => f.category === category);
}

// Two fields have a real but non-static default (platform-dependent or computed at runtime) --
// resolved here instead of in the schema table, which only holds fixed display strings.
function resolveDefaultDisplay(schema: FieldSchema, model: WaveConfigViewModel): string | undefined {
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
    onChange: (value: number) => void;
    fieldLabel: string;
    labelledBy: string;
    describedBy?: string;
}

const NumberControl = memo(
    ({ value, unit, min, max, step = 1, onChange, fieldLabel, labelledBy, describedBy }: NumberControlProps) => {
        const bump = (delta: number) => {
            let next = value + delta;
            if (min != null) next = Math.max(min, next);
            if (max != null) next = Math.min(max, next);
            onChange(next);
        };

        return (
            <div className="flex items-center gap-1 shrink-0 bg-black/25 border border-border rounded-md pl-2.5 pr-1 py-0.5">
                <input
                    type="number"
                    value={value}
                    min={min}
                    max={max}
                    step={step}
                    aria-labelledby={labelledBy}
                    aria-describedby={describedBy}
                    onChange={(e) => {
                        const next = Number(e.target.value);
                        if (Number.isFinite(next)) onChange(next);
                    }}
                    className="w-14 bg-transparent text-xs font-mono text-primary focus:outline-none"
                />
                {unit && <span className="text-caption text-muted mr-1">{unit}</span>}
                <div className="flex flex-col">
                    <button
                        type="button"
                        aria-label={`Increase ${fieldLabel}`}
                        onClick={() => bump(step)}
                        className="relative w-4 h-[11px] flex items-center justify-center bg-hover rounded-t-sm text-secondary cursor-pointer before:absolute before:-inset-x-1.5 before:-top-2 before:-bottom-0.5 before:content-['']"
                    >
                        <i aria-hidden="true" className="fa-sharp fa-solid fa-caret-up text-xxs" />
                    </button>
                    <button
                        type="button"
                        aria-label={`Decrease ${fieldLabel}`}
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
    model: WaveConfigViewModel;
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

interface CategoryPanelProps {
    category: string;
    model: WaveConfigViewModel;
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
        </div>
    );
});
CategoryPanel.displayName = "CategoryPanel";

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
    model: WaveConfigViewModel;
}

export const GeneralContent = memo(({ model }: GeneralContentProps) => {
    const settings = useAtomValue(model.settingsAtom);
    const rawSettings = useAtomValue(model.generalRawSettingsAtom);
    const [activeCategory, setActiveCategory] = useState<string>(Categories[0]);

    return (
        <div className="flex w-full h-full min-h-0">
            <CategoryRail active={activeCategory} onSelect={setActiveCategory} />
            <CategoryPanel category={activeCategory} model={model} settings={settings} rawSettings={rawSettings} />
        </div>
    );
});

GeneralContent.displayName = "GeneralContent";
