package gobatis

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/runner-mei/GoBatis/reflectx"
)

type StructMap struct {
	Inner *reflectx.StructMap

	Index []*FieldInfo
	Paths map[string]*FieldInfo
	Names map[string]*FieldInfo
}

type Mapper struct {
	mapper *reflectx.Mapper
	cache  atomic.Value
	mutex  sync.Mutex
}

func (m *Mapper) getCache() map[reflect.Type]*StructMap {
	o := m.cache.Load()
	if o == nil {
		return nil
	}

	c, _ := o.(map[reflect.Type]*StructMap)
	return c
}

// func (m *Mapper) FieldByName(v reflect.Value, name string) reflect.Value {
// 	tm := m.TypeMap(v.Type())
// 	fi, ok := tm.Names[name]
// 	if !ok {
// 		return v
// 	}
// 	return reflectx.FieldByIndexes(v, fi.Index)
// }

// func (m *Mapper) TraversalsByNameFunc(t reflect.Type, names []string, fn func(int, *FieldInfo) error) error {
// 	tm := m.TypeMap(t)
// 	for i, name := range names {
// 		fi, _ := tm.Names[name]
// 		if err := fn(i, fi); err != nil {
// 			return err
// 		}
// 	}
// 	return nil
// }

// TypeMap returns a mapping of field strings to int slices representing
// the traversal down the struct to reach the field.
func (m *Mapper) TypeMap(t reflect.Type) *StructMap {
	t = reflectx.Deref(t)

	var cache = m.getCache()
	if cache != nil {
		mapping, ok := cache[t]
		if ok {
			return mapping
		}
	}

	m.mutex.Lock()
	defer m.mutex.Unlock()
	// double check
	cache = m.getCache()
	if cache != nil {
		mapping, ok := cache[t]
		if ok {
			return mapping
		}
		newCache := map[reflect.Type]*StructMap{}
		for key, value := range cache {
			newCache[key] = value
		}
		cache = newCache
	} else {
		cache = map[reflect.Type]*StructMap{}
	}

	mapping := getMapping(m.mapper, t)
	cache[t] = mapping
	m.cache.Store(cache)
	return mapping
}

func getMapping(mapper *reflectx.Mapper, t reflect.Type) *StructMap {
	mapping := mapper.TypeMap(t)
	info := &StructMap{
		Inner: mapping,
		Paths: map[string]*FieldInfo{},
		Names: map[string]*FieldInfo{},
	}
	for idx := range mapping.Index {
		info.Index = append(info.Index, getFeildInfo(mapping.Index[idx]))
	}

	find := func(field *reflectx.FieldInfo) *FieldInfo {
		for idx := range info.Index {
			if info.Index[idx].FieldInfo == field {
				return info.Index[idx]
			}
		}
		panic(errors.New("field '" + field.Name + "' isnot found"))
	}
	for key, field := range mapping.Paths {
		info.Paths[key] = find(field)
	}
	for key, field := range mapping.Names {
		info.Names[key] = find(field)
	}
	return info
}

type FieldInfo struct {
	*reflectx.FieldInfo
	RValue func(dialect Dialect, param *Param, v reflect.Value) (interface{}, error)
	LValue func(dialect Dialect, column string, v reflect.Value) (interface{}, error)
}

