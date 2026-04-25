// Package http provides Lua bindings for HTTP client functionality.
package http

import (
	"crypto/tls"
	"io"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"time"

	lua "github.com/yuin/gopher-lua"
	exportshttp "github.com/bubblegutz/nklhd/internal/exports/http"
)

// Register adds the http module to the given Lua state.
// The module provides two functions:
//   http.request(method, url, opts) -> result, error
//   http.client(baseURL, opts) -> rest_client userdata
func Register(L *lua.LState) {
	mod := L.RegisterModule("http", map[string]lua.LGFunction{
		"request": request,
		"client":  client,
	}).(*lua.LTable)
	L.SetGlobal("http", mod)
}

// request implements http.request(method, url, opts)
func request(L *lua.LState) int {
	method := strings.ToUpper(L.CheckString(1))
	urlStr := L.CheckString(2)
	opts := L.OptTable(3, L.NewTable())

	// Parse options
	timeout := 5 * time.Second
	if v := opts.RawGetString("timeout_ms"); v.Type() == lua.LTNumber {
		timeout = time.Duration(lua.LVAsNumber(v)) * time.Millisecond
	}
	var tlsCfg *tls.Config
	if v := opts.RawGetString("tls"); v.Type() == lua.LTTable {
		tt := v.(*lua.LTable)
		if t := tt.RawGetString("insecure_skip_verify"); lua.LVAsBool(t) {
			tlsCfg = &tls.Config{InsecureSkipVerify: true}
		}
	}
	var body io.Reader
	if b := opts.RawGetString("body"); b.Type() == lua.LTString {
		body = strings.NewReader(lua.LVAsString(b))
	}
	headers := make(map[string]string)
	if h := opts.RawGetString("headers"); h.Type() == lua.LTTable {
		hTbl := h.(*lua.LTable)
		hTbl.ForEach(func(k, v lua.LValue) {
			if k.Type() == lua.LTString && v.Type() == lua.LTString {
				headers[lua.LVAsString(k)] = lua.LVAsString(v)
			}
		})
	}
	var auth *exportshttp.Auth
	if a := opts.RawGetString("auth"); a.Type() == lua.LTTable {
		atbl := a.(*lua.LTable)
		auth = &exportshttp.Auth{}
		if t := atbl.RawGetString("type"); t.Type() == lua.LTString {
			auth.Type = lua.LVAsString(t)
		}
		if auth.Type == "basic" {
			if u := atbl.RawGetString("user"); u.Type() == lua.LTString {
				auth.Username = lua.LVAsString(u)
			}
			if p := atbl.RawGetString("pass"); p.Type() == lua.LTString {
				auth.Password = lua.LVAsString(p)
			}
		} else if auth.Type == "bearer" {
			if t := atbl.RawGetString("token"); t.Type() == lua.LTString {
				auth.Token = lua.LVAsString(t)
			}
		}
	}

	var jar http.CookieJar
	if v := opts.RawGetString("cookie_jar"); lua.LVAsBool(v) {
		j, err := cookiejar.New(nil)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString("failed to create cookie jar: " + err.Error()))
			return 2
		}
		jar = j
	}

	// Build options
	options := &exportshttp.Options{
		Timeout:   timeout,
		TLS:       tlsCfg,
		Headers:   headers,
		Body:      body,
		Auth:      auth,
		CookieJar: jar,
	}
	result, err := exportshttp.SimpleRequest(method, urlStr, options)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString("request error: " + err.Error()))
		return 2
	}
	// Convert result to Lua table
	resTbl := L.NewTable()
	resTbl.RawSetString("status", lua.LNumber(result.Status))
	resTbl.RawSetString("body", lua.LString(result.Body))
	hTbl := L.NewTable()
	for k, v := range result.Headers {
		hTbl.RawSetString(k, lua.LString(v))
	}
	resTbl.RawSetString("headers", hTbl)
	L.Push(resTbl)
	L.Push(lua.LNil)
	return 2
}

