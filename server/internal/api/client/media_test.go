package client_test

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"strings"
	"testing"

	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/testutil"
)

type mediaJSON struct {
	ID           string `json:"id"`
	Kind         string `json:"kind"`
	MimeType     string `json:"mime_type"`
	ByteSize     int64  `json:"byte_size"`
	FileName     string `json:"file_name"`
	Width        int32  `json:"width"`
	Height       int32  `json:"height"`
	HasThumbnail bool   `json:"has_thumbnail"`
}

// attachmentJSON is a file as a message carries it: the same facts as an
// upload, under media_id, because in a body it names a file rather than
// being one.
type attachmentJSON struct {
	MediaID      string `json:"media_id"`
	Kind         string `json:"kind"`
	MimeType     string `json:"mime_type"`
	ByteSize     int64  `json:"byte_size"`
	FileName     string `json:"file_name"`
	Width        int32  `json:"width"`
	Height       int32  `json:"height"`
	HasThumbnail bool   `json:"has_thumbnail"`
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
			img.Set(x, y, color.RGBA{R: uint8(x), G: 40, B: uint8(y), A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

func upload(t *testing.T, c *testutil.Client, path, name string, data []byte) mediaJSON {
	t.Helper()
	var env struct {
		Media mediaJSON `json:"media"`
	}
	c.Upload(path, name, data).ExpectStatus(http.StatusCreated).Decode(&env)
	return env.Media
}

// The whole path a photo takes: uploaded on its own, named by a send, seen
// by the agent in the conversation, and its bytes fetched back.
func TestSendAPhotoWithACaption(t *testing.T) {
	f := setupChat(t)
	me := f.srv.AsUser(t, f.owner)
	original := picture(t, 800, 400)

	file := upload(t, me, "/v1/client/media", "beach.png", original)
	if file.Kind != "image" || file.Width != 800 || file.Height != 400 || !file.HasThumbnail {
		t.Fatalf("uploaded = %+v", file)
	}
	if !strings.HasPrefix(file.ID, "med_") {
		t.Errorf("id = %q, want med_ prefix", file.ID)
	}

	var env struct {
		Message attachedMessageJSON `json:"message"`
	}
	me.Post("/v1/client/conversations/"+f.dmID+"/messages", map[string]any{
		"text": "look at this", "attachments": []string{file.ID},
	}).ExpectStatus(http.StatusCreated).Decode(&env)

	atts := env.Message.Body.Attachments
	if len(atts) != 1 {
		t.Fatalf("message carries %d files: %+v", len(atts), env.Message.Body)
	}
	if atts[0].MediaID != file.ID || atts[0].Kind != "image" || atts[0].FileName != "beach.png" {
		t.Errorf("attachment = %+v", atts[0])
	}
	if atts[0].Width != 800 || atts[0].Height != 400 {
		t.Errorf("attachment has no shape: %+v", atts[0])
	}
	if env.Message.Body.Text != "look at this" {
		t.Errorf("caption = %q", env.Message.Body.Text)
	}

	// History says the same thing, so a phone that reopens the chat draws
	// the same bubble without asking anything else.
	var page struct {
		Messages []attachedMessageJSON `json:"messages"`
	}
	me.Get("/v1/client/conversations/" + f.dmID + "/messages").ExpectStatus(http.StatusOK).Decode(&page)
	if len(page.Messages) != 1 || len(page.Messages[0].Body.Attachments) != 1 {
		t.Fatalf("history = %+v", page.Messages)
	}

	// And the bytes come back, both sizes.
	full := me.Get("/v1/client/media/" + file.ID).ExpectStatus(http.StatusOK)
	if !bytes.Equal(full.Body, original) {
		t.Error("the picture came back changed")
	}
	if got := full.Header.Get("Content-Type"); got != "image/png" {
		t.Errorf("content type = %q, want image/png", got)
	}
	if got := full.Header.Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("nosniff = %q", got)
	}
	if got := full.Header.Get("Content-Disposition"); !strings.HasPrefix(got, "inline") {
		t.Errorf("disposition = %q, want inline for a picture", got)
	}
	thumb := me.Get("/v1/client/media/" + file.ID + "?variant=thumb").ExpectStatus(http.StatusOK)
	if len(thumb.Body) == 0 {
		t.Error("the thumbnail came back empty")
	}
	if ct := thumb.Header.Get("Content-Type"); ct != "image/jpeg" {
		t.Errorf("thumbnail type = %q, want image/jpeg", ct)
	}
}

// A photo with nothing written under it is a message. An empty message
// still is not.
func TestAPhotoNeedsNoCaption(t *testing.T) {
	f := setupChat(t)
	me := f.srv.AsUser(t, f.owner)

	file := upload(t, me, "/v1/client/media", "cat.png", picture(t, 20, 20))
	var env struct {
		Message attachedMessageJSON `json:"message"`
	}
	me.Post("/v1/client/conversations/"+f.dmID+"/messages", map[string]any{
		"attachments": []string{file.ID},
	}).ExpectStatus(http.StatusCreated).Decode(&env)
	if env.Message.Body.Text != "" || len(env.Message.Body.Attachments) != 1 {
		t.Errorf("body = %+v", env.Message.Body)
	}

	me.Post("/v1/client/conversations/"+f.dmID+"/messages", map[string]any{"text": "   "}).
		ExpectError(http.StatusUnprocessableEntity, "invalid_text")

	// The chat list says what arrived rather than that something did.
	var list struct {
		Conversations []struct {
			LastMessage *struct {
				Body struct {
					Attachments []attachmentJSON `json:"attachments"`
				} `json:"body"`
			} `json:"last_message"`
		} `json:"conversations"`
	}
	me.Get("/v1/client/conversations").ExpectStatus(http.StatusOK).Decode(&list)
	if lm := list.Conversations[0].LastMessage; lm == nil || len(lm.Body.Attachments) != 1 {
		t.Errorf("chat list row = %+v", list.Conversations[0].LastMessage)
	}
}

// A file belongs to whoever uploaded it, is sent once, and is readable only
// by the conversation it was sent into.
func TestAFileIsSentOnceByItsOwner(t *testing.T) {
	f := setupChat(t)
	me := f.srv.AsUser(t, f.owner)
	stranger := f.srv.AsUser(t, f.other)

	file := upload(t, me, "/v1/client/media", "receipt.pdf", []byte("%PDF-1.7 the receipt"))
	if file.Kind != "file" || file.HasThumbnail {
		t.Errorf("uploaded = %+v, want a plain file", file)
	}

	// Nobody else can send it, and nobody else can read it.
	stranger.Post("/v1/client/conversations/"+f.dmID+"/messages", map[string]any{
		"attachments": []string{file.ID},
	}).ExpectError(http.StatusForbidden, "not_participant")
	stranger.Get("/v1/client/media/"+file.ID).ExpectError(http.StatusNotFound, "media_not_found")

	me.Post("/v1/client/conversations/"+f.dmID+"/messages", map[string]any{
		"attachments": []string{file.ID},
	}).ExpectStatus(http.StatusCreated)

	// Sent once. A second send would be two messages claiming one file.
	me.Post("/v1/client/conversations/"+f.dmID+"/messages", map[string]any{
		"attachments": []string{file.ID},
	}).ExpectError(http.StatusUnprocessableEntity, "attachment_already_sent")

	// It is still not the stranger's to read: they are not in the
	// conversation it went to.
	stranger.Get("/v1/client/media/"+file.ID).ExpectError(http.StatusNotFound, "media_not_found")

	// A document is downloaded, never rendered: these are bytes a stranger
	// uploaded and this hub is not going to serve them as a page on its own
	// origin.
	got := me.Get("/v1/client/media/" + file.ID).ExpectStatus(http.StatusOK)
	if ct := got.Header.Get("Content-Type"); ct != "application/octet-stream" {
		t.Errorf("content type = %q, want application/octet-stream", ct)
	}
	if cd := got.Header.Get("Content-Disposition"); !strings.HasPrefix(cd, "attachment") {
		t.Errorf("disposition = %q, want attachment", cd)
	}
	if !strings.Contains(got.Header.Get("Content-Disposition"), `"receipt.pdf"`) {
		t.Errorf("disposition does not carry the name: %q", got.Header.Get("Content-Disposition"))
	}
}

func TestSendRefusesFilesThatAreNotThere(t *testing.T) {
	f := setupChat(t)
	me := f.srv.AsUser(t, f.owner)

	me.Post("/v1/client/conversations/"+f.dmID+"/messages", map[string]any{
		"attachments": []string{"med_notanid"},
	}).ExpectError(http.StatusUnprocessableEntity, "invalid_id")

	me.Post("/v1/client/conversations/"+f.dmID+"/messages", map[string]any{
		"attachments": []string{domain.FormatID(domain.PrefixMedia, domain.NewID())},
	}).ExpectError(http.StatusUnprocessableEntity, "unknown_attachment")

	file := upload(t, me, "/v1/client/media", "twice.png", picture(t, 10, 10))
	me.Post("/v1/client/conversations/"+f.dmID+"/messages", map[string]any{
		"attachments": []string{file.ID, file.ID},
	}).ExpectError(http.StatusUnprocessableEntity, "invalid_attachment")
}

// The other shape of upload: the bytes as the body, the name in the query,
// which is what one line of curl produces.
func TestUploadFromTheCommandLineShape(t *testing.T) {
	f := setupChat(t)
	me := f.srv.AsUser(t, f.owner)

	var env struct {
		Media mediaJSON `json:"media"`
	}
	me.UploadRaw("/v1/client/media?name=notes.txt", []byte("plain words in a file")).
		ExpectStatus(http.StatusCreated).Decode(&env)
	if env.Media.FileName != "notes.txt" || env.Media.Kind != "file" {
		t.Errorf("uploaded = %+v", env.Media)
	}

	// A name that is a path is reduced to a name.
	me.UploadRaw("/v1/client/media?name=../../etc/passwd", []byte("root:x:0:0")).
		ExpectStatus(http.StatusCreated).Decode(&env)
	if env.Media.FileName != "passwd" {
		t.Errorf("file name = %q, want the path stripped", env.Media.FileName)
	}

	me.UploadRaw("/v1/client/media?name=nothing.txt", nil).
		ExpectError(http.StatusUnprocessableEntity, "empty_file")
}
