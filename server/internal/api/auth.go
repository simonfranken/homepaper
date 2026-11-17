package api

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"html/template"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"homepaper/internal/ha"
)

// Login via Home Assistant (D-016, D-017). The browser logs in on HA's own
// login page (HA's OAuth2-style flow: /auth/authorize, /auth/token; the
// client ID is homepaper's URL, nothing is registered in HA). homepaper
// asks HA who logged in, revokes that login at once and keeps only its own
// signed session cookie. It never stores or uses a user's HA token.

// Auth configures the login. A nil *Auth on the API turns the login off
// (HOMEPAPER_AUTH=off, for development and the demo).
type Auth struct {
	// HAURL is where the server reaches Home Assistant; PublicHAURL where
	// the browser does (defaults to HAURL).
	HAURL, PublicHAURL string
	// PublicURL is homepaper's address as the browser sees it, e.g.
	// https://homepaper.example.org. Empty: taken from each request
	// (Host and X-Forwarded-Proto/-Host from a reverse proxy).
	PublicURL string
	// AdminsOnly lets only HA administrators (and the owner) in.
	AdminsOnly bool
	// Key signs session cookies (SessionKey).
	Key []byte
	// SessionTTL is how long a login lasts. Default 14 days.
	SessionTTL time.Duration
	HTTP       *http.Client
}

const (
	sessionCookie = "homepaper_session"
	loginCookie   = "homepaper_login"
)

// session is what the cookie carries, signed with Auth.Key.
type session struct {
	UserID  string `json:"u"`
	Name    string `json:"n"`
	Admin   bool   `json:"a"`
	Expires int64  `json:"e"`
}

// SessionKey loads the cookie signing key from dir, creating it on first
// use. Deleting the file logs everyone out.
func SessionKey(dir string) ([]byte, error) {
	path := filepath.Join(dir, "session.key")
	if data, err := os.ReadFile(path); err == nil {
		key, err := hex.DecodeString(strings.TrimSpace(string(data)))
		if err == nil && len(key) >= 32 {
			return key, nil
		}
		return nil, errors.New(path + " is not a valid key; delete it to create a new one")
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return key, os.WriteFile(path, []byte(hex.EncodeToString(key)+"\n"), 0o600)
}

func (au *Auth) ttl() time.Duration {
	if au.SessionTTL > 0 {
		return au.SessionTTL
	}
	return 14 * 24 * time.Hour
}

func (au *Auth) sign(payload []byte) string {
	m := hmac.New(sha256.New, au.Key)
	m.Write(payload)
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

func (au *Auth) encode(s session) string {
	data, _ := json.Marshal(s)
	p := base64.RawURLEncoding.EncodeToString(data)
	return p + "." + au.sign([]byte(p))
}

// user returns the logged-in user, if the session cookie is valid.
func (au *Auth) user(r *http.Request) (session, bool) {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return session{}, false
	}
	p, sig, ok := strings.Cut(c.Value, ".")
	if !ok || subtle.ConstantTimeCompare([]byte(sig), []byte(au.sign([]byte(p)))) != 1 {
		return session{}, false
	}
	data, err := base64.RawURLEncoding.DecodeString(p)
	if err != nil {
		return session{}, false
	}
	var s session
	if json.Unmarshal(data, &s) != nil || time.Now().Unix() > s.Expires || s.UserID == "" {
		return session{}, false
	}
	return s, true
}

// secure reports whether the browser talks HTTPS, directly or through a
// reverse proxy, so cookies get the Secure flag.
func secure(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

// baseURL is homepaper's address as the browser sees it.
func (au *Auth) baseURL(r *http.Request) string {
	if au.PublicURL != "" {
		return strings.TrimRight(au.PublicURL, "/")
	}
	scheme := "http"
	if secure(r) {
		scheme = "https"
	}
	host := r.Host
	if h := r.Header.Get("X-Forwarded-Host"); h != "" {
		host = strings.TrimSpace(strings.Split(h, ",")[0])
	}
	return scheme + "://" + host
}

// safeNext keeps a post-login redirect on this site.
func safeNext(next string) string {
	if !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") || strings.HasPrefix(next, "/\\") || strings.HasPrefix(next, "/auth/") {
		return "/"
	}
	return next
}

func (au *Auth) routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /auth/login", au.login)
	mux.HandleFunc("GET /auth/callback", au.callback)
	mux.HandleFunc("POST /auth/logout", au.logout)
	mux.HandleFunc("GET /auth/logged-out", func(w http.ResponseWriter, r *http.Request) {
		page(w, http.StatusOK, "Logged out", "You are logged out of homepaper.", "Log in again", "/auth/login")
	})
}