// client implements http.client(baseURL, opts) -> rest_client userdata
func client(L *lua.LState) int {
	base := L.CheckString(1)
	opts := L.OptTable(2, L.NewTable())

	timeout := 5 * time.Second
	if v := opts.RawGetString("timeout_ms"); v.Type() == lua.LTNumber {
		timeout = time.Duration(lua.LVAsNumber(v)) * time.Millisecond
	}
	var tlsCfg *tls.Config
	if v := opts.RawGetString("tls"); v.Type() == lua.LTTable {
		tt := v.(*lua.LTable)
		if t := tt.RawGetString("insecure_skip_verify"); lua.LVAsBool(t) {
			tlsCfg = &tls.Config{InsecureSkipVerify: true}
		}
	}
	var jar http.CookieJar
	if v := opts.RawGetString("cookie_jar"); lua.LVAsBool(v) {
		j, err := cookiejar.New(nil)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString("failed to create cookie jar: " + err.Error()))
			return 2
		}
		jar = j
	}
	options := &exportshttp.Options{
		Timeout:   timeout,
		TLS:       tlsCfg,
		CookieJar: jar,
	}
	c := exportshttp.NewClient(base, options)
	ud := L.NewUserData()
	ud.Value = c
	mt := L.NewTypeMetatable("rest_client")
	L.SetField(mt, "__index", L.SetFuncs(L.NewTable(), map[string]lua.LGFunction{
		"get":              restClientGet,
		"post":             restClientPost,
		"put":              restClientPut,
		"patch":            restClientPatch,
		"delete":           restClientDelete,
		"head":             restClientHead,
		"options":          restClientOptions,
		"withBasic":        restClientWithBasic,
		"withBearer":       restClientWithBearer,
		"withTLSInsecure":  restClientWithTLSInsecure,
	}))
	L.SetMetatable(ud, mt)
	L.Push(ud)
	return 1
}

func restClientGet(L *lua.LState) int {
	ud := L.CheckUserData(1)
	path := L.CheckString(2)
	opts := L.OptTable(3, L.NewTable())
	client := ud.Value.(*exportshttp.Client)

	headers := make(map[string]string)
	if h := opts.RawGetString("headers"); h.Type() == lua.LTTable {
		hTbl := h.(*lua.LTable)
		hTbl.ForEach(func(k, v lua.LValue) {
			if k.Type() == lua.LTString && v.Type() == lua.LTString {
				headers[lua.LVAsString(k)] = lua.LVAsString(v)
			}
		})
	}
	options := &exportshttp.Options{
		Headers: headers,
	}
	result, err := client.Do("GET", path, options)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString("GET error: " + err.Error()))
		return 2
	}
	resTbl := L.NewTable()
	resTbl.RawSetString("status", lua.LNumber(result.Status))
	resTbl.RawSetString("body", lua.LString(result.Body))
	hTbl := L.NewTable()
	for k, v := range result.Headers {
		hTbl.RawSetString(k, lua.LString(v))
	}
	resTbl.RawSetString("headers", hTbl)
	L.Push(resTbl)
	L.Push(lua.LNil)
	return 2
}

