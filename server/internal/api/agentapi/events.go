package agentapi

import (
	"net/http"

	"github.com/Saieshwar5/cuckoo/server/internal/api/httpx"
	"github.com/Saieshwar5/cuckoo/server/internal/delivery"
	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/events"
	"github.com/Saieshwar5/cuckoo/server/internal/principal"
)

// eventsEnvelope is a page of an agent's events, oldest first. The events
// are the same shape a webhook delivers, so a backend has one parser.
type eventsEnvelope struct {
	Events []events.Envelope `json:"events"`
}

// listEvents lets a backend read its events after downtime, or instead of
// receiving webhooks at all. Pass the last id seen as ?since=.
func (h *Handler) listEvents(w http.ResponseWriter, r *http.Request) {
	agentID, ok := principal.AgentID(r.Context())
	if !ok {
		httpx.Error(w, r, domain.Unauthorized("unauthorized", "Authenticate as an agent."))
		return
	}

	since, err := httpx.QueryID(r, "since", domain.PrefixEvent)
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	limit, err := httpx.QueryInt(r, "limit")
	if err != nil {
		httpx.Error(w, r, err)
		return
	}

	list, err := h.delivery.ListEvents(r.Context(), agentID, delivery.ListEventsInput{Since: since, Limit: limit})
	if err != nil {
		httpx.Error(w, r, err)
		return
	}
	httpx.JSON(w, r, http.StatusOK, eventsEnvelope{Events: list})
}
