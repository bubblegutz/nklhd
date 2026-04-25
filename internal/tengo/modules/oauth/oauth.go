// Package oauth provides Tengo bindings for OAuth 2.0 client functionality.
package oauth

import (
	"time"

	exportsoauth "github.com/bubblegutz/nklhd/internal/exports/oauth"
	"github.com/d5/tengo/v2"
)

// Module is the Tengo module map for oauth.
var Module = map[string]tengo.Object{
	"client": &tengo.UserFunction{
		Name: "client",
		Value: func(args ...tengo.Object) (tengo.Object, error) {
			if len(args) < 1 {
				return nil, tengo.ErrWrongNumArguments
			}
			m, ok := args[0].(*tengo.ImmutableMap)
			if !ok {
				m2, ok2 := args[0].(*tengo.Map)
				if !ok2 {
					return nil, tengo.ErrInvalidArgumentType{Name: "config", Expected: "map", Found: args[0].TypeName()}
				}
				m = &tengo.ImmutableMap{Value: m2.Value}
			}

			clientID := getMapString(m, "client_id")
			tokenURL := getMapString(m, "token_url")
			deviceURL := getMapString(m, "device_url")
			timeout := 5 * time.Second
			if t, ok := m.Value["timeout_ms"]; ok {
				if ti, ok := t.(*tengo.Int); ok {
					timeout = time.Duration(ti.Value) * time.Millisecond
				}
			}

			cfg := &exportsoauth.Config{
				ClientID:  clientID,
				TokenURL:  tokenURL,
				DeviceURL: deviceURL,
				Timeout:   timeout,
			}
			client := exportsoauth.NewClient(cfg)

			return &tengo.ImmutableMap{Value: clientMethods(client)}, nil
		},
	},
}

func getMapString(m *tengo.ImmutableMap, key string) string {
	if v, ok := m.Value[key]; ok {
		if s, ok := v.(*tengo.String); ok {
			return s.Value
		}
	}
	return ""
}

func clientMethods(client *exportsoauth.Client) map[string]tengo.Object {
	return map[string]tengo.Object{
		"device_flow_start": &tengo.UserFunction{
			Name: "device_flow_start",
			Value: func(args ...tengo.Object) (tengo.Object, error) {
				scope := ""
				if len(args) > 0 {
					if s, ok := args[0].(*tengo.String); ok {
						scope = s.Value
					}
				}
				data, err := client.DeviceFlowStart(scope)
				if err != nil {
					return &tengo.Error{Value: &tengo.String{Value: "device_flow_start error: " + err.Error()}}, nil
				}
				return goMapToTengo(data)
			},
		},
		"device_poll": &tengo.UserFunction{
			Name: "device_poll",
			Value: func(args ...tengo.Object) (tengo.Object, error) {
				if len(args) < 1 {
					return nil, tengo.ErrWrongNumArguments
				}
				deviceCode, ok := args[0].(*tengo.String)
				if !ok {
					return nil, tengo.ErrInvalidArgumentType{Name: "device_code", Expected: "string", Found: args[0].TypeName()}
				}
				interval := 5
				if len(args) > 1 {
					if i, ok := args[1].(*tengo.Int); ok {
						interval = int(i.Value)
					}
				}
				maxAttempts := 12
				if len(args) > 2 {
					if i, ok := args[2].(*tengo.Int); ok {
						maxAttempts = int(i.Value)
					}
				}
				data, err := client.DevicePoll(deviceCode.Value, interval, maxAttempts)
				if err != nil {
					return &tengo.Error{Value: &tengo.String{Value: "device_poll error: " + err.Error()}}, nil
				}
				return goMapToTengo(data)
			},
		},
		"auth_code_url": &tengo.UserFunction{
			Name: "auth_code_url",
			Value: func(args ...tengo.Object) (tengo.Object, error) {
				if len(args) < 1 {
					return nil, tengo.ErrWrongNumArguments
				}
				authEndpoint, ok := args[0].(*tengo.String)
				if !ok {
					return nil, tengo.ErrInvalidArgumentType{Name: "auth_endpoint", Expected: "string", Found: args[0].TypeName()}
				}
				scope := ""
				if len(args) > 1 {
					if s, ok := args[1].(*tengo.String); ok {
						scope = s.Value
					}
				}
				state := ""
				if len(args) > 2 {
					if s, ok := args[2].(*tengo.String); ok {
						state = s.Value
					}
				}
				url, err := client.AuthCodeURL(authEndpoint.Value, scope, state)
				if err != nil {
					return &tengo.Error{Value: &tengo.String{Value: "auth_code_url error: " + err.Error()}}, nil
				}
				return &tengo.String{Value: url}, nil
			},
		},
		"exchange_code": &tengo.UserFunction{
			Name: "exchange_code",
			Value: func(args ...tengo.Object) (tengo.Object, error) {
				if len(args) < 2 {
					return nil, tengo.ErrWrongNumArguments
				}
				code, ok := args[0].(*tengo.String)
				if !ok {
					return nil, tengo.ErrInvalidArgumentType{Name: "code", Expected: "string", Found: args[0].TypeName()}
				}
				redirectURI, ok := args[1].(*tengo.String)
				if !ok {
					return nil, tengo.ErrInvalidArgumentType{Name: "redirect_uri", Expected: "string", Found: args[1].TypeName()}
				}
				clientSecret := ""
				if len(args) > 2 {
					if s, ok := args[2].(*tengo.String); ok {
						clientSecret = s.Value
					}
				}
				data, err := client.ExchangeCode(code.Value, redirectURI.Value, clientSecret)
				if err != nil {
					return &tengo.Error{Value: &tengo.String{Value: "exchange_code error: " + err.Error()}}, nil
				}
				return goMapToTengo(data)
			},
		},
		"attach_to": &tengo.UserFunction{
			Name: "attach_to",
			Value: func(args ...tengo.Object) (tengo.Object, error) {
				if len(args) < 1 {
					return nil, tengo.ErrWrongNumArguments
				}
				// Accept http client argument; underlying AttachToHTTPClient is a no-op
				_ = args[0]
				return tengo.TrueValue, nil
			},
		},
	}
}

func goMapToTengo(data map[string]interface{}) (tengo.Object, error) {
	m := make(map[string]tengo.Object, len(data))
	for k, v := range data {
		obj, err := tengo.FromInterface(v)
		if err != nil {
			m[k] = tengo.UndefinedValue
		} else {
			m[k] = obj
		}
	}
	return &tengo.ImmutableMap{Value: m}, nil
}
