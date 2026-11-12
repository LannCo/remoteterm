// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"errors"
	"log"
	"reflect"
	"runtime"

	"github.com/LannCo/remoteterm/tsunami/engine"
	"github.com/LannCo/remoteterm/tsunami/util"
)

// AtomMeta provides metadata about an atom for validation and documentation
type AtomMeta struct {
	Desc    string   // short, user-facing
	Units   string   // "ms", "GiB", etc.
	Min     *float64 // optional minimum (numeric types)
	Max     *float64 // optional maximum (numeric types)
	Enum    []string // allowed values if finite set
	Pattern string   // regex constraint for strings
}

// SecretMeta provides metadata about a secret for documentation and validation
type SecretMeta struct {
	Desc     string
	Optional bool
}

// Atom[T] represents a typed atom implementation
type Atom[T any] struct {
	name   string
	client *engine.ClientImpl
}

// logInvalidAtomSet logs an error when an atom is being set during component render
func logInvalidAtomSet(atomName string) {
	_, file, line, ok := runtime.Caller(2)
	if ok {
		log.Printf("invalid Set of atom '%s' in component render function at %s:%d", atomName, file, line)
	} else {
		log.Printf("invalid Set of atom '%s' in component render function", atomName)
	}
}

var errAtomUpdateSkipped = errors.New("atom update skipped")

// sameRef returns true if oldVal and newVal share the same underlying reference
// (pointer, map, or slice backing array). Nil values return false. Only the top-level
// value is checked: references nested inside structs are not inspected.
func sameRef[T any](oldVal, newVal T) bool {
	vOld := reflect.ValueOf(oldVal)
	vNew := reflect.ValueOf(newVal)

	if !vOld.IsValid() || !vNew.IsValid() {
		return false
	}

	switch vNew.Kind() {
	case reflect.Ptr:
		if vNew.IsNil() {
			return false
		}
		return any(oldVal) == any(newVal)

	case reflect.Map:
		if vOld.Kind() != vNew.Kind() || vOld.IsNil() || vNew.IsNil() {
			return false
		}
		return vOld.Pointer() == vNew.Pointer()

	case reflect.Slice:
		if vOld.Kind() != vNew.Kind() || vOld.IsNil() || vNew.IsNil() {
			return false
		}
		elemSize := vNew.Type().Elem().Size()
		if elemSize == 0 || vOld.Cap() == 0 || vNew.Cap() == 0 {
			return false
		}
		// overlapping backing arrays also catch Set(cur[1:]) after mutating cur
		oldStart, newStart := vOld.Pointer(), vNew.Pointer()
		oldEnd := oldStart + uintptr(vOld.Cap())*elemSize
		newEnd := newStart + uintptr(vNew.Cap())*elemSize
		return oldStart < newEnd && newStart < oldEnd
	}

	// primitives, structs, etc. → not a reference type
	return false
}

// logMutationWarning logs a warning when mutation is detected
func logMutationWarning(atomName string) {
	_, file, line, ok := runtime.Caller(2)
	if ok {
		log.Printf("WARNING: atom '%s' appears to be mutated instead of copied at %s:%d - use app.DeepCopy to create a copy before mutating", atomName, file, line)
	} else {
		log.Printf("WARNING: atom '%s' appears to be mutated instead of copied - use app.DeepCopy to create a copy before mutating", atomName)
	}
}

// AtomName implements the vdom.Atom interface
func (a Atom[T]) AtomName() string {
	return a.name
}

// Get returns the current value of the atom. When called during component render,
// it automatically registers the component as a dependency for this atom, ensuring
// the component re-renders when the atom value changes.
func (a Atom[T]) Get() T {
	vc := engine.GetGlobalRenderContext()
	if vc != nil {
		vc.UsedAtoms[a.name] = true
	}
	val := a.client.Root.GetAtomVal(a.name)
	typedVal := util.GetTypedAtomValue[T](val, a.name)
	return typedVal
}

// Set updates the atom's value to the provided new value and triggers re-rendering
// of any components that depend on this atom. This method cannot be called during
// render cycles - use effects or event handlers instead.
func (a Atom[T]) Set(newVal T) {
	vc := engine.GetGlobalRenderContext()
	if vc != nil {
		logInvalidAtomSet(a.name)
		return
	}

	mutated := false
	err := a.client.Root.UpdateAtomVal(a.name, func(currentVal any) (any, error) {
		mutated = sameRef(util.GetTypedAtomValue[T](currentVal, a.name), newVal)
		return newVal, nil
	})
	if mutated {
		logMutationWarning(a.name)
	}
	if err != nil {
		log.Printf("Failed to set atom value for %s: %v", a.name, err)
		return
	}
	a.client.Root.AtomAddRenderWork(a.name)
}

// SetFn updates the atom's value by applying the provided function to the current value.
// The function receives a DeepCopy of the current atom value, which can be safely mutated
// without affecting the original data. The return value from the function becomes the
// new atom value. The read, fn call and write happen atomically with respect to other
// Set/SetFn calls on this atom, so concurrent SetFn calls never lose updates. fn must not
// call Set or SetFn on this same atom (it would deadlock). This method cannot be called
// during render cycles.
func (a Atom[T]) SetFn(fn func(T) T) {
	vc := engine.GetGlobalRenderContext()
	if vc != nil {
		logInvalidAtomSet(a.name)
		return
	}
	a.trySetFn(func(currentVal T) (T, bool) {
		return fn(DeepCopy(currentVal)), true
	})
}

// trySetFn atomically applies fn and stores its result only if fn returns true.
// Reports whether the value was stored.
func (a Atom[T]) trySetFn(fn func(T) (T, bool)) bool {
	if engine.GetGlobalRenderContext() != nil {
		logInvalidAtomSet(a.name)
		return false
	}
	err := a.client.Root.UpdateAtomVal(a.name, func(currentVal any) (any, error) {
		newVal, ok := fn(util.GetTypedAtomValue[T](currentVal, a.name))
		if !ok {
			return nil, errAtomUpdateSkipped
		}
		return newVal, nil
	})
	if errors.Is(err, errAtomUpdateSkipped) {
		return false
	}
	if err != nil {
		log.Printf("Failed to set atom value for %s: %v", a.name, err)
		return false
	}
	a.client.Root.AtomAddRenderWork(a.name)
	return true
}
