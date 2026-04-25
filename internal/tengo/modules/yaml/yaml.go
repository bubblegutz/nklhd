// Package yaml provides Tengo bindings for YAML serialization.
package yaml

import (
	"github.com/bubblegutz/nklhd/internal/exports/yaml"
	"github.com/d5/tengo/v2"
)

// Module is the Tengo module map for yaml.
var Module = map[string]tengo.Object{
	"encode": &tengo.UserFunction{
		Name: "encode",
		Value: func(args ...tengo.Object) (tengo.Object, error) {
			if len(args) < 1 {
				return nil, tengo.ErrWrongNumArguments
			}
			v := tengo.ToInterface(args[0])
			result, err := yaml.Encode(v)
			if err != nil {
				return &tengo.Error{Value: &tengo.String{Value: "encode error: " + err.Error()}}, nil
			}
			return &tengo.String{Value: result}, nil
		},
	},
	"decode": &tengo.UserFunction{
		Name: "decode",
		Value: func(args ...tengo.Object) (tengo.Object, error) {
			if len(args) < 1 {
				return nil, tengo.ErrWrongNumArguments
			}
			s, ok := args[0].(*tengo.String)
			if !ok {
				return nil, tengo.ErrInvalidArgumentType{Name: "s", Expected: "string", Found: args[0].TypeName()}
			}
			data, err := yaml.Decode(s.Value)
			if err != nil {
				return &tengo.Error{Value: &tengo.String{Value: "decode error: " + err.Error()}}, nil
			}
			obj, err := tengo.FromInterface(data)
			if err != nil {
				return &tengo.Error{Value: &tengo.String{Value: "convert error: " + err.Error()}}, nil
			}
			return obj, nil
		},
	},
	"fromJSON": &tengo.UserFunction{
		Name: "fromJSON",
		Value: func(args ...tengo.Object) (tengo.Object, error) {
			if len(args) < 1 {
				return nil, tengo.ErrWrongNumArguments
			}
			s, ok := args[0].(*tengo.String)
			if !ok {
				return nil, tengo.ErrInvalidArgumentType{Name: "s", Expected: "string", Found: args[0].TypeName()}
			}
			result, err := yaml.FromJSON(s.Value)
			if err != nil {
				return &tengo.Error{Value: &tengo.String{Value: "fromJSON error: " + err.Error()}}, nil
			}
			return &tengo.String{Value: result}, nil
		},
	},
}
