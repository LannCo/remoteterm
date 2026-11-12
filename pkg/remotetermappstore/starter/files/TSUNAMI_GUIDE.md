# Tsunami Framework Guide

The Tsunami framework brings React-style UI development to Go, letting you build graphical applications that run inside RemoteTerm. Tsunami is designed for quick, widget-like applications: dashboards, utilities and small tools rather than large monolithic apps. Most apps fit in a single `app.go`; larger ones can split their code across several `.go` files in the root of the app folder.

If you know React, you already understand Tsunami's core concepts. It uses the same patterns for components, props, hooks, state management and styling, implemented in Go.

## React Patterns in Go

Tsunami mirrors React's developer experience:

- **Components**: define reusable UI pieces with typed props structs
- **JSX-like syntax**: use vdom.H to build element trees (like React.createElement)
- **Hooks**: app.UseEffect and app.UseRef work like their React counterparts
- **Local state**: use app.UseLocal in place of React.useState
- **Global state**: use app.ConfigAtom, app.DataAtom and app.SharedAtom for cross-component state
- **Props and state**: familiar patterns for data flow and updates
- **Conditional rendering**: vdom.If and vdom.IfElse for dynamic UIs
- **Event handling**: onClick, onChange, onKeyDown with React-like event objects
- **Styling**: Tailwind v4 classes, plus inline styles via the `style` prop

The UI logic is Go code: you get React's mental model with Go's type safety, concurrency and ecosystem.

```go
// This reads like React JSX, but it is Go
return vdom.H("div", map[string]any{
	"className": "flex items-center gap-4 p-4",
},
	vdom.H("input", map[string]any{
		"type":     "checkbox",
		"checked":  todo.Completed,
		"onChange": handleToggle,
	}),
	vdom.H("span", map[string]any{
		"className": vdom.Classes("flex-1", vdom.If(todo.Completed, "line-through")),
	}, todo.Text),
)
```

## How It Works

A Tsunami application is a Go program that serves its UI over HTTP. Components render to a virtual DOM in Go; the Tsunami frontend, shown inside a RemoteTerm block, turns that into HTML and sends events (clicks, input changes, key presses) back to your Go handlers.

## Creating a Tsunami Application

A Tsunami application is a Go `package main` with an `App` component and an `AppMeta` variable. Here is a minimal "Hello World":

```go
package main

import (
	"github.com/LannCo/remoteterm/tsunami/app"
	"github.com/LannCo/remoteterm/tsunami/vdom"
)

// Required metadata: the title and a one-line description of the app
var AppMeta = app.AppMeta{
	Title:     "Hello World",
	ShortDesc: "A simple greeting widget",
}

// The App component is the required entry point for every Tsunami application
var App = app.DefineComponent("App", func(_ struct{}) any {
	return vdom.H("div", map[string]any{
		"className": "flex items-center justify-center h-screen text-xl font-bold",
	}, "Hello, Tsunami!")
})
```

Key points:

- Must use `package main`.
- The `App` component is required. It is the entry point to your application.
- Do NOT add a `main()` function; the framework provides it when building.
- Do NOT add an `init()` function; the build rejects one in `app.go`. Use `AppInit` (below) instead.
- Uses Tailwind v4 for styling; you can use any Tailwind classes in your components.
- Use React-style camelCase props (`className`, `onClick`).

**AppMeta fields:**

- `Title`: the display name for your application (used in window titles and app lists).
- `ShortDesc`: a one-line description of what the app does. Longer text is truncated to 120 characters.
- `Icon` (optional): a Font Awesome icon name for the app.
- `IconColor` (optional): an HTML colour (name, hex or rgb) for the icon.

### Start-up work with AppInit

If the app needs to do work once before it starts serving (load a file, prepare data), define `AppInit` in `app.go`. The build only looks for it in that file, and it must take no parameters and return `error`. Returning an error stops the app.

```go
func AppInit() error {
	data, err := app.ReadStaticFile("static/defaults.json")
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &defaults)
}
```

## Quick Reference

- Component: app.DefineComponent("Name", func(props PropsType) any { ... })
- Element: vdom.H("div", map[string]any{"className": "..."}, children...)
- Local state: atom := app.UseLocal(initialValue); atom.Get(); atom.Set(value)
- Event handler: "onClick": func() { ... }
- Conditional: vdom.If(condition, element)
- Lists: vdom.ForEach(items, func(item T, idx int) any { return ... })
- Styling: "className": vdom.Classes("bg-panel text-primary p-4", vdom.If(cond, "bg-accentbg"))

## Building Elements with vdom.H

The vdom.H function creates virtual DOM elements following a React-like pattern (React.createElement). It takes a tag name, a props map and any number of children:

```go
// Basic element with no props
vdom.H("div", nil, "Hello world")

// Element with props
vdom.H("div", map[string]any{
	"className": "max-w-4xl mx-auto p-4",
	"id":        "main",
	"onClick": func() {
		log.Printf("clicked!")
	},
},
	"child content",
)

// Element with style (for CSS properties Tailwind does not cover)
vdom.H("div", map[string]any{
	"style": map[string]any{
		"marginTop": 10,   // Numbers become px values (as in React)
		"zIndex":    1000, // Use React style names
		"transform": "rotate(45deg)",
	},
})

// Working with Tailwind classes
vdom.H("div", map[string]any{
	"className": vdom.Classes(
		"p-4 bg-panel rounded-lg",                  // Static Tailwind classes
		vdom.If(isActive, "bg-accentbg text-primary"), // Conditional class: condition first, then class
		vdom.If(isDisabled, "opacity-50"),            // Another conditional
	),
})

// Nesting elements
vdom.H("div", map[string]any{
	"className": "max-w-4xl mx-auto",
},
	vdom.H("h1", map[string]any{
		"className": "text-2xl font-bold mb-4",
	}, "Hello"),
	vdom.H("p", map[string]any{
		"className": "text-secondary leading-relaxed",
	}, "Some content"),
)

// Handling events
vdom.H("button", map[string]any{
	"className": "px-4 py-2 bg-accent/80 text-background rounded hover:bg-accent cursor-pointer",
	"onClick": func() {
		handleClick()
	},
	"onKeyDown": &vdom.VDomFunc{
		Fn:   handleKey,
		Keys: []string{"Enter", "Space"},
	},
})

// List rendering
vdom.H("ul", map[string]any{
	"className": "space-y-2",
},
	vdom.ForEach(items, func(item string, idx int) any {
		return vdom.H("li", map[string]any{
			"key":       idx,
			"className": "py-2 px-4 bg-panel rounded",
		}, item)
	}),
)

// Conditional rendering
vdom.H("div", nil,
	vdom.If(isVisible, vdom.H("span", map[string]any{
		"className": "text-accent font-semibold",
	}, "Visible content")),
)
```

