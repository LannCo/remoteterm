// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package util

import "reflect"

type deepCopyPtrKey struct {
	addr uintptr
	typ  reflect.Type
}

// DeepCopy recursively copies pointers, slices, maps, arrays, interfaces and exported
// struct fields. Unexported struct fields are copied by value but not recursed into, so
// any slice, map or pointer they hold is shared with the original (recursing would mean
// copying the private state of foreign types such as time.Location). Funcs and channels
// are shared. Pointer cycles and shared pointers are preserved.
func DeepCopy[T any](v T) T {
	var result T
	deepCopyValue(reflect.ValueOf(&result).Elem(), reflect.ValueOf(&v).Elem(), make(map[deepCopyPtrKey]reflect.Value))
	return result
}

func deepCopyValue(dst, src reflect.Value, seen map[deepCopyPtrKey]reflect.Value) {
	switch src.Kind() {
	case reflect.Pointer:
		if src.IsNil() {
			return
		}
		key := deepCopyPtrKey{addr: src.Pointer(), typ: src.Type()}
		if existing, ok := seen[key]; ok {
			dst.Set(existing)
			return
		}
		newPtr := reflect.New(src.Type().Elem())
		seen[key] = newPtr
		deepCopyValue(newPtr.Elem(), src.Elem(), seen)
		dst.Set(newPtr)
	case reflect.Interface:
		if src.IsNil() {
			return
		}
		inner := src.Elem()
		newInner := reflect.New(inner.Type()).Elem()
		deepCopyValue(newInner, inner, seen)
		dst.Set(newInner)
	case reflect.Slice:
		if src.IsNil() {
			return
		}
		newSlice := reflect.MakeSlice(src.Type(), src.Len(), src.Len())
		for i := 0; i < src.Len(); i++ {
			deepCopyValue(newSlice.Index(i), src.Index(i), seen)
		}
		dst.Set(newSlice)
	case reflect.Array:
		for i := 0; i < src.Len(); i++ {
			deepCopyValue(dst.Index(i), src.Index(i), seen)
		}
	case reflect.Map:
		if src.IsNil() {
			return
		}
		newMap := reflect.MakeMapWithSize(src.Type(), src.Len())
		iter := src.MapRange()
		for iter.Next() {
			newVal := reflect.New(src.Type().Elem()).Elem()
			deepCopyValue(newVal, iter.Value(), seen)
			newMap.SetMapIndex(iter.Key(), newVal)
		}
		dst.Set(newMap)
	case reflect.Struct:
		dst.Set(src)
		for i := 0; i < src.NumField(); i++ {
			if !src.Type().Field(i).IsExported() {
				continue
			}
			deepCopyValue(dst.Field(i), src.Field(i), seen)
		}
	default:
		dst.Set(src)
	}
}