func restClientPost(L *lua.LState) int {
	ud := L.CheckUserData(1)
	path := L.CheckString(2)
	client := ud.Value.(*exportshttp.Client)
	rawBody := L.CheckAny(3)
	opts := L.OptTable(4, L.NewTable())

	var body io.Reader
	if rawBody.Type() == lua.LTTable {
		// Convert table to JSON using json.encode (available globally)
		jsonMod := L.GetGlobal("json")
		if jsonMod.Type() != lua.LTTable {
			L.Push(lua.LNil)
			L.Push(lua.LString("json module not available"))
			return 2
		}
		encodeFn := L.GetField(jsonMod, "encode")
		if encodeFn.Type() != lua.LTFunction {
			L.Push(lua.LNil)
			L.Push(lua.LString("json.encode not found"))
			return 2
		}
		L.Push(encodeFn)
		L.Push(rawBody)
		if err := L.PCall(1, 2, nil); err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString("json.encode error: " + err.Error()))
			return 2
		}
		jsonStr := L.Get(-2)
		errVal := L.Get(-1)
		L.Pop(2)
		if errVal.Type() != lua.LTNil {
			L.Push(lua.LNil)
			L.Push(lua.LString("json.encode error: " + errVal.String()))
			return 2
		}
		body = strings.NewReader(jsonStr.String())
		// Set Content-Type header automatically
		if opts.RawGetString("headers").Type() != lua.LTTable {
			opts.RawSetString("headers", L.NewTable())
		}
		h := opts.RawGetString("headers").(*lua.LTable)
		h.RawSetString("Content-Type", lua.LString("application/json"))
	} else if rawBody.Type() == lua.LTString {
		body = strings.NewReader(lua.LVAsString(rawBody))
	} else {
		L.Push(lua.LNil)
		L.Push(lua.LString("body must be string or table"))
		return 2
	}

	headers := make(map[string]string)
	if h := opts.RawGetString("headers"); h.Type() == lua.LTTable {
		hTbl := h.(*lua.LTable)
		hTbl.ForEach(func(k, v lua.LValue) {
			if k.Type() == lua.LTString && v.Type() == lua.LTString {
				headers[lua.LVAsString(k)] = lua.LVAsString(v)
			}
		})
	}
	options := &exportshttp.Options{
		Headers: headers,
		Body:    body,
	}
	result, err := client.Do("POST", path, options)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString("POST error: " + err.Error()))
		return 2
	}
	resTbl := L.NewTable()
	resTbl.RawSetString("status", lua.LNumber(result.Status))
	resTbl.RawSetString("body", lua.LString(result.Body))
	hTbl := L.NewTable()
	for k, v := range result.Headers {
		hTbl.RawSetString(k, lua.LString(v))
	}
	resTbl.RawSetString("headers", hTbl)
	L.Push(resTbl)
	L.Push(lua.LNil)
	return 2
}

func restClientPut(L *lua.LState) int {
	ud := L.CheckUserData(1)
	path := L.CheckString(2)
	client := ud.Value.(*exportshttp.Client)
	rawBody := L.CheckAny(3)
	opts := L.OptTable(4, L.NewTable())

	var body io.Reader
	if rawBody.Type() == lua.LTTable {
		// Convert table to JSON using json.encode (available globally)
		jsonMod := L.GetGlobal("json")
		if jsonMod.Type() != lua.LTTable {
			L.Push(lua.LNil)
			L.Push(lua.LString("json module not available"))
			return 2
		}
		encodeFn := L.GetField(jsonMod, "encode")
		if encodeFn.Type() != lua.LTFunction {
			L.Push(lua.LNil)
			L.Push(lua.LString("json.encode not found"))
			return 2
		}
		L.Push(encodeFn)
		L.Push(rawBody)
		if err := L.PCall(1, 2, nil); err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString("json.encode error: " + err.Error()))
			return 2
		}
		jsonStr := L.Get(-2)
		errVal := L.Get(-1)
		L.Pop(2)
		if errVal.Type() != lua.LTNil {
			L.Push(lua.LNil)
			L.Push(lua.LString("json.encode error: " + errVal.String()))
			return 2
		}
		body = strings.NewReader(jsonStr.String())
		// Set Content-Type header automatically
		if opts.RawGetString("headers").Type() != lua.LTTable {
			opts.RawSetString("headers", L.NewTable())
		}
		h := opts.RawGetString("headers").(*lua.LTable)
		h.RawSetString("Content-Type", lua.LString("application/json"))
	} else if rawBody.Type() == lua.LTString {
		body = strings.NewReader(lua.LVAsString(rawBody))
	} else {
		L.Push(lua.LNil)
		L.Push(lua.LString("body must be string or table"))
		return 2
	}

	headers := make(map[string]string)
	if h := opts.RawGetString("headers"); h.Type() == lua.LTTable {
		hTbl := h.(*lua.LTable)
		hTbl.ForEach(func(k, v lua.LValue) {
			if k.Type() == lua.LTString && v.Type() == lua.LTString {
				headers[lua.LVAsString(k)] = lua.LVAsString(v)
			}
		})
	}
	options := &exportshttp.Options{
		Headers: headers,
		Body:    body,
	}
	result, err := client.Do("PUT", path, options)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString("PUT error: " + err.Error()))
		return 2
	}
	resTbl := L.NewTable()
	resTbl.RawSetString("status", lua.LNumber(result.Status))
	resTbl.RawSetString("body", lua.LString(result.Body))
	hTbl := L.NewTable()
	for k, v := range result.Headers {
		hTbl.RawSetString(k, lua.LString(v))
	}
	resTbl.RawSetString("headers", hTbl)
	L.Push(resTbl)
	L.Push(lua.LNil)
	return 2
}