func (fi *FieldInfo) makeRValue() func(dialect Dialect, param *Param, v reflect.Value) (interface{}, error) {
	if fi == nil {
		return func(dialect Dialect, param *Param, v reflect.Value) (interface{}, error) {
			return nil, nil
		}
	}
	typ := fi.Field.Type
	kind := typ.Kind()
	isPtr := false
	if typ.Kind() == reflect.Ptr {
		kind = typ.Elem().Kind()
		isPtr = true
	}

	switch kind {
	case reflect.Bool:
		if _, ok := fi.Options["notnull"]; ok && isPtr {
			return func(dialect Dialect, param *Param, v reflect.Value) (interface{}, error) {
				field := reflectx.FieldByIndexesReadOnly(v, fi.Index)
				if field.IsNil() {
					return nil, errors.New("field '" + fi.Field.Name + "' is zero value")
				}
				return field.Interface(), nil
			}
		}

		return func(dialect Dialect, param *Param, v reflect.Value) (interface{}, error) {
			field := reflectx.FieldByIndexesReadOnly(v, fi.Index)
			return field.Interface(), nil
		}
	case reflect.Int,
		reflect.Int8,
		reflect.Int16,
		reflect.Int32,
		reflect.Int64:
		if _, ok := fi.Options["null"]; ok {
			if isPtr {
				return func(dialect Dialect, param *Param, v reflect.Value) (interface{}, error) {
					field := reflectx.FieldByIndexesReadOnly(v, fi.Index)
					if field.IsNil() || !field.IsValid() {
						return nil, nil
					}

					if field.Elem().Int() == 0 {
						return nil, nil
					}
					return field.Interface(), nil
				}
			}

			return func(dialect Dialect, param *Param, v reflect.Value) (interface{}, error) {
				field := reflectx.FieldByIndexesReadOnly(v, fi.Index)
				if field.Int() == 0 {
					return nil, nil
				}
				return field.Interface(), nil
			}
		}

		if _, ok := fi.Options["notnull"]; ok {
			if isPtr {
				return func(dialect Dialect, param *Param, v reflect.Value) (interface{}, error) {
					field := reflectx.FieldByIndexesReadOnly(v, fi.Index)
					if field.IsNil() {
						return nil, errors.New("field '" + fi.Field.Name + "' is zero value")
					}
					if field.Elem().Int() == 0 {
						return nil, errors.New("field '" + fi.Field.Name + "' is zero value")
					}
					return field.Interface(), nil
				}
			}

			return func(dialect Dialect, param *Param, v reflect.Value) (interface{}, error) {
				field := reflectx.FieldByIndexesReadOnly(v, fi.Index)
				if field.Int() == 0 {
					return nil, errors.New("field '" + fi.Field.Name + "' is zero value")
				}
				return field.Interface(), nil
			}
		}
		return func(dialect Dialect, param *Param, v reflect.Value) (interface{}, error) {
			field := reflectx.FieldByIndexesReadOnly(v, fi.Index)
			return field.Interface(), nil
		}
	case reflect.Uint,
		reflect.Uint8,
		reflect.Uint16,
		reflect.Uint32,
		reflect.Uint64,
		reflect.Uintptr:
		if _, ok := fi.Options["null"]; ok {
			if isPtr {
				return func(dialect Dialect, param *Param, v reflect.Value) (interface{}, error) {
					field := reflectx.FieldByIndexesReadOnly(v, fi.Index)
					if field.IsNil() {
						return nil, nil
					}
					if field.Elem().Uint() == 0 {
						return nil, nil
					}
					return field.Interface(), nil
				}
			}

			return func(dialect Dialect, param *Param, v reflect.Value) (interface{}, error) {
				field := reflectx.FieldByIndexesReadOnly(v, fi.Index)
				if field.Uint() == 0 {
					return nil, nil
				}
				return field.Interface(), nil
			}
		}

		if _, ok := fi.Options["notnull"]; ok {
			if isPtr {
				return func(dialect Dialect, param *Param, v reflect.Value) (interface{}, error) {
					field := reflectx.FieldByIndexesReadOnly(v, fi.Index)
					if field.IsNil() {
						return nil, errors.New("field '" + fi.Field.Name + "' is zero value")
					}
					if field.Elem().Uint() == 0 {
						return nil, errors.New("field '" + fi.Field.Name + "' is zero value")
					}
					return field.Interface(), nil
				}
			}

			return func(dialect Dialect, param *Param, v reflect.Value) (interface{}, error) {
				field := reflectx.FieldByIndexesReadOnly(v, fi.Index)
				if field.Uint() == 0 {
					return nil, errors.New("field '" + fi.Field.Name + "' is zero value")
				}
				return field.Interface(), nil
			}
		}

		return func(dialect Dialect, param *Param, v reflect.Value) (interface{}, error) {
			field := reflectx.FieldByIndexesReadOnly(v, fi.Index)
			return field.Interface(), nil
		}
	case reflect.Float32,
		reflect.Float64:
		if _, ok := fi.Options["null"]; ok {
			if isPtr {
				return func(dialect Dialect, param *Param, v reflect.Value) (interface{}, error) {
					field := reflectx.FieldByIndexesReadOnly(v, fi.Index)
					if field.IsNil() {
						return nil, nil
					}
					if field.Elem().Float() == 0 {
						return nil, nil
					}
					return field.Interface(), nil
				}
			}

			return func(dialect Dialect, param *Param, v reflect.Value) (interface{}, error) {
				field := reflectx.FieldByIndexesReadOnly(v, fi.Index)
				if field.Float() == 0 {
					return nil, nil
				}
				return field.Interface(), nil
			}
		}

		if _, ok := fi.Options["notnull"]; ok {
			if isPtr {
				return func(dialect Dialect, param *Param, v reflect.Value) (interface{}, error) {
					field := reflectx.FieldByIndexesReadOnly(v, fi.Index)
					if field.IsNil() {
						return nil, errors.New("field '" + fi.Field.Name + "' is zero value")
					}
					if field.Elem().Float() == 0 {
						return nil, errors.New("field '" + fi.Field.Name + "' is zero value")
					}
					return field.Interface(), nil
				}
			}

			return func(dialect Dialect, param *Param, v reflect.Value) (interface{}, error) {
				field := reflectx.FieldByIndexesReadOnly(v, fi.Index)
				if field.Float() == 0 {
					return nil, errors.New("field '" + fi.Field.Name + "' is zero value")
				}
				return field.Interface(), nil
			}
		}

		return func(dialect Dialect, param *Param, v reflect.Value) (interface{}, error) {
			field := reflectx.FieldByIndexesReadOnly(v, fi.Index)
			return field.Interface(), nil
		}

	// case reflect.Complex64,
	// reflect.Complex128:
	case reflect.String:
		if _, ok := fi.Options["null"]; ok {
			if isPtr {
				return func(dialect Dialect, param *Param, v reflect.Value) (interface{}, error) {
					field := reflectx.FieldByIndexesReadOnly(v, fi.Index)
					if field.IsNil() {
						return nil, nil
					}
					if field.Elem().Len() == 0 {
						return nil, nil
					}
					return field.Interface(), nil
				}
			}

			return func(dialect Dialect, param *Param, v reflect.Value) (interface{}, error) {
				field := reflectx.FieldByIndexesReadOnly(v, fi.Index)
				if field.Len() == 0 {
					return nil, nil
				}
				return field.Interface(), nil
			}
		}

		if _, ok := fi.Options["notnull"]; ok {
			if isPtr {
				return func(dialect Dialect, param *Param, v reflect.Value) (interface{}, error) {
					field := reflectx.FieldByIndexesReadOnly(v, fi.Index)
					if field.IsNil() {
						return nil, errors.New("field '" + fi.Field.Name + "' is zero value")
					}
					if field.Elem().Len() == 0 {
						return nil, errors.New("field '" + fi.Field.Name + "' is zero value")
					}
					return field.Interface(), nil
				}
			}

			return func(dialect Dialect, param *Param, v reflect.Value) (interface{}, error) {
				field := reflectx.FieldByIndexesReadOnly(v, fi.Index)
				if field.Len() == 0 {
					return nil, errors.New("field '" + fi.Field.Name + "' is zero value")
				}
				return field.Interface(), nil
			}
		}
		return func(dialect Dialect, param *Param, v reflect.Value) (interface{}, error) {
			field := reflectx.FieldByIndexesReadOnly(v, fi.Index)
			return field.Interface(), nil
		}
	case reflect.Chan, reflect.Func, reflect.UnsafePointer:
		return func(dialect Dialect, param *Param, v reflect.Value) (interface{}, error) {
			field := reflectx.FieldByIndexesReadOnly(v, fi.Index)
			return nil, fmt.Errorf("field '%s' isnot a sql type got %T", fi.Field.Name, field.Interface())
		}
	default:
		if typ.Implements(_valuerInterface) {
			if isPtr {
				return func(dialect Dialect, param *Param, v reflect.Value) (interface{}, error) {
					field := reflectx.FieldByIndexesReadOnly(v, fi.Index)
					if field.IsNil() {
						return nil, nil
					}
					return field.Interface(), nil
				}
			}

			return func(dialect Dialect, param *Param, v reflect.Value) (interface{}, error) {
				field := reflectx.FieldByIndexesReadOnly(v, fi.Index)
				return field.Interface(), nil
			}
		}
		if reflect.PtrTo(typ).Implements(_valuerInterface) {
			if isPtr {
				return func(dialect Dialect, param *Param, v reflect.Value) (interface{}, error) {
					field := reflectx.FieldByIndexesReadOnly(v, fi.Index)
					if field.IsNil() {
						return nil, nil
					}
					return field.Addr().Interface(), nil
				}
			}

			return func(dialect Dialect, param *Param, v reflect.Value) (interface{}, error) {
				field := reflectx.FieldByIndexesReadOnly(v, fi.Index)
				return field.Addr().Interface(), nil
			}
		}

		switch typ {
		case _timeType:
			if _, ok := fi.Options["null"]; ok {
				return func(dialect Dialect, param *Param, v reflect.Value) (interface{}, error) {
					field := reflectx.FieldByIndexesReadOnly(v, fi.Index)
					fvalue := field.Interface()
					if t, _ := fvalue.(time.Time); !t.IsZero() {
						return fvalue, nil
					}
					return nil, nil
				}
			}
			if _, ok := fi.Options["notnull"]; ok {
				return func(dialect Dialect, param *Param, v reflect.Value) (interface{}, error) {
					field := reflectx.FieldByIndexesReadOnly(v, fi.Index)
					fvalue := field.Interface()
					if fvalue == nil {
						return nil, errors.New("field '" + fi.Field.Name + "' is zero value")
					}
					if t, _ := fvalue.(time.Time); t.IsZero() {
						return nil, errors.New("field '" + fi.Field.Name + "' is zero value")
					}
					return fvalue, nil
				}
			}
			return func(dialect Dialect, param *Param, v reflect.Value) (interface{}, error) {
				field := reflectx.FieldByIndexesReadOnly(v, fi.Index)
				return field.Interface(), nil
			}
		case _timePtr:
			if _, ok := fi.Options["notnull"]; ok {
				return func(dialect Dialect, param *Param, v reflect.Value) (interface{}, error) {
					field := reflectx.FieldByIndexesReadOnly(v, fi.Index)
					if field.IsNil() {
						return nil, errors.New("field '" + fi.Field.Name + "' is zero value")
					}
					fvalue := field.Interface()
					if fvalue == nil {
						return nil, errors.New("field '" + fi.Field.Name + "' is zero value")
					}
					if t, _ := fvalue.(*time.Time); t == nil || t.IsZero() {
						return nil, errors.New("field '" + fi.Field.Name + "' is zero value")
					}
					return fvalue, nil
				}
			}

			if _, ok := fi.Options["null"]; ok {
				return func(dialect Dialect, param *Param, v reflect.Value) (interface{}, error) {
					field := reflectx.FieldByIndexesReadOnly(v, fi.Index)
					if field.IsNil() {
						return nil, nil
					}
					fvalue := field.Interface()
					if fvalue == nil {
						return nil, nil
					}
					if t, _ := fvalue.(*time.Time); t == nil || t.IsZero() {
						return nil, nil
					}
					return fvalue, nil
				}
			}
			return func(dialect Dialect, param *Param, v reflect.Value) (interface{}, error) {
				field := reflectx.FieldByIndexesReadOnly(v, fi.Index)
				if field.IsNil() {
					return nil, nil
				}
				fvalue := field.Interface()
				if fvalue == nil {
					return nil, nil
				}
				if t, _ := fvalue.(*time.Time); t == nil {
					return nil, nil
				}
				return fvalue, nil
			}
		case _ipType:
			if _, ok := fi.Options["null"]; ok {
				return func(dialect Dialect, param *Param, v reflect.Value) (interface{}, error) {
					field := reflectx.FieldByIndexesReadOnly(v, fi.Index)
					if field.IsNil() {
						return nil, nil
					}
					fvalue := field.Interface()
					if fvalue == nil {
						return nil, nil
					}

					ip, _ := fvalue.(net.IP)
					if len(ip) == 0 || ip.IsUnspecified() {
						return nil, nil
					}
					return ip.String(), nil
				}
			}

			if _, ok := fi.Options["notnull"]; ok {
				return func(dialect Dialect, param *Param, v reflect.Value) (interface{}, error) {
					field := reflectx.FieldByIndexesReadOnly(v, fi.Index)
					if field.IsNil() {
						return nil, errors.New("field '" + fi.Field.Name + "' is zero value")
					}
					fvalue := field.Interface()
					if fvalue == nil {
						return nil, errors.New("field '" + fi.Field.Name + "' is zero value")
					}
					ip, _ := fvalue.(net.IP)
					if len(ip) == 0 || ip.IsUnspecified() {
						return nil, errors.New("field '" + fi.Field.Name + "' is zero value")
					}
					return ip.String(), nil
				}
			}

			return func(dialect Dialect, param *Param, v reflect.Value) (interface{}, error) {
				field := reflectx.FieldByIndexesReadOnly(v, fi.Index)
				fvalue := field.Interface()
				if ip, _ := fvalue.(net.IP); len(ip) != 0 && !ip.IsUnspecified() {
					return ip.String(), nil
				}
				return nil, nil
			}
		case _ipPtr:
			if _, ok := fi.Options["notnull"]; ok {
				return func(dialect Dialect, param *Param, v reflect.Value) (interface{}, error) {
					field := reflectx.FieldByIndexesReadOnly(v, fi.Index)
					if field.IsNil() {
						return nil, errors.New("field '" + fi.Field.Name + "' is zero value")
					}
					fvalue := field.Interface()
					if fvalue == nil {
						return nil, errors.New("field '" + fi.Field.Name + "' is zero value")
					}

					ip, _ := fvalue.(*net.IP)
					if ip == nil || len(*ip) == 0 || ip.IsUnspecified() {
						return nil, errors.New("field '" + fi.Field.Name + "' is zero value")
					}
					return ip.String(), nil
				}
			}

			return func(dialect Dialect, param *Param, v reflect.Value) (interface{}, error) {
				field := reflectx.FieldByIndexesReadOnly(v, fi.Index)
				if field.IsNil() {
					return nil, nil
				}
				fvalue := field.Interface()
				if fvalue == nil {
					return nil, nil
				}

				ip, _ := fvalue.(*net.IP)
				if ip == nil || len(*ip) == 0 || ip.IsUnspecified() {
					return nil, nil
				}
				return ip.String(), nil
			}
		case _macType:
			if _, ok := fi.Options["null"]; ok {
				return func(dialect Dialect, param *Param, v reflect.Value) (interface{}, error) {
					field := reflectx.FieldByIndexesReadOnly(v, fi.Index)
					if field.IsNil() {
						return nil, nil
					}
					fvalue := field.Interface()
					if fvalue == nil {
						return nil, nil
					}

					hwAddr, _ := fvalue.(net.HardwareAddr)
					if len(hwAddr) == 0 || isZeroHwAddr(hwAddr) {
						return nil, nil
					}
					return hwAddr.String(), nil
				}
			}

			if _, ok := fi.Options["notnull"]; ok {
				return func(dialect Dialect, param *Param, v reflect.Value) (interface{}, error) {
					field := reflectx.FieldByIndexesReadOnly(v, fi.Index)
					fvalue := field.Interface()
					if fvalue == nil {
						return nil, errors.New("field '" + fi.Field.Name + "' is zero value")
					}

					hwAddr, _ := fvalue.(net.HardwareAddr)
					if len(hwAddr) == 0 || isZeroHwAddr(hwAddr) {
						return nil, errors.New("field '" + fi.Field.Name + "' is zero value")
					}
					return hwAddr.String(), nil
				}
			}

			return func(dialect Dialect, param *Param, v reflect.Value) (interface{}, error) {
				field := reflectx.FieldByIndexesReadOnly(v, fi.Index)
				fvalue := field.Interface()
				if fvalue == nil {
					return nil, nil
				}

				hwAddr, _ := fvalue.(net.HardwareAddr)
				if len(hwAddr) == 0 || isZeroHwAddr(hwAddr) {
					return nil, nil
				}
				return hwAddr.String(), nil
			}
		case _macPtr:
			if _, ok := fi.Options["notnull"]; ok {
				return func(dialect Dialect, param *Param, v reflect.Value) (interface{}, error) {
					field := reflectx.FieldByIndexesReadOnly(v, fi.Index)
					if field.IsNil() {
						return nil, errors.New("field '" + fi.Field.Name + "' is zero value")
					}
					fvalue := field.Interface()
					if fvalue == nil {
						return nil, errors.New("field '" + fi.Field.Name + "' is zero value")
					}

					hwAddr, _ := fvalue.(*net.HardwareAddr)
					if hwAddr == nil || len(*hwAddr) == 0 || isZeroHwAddr(*hwAddr) {
						return nil, errors.New("field '" + fi.Field.Name + "' is zero value")
					}
					return hwAddr.String(), nil
				}
			}

			return func(dialect Dialect, param *Param, v reflect.Value) (interface{}, error) {
				field := reflectx.FieldByIndexesReadOnly(v, fi.Index)
				if field.IsNil() {
					return nil, nil
				}
				fvalue := field.Interface()
				if fvalue == nil {
					return nil, nil
				}
				hwAddr, _ := fvalue.(*net.HardwareAddr)
				if hwAddr == nil || len(*hwAddr) == 0 || isZeroHwAddr(*hwAddr) {
					return nil, nil
				}
				return hwAddr.String(), nil
			}
		}

		canNil := isPtr
		if kind == reflect.Map || kind == reflect.Slice {
			canNil = true
		}

		if _, ok := fi.Options["notnull"]; ok {
			return func(dialect Dialect, param *Param, v reflect.Value) (interface{}, error) {
				field := reflectx.FieldByIndexesReadOnly(v, fi.Index)
				if canNil {
					if field.IsNil() {
						return nil, fmt.Errorf("field '%s' is nil", fi.Field.Name)
					}
				}
				fvalue := field.Interface()
				if fvalue == nil {
					return nil, fmt.Errorf("field '%s' is nil", fi.Field.Name)
				}
				bs, err := json.Marshal(fvalue)
				if err != nil {
					return nil, fmt.Errorf("field '%s' convert to json, %s", fi.Field.Name, err)
				}
				return string(bs), nil
			}
		}

		return func(dialect Dialect, param *Param, v reflect.Value) (interface{}, error) {
			field := reflectx.FieldByIndexesReadOnly(v, fi.Index)
			if canNil {
				if field.IsNil() {
					return nil, nil
				}
			}
			fvalue := field.Interface()
			if fvalue == nil {
				return nil, nil
			}
			bs, err := json.Marshal(fvalue)
			if err != nil {
				return nil, fmt.Errorf("field '%s' convert to json, %s", fi.Field.Name, err)
			}
			return string(bs), nil
		}
	}
}

