package api

import (
	"errors"
	"html/template"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/pairing"
)

// pairPage is what a QR code opens for someone without the app: the
// agent's name, who is behind it, and a way into Cuckoo. The app itself
// intercepts the same link and never shows this.
var pairPage = template.Must(template.New("pair").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}} · Cuckoo</title>
<style>
  body { margin: 0; font-family: -apple-system, system-ui, Roboto, sans-serif; background: #000; color: #f2f2f2; }
  main { max-width: 420px; margin: 0 auto; padding: 48px 24px; text-align: center; }
  .disc { width: 96px; height: 96px; border-radius: 48px; background: #3a3a3a; color: #fff; font-size: 36px; font-weight: 600; line-height: 96px; margin: 0 auto 16px; overflow: hidden; }
  .disc img { width: 96px; height: 96px; object-fit: cover; display: block; }
  h1 { font-size: 24px; margin: 0 0 4px; }
  .muted { color: #8e8e8e; font-size: 14px; margin: 0 0 16px; }
  p { line-height: 1.5; }
  a.button { display: block; margin: 24px 0 12px; padding: 14px; border-radius: 999px; background: #f2f2f2; color: #000; text-decoration: none; font-weight: 600; }
  a.plain { color: #f2f2f2; font-size: 14px; }
</style>
</head>
<body>
<main>
{{if .Agent}}
  <div class="disc">{{if .AvatarURL}}<img src="{{.AvatarURL}}" alt="">{{else}}{{.Initials}}{{end}}</div>
  <h1>{{.Agent.DisplayName}}</h1>
  <p class="muted">@{{.Agent.Handle}} · by {{.OwnerName}} · Unverified</p>
  {{if .Agent.Description}}<p>{{.Agent.Description}}</p>{{end}}
  <a class="button" href="{{.AppLink}}">Open in Cuckoo</a>
  <a class="plain" href="https://github.com/Saieshwar5/cuckoo">Get Cuckoo</a>
{{else}}
  <h1>{{.Title}}</h1>
  <p class="muted">{{.Message}}</p>
  <a class="plain" href="https://github.com/Saieshwar5/cuckoo">Get Cuckoo</a>
{{end}}
</main>
</body>
</html>
`))

type pairPageData struct {
	Title    string
	Message  string
	Agent    *pairPageAgent
	Initials string
	// AvatarURL is the agent's published picture, when it has one. Empty
	// falls back to the initials disc.
	AvatarURL string
	OwnerName string
	// AppLink is on the app's own scheme, which the template would otherwise
	// refuse as an unknown protocol.
	AppLink template.URL
}

type pairPageAgent struct {
	DisplayName string
	Handle      string
	Description string
}

// pairPageHandler serves /p/{code}.
func pairPageHandler(p *pairing.Service, publicURL string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		code := chi.URLParam(r, "code")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		data := pairPageData{AppLink: template.URL("cuckoo://p/" + code)} //nolint:gosec // our scheme, our code
		card, err := p.Resolve(r.Context(), uuid.Nil, code)
		if err != nil {
			var derr *domain.Error
			if errors.As(err, &derr) {
				w.WriteHeader(http.StatusNotFound)
				data.Title = "This link is not valid"
				data.Message = derr.Message
			} else {
				w.WriteHeader(http.StatusInternalServerError)
				data.Title = "Something went wrong"
				data.Message = "Try again in a moment."
			}
		} else {
			data.Title = card.Agent.DisplayName
			data.Agent = &pairPageAgent{
				DisplayName: card.Agent.DisplayName,
				Handle:      card.Agent.Handle,
				Description: card.Agent.Description,
			}
			data.Initials = initialsOf(card.Agent.DisplayName)
			if card.Agent.AvatarMediaID != nil {
				data.AvatarURL = publicURL + "/a/" + domain.FormatID(domain.PrefixAgent, card.Agent.ID) + "/avatar"
			}
			data.OwnerName = card.OwnerName
		}
		_ = pairPage.Execute(w, data)
	}
}

// initialsOf is the first letter of the first and last words, as the app
// draws an avatar.
func initialsOf(name string) string {
	var out []rune
	words := 0
	inWord := false
	for _, r := range name {
		if r == ' ' {
			inWord = false
			continue
		}
		if !inWord {
			inWord = true
			words++
			if words == 1 {
				out = []rune{r}
			} else {
				out = append(out[:1], r)
			}
		}
	}
	if len(out) == 0 {
		return "?"
	}
	for i, r := range out {
		if r >= 'a' && r <= 'z' {
			out[i] = r - 'a' + 'A'
		}
	}
	return string(out)
}