func restClientPatch(L *lua.LState) int {
	ud := L.CheckUserData(1)
	path := L.CheckString(2)
	client := ud.Value.(*exportshttp.Client)
	rawBody := L.CheckAny(3)
	opts := L.OptTable(4, L.NewTable())

	var body io.Reader
	if rawBody.Type() == lua.LTTable {
		// Convert table to JSON using json.encode (available globally)
		jsonMod := L.GetGlobal("json")
		if jsonMod.Type() != lua.LTTable {
			L.Push(lua.LNil)
			L.Push(lua.LString("json module not available"))
			return 2
		}
		encodeFn := L.GetField(jsonMod, "encode")
		if encodeFn.Type() != lua.LTFunction {
			L.Push(lua.LNil)
			L.Push(lua.LString("json.encode not found"))
			return 2
		}
		L.Push(encodeFn)
		L.Push(rawBody)
		if err := L.PCall(1, 2, nil); err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString("json.encode error: " + err.Error()))
			return 2
		}
		jsonStr := L.Get(-2)
		errVal := L.Get(-1)
		L.Pop(2)
		if errVal.Type() != lua.LTNil {
			L.Push(lua.LNil)
			L.Push(lua.LString("json.encode error: " + errVal.String()))
			return 2
		}
		body = strings.NewReader(jsonStr.String())
		// Set Content-Type header automatically
		if opts.RawGetString("headers").Type() != lua.LTTable {
			opts.RawSetString("headers", L.NewTable())
		}
		h := opts.RawGetString("headers").(*lua.LTable)
		h.RawSetString("Content-Type", lua.LString("application/json"))
	} else if rawBody.Type() == lua.LTString {
		body = strings.NewReader(lua.LVAsString(rawBody))
	} else {
		L.Push(lua.LNil)
		L.Push(lua.LString("body must be string or table"))
		return 2
	}

	headers := make(map[string]string)
	if h := opts.RawGetString("headers"); h.Type() == lua.LTTable {
		hTbl := h.(*lua.LTable)
		hTbl.ForEach(func(k, v lua.LValue) {
			if k.Type() == lua.LTString && v.Type() == lua.LTString {
				headers[lua.LVAsString(k)] = lua.LVAsString(v)
			}
		})
	}
	options := &exportshttp.Options{
		Headers: headers,
		Body:    body,
	}
	result, err := client.Do("PATCH", path, options)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString("PATCH error: " + err.Error()))
		return 2
	}
	resTbl := L.NewTable()
	resTbl.RawSetString("status", lua.LNumber(result.Status))
	resTbl.RawSetString("body", lua.LString(result.Body))
	hTbl := L.NewTable()
	for k, v := range result.Headers {
		hTbl.RawSetString(k, lua.LString(v))
	}
	resTbl.RawSetString("headers", hTbl)
	L.Push(resTbl)
	L.Push(lua.LNil)
	return 2
}

func restClientDelete(L *lua.LState) int {
	ud := L.CheckUserData(1)
	path := L.CheckString(2)
	opts := L.OptTable(3, L.NewTable())
	client := ud.Value.(*exportshttp.Client)

	headers := make(map[string]string)
	if h := opts.RawGetString("headers"); h.Type() == lua.LTTable {
		hTbl := h.(*lua.LTable)
		hTbl.ForEach(func(k, v lua.LValue) {
			if k.Type() == lua.LTString && v.Type() == lua.LTString {
				headers[lua.LVAsString(k)] = lua.LVAsString(v)
			}
		})
	}
	options := &exportshttp.Options{
		Headers: headers,
	}
	result, err := client.Do("DELETE", path, options)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString("DELETE error: " + err.Error()))
		return 2
	}
	resTbl := L.NewTable()
	resTbl.RawSetString("status", lua.LNumber(result.Status))
	resTbl.RawSetString("body", lua.LString(result.Body))
	hTbl := L.NewTable()
	for k, v := range result.Headers {
		hTbl.RawSetString(k, lua.LString(v))
	}
	resTbl.RawSetString("headers", hTbl)
	L.Push(resTbl)
	L.Push(lua.LNil)
	return 2
}

