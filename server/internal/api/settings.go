package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"homepaper/internal/ha"
	"homepaper/internal/store"
)

// haSettingsDTO describes the Home Assistant connection to the UI. The token
// never leaves the server; the UI only learns whether one is saved.
type haSettingsDTO struct {
	URL      string `json:"url"`
	HasToken bool   `json:"has_token"`
	// Origin says where the connection in use comes from: "ui" (saved here),
	// "env" (HA_URL/HA_TOKEN) or "none" (demo data).
	Origin string    `json:"origin"`
	Status ha.Status `json:"status"`
}

// effectiveHA returns the connection in use and where it comes from.
// Settings saved in the UI win over the environment.
func (a *API) effectiveHA() (store.HASettings, string) {
	if s := a.Store.HA(); s.URL != "" {
		return s, "ui"
	}
	if a.EnvHA.URL != "" {
		return a.EnvHA, "env"
	}
	return store.HASettings{}, "none"
}

// StartHA starts the entity source from the saved or environment settings.
func (a *API) StartHA() {
	s, origin := a.effectiveHA()
	switch origin {
	case "ui":
		a.Log.Info("home assistant settings from the web UI", "url", s.URL)
		if a.EnvHA.URL != "" {
			a.Log.Warn("HA_URL and HA_TOKEN are ignored: settings saved in the web UI take precedence. Remove them there to use the environment again.")
		}
	case "env":
		a.Log.Info("home assistant settings from HA_URL and HA_TOKEN", "url", s.URL)
	default:
		a.Log.Info("no home assistant configured: showing demo entities. Connect one in the web UI.")
	}
	a.HA.Use(s.URL, s.Token)
}

func (a *API) haDTO() haSettingsDTO {
	s, origin := a.effectiveHA()
	return haSettingsDTO{URL: s.URL, HasToken: s.Token != "", Origin: origin, Status: a.States.Status()}
}

func (a *API) getHA(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, a.haDTO())
}

// putHA tests and saves the Home Assistant connection, then reconnects.
// An empty URL removes the saved connection.
func (a *API) putHA(w http.ResponseWriter, r *http.Request) {
	var body struct {
		URL   string `json:"url"`
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, err)
		return
	}
	if a.HA == nil {
		writeErr(w, errors.New("this server cannot change its Home Assistant connection"))
		return
	}

	if strings.TrimSpace(body.URL) == "" {
		if err := a.Store.SetHA(store.HASettings{}); err != nil {
			writeErr(w, err)
			return
		}
		a.StartHA()
		a.events.send(map[string]string{"type": "settings"})
		writeJSON(w, http.StatusOK, a.haDTO())
		return
	}

	url, err := ha.NormalizeURL(body.URL)
	if err != nil {
		writeErr(w, err)
		return
	}
	token := strings.TrimSpace(body.Token)
	if token == "" {
		// Keep the known token, but only for the host it was entered for.
		// Otherwise anyone who can open this page could have the server send
		// the token to an address of their choosing.
		cur, _ := a.effectiveHA()
		if cur.Token == "" {
			writeErr(w, errors.New("enter a long-lived access token"))
			return
		}
		if !ha.SameTarget(cur.URL, url) {
			writeErr(w, errors.New("enter the access token again: the saved one is only sent to the address it was saved with, and never over a less secure connection"))
			return
		}
		token = cur.Token
	}

	if err := ha.Test(r.Context(), url, token); err != nil {
		writeErr(w, err)
		return
	}
	if err := a.Store.SetHA(store.HASettings{URL: url, Token: token}); err != nil {
		writeErr(w, err)
		return
	}
	a.StartHA()
	a.events.send(map[string]string{"type": "settings"})
	writeJSON(w, http.StatusOK, a.haDTO())
}
