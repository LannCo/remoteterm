# Tsunami app: notes for coding agents

This folder is a Tsunami app. Tsunami is a Go framework that renders a React-style UI from Go code; the RemoteTerm app builder compiles and previews it. `TSUNAMI_GUIDE.md` in this folder documents the API with examples.

## What the build compiles

- Every `*.go` file directly in this folder, as `package main`. Files in subfolders are not compiled.
- Everything under `static/`, embedded and served at `/static/<path>`.
- The builder supplies `main()` and the module setup. Do not write a `main` function.
- Never define `func init()` in any file (the build checks `app.go` and rejects it there). For start-up work, define `func AppInit() error` in `app.go`; the build looks for it only in that file. It runs once before the app starts serving, and returning an error stops the app.
- Prefer the Go standard library and the Tsunami SDK. If you import a third-party module, the build adds it to `go.mod` and must download it, so the build needs network access; if the user's machine may be offline, say so before adding one.

## Required declarations

`app.go` declares the app's metadata and its root component, which must be named `App`:

```go
package main

import (
	"github.com/LannCo/remoteterm/tsunami/app"
	"github.com/LannCo/remoteterm/tsunami/vdom"
)

var AppMeta = app.AppMeta{
	Title:     "My App",
	ShortDesc: "One line about what it does",
}

var App = app.DefineComponent("App", func(_ any) any {
	return vdom.H("div", map[string]any{"className": "p-4"}, "Hello")
})
```

## Files the build writes

The builder rewrites these on every build. Do not edit them; changes are lost:

- `go.mod` and `go.sum`. The SDK location in `go.mod` is set per build and can change between launches.
- `manifest.json`, generated from `AppMeta` and the app's config, data and secret declarations.
- `static/tw.css`, the generated Tailwind stylesheet.
- `bin/`, the compiled app.

The `.tsunami/` folder belongs to the builder; read from it, never write to it.

## After you change a file

- The result of the most recent build is in `.tsunami/build.log`. Its last line is the build status: `status: running on port <n>` or `status: error`.
- The builder rewrites this file when a build finishes. If the log is older than your last save, it does not describe your change yet: say the result is pending, or ask the user to rebuild.
- If the status is `status: error`, the lines above it hold the compiler or build output. Fix the file and line it reports, then wait for the next build.
- If `.tsunami/build.log` does not exist, no build has finished yet.
- If the user has "Rebuild on external changes" turned off, saving a file does not start a build; the user clicks Rebuild in the builder. Until then `.tsunami/build.log` describes the previous build.
- If you cannot tell whether a change worked, say so to the user rather than assuming it did.

## Styling

Use Tailwind utility classes in `className`. Tailwind only generates classes it finds written out in the source, so write each class name in full (`"text-red-500"`), never assembled from parts at run time. The app renders on a dark background; the guide lists the theme colours.

## Rules that prevent most bugs

- Components are values created with `app.DefineComponent`; component names start with an uppercase letter; props are plain Go structs with `json` tags.
- Call hooks (`app.UseLocal`, `app.UseEffect`, `app.UseRef` and the others in the guide) at the top of a component body, unconditionally, in the same order on every render.
- Read an atom with `.Get()` and change it with `.Set()` or `.SetFn()`. Never call `.Set()` while rendering; call it from event handlers, effects or goroutines.
- Never modify a value returned by `.Get()` in place; copy it first (`app.DeepCopy`) or use `.SetFn()`, which hands you a copy.
- State that several components share goes in atoms declared at package level with `app.SharedAtom`, `app.ConfigAtom` or `app.DataAtom`.
- Stay inside this folder: do not read or write files elsewhere.

## Where to look next

`TSUNAMI_GUIDE.md` covers elements and attributes, conditional rendering and lists, hooks, atoms, async work with goroutines, keyboard handling, static files and charts.