func isZeroHwAddr(hwAddr net.HardwareAddr) bool {
	for i := range hwAddr {
		if hwAddr[i] != 0 {
			return false
		}
	}
	return true
}

func (fi *FieldInfo) makeLValue() func(dialect Dialect, column string, v reflect.Value) (interface{}, error) {
	if fi == nil {
		return func(dialect Dialect, column string, v reflect.Value) (interface{}, error) {
			return emptyScan, nil
		}
	}
	typ := fi.Field.Type
	kind := typ.Kind()
	if kind == reflect.Ptr {
		kind = typ.Elem().Kind()
		if kind == reflect.Ptr {
			kind = typ.Elem().Kind()
		}
	}

	switch kind {
	case reflect.Bool,
		reflect.Int,
		reflect.Int8,
		reflect.Int16,
		reflect.Int32,
		reflect.Int64,
		reflect.Uint,
		reflect.Uint8,
		reflect.Uint16,
		reflect.Uint32,
		reflect.Uint64,
		reflect.Uintptr,
		reflect.Float32,
		reflect.Float64,
		reflect.Complex64,
		reflect.Complex128,
		reflect.String:
		if _, ok := fi.Options["null"]; ok {
			return func(dialect Dialect, column string, v reflect.Value) (interface{}, error) {
				field := reflectx.FieldByIndexes(v, fi.Index)
				fvalue := &Nullable{Name: fi.Name, Value: field.Addr().Interface()}
				return fvalue, nil
			}
		}
		return func(dialect Dialect, column string, v reflect.Value) (interface{}, error) {
			field := reflectx.FieldByIndexes(v, fi.Index)
			return field.Addr().Interface(), nil
		}
	case reflect.Chan, reflect.Func, reflect.UnsafePointer:
		return func(dialect Dialect, column string, v reflect.Value) (interface{}, error) {
			return nil, fmt.Errorf("cannot convert column '%s' into '%T' fail, error type %T", column, v.Interface(), typ.Name())
		}
	default:
		if reflect.PtrTo(typ).Implements(_scannerInterface) {
			return func(dialect Dialect, column string, v reflect.Value) (interface{}, error) {
				field := reflectx.FieldByIndexes(v, fi.Index)
				return field.Addr().Interface(), nil
			}
		}

		switch typ {
		case _timeType:
			if _, ok := fi.Options["null"]; ok {
				return func(dialect Dialect, column string, v reflect.Value) (interface{}, error) {
					field := reflectx.FieldByIndexes(v, fi.Index)
					fvalue := &Nullable{Name: fi.Name, Value: field.Addr().Interface()}
					return fvalue, nil
				}
			}
			return func(dialect Dialect, column string, v reflect.Value) (interface{}, error) {
				field := reflectx.FieldByIndexes(v, fi.Index)
				return field.Addr().Interface(), nil
			}
		case _timePtr:
			return func(dialect Dialect, column string, v reflect.Value) (interface{}, error) {
				field := reflectx.FieldByIndexes(v, fi.Index)
				return field.Addr().Interface(), nil
			}
		case _ipType:
			return func(dialect Dialect, column string, v reflect.Value) (interface{}, error) {
				field := reflectx.FieldByIndexes(v, fi.Index)
				return &sScanner{name: column, field: field, scanFunc: scanIP}, nil
			}
		case _ipPtr:
			return func(dialect Dialect, column string, v reflect.Value) (interface{}, error) {
				field := reflectx.FieldByIndexes(v, fi.Index)
				return &sScanner{name: column, field: field, scanFunc: scanIP}, nil
			}
		case _macType:
			return func(dialect Dialect, column string, v reflect.Value) (interface{}, error) {
				field := reflectx.FieldByIndexes(v, fi.Index)
				return &sScanner{name: column, field: field, scanFunc: scanMAC}, nil
			}
		case _macPtr:
			return func(dialect Dialect, column string, v reflect.Value) (interface{}, error) {
				field := reflectx.FieldByIndexes(v, fi.Index)
				return &sScanner{name: column, field: field, scanFunc: scanMAC}, nil
			}
		}

		return func(dialect Dialect, column string, v reflect.Value) (interface{}, error) {
			field := reflectx.FieldByIndexes(v, fi.Index)
			return &scanner{name: column, value: field.Addr().Interface()}, nil
		}
	}
}