// login sends the browser to Home Assistant's login page.
func (au *Auth) login(w http.ResponseWriter, r *http.Request) {
	buf := make([]byte, 16)
	_, _ = rand.Read(buf)
	state := hex.EncodeToString(buf)
	base := au.baseURL(r)
	http.SetCookie(w, &http.Cookie{
		Name: loginCookie, Value: state + "|" + safeNext(r.URL.Query().Get("next")), Path: "/auth/",
		MaxAge: 600, HttpOnly: true, Secure: secure(r), SameSite: http.SameSiteLaxMode,
	})
	q := url.Values{
		"response_type": {"code"},
		"client_id":     {base + "/"},
		"redirect_uri":  {base + "/auth/callback"},
		"state":         {state},
	}
	http.Redirect(w, r, strings.TrimRight(au.PublicHAURL, "/")+"/auth/authorize?"+q.Encode(), http.StatusFound)
}

// callback finishes the login: checks state, trades the code for tokens,
// asks HA who this is, revokes the login and sets homepaper's own session.
func (au *Auth) callback(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie(loginCookie)
	state, next, _ := strings.Cut(func() string {
		if err != nil {
			return ""
		}
		return c.Value
	}(), "|")
	http.SetCookie(w, &http.Cookie{Name: loginCookie, Value: "", Path: "/auth/", MaxAge: -1})
	q := r.URL.Query()
	if state == "" || subtle.ConstantTimeCompare([]byte(state), []byte(q.Get("state"))) != 1 || q.Get("code") == "" {
		page(w, http.StatusBadRequest, "Login failed", "The login took too long or came from somewhere else. Please try again.", "Log in", "/auth/login")
		return
	}
	ctx := r.Context()
	tokens, err := ha.ExchangeCode(ctx, au.HTTP, au.HAURL, q.Get("code"), au.baseURL(r)+"/")
	if err != nil {
		page(w, http.StatusBadGateway, "Login failed", "Home Assistant did not confirm the login: "+err.Error(), "Try again", "/auth/login")
		return
	}
	user, uerr := ha.CurrentUser(ctx, au.HAURL, tokens.AccessToken)
	// homepaper keeps no HA token of the user: end that login right away.
	if tokens.RefreshToken != "" {
		_ = ha.Revoke(ctx, au.HTTP, au.HAURL, tokens.RefreshToken)
	}
	if uerr != nil {
		page(w, http.StatusBadGateway, "Login failed", "Home Assistant did not say who logged in: "+uerr.Error(), "Try again", "/auth/login")
		return
	}
	if au.AdminsOnly && !user.IsAdmin && !user.IsOwner {
		page(w, http.StatusForbidden, "No access",
			"Only Home Assistant administrators can use homepaper here. "+user.Name+" is not one. Ask an administrator, or log in to Home Assistant as another user.",
			"Log in as someone else", "/auth/login")
		return
	}
	s := session{UserID: user.ID, Name: user.Name, Admin: user.IsAdmin || user.IsOwner, Expires: time.Now().Add(au.ttl()).Unix()}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: au.encode(s), Path: "/",
		MaxAge: int(au.ttl().Seconds()), HttpOnly: true, Secure: secure(r), SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, safeNext(next), http.StatusFound)
}

func (au *Auth) logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: secure(r), SameSite: http.SameSiteLaxMode})
	http.Redirect(w, r, "/auth/logged-out", http.StatusSeeOther)
}

// require lets only logged-in users through. API calls get 401, pages are
// sent to the login.
func (au *Auth) require(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := au.user(r); ok {
			next.ServeHTTP(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "login required"})
			return
		}
		http.Redirect(w, r, "/auth/login?next="+url.QueryEscape(r.URL.RequestURI()), http.StatusFound)
	})
}

var pageTmpl = template.Must(template.New("page").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}} · homepaper</title>
<style>
@font-face{font-family:Go;src:url(/auth/font/go-regular.ttf)}
@font-face{font-family:Go;font-weight:700;src:url(/auth/font/go-bold.ttf)}
body{margin:0;min-height:100vh;display:grid;place-items:center;background:#f3f2ec;color:#111;font:14px/1.45 Go,system-ui,sans-serif}
main{max-width:420px;margin:16px;padding:24px;background:#fff;border:2px solid #111;border-radius:12px;box-shadow:6px 6px 0 #111}
h1{margin:0 0 8px;font-size:25px}
p{margin:0 0 18px;font-size:17px;color:#55554f}
a{display:inline-flex;align-items:center;min-height:40px;padding:0 14px;border-radius:8px;background:#111;color:#fff;font-weight:700;text-decoration:none}
a:focus-visible{outline:3px solid #1d4ed8;outline-offset:2px}
</style></head>
<body><main><h1>{{.Title}}</h1><p>{{.Text}}</p><a href="{{.Href}}">{{.Action}}</a></main></body></html>`))

func page(w http.ResponseWriter, status int, title, text, action, href string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = pageTmpl.Execute(w, map[string]string{"Title": title, "Text": text, "Action": action, "Href": href})
}
