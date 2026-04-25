// Package http provides Tengo bindings for HTTP client functionality.
package http

import (
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"time"

	exportshttp "github.com/bubblegutz/nklhd/internal/exports/http"
	"github.com/d5/tengo/v2"
)

// Module is the Tengo module map for http.
var Module = map[string]tengo.Object{
	"request": &tengo.UserFunction{
		Name: "request",
		Value: func(args ...tengo.Object) (tengo.Object, error) {
			if len(args) < 2 {
				return nil, tengo.ErrWrongNumArguments
			}
			method, ok := args[0].(*tengo.String)
			if !ok {
				return nil, tengo.ErrInvalidArgumentType{Name: "method", Expected: "string", Found: args[0].TypeName()}
			}
			urlStr, ok := args[1].(*tengo.String)
			if !ok {
				return nil, tengo.ErrInvalidArgumentType{Name: "url", Expected: "string", Found: args[1].TypeName()}
			}

			opts := parseOptions(args, 2)
			result, err := exportshttp.SimpleRequest(strings.ToUpper(method.Value), urlStr.Value, opts)
			if err != nil {
				return &tengo.Error{Value: &tengo.String{Value: "request error: " + err.Error()}}, nil
			}
			return responseResultToTengo(result), nil
		},
	},
	"client": &tengo.UserFunction{
		Name: "client",
		Value: func(args ...tengo.Object) (tengo.Object, error) {
			if len(args) < 1 {
				return nil, tengo.ErrWrongNumArguments
			}
			base, ok := args[0].(*tengo.String)
			if !ok {
				return nil, tengo.ErrInvalidArgumentType{Name: "baseURL", Expected: "string", Found: args[0].TypeName()}
			}
			opts := parseOptions(args, 1)
			client := exportshttp.NewClient(base.Value, opts)

			return &tengo.ImmutableMap{Value: clientMethods(client)}, nil
		},
	},
}

func parseOptions(args []tengo.Object, startIdx int) *exportshttp.Options {
	opts := &exportshttp.Options{
		Timeout: 5 * time.Second,
	}
	if len(args) <= startIdx {
		return opts
	}

	var m *tengo.ImmutableMap
	if im, ok := args[startIdx].(*tengo.ImmutableMap); ok {
		m = im
	} else if rm, ok := args[startIdx].(*tengo.Map); ok {
		m = &tengo.ImmutableMap{Value: rm.Value}
	} else {
		return opts
	}

	if t, ok := getMapInt(m, "timeout_ms"); ok {
		opts.Timeout = time.Duration(t) * time.Millisecond
	}

	if h, ok := m.Value["headers"]; ok {
		if hm, ok := h.(*tengo.ImmutableMap); ok {
			headers := make(map[string]string, len(hm.Value))
			for k, v := range hm.Value {
				if s, ok := v.(*tengo.String); ok {
					headers[k] = s.Value
				}
			}
			opts.Headers = headers
		} else if rm, ok := h.(*tengo.Map); ok {
			headers := make(map[string]string, len(rm.Value))
			for k, v := range rm.Value {
				if s, ok := v.(*tengo.String); ok {
					headers[k] = s.Value
				}
			}
			opts.Headers = headers
		}
	}

	if b, ok := m.Value["body"]; ok {
		switch body := b.(type) {
		case *tengo.String:
			opts.Body = strings.NewReader(body.Value)
		case *tengo.Map, *tengo.ImmutableMap:
			// Auto-encode map to JSON
			v := tengo.ToInterface(b)
			jsonBytes, err := json.Marshal(v)
			if err == nil {
				opts.Body = strings.NewReader(string(jsonBytes))
				if opts.Headers == nil {
					opts.Headers = make(map[string]string)
				}
				if _, ok := opts.Headers["Content-Type"]; !ok {
					opts.Headers["Content-Type"] = "application/json"
				}
			}
		}
	}

	if t, ok := m.Value["tls"]; ok {
		if tm, ok := t.(*tengo.ImmutableMap); ok {
			if v, ok := tm.Value["insecure_skip_verify"]; ok {
				if b, ok := v.(*tengo.Bool); ok && !b.IsFalsy() {
					opts.TLS = &tls.Config{InsecureSkipVerify: true}
				}
			}
		} else if rm, ok := t.(*tengo.Map); ok {
			if v, ok := rm.Value["insecure_skip_verify"]; ok {
				if b, ok := v.(*tengo.Bool); ok && !b.IsFalsy() {
					opts.TLS = &tls.Config{InsecureSkipVerify: true}
				}
			}
		}
	}

	if a, ok := m.Value["auth"]; ok {
		if am, ok := a.(*tengo.ImmutableMap); ok {
			if authType := getMapString(am, "type"); authType == "basic" {
				opts.Auth = &exportshttp.Auth{
					Type:     "basic",
					Username: getMapString(am, "user"),
					Password: getMapString(am, "pass"),
				}
			} else if authType == "bearer" {
				opts.Auth = &exportshttp.Auth{
					Type:  "bearer",
					Token: getMapString(am, "token"),
				}
			}
		} else if rm, ok := a.(*tengo.Map); ok {
			imm := &tengo.ImmutableMap{Value: rm.Value}
			if authType := getMapString(imm, "type"); authType == "basic" {
				opts.Auth = &exportshttp.Auth{
					Type:     "basic",
					Username: getMapString(imm, "user"),
					Password: getMapString(imm, "pass"),
				}
			} else if authType == "bearer" {
				opts.Auth = &exportshttp.Auth{
					Type:  "bearer",
					Token: getMapString(imm, "token"),
				}
			}
		}
	}

	if v, ok := m.Value["cookie_jar"]; ok {
		if b, ok := v.(*tengo.Bool); ok && !b.IsFalsy() {
			jar, err := cookiejar.New(nil)
			if err == nil {
				opts.CookieJar = jar
			}
		}
	}

	return opts
}

