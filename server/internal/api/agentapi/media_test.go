package agentapi_test

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"strings"
	"testing"
)

type mediaJSON struct {
	ID           string `json:"id"`
	Kind         string `json:"kind"`
	MimeType     string `json:"mime_type"`
	FileName     string `json:"file_name"`
	HasThumbnail bool   `json:"has_thumbnail"`
}

type attachmentJSON struct {
	MediaID    string  `json:"media_id"`
	Kind       string  `json:"kind"`
	MimeType   string  `json:"mime_type"`
	ByteSize   int64   `json:"byte_size"`
	FileName   string  `json:"file_name"`
	Width      int32   `json:"width"`
	Height     int32   `json:"height"`
	DurationMS int32   `json:"duration_ms"`
	Waveform   []int32 `json:"waveform"`
}

type attachedMessageJSON struct {
	ID   string `json:"id"`
	Body struct {
		Text        string           `json:"text"`
		Attachments []attachmentJSON `json:"attachments"`
	} `json:"body"`
}

func picture(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.RGBA{R: uint8(x), G: 30, B: uint8(y), A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

// A person sends a photo of a bill. The backend hears about it with enough
// to decide whether to fetch it, and then fetches it.
func TestAgentReceivesAndDownloadsAPhoto(t *testing.T) {
	f := setupChat(t)
	me := f.srv.AsUser(t, f.owner)
	original := picture(t, 300, 200)

	var uploaded struct {
		Media mediaJSON `json:"media"`
	}
	me.Upload("/v1/client/media", "bill.png", original).ExpectStatus(http.StatusCreated).Decode(&uploaded)
	me.Post("/v1/client/conversations/"+f.dmID+"/messages", map[string]any{
		"text": "is this right?", "attachments": []string{uploaded.Media.ID},
	}).ExpectStatus(http.StatusCreated)

	agent := f.srv.AsAgent(t, f.secret)
	var page struct {
		Messages []attachedMessageJSON `json:"messages"`
	}
	agent.Get("/v1/agent/conversations/" + f.dmID + "/messages").ExpectStatus(http.StatusOK).Decode(&page)
	if len(page.Messages) != 1 {
		t.Fatalf("agent sees %d messages", len(page.Messages))
	}
	atts := page.Messages[0].Body.Attachments
	if len(atts) != 1 || atts[0].MediaID != uploaded.Media.ID {
		t.Fatalf("agent sees attachments %+v", atts)
	}
	if atts[0].Kind != "image" || atts[0].ByteSize != int64(len(original)) || atts[0].Width != 300 {
		t.Errorf("attachment = %+v", atts[0])
	}

	got := agent.Get("/v1/agent/media/" + uploaded.Media.ID).ExpectStatus(http.StatusOK)
	if !bytes.Equal(got.Body, original) {
		t.Error("the agent got different bytes than were sent")
	}
	// A backend saves what it downloads; nothing renders it in a browser.
	if cd := got.Header.Get("Content-Disposition"); !strings.Contains(cd, `"bill.png"`) {
		t.Errorf("disposition = %q, want the file's name", cd)
	}

	// A different agent, in no conversation with that file, cannot have it.
	f.srv.AsAgent(t, f.otherSecret).Get("/v1/agent/media/"+uploaded.Media.ID).
		ExpectError(http.StatusNotFound, "media_not_found")
}

// The other direction: a backend sends a document, and the person gets it.
func TestAgentSendsAFile(t *testing.T) {
	f := setupChat(t)
	agent := f.srv.AsAgent(t, f.secret)

	var uploaded struct {
		Media mediaJSON `json:"media"`
	}
	agent.Upload("/v1/agent/media", "statement.pdf", []byte("%PDF-1.7 your statement")).
		ExpectStatus(http.StatusCreated).Decode(&uploaded)
	if uploaded.Media.Kind != "file" {
		t.Errorf("uploaded = %+v", uploaded.Media)
	}

	var sent struct {
		Message attachedMessageJSON `json:"message"`
	}
	agent.Post("/v1/agent/conversations/"+f.dmID+"/messages", map[string]any{
		"text": "here is your statement", "attachments": []string{uploaded.Media.ID},
	}).ExpectStatus(http.StatusCreated).Decode(&sent)
	if len(sent.Message.Body.Attachments) != 1 {
		t.Fatalf("sent message = %+v", sent.Message.Body)
	}

	// The person in the conversation can read it, through their own API.
	got := f.srv.AsUser(t, f.owner).Get("/v1/client/media/" + uploaded.Media.ID).ExpectStatus(http.StatusOK)
	if string(got.Body) != "%PDF-1.7 your statement" {
		t.Errorf("person got %q", got.Body)
	}

	// One agent cannot send another's upload.
	other := f.srv.AsAgent(t, f.otherSecret)
	var theirs struct {
		Media mediaJSON `json:"media"`
	}
	other.Upload("/v1/agent/media", "theirs.txt", []byte("not yours to send")).
		ExpectStatus(http.StatusCreated).Decode(&theirs)
	agent.Post("/v1/agent/conversations/"+f.dmID+"/messages", map[string]any{
		"attachments": []string{theirs.Media.ID},
	}).ExpectError(http.StatusUnprocessableEntity, "unknown_attachment")
}

// A stream is text. Files go as their own message.
func TestStreamsCarryNoFiles(t *testing.T) {
	f := setupChat(t)
	agent := f.srv.AsAgent(t, f.secret)

	var uploaded struct {
		Media mediaJSON `json:"media"`
	}
	agent.Upload("/v1/agent/media", "chart.png", picture(t, 30, 30)).
		ExpectStatus(http.StatusCreated).Decode(&uploaded)

	agent.Post("/v1/agent/conversations/"+f.dmID+"/messages", map[string]any{
		"stream": true, "attachments": []string{uploaded.Media.ID},
	}).ExpectError(http.StatusUnprocessableEntity, "stream_with_attachments")
}

// A backend hears a voice note as a file with a length and a shape, and
// can answer with one of its own.
func TestAgentHearsAndSendsAVoiceNote(t *testing.T) {
	f := setupChat(t)
	me := f.srv.AsUser(t, f.owner)
	recording := "\x00\x00\x00\x20ftypM4A \x00\x00\x00\x00M4A mp42isom" + strings.Repeat("\x00", 600)

	var uploaded struct {
		Media mediaJSON `json:"media"`
	}
	me.UploadRaw("/v1/client/media?name=question.m4a&duration_ms=4300&waveform=5,60,20&kind=audio",
		[]byte(recording)).ExpectStatus(http.StatusCreated).Decode(&uploaded)
	me.Post("/v1/client/conversations/"+f.dmID+"/messages", map[string]any{
		"attachments": []string{uploaded.Media.ID},
	}).ExpectStatus(http.StatusCreated)

	agent := f.srv.AsAgent(t, f.secret)
	var page struct {
		Messages []attachedMessageJSON `json:"messages"`
	}
	agent.Get("/v1/agent/conversations/" + f.dmID + "/messages").ExpectStatus(http.StatusOK).Decode(&page)
	if len(page.Messages) != 1 || len(page.Messages[0].Body.Attachments) != 1 {
		t.Fatalf("agent sees %+v", page.Messages)
	}
	heard := page.Messages[0].Body.Attachments[0]
	if heard.Kind != "audio" || heard.DurationMS != 4300 {
		t.Errorf("agent heard %+v", heard)
	}
	if len(heard.Waveform) != 3 {
		t.Errorf("waveform = %v", heard.Waveform)
	}

	// Answering in kind.
	var spoken struct {
		Media mediaJSON `json:"media"`
	}
	agent.UploadRaw("/v1/agent/media?name=answer.m4a&duration_ms=2100&kind=audio", []byte(recording)).
		ExpectStatus(http.StatusCreated).Decode(&spoken)
	if spoken.Media.Kind != "audio" {
		t.Errorf("the agent's recording is %s", spoken.Media.Kind)
	}
	agent.Post("/v1/agent/conversations/"+f.dmID+"/messages", map[string]any{
		"attachments": []string{spoken.Media.ID},
	}).ExpectStatus(http.StatusCreated)
}
