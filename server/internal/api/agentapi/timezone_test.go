package agentapi_test

import (
	"net/http"
	"testing"
)

// An agent is told the person's time zone with every message, so "tomorrow
// at 7" is read on their clock, not the server's or India's.
func TestAgentHearsThePersonsTimezone(t *testing.T) {
	f := setupChat(t)
	person := f.srv.AsUser(t, f.owner)
	person.Patch("/v1/client/me", map[string]any{"timezone": "Europe/London"}).ExpectStatus(http.StatusOK)
	f.userSends(t, "remind me at 7")

	var evs struct {
		Events []struct {
			Type string `json:"type"`
			Data struct {
				Participants []struct {
					Kind     string `json:"kind"`
					Timezone string `json:"timezone"`
				} `json:"participants"`
			} `json:"data"`
		} `json:"events"`
	}
	f.srv.AsAgent(t, f.secret).Get("/v1/agent/events").ExpectStatus(http.StatusOK).Decode(&evs)
	for _, ev := range evs.Events {
		if ev.Type != "message.created" {
			continue
		}
		for _, p := range ev.Data.Participants {
			if p.Kind == "user" && p.Timezone == "Europe/London" {
				return
			}
		}
	}
	t.Errorf("events = %+v, want the person's zone among the participants", evs.Events)
}
