// Package jq provides Tengo bindings for jQuery-like query operations.
package jq

import (
	"github.com/bubblegutz/nklhd/internal/exports/jq"
	"github.com/d5/tengo/v2"
)

// toGoItems converts a Tengo object to []interface{} for the jq package.
func toGoItems(obj tengo.Object) ([]interface{}, error) {
	v := tengo.ToInterface(obj)
	switch val := v.(type) {
	case []interface{}:
		return val, nil
	default:
		return []interface{}{val}, nil
	}
}

// fromQuery converts a *jq.Query into a Tengo object for return to the script.
func fromQuery(q *jq.Query) (tengo.Object, error) {
	items := q.Items()
	if len(items) == 0 {
		return &tengo.Array{Value: []tengo.Object{}}, nil
	}
	return tengo.FromInterface(items)
}

// Module is the Tengo module map for jq.
// Unlike Lua's chainable userdata API, Tengo uses a flat functional style.
// All functions take raw data as the first argument and return results directly.
var Module = map[string]tengo.Object{
	"pluck": &tengo.UserFunction{
		Name: "pluck",
		Value: func(args ...tengo.Object) (tengo.Object, error) {
			if len(args) < 2 {
				return nil, tengo.ErrWrongNumArguments
			}
			items, err := toGoItems(args[0])
			if err != nil {
				return &tengo.Error{Value: &tengo.String{Value: "pluck error: " + err.Error()}}, nil
			}
			key, ok := args[1].(*tengo.String)
			if !ok {
				return nil, tengo.ErrInvalidArgumentType{Name: "key", Expected: "string", Found: args[1].TypeName()}
			}
			q := jq.NewQuery(items).Pluck(key.Value)
			return fromQuery(q)
		},
	},
	"where": &tengo.UserFunction{
		Name: "where",
		Value: func(args ...tengo.Object) (tengo.Object, error) {
			if len(args) < 3 {
				return nil, tengo.ErrWrongNumArguments
			}
			items, err := toGoItems(args[0])
			if err != nil {
				return &tengo.Error{Value: &tengo.String{Value: "where error: " + err.Error()}}, nil
			}
			key, ok := args[1].(*tengo.String)
			if !ok {
				return nil, tengo.ErrInvalidArgumentType{Name: "key", Expected: "string", Found: args[1].TypeName()}
			}
			val := tengo.ToInterface(args[2])
			q := jq.NewQuery(items).Where(key.Value, val)
			return fromQuery(q)
		},
	},
	"has": &tengo.UserFunction{
		Name: "has",
		Value: func(args ...tengo.Object) (tengo.Object, error) {
			if len(args) < 2 {
				return nil, tengo.ErrWrongNumArguments
			}
			items, err := toGoItems(args[0])
			if err != nil {
				return &tengo.Error{Value: &tengo.String{Value: "has error: " + err.Error()}}, nil
			}
			key, ok := args[1].(*tengo.String)
			if !ok {
				return nil, tengo.ErrInvalidArgumentType{Name: "key", Expected: "string", Found: args[1].TypeName()}
			}
			q := jq.NewQuery(items).Has(key.Value)
			return fromQuery(q)
		},
	},
	"find": &tengo.UserFunction{
		Name: "find",
		Value: func(args ...tengo.Object) (tengo.Object, error) {
			if len(args) < 2 {
				return nil, tengo.ErrWrongNumArguments
			}
			items, err := toGoItems(args[0])
			if err != nil {
				return &tengo.Error{Value: &tengo.String{Value: "find error: " + err.Error()}}, nil
			}
			key, ok := args[1].(*tengo.String)
			if !ok {
				return nil, tengo.ErrInvalidArgumentType{Name: "key", Expected: "string", Found: args[1].TypeName()}
			}
			q := jq.NewQuery(items).Find(key.Value)
			return fromQuery(q)
		},
	},
	"first": &tengo.UserFunction{
		Name: "first",
		Value: func(args ...tengo.Object) (tengo.Object, error) {
			if len(args) < 1 {
				return nil, tengo.ErrWrongNumArguments
			}
			items, err := toGoItems(args[0])
			if err != nil {
				return &tengo.Error{Value: &tengo.String{Value: "first error: " + err.Error()}}, nil
			}
			val := jq.NewQuery(items).First()
			obj, err := tengo.FromInterface(val)
			if err != nil {
				return tengo.UndefinedValue, nil
			}
			return obj, nil
		},
	},
	"last": &tengo.UserFunction{
		Name: "last",
		Value: func(args ...tengo.Object) (tengo.Object, error) {
			if len(args) < 1 {
				return nil, tengo.ErrWrongNumArguments
			}
			items, err := toGoItems(args[0])
			if err != nil {
				return &tengo.Error{Value: &tengo.String{Value: "last error: " + err.Error()}}, nil
			}
			val := jq.NewQuery(items).Last()
			obj, err := tengo.FromInterface(val)
			if err != nil {
				return tengo.UndefinedValue, nil
			}
			return obj, nil
		},
	},
	"count": &tengo.UserFunction{
		Name: "count",
		Value: func(args ...tengo.Object) (tengo.Object, error) {
			if len(args) < 1 {
				return nil, tengo.ErrWrongNumArguments
			}
			items, err := toGoItems(args[0])
			if err != nil {
				return &tengo.Error{Value: &tengo.String{Value: "count error: " + err.Error()}}, nil
			}
			n := jq.NewQuery(items).Count()
			return &tengo.Int{Value: int64(n)}, nil
		},
	},
}