// clientMethods returns the methods map for an HTTP client.
func clientMethods(client *exportshttp.Client) map[string]tengo.Object {
	return map[string]tengo.Object{
		"get": &tengo.UserFunction{
			Name: "get",
			Value: func(args ...tengo.Object) (tengo.Object, error) {
				if len(args) < 1 {
					return nil, tengo.ErrWrongNumArguments
				}
				path, ok := args[0].(*tengo.String)
				if !ok {
					return nil, tengo.ErrInvalidArgumentType{Name: "path", Expected: "string", Found: args[0].TypeName()}
				}
				opts := parseClientOptions(args, 1)
				result, err := client.Do("GET", path.Value, opts)
				if err != nil {
					return &tengo.Error{Value: &tengo.String{Value: "GET error: " + err.Error()}}, nil
				}
				return responseResultToTengo(result), nil
			},
		},
		"post": &tengo.UserFunction{
			Name: "post",
			Value: func(args ...tengo.Object) (tengo.Object, error) {
				if len(args) < 2 {
					return nil, tengo.ErrWrongNumArguments
				}
				path, ok := args[0].(*tengo.String)
				if !ok {
					return nil, tengo.ErrInvalidArgumentType{Name: "path", Expected: "string", Found: args[0].TypeName()}
				}
				opts := &exportshttp.Options{}
				bodyArg := args[1]
				switch body := bodyArg.(type) {
				case *tengo.String:
					opts.Body = strings.NewReader(body.Value)
				case *tengo.Map, *tengo.ImmutableMap:
					v := tengo.ToInterface(bodyArg)
					jsonBytes, err := json.Marshal(v)
					if err == nil {
						opts.Body = strings.NewReader(string(jsonBytes))
						if opts.Headers == nil {
							opts.Headers = make(map[string]string)
						}
						opts.Headers["Content-Type"] = "application/json"
					}
				}
				// Merge additional options
				if extraOpts := parseClientOptions(args, 2); extraOpts != nil {
					if extraOpts.Headers != nil {
						if opts.Headers == nil {
							opts.Headers = extraOpts.Headers
						} else {
							for k, v := range extraOpts.Headers {
								opts.Headers[k] = v
							}
						}
					}
				}
				result, err := client.Do("POST", path.Value, opts)
				if err != nil {
					return &tengo.Error{Value: &tengo.String{Value: "POST error: " + err.Error()}}, nil
				}
				return responseResultToTengo(result), nil
			},
		},
		"put": &tengo.UserFunction{
			Name: "put",
			Value: func(args ...tengo.Object) (tengo.Object, error) {
				if len(args) < 2 {
					return nil, tengo.ErrWrongNumArguments
				}
				path, ok := args[0].(*tengo.String)
				if !ok {
					return nil, tengo.ErrInvalidArgumentType{Name: "path", Expected: "string", Found: args[0].TypeName()}
				}
				opts := &exportshttp.Options{}
				bodyArg := args[1]
				switch body := bodyArg.(type) {
				case *tengo.String:
					opts.Body = strings.NewReader(body.Value)
				case *tengo.Map, *tengo.ImmutableMap:
					v := tengo.ToInterface(bodyArg)
					jsonBytes, err := json.Marshal(v)
					if err == nil {
						opts.Body = strings.NewReader(string(jsonBytes))
						if opts.Headers == nil {
							opts.Headers = make(map[string]string)
						}
						opts.Headers["Content-Type"] = "application/json"
					}
				}
				if extraOpts := parseClientOptions(args, 2); extraOpts != nil {
					if extraOpts.Headers != nil {
						if opts.Headers == nil {
							opts.Headers = extraOpts.Headers
						} else {
							for k, v := range extraOpts.Headers {
								opts.Headers[k] = v
							}
						}
					}
				}
				result, err := client.Do("PUT", path.Value, opts)
				if err != nil {
					return &tengo.Error{Value: &tengo.String{Value: "PUT error: " + err.Error()}}, nil
				}
				return responseResultToTengo(result), nil
			},
		},
		"patch": &tengo.UserFunction{
			Name: "patch",
			Value: func(args ...tengo.Object) (tengo.Object, error) {
				if len(args) < 2 {
					return nil, tengo.ErrWrongNumArguments
				}
				path, ok := args[0].(*tengo.String)
				if !ok {
					return nil, tengo.ErrInvalidArgumentType{Name: "path", Expected: "string", Found: args[0].TypeName()}
				}
				opts := &exportshttp.Options{}
				bodyArg := args[1]
				switch body := bodyArg.(type) {
				case *tengo.String:
					opts.Body = strings.NewReader(body.Value)
				case *tengo.Map, *tengo.ImmutableMap:
					v := tengo.ToInterface(bodyArg)
					jsonBytes, err := json.Marshal(v)
					if err == nil {
						opts.Body = strings.NewReader(string(jsonBytes))
						if opts.Headers == nil {
							opts.Headers = make(map[string]string)
						}
						opts.Headers["Content-Type"] = "application/json"
					}
				}
				if extraOpts := parseClientOptions(args, 2); extraOpts != nil {
					if extraOpts.Headers != nil {
						if opts.Headers == nil {
							opts.Headers = extraOpts.Headers
						} else {
							for k, v := range extraOpts.Headers {
								opts.Headers[k] = v
							}
						}
					}
				}
				result, err := client.Do("PATCH", path.Value, opts)
				if err != nil {
					return &tengo.Error{Value: &tengo.String{Value: "PATCH error: " + err.Error()}}, nil
				}
				return responseResultToTengo(result), nil
			},
		},
		"delete": &tengo.UserFunction{
			Name: "delete",
			Value: func(args ...tengo.Object) (tengo.Object, error) {
				if len(args) < 1 {
					return nil, tengo.ErrWrongNumArguments
				}
				path, ok := args[0].(*tengo.String)
				if !ok {
					return nil, tengo.ErrInvalidArgumentType{Name: "path", Expected: "string", Found: args[0].TypeName()}
				}
				opts := parseClientOptions(args, 1)
				result, err := client.Do("DELETE", path.Value, opts)
				if err != nil {
					return &tengo.Error{Value: &tengo.String{Value: "DELETE error: " + err.Error()}}, nil
				}
				return responseResultToTengo(result), nil
			},
		},
		"head": &tengo.UserFunction{
			Name: "head",
			Value: func(args ...tengo.Object) (tengo.Object, error) {
				if len(args) < 1 {
					return nil, tengo.ErrWrongNumArguments
				}
				path, ok := args[0].(*tengo.String)
				if !ok {
					return nil, tengo.ErrInvalidArgumentType{Name: "path", Expected: "string", Found: args[0].TypeName()}
				}
				opts := parseClientOptions(args, 1)
				result, err := client.Do("HEAD", path.Value, opts)
				if err != nil {
					return &tengo.Error{Value: &tengo.String{Value: "HEAD error: " + err.Error()}}, nil
				}
				return responseResultToTengo(result), nil
			},
		},
		"options": &tengo.UserFunction{
			Name: "options",
			Value: func(args ...tengo.Object) (tengo.Object, error) {
				if len(args) < 1 {
					return nil, tengo.ErrWrongNumArguments
				}
				path, ok := args[0].(*tengo.String)
				if !ok {
					return nil, tengo.ErrInvalidArgumentType{Name: "path", Expected: "string", Found: args[0].TypeName()}
				}
				opts := parseClientOptions(args, 1)
				result, err := client.Do("OPTIONS", path.Value, opts)
				if err != nil {
					return &tengo.Error{Value: &tengo.String{Value: "OPTIONS error: " + err.Error()}}, nil
				}
				return responseResultToTengo(result), nil
			},
		},
		"withBasic": &tengo.UserFunction{
			Name: "withBasic",
			Value: func(args ...tengo.Object) (tengo.Object, error) {
				if len(args) < 2 {
					return nil, tengo.ErrWrongNumArguments
				}
				user, ok := args[0].(*tengo.String)
				if !ok {
					return nil, tengo.ErrInvalidArgumentType{Name: "user", Expected: "string", Found: args[0].TypeName()}
				}
				pass, ok := args[1].(*tengo.String)
				if !ok {
					return nil, tengo.ErrInvalidArgumentType{Name: "pass", Expected: "string", Found: args[1].TypeName()}
				}
				client.AuthHeader = "Basic " + exportshttp.BasicAuth(user.Value, pass.Value)
				return tengo.UndefinedValue, nil
			},
		},
		"withBearer": &tengo.UserFunction{
			Name: "withBearer",
			Value: func(args ...tengo.Object) (tengo.Object, error) {
				if len(args) < 1 {
					return nil, tengo.ErrWrongNumArguments
				}
				token, ok := args[0].(*tengo.String)
				if !ok {
					return nil, tengo.ErrInvalidArgumentType{Name: "token", Expected: "string", Found: args[0].TypeName()}
				}
				client.AuthHeader = "Bearer " + token.Value
				return tengo.UndefinedValue, nil
			},
		},
		"withTLSInsecure": &tengo.UserFunction{
			Name: "withTLSInsecure",
			Value: func(args ...tengo.Object) (tengo.Object, error) {
				skip := true
				if len(args) > 0 {
					if b, ok := args[0].(*tengo.Bool); ok {
						skip = !b.IsFalsy()
					}
				}
				if tr, ok := client.HTTPClient.Transport.(*http.Transport); ok {
					if tr.TLSClientConfig == nil {
						tr.TLSClientConfig = &tls.Config{}
					}
					tr.TLSClientConfig.InsecureSkipVerify = skip
				}
				return tengo.UndefinedValue, nil
			},
		},
	}
}