var _timeType = reflect.TypeOf((*time.Time)(nil)).Elem()
var _timePtr = reflect.TypeOf((*time.Time)(nil))
var _timePtrPtr = reflect.TypeOf((**time.Time)(nil))

var _ipType = reflect.TypeOf((*net.IP)(nil)).Elem()
var _ipPtr = reflect.TypeOf((*net.IP)(nil))
var _ipPtrPtr = reflect.TypeOf((**net.IP)(nil))

var _macType = reflect.TypeOf((*net.HardwareAddr)(nil)).Elem()
var _macPtr = reflect.TypeOf((*net.HardwareAddr)(nil))
var _macPtrPtr = reflect.TypeOf((**net.HardwareAddr)(nil))

func getFeildInfo(field *reflectx.FieldInfo) *FieldInfo {
	info := &FieldInfo{
		FieldInfo: field,
	}
	info.LValue = info.makeLValue()
	info.RValue = info.makeRValue()
	return info
}

const tagPrefix = "db"

// CreateMapper returns a valid mapper using the configured NameMapper func.
func CreateMapper(prefix string, nameMapper func(string) string, tagMapper func(string) []string) *Mapper {
	if nameMapper == nil {
		nameMapper = strings.ToLower
	}
	if prefix == "" {
		prefix = tagPrefix
	}
	return &Mapper{mapper: reflectx.NewMapperTagFunc(prefix, nameMapper, tagMapper)}
}
