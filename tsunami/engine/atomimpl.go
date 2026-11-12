// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package engine

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sync"
)

// AtomMeta provides metadata about an atom for validation and documentation
type AtomMeta struct {
	Description string   // short, user-facing
	Units       string   // "ms", "GiB", etc.
	Min         *float64 // optional minimum (numeric types)
	Max         *float64 // optional maximum (numeric types)
	Enum        []string // allowed values if finite set
	Pattern     string   // regex constraint for strings
}

type AtomImpl[T any] struct {
	lock *sync.Mutex
	// writeLock serialises writers across a whole read-modify-write. It is separate from
	// lock so that UpdateVal's callback (user code, which may read other atoms and so take
	// RootElem.atomLock) never runs while holding lock, which RootElem takes under atomLock.
	writeLock *sync.Mutex
	val       T
	usedBy    map[string]bool // component waveid -> true
	meta      *AtomMeta       // optional metadata
}

func MakeAtomImpl[T any](initialVal T, meta *AtomMeta) *AtomImpl[T] {
	return &AtomImpl[T]{
		lock:      &sync.Mutex{},
		writeLock: &sync.Mutex{},
		val:       initialVal,
		usedBy:    make(map[string]bool),
		meta:      meta,
	}
}

func (a *AtomImpl[T]) GetVal() any {
	a.lock.Lock()
	defer a.lock.Unlock()
	return a.val
}

func (a *AtomImpl[T]) setVal_nolock(val any) error {
	if val == nil {
		var zero T
		a.val = zero
		return nil
	}

	// Try direct assignment if it's already type T
	if typed, ok := val.(T); ok {
		a.val = typed
		return nil
	}

	// Try JSON marshaling/unmarshaling
	jsonBytes, err := json.Marshal(val)
	if err != nil {
		var result T
		return fmt.Errorf("failed to adapt type from %T => %T, input type failed to marshal: %w", val, result, err)
	}

	var result T
	if err := json.Unmarshal(jsonBytes, &result); err != nil {
		return fmt.Errorf("failed to adapt type from %T => %T: %w", val, result, err)
	}

	a.val = result
	return nil
}

func (a *AtomImpl[T]) storeVal(val any) error {
	a.lock.Lock()
	defer a.lock.Unlock()
	return a.setVal_nolock(val)
}

func (a *AtomImpl[T]) SetVal(val any) error {
	a.writeLock.Lock()
	defer a.writeLock.Unlock()
	return a.storeVal(val)
}

// UpdateVal atomically replaces the value with fn(current). If fn returns an error the
// value is left unchanged and the error is returned. fn must not write to this atom.
func (a *AtomImpl[T]) UpdateVal(fn func(any) (any, error)) error {
	a.writeLock.Lock()
	defer a.writeLock.Unlock()
	newVal, err := fn(a.GetVal())
	if err != nil {
		return err
	}
	return a.storeVal(newVal)
}

func (a *AtomImpl[T]) SetUsedBy(waveId string, used bool) {
	a.lock.Lock()
	defer a.lock.Unlock()
	if used {
		a.usedBy[waveId] = true
	} else {
		delete(a.usedBy, waveId)
	}
}

func (a *AtomImpl[T]) GetUsedBy() []string {
	a.lock.Lock()
	defer a.lock.Unlock()

	keys := make([]string, 0, len(a.usedBy))
	for compId := range a.usedBy {
		keys = append(keys, compId)
	}
	return keys
}

func (a *AtomImpl[T]) GetMeta() *AtomMeta {
	a.lock.Lock()
	defer a.lock.Unlock()
	return a.meta
}

func (a *AtomImpl[T]) GetAtomType() reflect.Type {
	return reflect.TypeOf((*T)(nil)).Elem()
}