func restClientHead(L *lua.LState) int {
	ud := L.CheckUserData(1)
	path := L.CheckString(2)
	opts := L.OptTable(3, L.NewTable())
	client := ud.Value.(*exportshttp.Client)

	headers := make(map[string]string)
	if h := opts.RawGetString("headers"); h.Type() == lua.LTTable {
		hTbl := h.(*lua.LTable)
		hTbl.ForEach(func(k, v lua.LValue) {
			if k.Type() == lua.LTString && v.Type() == lua.LTString {
				headers[lua.LVAsString(k)] = lua.LVAsString(v)
			}
		})
	}
	options := &exportshttp.Options{
		Headers: headers,
	}
	result, err := client.Do("HEAD", path, options)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString("HEAD error: " + err.Error()))
		return 2
	}
	resTbl := L.NewTable()
	resTbl.RawSetString("status", lua.LNumber(result.Status))
	resTbl.RawSetString("body", lua.LString(result.Body))
	hTbl := L.NewTable()
	for k, v := range result.Headers {
		hTbl.RawSetString(k, lua.LString(v))
	}
	resTbl.RawSetString("headers", hTbl)
	L.Push(resTbl)
	L.Push(lua.LNil)
	return 2
}

func restClientOptions(L *lua.LState) int {
	ud := L.CheckUserData(1)
	path := L.CheckString(2)
	opts := L.OptTable(3, L.NewTable())
	client := ud.Value.(*exportshttp.Client)

	headers := make(map[string]string)
	if h := opts.RawGetString("headers"); h.Type() == lua.LTTable {
		hTbl := h.(*lua.LTable)
		hTbl.ForEach(func(k, v lua.LValue) {
			if k.Type() == lua.LTString && v.Type() == lua.LTString {
				headers[lua.LVAsString(k)] = lua.LVAsString(v)
			}
		})
	}
	options := &exportshttp.Options{
		Headers: headers,
	}
	result, err := client.Do("OPTIONS", path, options)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString("OPTIONS error: " + err.Error()))
		return 2
	}
	resTbl := L.NewTable()
	resTbl.RawSetString("status", lua.LNumber(result.Status))
	resTbl.RawSetString("body", lua.LString(result.Body))
	hTbl := L.NewTable()
	for k, v := range result.Headers {
		hTbl.RawSetString(k, lua.LString(v))
	}
	resTbl.RawSetString("headers", hTbl)
	L.Push(resTbl)
	L.Push(lua.LNil)
	return 2
}

func restClientWithBasic(L *lua.LState) int {
	ud := L.CheckUserData(1)
	user := L.CheckString(2)
	pass := L.CheckString(3)
	client := ud.Value.(*exportshttp.Client)
	// Update client's auth header
	client.AuthHeader = "Basic " + exportshttp.BasicAuth(user, pass)
	L.Push(lua.LTrue)
	return 1
}

func restClientWithBearer(L *lua.LState) int {
	ud := L.CheckUserData(1)
	token := L.CheckString(2)
	client := ud.Value.(*exportshttp.Client)
	client.AuthHeader = "Bearer " + token
	L.Push(lua.LTrue)
	return 1
}

func restClientWithTLSInsecure(L *lua.LState) int {
	ud := L.CheckUserData(1)
	inq := L.CheckBool(2)
	client := ud.Value.(*exportshttp.Client)
	if tr, ok := client.HTTPClient.Transport.(*http.Transport); ok {
		if tr.TLSClientConfig == nil {
			tr.TLSClientConfig = &tls.Config{}
		}
		tr.TLSClientConfig.InsecureSkipVerify = inq
	}
	L.Push(lua.LTrue)
	return 1
}

