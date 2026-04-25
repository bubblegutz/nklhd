// Package oauth provides Lua bindings for OAuth 2.0 client functionality.
package oauth

import (
	"crypto/tls"
	"time"
	lua "github.com/yuin/gopher-lua"
	exportsoauth "github.com/bubblegutz/nklhd/internal/exports/oauth"
)

// Register adds the oauth module to the given Lua state.
// The module provides one function:
//   oauth.client(config) -> oauth_client userdata
func Register(L *lua.LState) {
	mod := L.RegisterModule("oauth", map[string]lua.LGFunction{
		"client": clientConstructor,
	}).(*lua.LTable)
	L.SetGlobal("oauth", mod)
	
	// Register oauth_client metatable
	mt := L.NewTypeMetatable("oauth_client")
	L.SetField(mt, "__index", L.SetFuncs(L.NewTable(), map[string]lua.LGFunction{
		"device_flow_start": deviceFlowStart,
		"device_poll":       devicePoll,
		"auth_code_url":     authCodeURL,
		"exchange_code":     exchangeCode,
		"attach_to":         attachToClient,
	}))
}

func clientConstructor(L *lua.LState) int {
	cfg := L.CheckTable(1)
	cid := cfg.RawGetString("client_id").String()
	tokenURL := cfg.RawGetString("token_url").String()
	deviceURL := cfg.RawGetString("device_url").String()
	timeout := 5 * time.Second
	if t := cfg.RawGetString("timeout_ms"); t.Type() == lua.LTNumber {
		timeout = time.Duration(lua.LVAsNumber(t)) * time.Millisecond
	}
	var tlsCfg *tls.Config
	if t := cfg.RawGetString("tls"); t.Type() == lua.LTTable {
		tt := t.(*lua.LTable)
		if v := tt.RawGetString("insecure_skip_verify"); lua.LVAsBool(v) {
			tlsCfg = &tls.Config{InsecureSkipVerify: true}
		}
	}
	config := &exportsoauth.Config{
		ClientID:  cid,
		TokenURL:  tokenURL,
		DeviceURL: deviceURL,
		Timeout:   timeout,
		TLS:       tlsCfg,
	}
	client := exportsoauth.NewClient(config)
	ud := L.NewUserData()
	ud.Value = client
	L.SetMetatable(ud, L.GetTypeMetatable("oauth_client"))
	L.Push(ud)
	return 1
}

func deviceFlowStart(L *lua.LState) int {
	ud := L.CheckUserData(1)
	client := ud.Value.(*exportsoauth.Client)
	scope := L.OptString(2, "")
	data, err := client.DeviceFlowStart(scope)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString("device flow start error: " + err.Error()))
		return 2
	}
	tbl := L.NewTable()
	for k, v := range data {
		setLuaValue(L, tbl, k, v)
	}
	L.Push(tbl)
	L.Push(lua.LNil)
	return 2
}

func devicePoll(L *lua.LState) int {
	ud := L.CheckUserData(1)
	client := ud.Value.(*exportsoauth.Client)
	deviceCode := L.CheckString(2)
	interval := L.OptInt(3, 5)
	maxAttempts := L.OptInt(4, 12)
	data, err := client.DevicePoll(deviceCode, interval, maxAttempts)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString("device poll error: " + err.Error()))
		return 2
	}
	tbl := L.NewTable()
	for k, v := range data {
		setLuaValue(L, tbl, k, v)
	}
	L.Push(tbl)
	L.Push(lua.LNil)
	return 2
}

func authCodeURL(L *lua.LState) int {
	ud := L.CheckUserData(1)
	client := ud.Value.(*exportsoauth.Client)
	authEndpoint := L.CheckString(2)
	scope := L.OptString(3, "")
	state := L.OptString(4, "")
	url, err := client.AuthCodeURL(authEndpoint, scope, state)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString("auth code URL error: " + err.Error()))
		return 2
	}
	L.Push(lua.LString(url))
	L.Push(lua.LNil)
	return 2
}

func exchangeCode(L *lua.LState) int {
	ud := L.CheckUserData(1)
	client := ud.Value.(*exportsoauth.Client)
	code := L.CheckString(2)
	redirect := L.CheckString(3)
	clientSecret := L.OptString(4, "")
	data, err := client.ExchangeCode(code, redirect, clientSecret)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString("exchange code error: " + err.Error()))
		return 2
	}
	tbl := L.NewTable()
	for k, v := range data {
		setLuaValue(L, tbl, k, v)
	}
	L.Push(tbl)
	L.Push(lua.LNil)
	return 2
}

func attachToClient(L *lua.LState) int {
	ud := L.CheckUserData(1)
	_ = ud.Value.(*exportsoauth.Client)
	_ = L.CheckUserData(2)
	// Expecting a rest_client (http client userdata)
	// For now, just return true; attachment logic would be implemented later.
	L.Push(lua.LTrue)
	return 1
}

func setLuaValue(L *lua.LState, tbl *lua.LTable, key string, val interface{}) {
	switch v := val.(type) {
	case string:
		tbl.RawSetString(key, lua.LString(v))
	case int:
		tbl.RawSetString(key, lua.LNumber(v))
	case float64:
		tbl.RawSetString(key, lua.LNumber(v))
	case bool:
		tbl.RawSetString(key, lua.LBool(v))
	case nil:
		tbl.RawSetString(key, lua.LNil)
	default:
		// Unsupported type, set as nil
		tbl.RawSetString(key, lua.LNil)
	}
}