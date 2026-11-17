package ha

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// User is a Home Assistant user, as auth/current_user reports it.
type User struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	IsOwner bool   `json:"is_owner"`
	IsAdmin bool   `json:"is_admin"`
}

// Tokens is the answer of HA's token endpoint.
type Tokens struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
}

// ExchangeCode trades the code from HA's login page for tokens
// (POST /auth/token, grant_type=authorization_code). clientID must be the
// one the login started with.
func ExchangeCode(ctx context.Context, client *http.Client, baseURL, code, clientID string) (Tokens, error) {
	form := url.Values{"grant_type": {"authorization_code"}, "code": {code}, "client_id": {clientID}}
	var t Tokens
	if err := postForm(ctx, client, strings.TrimRight(baseURL, "/")+"/auth/token", form, &t); err != nil {
		return Tokens{}, err
	}
	if t.AccessToken == "" {
		return Tokens{}, fmt.Errorf("Home Assistant returned no access token")
	}
	return t, nil
}

// Revoke ends a login: revoking its refresh token also invalidates the
// access tokens issued with it. Newer HA versions have POST /auth/revoke;
// older ones take action=revoke on /auth/token.
func Revoke(ctx context.Context, client *http.Client, baseURL, refreshToken string) error {
	base := strings.TrimRight(baseURL, "/")
	err := postForm(ctx, client, base+"/auth/revoke", url.Values{"token": {refreshToken}}, nil)
	if err == nil {
		return nil
	}
	return postForm(ctx, client, base+"/auth/token", url.Values{"action": {"revoke"}, "token": {refreshToken}}, nil)
}

func postForm(ctx context.Context, client *http.Client, u string, form url.Values, out any) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("cannot reach Home Assistant: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode/100 != 2 {
		return fmt.Errorf("Home Assistant answered %s", res.Status)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(res.Body).Decode(out)
}

// CurrentUser asks HA whose access token this is (auth/current_user over
// the WebSocket API; there is no REST endpoint for it).
func CurrentUser(ctx context.Context, baseURL, accessToken string) (User, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	conn, err := (&Client{URL: baseURL, Token: accessToken}).dial(ctx)
	if err != nil {
		return User{}, err
	}
	defer conn.Close(websocket.StatusNormalClosure, "")
	if err := wsjson.Write(ctx, conn, map[string]any{"id": 1, "type": "auth/current_user"}); err != nil {
		return User{}, err
	}
	for {
		var m struct {
			ID      int    `json:"id"`
			Type    string `json:"type"`
			Success bool   `json:"success"`
			Result  User   `json:"result"`
		}
		if err := wsjson.Read(ctx, conn, &m); err != nil {
			return User{}, err
		}
		if m.Type == "result" && m.ID == 1 {
			if !m.Success || m.Result.ID == "" {
				return User{}, fmt.Errorf("Home Assistant did not say who is logged in")
			}
			return m.Result, nil
		}
	}
}