Arguments to H:

1. `tag` (string): the HTML tag name
2. `props` (map[string]any or nil): props map including:
   - className: string of space-separated classes (like React)
   - style: map[string]any of CSS properties (like React)
   - Event handlers (onClick, onChange, etc)
   - Any other valid HTML attributes
3. `children` (...any): any number of child elements:
   - Other H() elements
   - Strings (become text nodes)
   - Numbers (converted to strings)
   - Slices of the above
   - nil values are ignored
   - Anything else is converted to text with fmt.Sprint (so a String() method is used)

Supported tags are the common HTML layout, text, list, table, form and media elements (`div`, `span`, `p`, `h1`-`h6`, `ul`, `li`, `table`, `form`, `input`, `button`, `select`, `textarea`, `img`, `video`, `canvas`, `details` and similar) plus SVG elements. Tags outside that set, including `script`, `link` and `iframe`, render as an "Invalid Tag" placeholder.

Best practices:

- Use vdom.Classes with vdom.If for conditional classes (similar to React's conditional className patterns)
- Use camelCase for style properties (exactly like React)
- Numbers in style are converted to pixel values (like React)
- Always create new slices when updating arrays in state (like React's immutability principle)
- Use vdom.ForEach for list rendering (always passes the index, like React's map with index)
- Include a key prop when rendering lists (needed for React-like reconciliation)

## Conditional Rendering and Lists

The vdom package provides helpers for conditional and list rendering:

```go
// Conditional rendering with vdom.If()
vdom.H("div", nil,
	vdom.If(isVisible,
		vdom.H("span", nil, "Visible content"),
	),
)

// Branching with vdom.IfElse()
vdom.H("div", nil,
	vdom.IfElse(isActive,
		vdom.H("span", nil, "Active"),
		vdom.H("span", nil, "Inactive"),
	),
)

// List rendering (adding a "key" prop to the li element)
items := []string{"A", "B", "C"}
vdom.H("ul", nil,
	vdom.ForEach(items, func(item string, idx int) any {
		return vdom.H("li", map[string]any{
			"key":       idx,
			"className": "py-2 px-3 border-b border-border",
		}, item)
	}),
)
```

Helper functions:

- `vdom.If(cond bool, part any) any`: returns part if the condition is true, nil otherwise
- `vdom.IfElse(cond bool, part any, elsePart any) any`: returns part if the condition is true, elsePart otherwise
- `vdom.Ternary[T any](cond bool, trueRtn T, falseRtn T) T`: type-safe ternary; returns trueRtn if the condition is true, falseRtn otherwise
- `vdom.ForEach[T any](items []T, fn func(T, int) any) []any`: maps over items; the function receives each item and its index
- `vdom.Classes(classes ...any) string`: combines class values into one space-separated string, like the JavaScript clsx library (accepts string, []string, map[string]bool, []any and nil)

Notes:

- vdom.If and vdom.IfElse work for conditional elements, conditional classes and conditional props.
- For vdom.If and vdom.IfElse, the condition (bool) always comes first, then the value(s).
- Use vdom.IfElse when the two branches have different types; use vdom.Ternary when they have the same type.

## Using Hooks in Tsunami

Functions in the app package whose names start with `Use` (for example `app.UseLocal`) are hooks, and they follow the same rules as React hooks.

**Key rules (identical to React):**

- Only call hooks inside app.DefineComponent functions
- Always call hooks at the top level of your component function
- Call hooks before any early returns or conditional logic
- Never call hooks inside loops, conditions, or after conditional returns

```go
var MyComponent = app.DefineComponent("MyComponent", func(props MyProps) any {
	// Good: hooks at top level
	count := app.UseLocal(0)
	app.UseEffect(func() func() {
		// effect body
		return nil
	}, nil)

	// Now safe to have conditional logic
	if someCondition {
		return vdom.H("div", nil, "Early return")
	}

	return vdom.H("div", nil, "Content: ", count.Get())
})
```

**Hook categories:**

- **State management**: app.UseLocal creates local component atoms (see State Management with Atoms)
- **Component lifecycle**: app.UseEffect, app.UseRef, app.UseVDomRef (see Component Lifecycle Hooks)
- **Async operations**: app.UseGoRoutine, app.UseTicker, app.UseAfter manage goroutine and timer lifecycles (see Async Operations and Goroutines)
- **Utility**: app.UseId, app.UseRenderTs, app.UseResync

## State Management with Atoms

Tsunami uses **atoms** for all state. Whether you are managing local component state or global application state, you work with the same atom interface, which prevents common bugs and keeps types checked.

### What Are Atoms?

An atom is an object that holds a value and provides methods to read and update it:

```go
// Create an atom (local component state)
count := app.UseLocal(0)

// Read the current value (always up to date)
currentValue := count.Get()

// Update the value (only in event handlers, effects, or async code)
count.Set(42)

// Functional update based on the current value
count.SetFn(func(current int) int {
	return current + 1
})
```

### The Atom Interface

Every atom (an `app.Atom[T]`) has the same methods:

- **`Get()`**: returns the current value; during render it also registers the component as dependent on the atom
- **`Set(value)`**: updates the atom with a new value
- **`SetFn(func(current) new)`**: updates the atom using a function that receives a copy of the current value

### Key Benefits

**Prevents stale closures**: unlike React's useState, where captured values go stale, `atom.Get()` always returns the current value:

```javascript
// React problem: count is stale in setTimeout
const [count, setCount] = useState(0);
setTimeout(() => console.log(count), 1000); // Always logs 0
```

```go
// Tsunami: always current
count := app.UseLocal(0)
time.AfterFunc(time.Second, func() {
	log.Printf("%d", count.Get()) // Logs the current value
})
```

**Type safety**: atoms are strongly typed. An `app.Atom[int]` can only hold integers:

```go
var userCount = app.SharedAtom("userCount", 0)

// userCount.Set("hello") // Compile error: cannot assign a string to an int atom
```

**No stale references**: when atoms are shared across components, every component gets the same typed object, with no typos or type mismatches.

### Important Rules

**Read with Get()**: always use `atom.Get()` to read values in render code:

```go
var MyComponent = app.DefineComponent("MyComponent", func(_ struct{}) any {
	count := app.UseLocal(0)

	// Correct: read with Get()
	currentCount := count.Get()

	return vdom.H("div", nil, "Count: ", currentCount)
})
```

**Write in handlers only**: never call `atom.Set()` or `atom.SetFn()` in render code, only in event handlers, effects or async code. A Set during render is ignored and logged as an error:

```go
var MyComponent = app.DefineComponent("MyComponent", func(_ struct{}) any {
	count := app.UseLocal(0)

	// Correct: update in an event handler
	handleClick := func() {
		count.Set(count.Get() + 1)
	}

	// Wrong: never update in render code
	// count.Set(42)

	return vdom.H("button", map[string]any{
		"onClick": handleClick,
	}, "Click me")
})
```

**Never mutate values from Get()**: for slices, maps, pointers and structs containing them, never modify the value returned from `atom.Get()`.

**Use SetFn() for safe mutations**: `SetFn()` passes your function a deep copy of the current value, so it is safe to modify:

```go
var MyComponent = app.DefineComponent("MyComponent", func(_ struct{}) any {
	todos := app.UseLocal([]Todo{{Text: "Learn Tsunami"}})

	addTodo := func() {
		// Correct: SetFn passes a deep copy of the current value
		todos.SetFn(func(current []Todo) []Todo {
			return append(current, Todo{Text: "New task"})
		})
	}

	// Wrong: never mutate the original
	// badUpdate := func() {
	//     current := todos.Get()
	//     current[0].Text = "Modified" // Mutates shared state
	//     todos.Set(current)
	// }

	return vdom.H("div", map[string]any{"onClick": addTodo}, "Todo count: ", len(todos.Get()))
})
```

**Capture atoms, not values**: in closures and async code, capture the atom itself, never a value read during render:

```go
var MyComponent = app.DefineComponent("MyComponent", func(_ struct{}) any {
	count := app.UseLocal(0)
	currentCount := count.Get() // Read in render

	// Correct: capture the atom
	handleDelayedIncrement := func() {
		time.AfterFunc(time.Second, func() {
			count.SetFn(func(current int) int { return current + 1 })
		})
	}

	// Wrong: captures a stale value from render
	// handleStaleIncrement := func() {
	//     time.AfterFunc(time.Second, func() {
	//         count.Set(currentCount + 1) // Uses stale currentCount
	//     })
	// }

	return vdom.H("button", map[string]any{
		"onClick": handleDelayedIncrement,
	}, "Count: ", currentCount)
})
```

**Key points:**

- `SetFn()` deep copies the current value before passing it to your function
- When updating with `Set()`, use `app.DeepCopy(value)` before modifying complex data from `atom.Get()`
- Always capture atoms in closures, never values read during render
- `app.DeepCopy[T any](value T) T` copies pointers, slices, maps and exported struct fields recursively; unexported struct fields are copied by value only, so slices, maps or pointers held in them stay shared

### Local State with app.UseLocal

For component-specific state, use app.UseLocal:

```go
var MyComponent = app.DefineComponent("MyComponent", func(_ struct{}) any {
	// Like React.useState, but with the atom interface
	name := app.UseLocal("John")
	items := app.UseLocal([]string{})

	// Read values in render code
	currentName := name.Get()
	currentItems := items.Get()

	// Update in event handlers
	handleAddItem := func() {
		items.SetFn(func(current []string) []string {
			return append(current, "item")
		})
	}

	return vdom.H("div", map[string]any{"onClick": handleAddItem},
		"Name: ", currentName, " (", len(currentItems), " items)")
})
```

### Global State Management

For state shared across components or visible to external tools, declare global atoms as package variables.

#### app.AtomMeta for External Integration

app.ConfigAtom and app.DataAtom take an `*app.AtomMeta` parameter (pass nil if you do not need it) that provides schema information for external tools. app.SharedAtom has no app.AtomMeta parameter because it is only for internal state sharing.

```go
type AtomMeta struct {
	Desc    string   // Short, user-facing description
	Units   string   // Units of measurement: "ms", "px", "GiB", etc. Leave blank for counts and unitless values
	Min     *float64 // Optional minimum value (numeric types only)
	Max     *float64 // Optional maximum value (numeric types only)
	Enum    []string // Allowed values if finite set
	Pattern string   // Regex constraint for strings
}
```

#### Declaring Global Atoms

```go
// Declare global atoms as package-level variables
var (
	// SharedAtom: basic shared state between components
	isLoading = app.SharedAtom("isLoading", false)
	userPrefs = app.SharedAtom("userPrefs", UserPreferences{})

	// ConfigAtom: configuration that external tools can read and write
	theme = app.ConfigAtom("theme", "dark", &app.AtomMeta{
		Desc: "UI theme preference",
		Enum: []string{"light", "dark"},
	})
	apiKey = app.ConfigAtom("apiKey", "", &app.AtomMeta{
		Desc:    "Authentication key for external services",
		Pattern: "^[A-Za-z0-9]{32}$",
	})
	maxRetries = app.ConfigAtom("maxRetries", 3, &app.AtomMeta{
		Desc: "Maximum retry attempts for failed requests",
		Min:  app.Ptr(0.0),
		Max:  app.Ptr(10.0),
	})

	// DataAtom: application data that external tools can read
	currentUser = app.DataAtom("currentUser", UserStats{}, &app.AtomMeta{
		Desc: "Current user statistics and profile data",
	})
	lastPollResult = app.DataAtom("lastPoll", APIResult{}, &app.AtomMeta{
		Desc: "Result from the most recent API polling operation",
	})
)
```

- `app.Ptr(value)` is a helper that creates pointers for the Min and Max fields. Use float64 literals such as `app.Ptr(10.0)`, since Min and Max are `*float64`.

app.AtomMeta provides top-level constraints for the atom value. For struct types, use struct tags on individual fields (see Schema Generation below).

#### Using Global Atoms

Global atoms work exactly like local atoms: the same Get, Set and SetFn methods.

#### Global Atom Types

**SharedAtom**: basic shared state between components.

- Shared within the application only
- Not visible to external tools
- Suited to UI state, user preferences and app-wide flags

**ConfigAtom**: configuration that external tools can read and write.

- External tools can GET or POST `/api/config` to read or modify these
- Suited to user settings, API keys and feature flags
- Triggers re-renders when updated internally or externally

**DataAtom**: application data that external tools can read.

- External tools can GET `/api/data` to inspect app state
- Suited to application state, user data and API results
- Read-only from outside the app

#### External API Integration

ConfigAtom and DataAtom are exposed through REST endpoints on the app's server:

- `GET /api/config`: returns all config atom values
- `POST /api/config`: updates (merges) config atom values. The body must be JSON sent with `Content-Type: application/json`. Each value is checked against the atom's type and its app.AtomMeta (Min, Max, Enum, Pattern); if any value fails, nothing is updated and the response lists the invalid keys.
- `GET /api/data`: returns all data atom values
- `GET /api/schemas`: returns JSON schema information for the /api/config and /api/data endpoints, built from app.AtomMeta and type reflection

This makes Tsunami applications straightforward to integrate with external tools and monitoring systems that need to inspect or configure them.

**Note**: you can also update your app's title and description at runtime with `app.SetTitle(title string)` and `app.SetShortDesc(shortDesc string)`, for example to show the current project or file. The change reaches the frontend with the next render.

#### Schema Generation for External Tools

When using ConfigAtom and DataAtom, you can describe the fields of struct values with struct tags, alongside the optional app.AtomMeta:

```go
type UserPrefs struct {
	Theme       string `json:"theme" desc:"UI theme preference" enum:"light,dark"`
	FontSize    int    `json:"fontSize" desc:"Font size in pixels" units:"px" min:"8" max:"32"`
	APIEndpoint string `json:"apiEndpoint" desc:"API base URL" pattern:"^https?://.*"`
}

var userPrefs = app.ConfigAtom("userPrefs", UserPrefs{}, &app.AtomMeta{
	Desc: "User interface and behaviour preferences",
})
```

**Supported schema tags:**

- `desc:"..."`: human-readable description of the field
- `units:"..."`: units of measurement (ms, px, MB, GB, etc.)
- `min:"123"`: minimum value for numeric fields (parsed as a float)
- `max:"456"`: maximum value for numeric fields (parsed as a float)
- `enum:"val1,val2,val3"`: comma-separated list of allowed values for string fields
- `pattern:"regex"`: regular expression for string fields

Struct tags appear in the `/api/schemas` output only; `POST /api/config` enforces just the top-level app.AtomMeta constraints. For validation rules the tags cannot express, describe them in the app.AtomMeta description (for example "Note: 'retryDelays' must contain exactly 3 values in ascending order").

## Component Code Conventions

Tsunami code is easiest to read and debug when every component follows the same layout. Organise components in this order to prevent stale closure bugs and keep them clear:

```go
type ToggleCounterProps struct {
	Title string `json:"title"`
}

var ToggleCounter = app.DefineComponent("ToggleCounter", func(props ToggleCounterProps) any {
	// 1. Atoms and refs defined at the top
	visibleAtom := app.UseLocal(true)
	renderCountRef := app.UseRef(0)

	// 2. Effects and goroutines next. Two steps: first define the function, then call the hook.
	//    Only close over atoms and refs, not values (they can be stale)
	logVisibilityFn := func() func() {
		log.Printf("%s visible: %v", props.Title, visibleAtom.Get())
		return nil
	}
	app.UseEffect(logVisibilityFn, []any{visibleAtom.Get()})

	// 3. Event handlers (close over atoms, not values)
	handleToggle := func() {
		visibleAtom.SetFn(func(isVisible bool) bool { return !isVisible })
	}

	handleReset := func() {
		renderCountRef.Current = 0
		visibleAtom.Set(true)
	}

	// 4. Atom and ref reads (fresh values right before render)
	//    Read here so these values are not used by accident in the closures above.
	//    Writing a ref does not cause a re-render, so this counts renders without adding any
	renderCountRef.Current++
	isVisible := visibleAtom.Get()
	renderCount := renderCountRef.Current

	// 5. Render (return statement)
	return vdom.H("div", map[string]any{
		"className": "p-4 border border-border rounded-lg",
	},
		vdom.H("h3", map[string]any{
			"className": "text-lg font-bold mb-2",
		}, props.Title),

		vdom.H("div", map[string]any{
			"className": "mb-4 space-x-2",
		},
			vdom.H("button", map[string]any{
				"className": "px-3 py-1 bg-accent/80 text-background rounded cursor-pointer",
				"onClick":   handleToggle,
			}, vdom.IfElse(isVisible, "Hide", "Show")),

			vdom.H("button", map[string]any{
				"className": "px-3 py-1 bg-panel text-primary rounded cursor-pointer",
				"onClick":   handleReset,
			}, "Reset"),
		),

		vdom.H("div", map[string]any{
			"className": vdom.Classes("p-3 bg-panel rounded", vdom.If(!isVisible, "hidden")),
		},
			vdom.H("p", nil, "This content can be toggled!"),
			vdom.H("p", map[string]any{
				"className": "text-sm text-secondary mt-2",
			}, "Render count: ", renderCount),
		),
	)
})
```

**Why this order matters:**

- **Props**: always declare a props type for your component (the component name + Props)
- **Define component**: always use DefineComponent to register components. The variable name and the component name should match; component names must start with an uppercase letter.
- **UseLocal / UseRef hooks first**: React rule, always call hooks at the top level; later closures can use these values
- **UseEffect / UseGoRoutine hooks next**: React rule, always call hooks at the top level; they can use the atoms and refs above
- **Handlers next**: they can reference atoms without stale closures
- **Atom reads last**: fresh values right before render
- **Render last**: a clean separation of logic and presentation. You can also return early at this point based on the data, since all the hooks have been called.

## Style Handling

Tsunami applications use Tailwind v4 by default (the className prop), and you should prefer Tailwind wherever possible. Tsunami apps render on a dark background, so design for dark mode: dark backgrounds and light text. You can also define inline styles with a map[string]any in the props:

```go
vdom.H("div", map[string]any{
	"style": map[string]any{
		"marginRight":     10,     // Numbers for px values
		"backgroundColor": "#222", // Colours as strings
		"display":         "flex", // CSS values as strings
		"fontSize":        16,     // More numbers
		"borderRadius":    4,      // Numbers to px
	},
})

// Style values can be dynamic
vdom.H("div", map[string]any{
	"style": map[string]any{
		"marginTop": spacing, // Variables work too
		"color":     vdom.IfElse(isActive, "lightblue", "gray"),
		"display":   "flex",
		"opacity":   vdom.If(isVisible, 1.0), // Conditional styles
	},
})
```

Properties use camelCase (as in React) and values can be:

- Numbers (converted to pixel values)
- Colours as strings
- Other CSS values as strings
- Conditional values using If/IfElse

The style map mirrors React's style object, so it is familiar to React developers.

### Theme Colours

The Tailwind theme defines these colours for use in classes such as `bg-panel`, `text-secondary` or `border-border`:

- `background`: the app background
- `primary`, `secondary`, `muted`: text colours, from strongest to faintest
- `accent` (and `accent-50` to `accent-900`), `accenthover`, `accentbg`: the green accent
- `panel`, `hoverbg`: translucent backgrounds for panels and hover states
- `border`, `strongborder`: border and divider colours
- `error`, `warning`, `success`: status backgrounds, used with `text-primary`

The theme also defines the fonts `font-sans` and `font-mono` and the text sizes `text-xxs`, `text-default` and `text-title`.

### Extra CSS

Quick styles can be added with a `vdom.H("style", nil, "...")` element, whose text children become a stylesheet. The `link` tag is not supported, so stylesheets cannot be linked from `static/`.

## Component Definition Pattern

Create typed, reusable components with app.DefineComponent:

```go
// Define prop types with json tags
type TodoItemProps struct {
	Todo     Todo   `json:"todo"`
	OnToggle func() `json:"onToggle"`
	IsActive bool   `json:"isActive"`
}

// Create a component with typed props
var TodoItem = app.DefineComponent("TodoItem", func(props TodoItemProps) any {
	return vdom.H("div", map[string]any{
		"className": vdom.Classes(
			"p-3 border-b border-border cursor-pointer transition-opacity",
			vdom.IfElse(props.IsActive, "opacity-100 bg-accentbg", "opacity-70 hover:bg-hoverbg"),
		),
		"onClick": props.OnToggle,
	}, props.Todo.Text)
})

// Usage in a parent component:
vdom.H("div", map[string]any{
	"className": "bg-panel rounded-lg border border-border",
},
	TodoItem(TodoItemProps{
		Todo:     todo,
		OnToggle: handleToggle,
		IsActive: isCurrentItem,
	}),
)

// Usage with a key (when in lists)
TodoItem(TodoItemProps{
	Todo:     todo,
	OnToggle: handleToggle,
}).WithKey(idx)
```

Components in Tsunami:

- Use Go structs with json tags for props. Props must be a struct: other props values are dropped silently, so a component without props takes `struct{}`
- Take props as their single argument
- Return elements created with vdom.H (or nil to render nothing)
- Can use all hooks (app.UseLocal, app.UseRef, etc)
- Are registered under a name, which must start with an uppercase letter
- Are called as functions with their props struct

Special handling for the component "key" prop:

- Use the `WithKey(key any)` chaining method to set a key on a component
- Keys must be added for components rendered in lists (just like in React)
- Keys should be unique among siblings and stable across renders
- Keys are handled at the framework level and should not be declared in component props
- `WithKey` accepts any type and converts it to a string using fmt.Sprint

This pattern matches React's functional components while keeping Go's type safety and explicit props.

## Handler Functions

For most event handling, passing a function directly in the props map works. A handler takes either no arguments or one `vdom.VDomEvent`:

```go
vdom.H("button", map[string]any{
	"onClick": func() {
		log.Printf("clicked!")
	},
})

// With event data
vdom.H("input", map[string]any{
	"onChange": func(e vdom.VDomEvent) {
		log.Printf("new value: %s", e.TargetValue)
	},
})
```

Useful vdom.VDomEvent fields:

- `TargetValue`: the new value, for onChange on input, textarea and select
- `TargetChecked`: the checked state, for onChange on checkboxes and radio buttons
- `KeyData`: the key and modifiers, for onKeyDown
- `MouseData`: button, position and modifiers, for onClick, onMouseDown, onMouseUp and onDoubleClick
- `FormData`: the submitted fields, for onSubmit on forms (`e.FormData.GetField("name")` returns the first value of a field)

To prevent default browser behaviour or stop propagation, wrap the handler in a vdom.VDomFunc:

```go
vdom.H("form", map[string]any{
	"onSubmit": &vdom.VDomFunc{
		Fn:             handleSubmit,
		PreventDefault: true, // Prevent the browser's form submission
	},
})
```

## Keyboard Handling

Keyboard events are delivered to the element that has focus, through its `onKeyDown` prop. A plain handler receives every key press, with the details in `e.KeyData`:

```go
vdom.H("input", map[string]any{
	"onKeyDown": func(e vdom.VDomEvent) {
		if e.KeyData == nil {
			return
		}
		switch e.KeyData.Key {
		case "ArrowUp":
			// Handle up arrow
		case "ArrowDown":
			// Handle down arrow
		}
	},
})
```

`e.KeyData` is a `*vdom.VDomKeyboardEvent` with these fields:

- `Key`: the key value (for example "ArrowUp" or "a")
- `Code`: the physical key code
- `Shift`, `Control`, `Alt`, `Meta`: modifier states
- `Cmd`: Meta on macOS, Alt on Windows and Linux
- `Option`: Alt on macOS, Meta on Windows and Linux

To handle only specific keys, use a vdom.VDomFunc with `Keys`:

```go
keyHandler := &vdom.VDomFunc{
	Fn: func(event vdom.VDomEvent) {
		// handle key press
	},
	Keys: []string{
		"Enter",     // Just Enter key
		"Shift:Tab", // Shift+Tab
		"Ctrl:c",    // Ctrl+C
		"Meta:v",    // Meta+V
		"Alt:x",     // Alt+X
		"Cmd:s",     // Command+S (macOS) / Alt+S (Windows, Linux)
		"Option:f",  // Option+F (macOS) / Meta+F (Windows, Linux)
	},
}

vdom.H("input", map[string]any{
	"className": "px-3 py-2 bg-panel border border-border rounded-md focus:outline-none focus:ring-2 focus:ring-accent",
	"onKeyDown": keyHandler,
})
```

The Keys field on VDomFunc:

- Only works with onKeyDown events
- Format is "[modifier]:key" or just "key"; "Space" matches the space bar
- Modifiers are `Shift`, `Ctrl`, `Alt`, `Meta`, `Cmd` and `Option` (`Cmd` and `Option` map as listed above)
- When a listed key matches, the handler runs and the browser's default action and propagation are stopped; other keys are ignored and not sent to the handler

## Component Lifecycle Hooks

Beyond state management with atoms, Tsunami provides hooks for component lifecycle, side effects and DOM interaction. These work like their React counterparts.

### Side Effects with app.UseEffect

app.UseEffect lets you perform side effects after render: subscriptions, timers or any work that needs cleanup. Its signature is `app.UseEffect(fn func() func(), deps []any)`:

```go
var MyComponent = app.DefineComponent("MyComponent", func(_ struct{}) any {
	count := app.UseLocal(0)

	// Effect that runs once on mount
	app.UseEffect(func() func() {
		log.Printf("Component mounted")

		// Return a cleanup function (runs on unmount)
		return func() {
			log.Printf("Component unmounting")
		}
	}, []any{}) // Empty deps = run once

	// Effect that runs when count changes
	app.UseEffect(func() func() {
		log.Printf("Count changed to: %d", count.Get())
		return nil // No cleanup needed
	}, []any{count.Get()}) // Runs when the count.Get() value changes

	return vdom.H("div", nil, "Count: ", count.Get())
})
```

**Dependency array rules (like React):**

- `[]any{}`: runs once on mount
- `[]any{value1, value2}`: runs when any dependency changes (compared with ==, or by content for slices and maps)
- `nil`: runs on every render (usually not what you want)

**Cleanup functions (same rules as React):**

- Return a function from your effect to handle cleanup
- Cleanup runs before the effect runs again and when the component unmounts
- Needed to avoid leaks with timers, subscriptions and goroutines

### References with app.UseRef

app.UseRef creates mutable values that persist across renders without triggering re-renders. Read and modify the value through the `Current` field. The ref is type-safe: the type of `Current` is inferred from the initial value you provide.

```go
var MyComponent = app.DefineComponent("MyComponent", func(_ struct{}) any {
	// Count renders without triggering re-renders
	renderCount := app.UseRef(0)
	renderCount.Current++

	// Store previous values for comparison
	prevCount := app.UseRef(0)
	count := app.UseLocal(0)

	currentCount := count.Get()
	if prevCount.Current != currentCount {
		log.Printf("Count changed from %d to %d", prevCount.Current, currentCount)
		prevCount.Current = currentCount
	}

	return vdom.H("div", nil,
		vdom.H("p", nil, "Render #", renderCount.Current),
		vdom.H("p", nil, "Count: ", currentCount),
		vdom.H("button", map[string]any{
			"onClick": func() { count.SetFn(func(c int) int { return c + 1 }) },
		}, "Increment"),
	)
})
```

**Key points:**

- Read and modify values through the `Current` field
- Type safety: `Current` has the same type as your initial value
- Changes to ref.Current do not trigger re-renders
- Cannot be used as the ref prop on DOM elements (use app.UseVDomRef for that)

### DOM References with app.UseVDomRef

app.UseVDomRef returns a `*vdom.VDomRef` that you attach to an element with the `ref` prop. The ref is not current on the first render; it becomes current once the element is mounted in the frontend (`ref.HasCurrent.Load()` reports this). To act on the element, queue an operation with app.QueueRefOp; on ordinary elements the supported operation is `"focus"`:

```go
var SearchBox = app.DefineComponent("SearchBox", func(_ struct{}) any {
	inputRef := app.UseVDomRef()

	focusInput := func() {
		// Ignored if the ref is not current yet
		app.QueueRefOp(inputRef, vdom.VDomRefOperation{Op: "focus"})
	}

	return vdom.H("div", nil,
		vdom.H("input", map[string]any{
			"ref":  inputRef, // Attach the ref to the DOM element
			"type": "text",
		}),
		vdom.H("button", map[string]any{
			"onClick": focusInput,
		}, "Focus Input"),
	)
})
```

### Utility Hooks

**Specialty hooks** (rarely needed):

- `app.UseId()`: the component's unique identifier
- `app.UseRenderTs()`: the current render's timestamp
- `app.UseResync()`: whether this is a resync render (an initial load or full refresh)

## Best Practices

- **Effects**: always include proper dependency arrays to avoid infinite loops
- **Cleanup**: return cleanup functions from effects for timers, subscriptions and goroutines
- **Refs**: use app.UseRef for goroutine communication and app.UseVDomRef for DOM access
- **Performance**: do not overuse effects; most logic belongs in event handlers

## Async Operations and Goroutines

When working with goroutines, timers or other async operations, follow these patterns to update state safely and manage cleanup. Atom updates made from goroutines trigger a re-render automatically.

### Timer Hooks

For common timing operations, Tsunami provides hooks that handle cleanup automatically.

#### UseTicker for Recurring Operations

Use `app.UseTicker(interval time.Duration, tickFn func(), deps []any)` for work that runs at regular intervals:

```go
var ClockComponent = app.DefineComponent("ClockComponent", func(_ struct{}) any {
	currentTime := app.UseLocal(time.Now().Format("15:04:05"))

	// Update every second; stopped automatically on unmount
	app.UseTicker(time.Second, func() {
		currentTime.Set(time.Now().Format("15:04:05"))
	}, []any{})

	return vdom.H("div", map[string]any{
		"className": "text-2xl font-mono",
	}, "Current time: ", currentTime.Get())
})
```

#### UseAfter for Delayed Operations

Use `app.UseAfter(duration time.Duration, timeoutFn func(), deps []any)` for one-time delayed work:

```go
type ToastComponentProps struct {
	Message  string        `json:"message"`
	Duration time.Duration `json:"duration"`
}

var ToastComponent = app.DefineComponent("ToastComponent", func(props ToastComponentProps) any {
	visible := app.UseLocal(true)

	// Auto-hide after the given duration; cancelled if the component unmounts
	app.UseAfter(props.Duration, func() {
		visible.Set(false)
	}, []any{props.Duration})

	if !visible.Get() {
		return nil
	}

	return vdom.H("div", map[string]any{
		"className": "bg-accentbg text-primary p-4 rounded",
	}, props.Message)
})
```

**Benefits of timer hooks:**

- **Automatic cleanup**: timers stop when the component unmounts or dependencies change
- **No goroutine leaks**: built on `UseGoRoutine` with context cancellation
- **Simpler API**: no ticker channels or timer cleanup to manage
- **Dependency tracking**: change the dependencies to restart a timer with new settings

### Complex Async Operations with UseGoRoutine

For polling, background processing or custom timing logic, use `app.UseGoRoutine(fn func(ctx context.Context), deps []any)` directly:

```go
var DataPollerComponent = app.DefineComponent("DataPollerComponent", func(_ struct{}) any {
	data := app.UseLocal([]APIResult{})
	status := app.UseLocal("idle")

	pollDataFn := func(ctx context.Context) {
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(30 * time.Second):
				status.Set("fetching")

				// Async work: fetch, process, validate
				newData, err := fetchAndProcessData()
				if err != nil {
					status.Set("error")
				} else {
					data.SetFn(func(current []APIResult) []APIResult {
						// SetFn passes a deep copy of current; safe to modify
						return mergeResults(current, newData)
					})
					status.Set("success")
				}
			}
		}
	}

	// Start polling on mount, clean up on unmount
	app.UseGoRoutine(pollDataFn, []any{})

	return vdom.H("div", nil,
		vdom.H("div", nil, "Status: ", status.Get()),
		vdom.H("div", nil, "Data count: ", len(data.Get())),
	)
})
```

app.UseGoRoutine handles the lifecycle:

- Spawns a new goroutine with your function
- Provides a context that is cancelled on dependency changes or component unmount
- Cancels the existing goroutine before starting a new one when dependencies change
- Recovers and logs a panic in your function instead of crashing the app

### Key Patterns

**Context cancellation**: always check ctx.Done() in goroutine loops so they exit cleanly:

```go
pollData := func(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return // Clean exit when the component unmounts or deps change
		case <-time.After(5 * time.Second):
			// Do work
		}
	}
}
```

**Functional setters**: use atom.SetFn() when updating state from goroutines to avoid lost updates:

```go
// Safe: works from the current value
count.SetFn(func(current int) int { return current + 1 })

// Risky: another update can land between Get and Set
count.Set(count.Get() + 1)
```

### Thread Safety

Atoms are synchronised internally, so multiple goroutines can safely call Get() and Set() on the same atom. A SetFn() call reads, applies your function and writes as one step with respect to other Set and SetFn calls on that atom, so concurrent SetFn calls never lose updates. Inside a SetFn function, do not call Set or SetFn on the same atom; it would deadlock.

Never mutate data returned from atom.Get(); use SetFn() or app.DeepCopy() for modifications:

```go
// Safe pattern for concurrent updates using SetFn
updateTodos := func() {
	todosAtom.SetFn(func(current []Todo) []Todo {
		// SetFn passes a deep copy of the current value
		return append(current, newTodo)
	})
}
```

Because atoms handle the synchronisation, you do not need extra locking for basic reads and writes.

## Static Files

For static assets (images, fonts, data files, etc.), create a `static/` directory in your application directory. Its files are embedded in the app and served under the `/static/` URL path:

```
your-app/
├── app.go
└── static/
    ├── logo.png
    ├── data.json
    └── images/
        └── icon.svg
```

Use these files in your components with `/static/` URLs:

```go
vdom.H("img", map[string]any{
	"src": "/static/logo.png",
	"alt": "Logo",
})

vdom.H("div", map[string]any{
	"style": map[string]any{
		"background": "url(/static/images/icon.svg)",
	},
})
```

To read a static file from Go code, use `app.ReadStaticFile("static/data.json")`, `app.OpenStaticFile` or `app.ListStaticFiles`. Paths must start with `static/`. These work from AppInit onwards, not in package-level variable initialisers.

Key points:

- Create a `static` directory and use `/static/` URLs
- Content-Type is detected automatically for static files
- `static/tw.css` is the generated Tailwind stylesheet; the build overwrites it

## Charts

Tsunami includes the Recharts library. Use Recharts components as elements with the `recharts:` prefix and the same names and props as in React:

```go
type MetricsPoint struct {
	Time int     `json:"time"`
	CPU  float64 `json:"cpu"`
	Mem  float64 `json:"mem"`
}

func renderLineChart(data []MetricsPoint) any {
	return vdom.H("div", map[string]any{"className": "w-full h-64"},
		vdom.H("recharts:ResponsiveContainer", map[string]any{
			"width":  "100%",
			"height": "100%",
		},
			vdom.H("recharts:LineChart", map[string]any{
				"data": data,
			},
				vdom.H("recharts:CartesianGrid", map[string]any{"strokeDasharray": "3 3"}),
				vdom.H("recharts:XAxis", map[string]any{"dataKey": "time"}),
				vdom.H("recharts:YAxis", nil),
				vdom.H("recharts:Tooltip", nil),
				vdom.H("recharts:Legend", nil),
				vdom.H("recharts:Line", map[string]any{
					"type":    "monotone",
					"dataKey": "cpu",
					"stroke":  "#8884d8",
					"name":    "CPU %",
					"dot":     false,
				}),
				vdom.H("recharts:Line", map[string]any{
					"type":    "monotone",
					"dataKey": "mem",
					"stroke":  "#82ca9d",
					"name":    "Memory %",
					"dot":     false,
				}),
			),
		),
	)
}
```

Supported components: `ResponsiveContainer`; the charts `LineChart`, `AreaChart`, `BarChart`, `PieChart`, `ScatterChart`, `RadarChart`, `ComposedChart`, `FunnelChart` and `Treemap`; the series `Line`, `Area`, `Bar`, `Pie`, `Cell`, `Scatter`, `Radar` and `Funnel`; and `CartesianGrid`, `XAxis`, `YAxis`, `ZAxis`, `Tooltip`, `Legend`, `PolarGrid`, `PolarAngleAxis`, `PolarRadiusAxis`, `ReferenceLine`, `ReferenceArea`, `ReferenceDot`, `Brush`, `ErrorBar` and `LabelList`. An unknown `recharts:` name renders an error placeholder when it is the outermost chart element; nested inside a chart, it is dropped silently.

Rules for chart elements:

- Pass data as a slice of structs with json tags; the json names are what `dataKey` refers to.
- Props are passed to Recharts as plain values. Event handler props (Go functions) do not work on `recharts:` elements.
- Inside a chart, only `recharts:` elements (and text) are rendered; other elements are dropped. Put titles and layout in ordinary elements around the `ResponsiveContainer`.
- `ResponsiveContainer` with `"width": "100%", "height": "100%"` fills its parent, so give the parent a height (for example `h-64`).

For live charts, keep the data in an atom and update it from a timer hook; the chart re-renders when the atom changes:

```go
var metricsAtom = app.DataAtom("metrics", []MetricsPoint{}, &app.AtomMeta{
	Desc: "Recent CPU and memory samples",
})

var App = app.DefineComponent("App", func(_ struct{}) any {
	app.UseTicker(time.Second, func() {
		metricsAtom.SetFn(func(current []MetricsPoint) []MetricsPoint {
			updated := append(current, samplePoint())
			// Keep only the last 60 points
			if len(updated) > 60 {
				updated = updated[len(updated)-60:]
			}
			return updated
		})
	}, []any{})

	return renderLineChart(metricsAtom.Get())
})
```

## Critical Rules

### Hooks (same as React)

- Only call hooks at the component top level, before any returns
- Never call hooks in loops, conditions or after early returns

### Atoms (Tsunami-specific)

- Read with atom.Get() in render code
- Never call atom.Set() in render code, only in handlers, effects and goroutines
- Use SetFn() for concurrent updates from goroutines (it passes a deep copy of the value)

## Tsunami App Template

```go
package main

import (
	"github.com/LannCo/remoteterm/tsunami/app"
	"github.com/LannCo/remoteterm/tsunami/vdom"
)

var AppMeta = app.AppMeta{
	Title:     "Todos",
	ShortDesc: "A todo list manager",
}

// Tsunami applications include Tailwind v4 CSS automatically:
// no setup required, just use Tailwind classes in your components

// Basic domain types with json tags for props
type Todo struct {
	Id        int    `json:"id"`
	Text      string `json:"text"`
	Completed bool   `json:"completed"`
}

// Global state using DataAtom so external tools can read it
var todosAtom = app.DataAtom("todos", []Todo{
	{Id: 1, Text: "Learn Tsunami", Completed: false},
	{Id: 2, Text: "Build an app", Completed: false},
}, &app.AtomMeta{
	Desc: "List of todo items with completion status",
})

type TodoItemProps struct {
	Todo     Todo   `json:"todo"`
	OnToggle func() `json:"onToggle"`
	OnDelete func() `json:"onDelete"`
}

// Reusable components
var TodoItem = app.DefineComponent("TodoItem", func(props TodoItemProps) any {
	return vdom.H("div", map[string]any{
		"className": vdom.Classes("flex items-center gap-2.5 p-2 border border-border rounded", vdom.If(props.Todo.Completed, "opacity-70")),
	},
		vdom.H("input", map[string]any{
			"className": "w-4 h-4",
			"type":      "checkbox",
			"checked":   props.Todo.Completed,
			"onChange":  props.OnToggle,
		}),
		vdom.H("span", map[string]any{
			"className": vdom.Classes("flex-1", vdom.If(props.Todo.Completed, "line-through")),
		}, props.Todo.Text),
		vdom.H("button", map[string]any{
			"className": "text-error cursor-pointer px-2 py-1 rounded",
			"onClick":   props.OnDelete,
		}, "×"),
	)
})

// Root component must be named "App"
var App = app.DefineComponent("App", func(_ struct{}) any {
	// Local state for the form and ID management
	nextIdAtom := app.UseLocal(3)
	inputTextAtom := app.UseLocal("")

	// Event handlers
	addTodo := func() {
		currentInput := inputTextAtom.Get()
		if currentInput == "" {
			return
		}
		currentNextId := nextIdAtom.Get()

		todosAtom.SetFn(func(current []Todo) []Todo {
			return append(current, Todo{
				Id:        currentNextId,
				Text:      currentInput,
				Completed: false,
			})
		})
		nextIdAtom.Set(currentNextId + 1)
		inputTextAtom.Set("")
	}

	toggleTodo := func(id int) {
		todosAtom.SetFn(func(current []Todo) []Todo {
			// SetFn passes a deep copy of the current value
			for i := range current {
				if current[i].Id == id {
					current[i].Completed = !current[i].Completed
					break
				}
			}
			return current
		})
	}

	deleteTodo := func(id int) {
		currentTodos := todosAtom.Get()
		newTodos := make([]Todo, 0)
		for _, todo := range currentTodos {
			if todo.Id != id {
				newTodos = append(newTodos, todo)
			}
		}
		todosAtom.Set(newTodos)
	}

	// Read atom values in render code
	todoList := todosAtom.Get()
	currentInput := inputTextAtom.Get()

	return vdom.H("div", map[string]any{
		"className": "max-w-[500px] m-5 font-sans",
	},
		vdom.H("h1", map[string]any{
			"className": "text-2xl font-bold mb-5",
		}, "My Tsunami App"),

		vdom.H("div", map[string]any{
			"className": "flex gap-2.5 mb-5",
		},
			vdom.H("input", map[string]any{
				"className":   "flex-1 p-2 border border-border rounded",
				"type":        "text",
				"placeholder": "Add new item...",
				"value":       currentInput,
				"onChange": func(e vdom.VDomEvent) {
					inputTextAtom.Set(e.TargetValue)
				},
			}),
			vdom.H("button", map[string]any{
				"className": "px-4 py-2 border border-border rounded cursor-pointer",
				"onClick":   addTodo,
			}, "Add"),
		),

		vdom.H("div", map[string]any{
			"className": "flex flex-col gap-2",
		}, vdom.ForEach(todoList, func(todo Todo, _ int) any {
			return TodoItem(TodoItemProps{
				Todo:     todo,
				OnToggle: func() { toggleTodo(todo.Id) },
				OnDelete: func() { deleteTodo(todo.Id) },
			}).WithKey(todo.Id)
		})),
	)
})
```

Key points:

1. The root component must be named "App"
2. Do NOT write a main() function; the framework handles the app lifecycle
3. Do NOT write an init() function; put start-up work in `AppInit() error` in `app.go`

## Common Mistakes to Avoid

1. **Calling Set in render**: `countAtom.Set(42)` in a component body is ignored and logged as an error
2. **Missing keys in lists**: always use `.WithKey(id)` for list items
3. **Stale closures in goroutines**: use `atom.Get()` inside event handlers, effects and goroutines, not values captured during render
4. **Wrong prop format**: use `"className"` not `"class"`, `"onClick"` not `"onclick"` (matching React prop and style names)
5. **Mutating state**: with `SetFn()`, you can modify the current value because it is a deep copy. With `Set()`, create new slices or objects, or use the app.DeepCopy helper
6. **Wrong key modifier name**: use `"Ctrl:c"` in `Keys`, not `"Control:c"`

## Styling Requirements

**IMPORTANT**: Tsunami apps render on a dark background. Always use dark-friendly styles:

- Good: `"bg-panel text-primary"`, `"bg-slate-800 border-gray-600"`
- Avoid: `"bg-white text-black"` (light backgrounds)

## Important Technical Details

- Props must be defined as Go structs with json tags
- Components take their props type directly as a parameter
- Always use app.DefineComponent for component registration
- Provide keys when using vdom.ForEach with lists (using the WithKey method)
- Use vdom.Classes with vdom.If for combining static and conditional class names
- `<script>` tags are NOT supported
- Every `.go` file in the root of the app folder is compiled into the app; files in subfolders are not
- Styling is handled through Tailwind v4 CSS classes
- Create apps that work well in dark mode (dark backgrounds and light text)
- Do NOT write a main() or init() function; use AppInit for start-up work
- Build the UI with Go and vdom.H, including complex visualisations; do not write React components

**Async operation guidelines**

- Use app.UseGoRoutine instead of raw go statements for component-related async work
- Use app.UseTicker instead of managing a time.Ticker by hand for recurring operations
- Use app.UseAfter instead of time.AfterFunc for delayed operations
- Always respect ctx.Done() in app.UseGoRoutine functions to prevent goroutine leaks
- Timer and goroutine cleanup happens automatically on component unmount or dependency changes