func parseClientOptions(args []tengo.Object, startIdx int) *exportshttp.Options {
	if len(args) <= startIdx {
		return nil
	}
	var m *tengo.ImmutableMap
	if im, ok := args[startIdx].(*tengo.ImmutableMap); ok {
		m = im
	} else if rm, ok := args[startIdx].(*tengo.Map); ok {
		m = &tengo.ImmutableMap{Value: rm.Value}
	} else {
		return nil
	}

	opts := &exportshttp.Options{}
	if h, ok := m.Value["headers"]; ok {
		if hm, ok := h.(*tengo.ImmutableMap); ok {
			headers := make(map[string]string, len(hm.Value))
			for k, v := range hm.Value {
				if s, ok := v.(*tengo.String); ok {
					headers[k] = s.Value
				}
			}
			opts.Headers = headers
		} else if rm, ok := h.(*tengo.Map); ok {
			headers := make(map[string]string, len(rm.Value))
			for k, v := range rm.Value {
				if s, ok := v.(*tengo.String); ok {
					headers[k] = s.Value
				}
			}
			opts.Headers = headers
		}
	}
	return opts
}

func responseResultToTengo(result *exportshttp.ResponseResult) tengo.Object {
	m := make(map[string]tengo.Object)
	m["status"] = &tengo.Int{Value: int64(result.Status)}
	m["body"] = &tengo.String{Value: result.Body}
	headers := make(map[string]tengo.Object, len(result.Headers))
	for k, v := range result.Headers {
		headers[k] = &tengo.String{Value: v}
	}
	m["headers"] = &tengo.ImmutableMap{Value: headers}
	return &tengo.ImmutableMap{Value: m}
}

func getMapString(m *tengo.ImmutableMap, key string) string {
	if v, ok := m.Value[key]; ok {
		if s, ok := v.(*tengo.String); ok {
			return s.Value
		}
	}
	return ""
}

func getMapInt(m *tengo.ImmutableMap, key string) (int64, bool) {
	if v, ok := m.Value[key]; ok {
		if i, ok := v.(*tengo.Int); ok {
			return i.Value, true
		}
	}
	return 0, false
}
