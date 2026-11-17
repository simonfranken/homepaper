package api

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// fakeHALogin plays Home Assistant's login side: the token endpoint trades
// a code for tokens, the WebSocket says whose token it is, and revocations
// are counted. Code "admin" belongs to an administrator, "user" to a
// regular user.
func fakeHALogin(t *testing.T, revoked *atomic.Int32, gotClientID *atomic.Value) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/auth/token":
			_ = r.ParseForm()
			if r.Form.Get("grant_type") != "authorization_code" || (r.Form.Get("code") != "admin" && r.Form.Get("code") != "user") {
				http.Error(w, "invalid", http.StatusBadRequest)
				return
			}
			gotClientID.Store(r.Form.Get("client_id"))
			writeJSON(w, 200, map[string]any{"access_token": "acc-" + r.Form.Get("code"), "refresh_token": "ref-" + r.Form.Get("code"), "expires_in": 1800})
		case "/auth/revoke":
			_ = r.ParseForm()
			if strings.HasPrefix(r.Form.Get("token"), "ref-") {
				revoked.Add(1)
			}
		case "/api/websocket":
			conn, err := websocket.Accept(w, r, nil)
			if err != nil {
				return
			}
			defer conn.CloseNow()
			ctx := r.Context()
			_ = wsjson.Write(ctx, conn, map[string]string{"type": "auth_required"})
			var auth struct {
				Token string `json:"access_token"`
			}
			if wsjson.Read(ctx, conn, &auth) != nil || !strings.HasPrefix(auth.Token, "acc-") {
				_ = wsjson.Write(ctx, conn, map[string]string{"type": "auth_invalid"})
				return
			}
			_ = wsjson.Write(ctx, conn, map[string]string{"type": "auth_ok"})
			var m struct {
				ID   int    `json:"id"`
				Type string `json:"type"`
			}
			if wsjson.Read(ctx, conn, &m) != nil || m.Type != "auth/current_user" {
				return
			}
			admin := auth.Token == "acc-admin"
			name := map[bool]string{true: "Simon", false: "Guest"}[admin]
			_ = wsjson.Write(ctx, conn, map[string]any{"id": m.ID, "type": "result", "success": true,
				"result": map[string]any{"id": "u-" + name, "name": name, "is_owner": false, "is_admin": admin}})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestLoginViaHomeAssistant(t *testing.T) {
	var revoked atomic.Int32
	var clientID atomic.Value
	fake := fakeHALogin(t, &revoked, &clientID)
	a, _ := newTestAPI(t)
	a.Auth = &Auth{HAURL: fake.URL, PublicHAURL: "https://ha.example.org", AdminsOnly: true,
		Key: []byte("0123456789abcdef0123456789abcdef"), HTTP: fake.Client()}
	h := a.Handler()

	do := func(method, target string, cookies []*http.Cookie, hdr map[string]string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, target, nil)
		req.Host = "homepaper.example.org"
		for k, v := range hdr {
			req.Header.Set(k, v)
		}
		for _, c := range cookies {
			req.AddCookie(c)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	proxied := map[string]string{"X-Forwarded-Proto": "https"}

	// Without a session: the API says 401, pages go to the login.
	if rec := do("GET", "/api/devices", nil, nil); rec.Code != 401 {
		t.Fatalf("API without login: %d", rec.Code)
	}
	if rec := do("GET", "/some/page?x=1", nil, nil); rec.Code != 302 || rec.Header().Get("Location") != "/auth/login?next=%2Fsome%2Fpage%3Fx%3D1" {
		t.Fatalf("page without login: %d %s", rec.Code, rec.Header().Get("Location"))
	}
	// The display's WebSocket isn't behind the login (it has device tokens).
	if rec := do("GET", "/ws/device", nil, nil); rec.Code == 401 || rec.Code == 302 {
		t.Fatalf("device endpoint is behind the login: %d", rec.Code)
	}

	login := func(code string) *httptest.ResponseRecorder {
		t.Helper()
		rec := do("GET", "/auth/login?next=/%23/lr", nil, proxied)
		loc, _ := url.Parse(rec.Header().Get("Location"))
		if rec.Code != 302 || loc.Host != "ha.example.org" || loc.Path != "/auth/authorize" {
			t.Fatalf("login redirect: %d %s", rec.Code, loc)
		}
		q := loc.Query()
		if q.Get("client_id") != "https://homepaper.example.org/" || q.Get("redirect_uri") != "https://homepaper.example.org/auth/callback" {
			t.Fatalf("client: %v", q)
		}
		state := rec.Result().Cookies()
		return do("GET", "/auth/callback?code="+code+"&state="+q.Get("state"), state, proxied)
	}

	// An administrator gets in; the HA login is revoked at once.
	rec := login("admin")
	if rec.Code != 302 || rec.Header().Get("Location") != "/#/lr" {
		t.Fatalf("callback: %d %s %s", rec.Code, rec.Header().Get("Location"), rec.Body)
	}
	if clientID.Load() != "https://homepaper.example.org/" {
		t.Errorf("token exchange used client_id %v", clientID.Load())
	}
	if revoked.Load() != 1 {
		t.Errorf("HA login not revoked (%d)", revoked.Load())
	}
	var sess []*http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookie && c.Value != "" {
			if !c.HttpOnly || !c.Secure {
				t.Errorf("session cookie flags: %+v", c)
			}
			sess = append(sess, c)
		}
	}
	if len(sess) != 1 {
		t.Fatal("no session cookie")
	}
	if rec := do("GET", "/api/devices", sess, nil); rec.Code != 200 {
		t.Fatalf("API with session: %d", rec.Code)
	}
	if rec := do("GET", "/api/me", sess, nil); !strings.Contains(rec.Body.String(), `"name":"Simon"`) {
		t.Fatalf("me: %s", rec.Body)
	}

	// A tampered cookie is rejected.
	bad := *sess[0]
	bad.Value = strings.Replace(bad.Value, ".", "x.", 1)
	if rec := do("GET", "/api/devices", []*http.Cookie{&bad}, nil); rec.Code != 401 {
		t.Fatalf("tampered cookie: %d", rec.Code)
	}

	// A regular user is turned away while only admins may log in.
	if rec := login("user"); rec.Code != 403 {
		t.Fatalf("non-admin: %d", rec.Code)
	}
	if revoked.Load() != 2 {
		t.Errorf("non-admin login not revoked")
	}

	// A wrong state is refused before HA is asked.
	if rec := do("GET", "/auth/callback?code=admin&state=forged", nil, proxied); rec.Code != 400 {
		t.Fatalf("forged state: %d", rec.Code)
	}

	// The post-login redirect never leaves the site.
	for _, next := range []string{"//evil.example", "https://evil.example", "/\\evil.example"} {
		if got := safeNext(next); got != "/" {
			t.Errorf("safeNext(%q) = %q", next, got)
		}
	}

	// Logging out clears the session.
	rec = do("POST", "/auth/logout", sess, nil)
	if rec.Code != 303 {
		t.Fatalf("logout: %d", rec.Code)
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookie && c.MaxAge >= 0 {
			t.Errorf("session cookie not cleared: %+v", c)
		}
	}
}
