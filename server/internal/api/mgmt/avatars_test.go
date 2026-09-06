package mgmt_test

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"testing"

	"github.com/Saieshwar5/cuckoo/server/internal/domain"
	"github.com/Saieshwar5/cuckoo/server/internal/testutil"
)

func logo(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 240, 240))
	for y := range 240 {
		for x := range 240 {
			img.Set(x, y, color.RGBA{R: uint8(x), G: 20, B: uint8(y), A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

// uploadPicture puts a picture on the hub as a person, ready to be a face.
func uploadPicture(t *testing.T, c *testutil.Client, name string) string {
	t.Helper()
	var env struct {
		Media struct {
			ID string `json:"id"`
		} `json:"media"`
	}
	c.Upload("/v1/client/media", name, logo(t)).ExpectStatus(http.StatusCreated).Decode(&env)
	return env.Media.ID
}

// A company creates an agent with its logo in one call, and a stranger with
// no account at all can see it.
func TestAgentIsPublishedWithItsPicture(t *testing.T) {
	srv, owner, _ := setup(t)
	me := srv.AsUser(t, owner)
	picture := uploadPicture(t, me, "logo.png")

	var env agentEnvelope
	me.Post("/v1/mgmt/agents", map[string]any{
		"handle": "sbi-cards", "display_name": "SBI Cards", "description": "Card queries",
		"avatar_media_id": picture,
	}).ExpectStatus(http.StatusCreated).Decode(&env)
	if !env.Agent.HasAvatar {
		t.Fatalf("agent = %+v, want a picture", env.Agent)
	}

	// The card a stranger opens: no credential at all.
	got := srv.Anonymous(t).Get("/a/" + env.Agent.ID + "/avatar").ExpectStatus(http.StatusOK)
	if len(got.Body) == 0 {
		t.Error("the picture came back empty")
	}
	if ct := got.Header.Get("Content-Type"); ct != "image/jpeg" {
		t.Errorf("content type = %q, want the small copy as image/jpeg", ct)
	}
	if got.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Error("a public route served bytes without nosniff")
	}

	// An agent with no picture says so rather than serving something else.
	plain := create(t, me, "plain-one")
	if plain.HasAvatar {
		t.Error("an agent nobody gave a picture has one")
	}
	srv.Anonymous(t).Get("/a/"+plain.ID+"/avatar").ExpectError(http.StatusNotFound, "no_avatar")
	srv.Anonymous(t).Get("/a/"+domain.FormatID(domain.PrefixAgent, domain.NewID())+"/avatar").
		ExpectError(http.StatusNotFound, "no_avatar")
}

func TestAgentPictureCanBeChangedAndMustBeYourOwn(t *testing.T) {
	srv, owner, other := setup(t)
	me := srv.AsUser(t, owner)
	agent := create(t, me, "helper")

	picture := uploadPicture(t, me, "logo.png")
	var env agentEnvelope
	me.Patch("/v1/mgmt/agents/"+agent.ID, map[string]any{"avatar_media_id": picture}).
		ExpectStatus(http.StatusOK).Decode(&env)
	if !env.Agent.HasAvatar {
		t.Fatal("the picture was not set")
	}

	// Someone else's upload is not a picture you can publish, and neither
	// is one that does not exist.
	theirs := uploadPicture(t, srv.AsUser(t, other), "theirs.png")
	me.Patch("/v1/mgmt/agents/"+agent.ID, map[string]any{"avatar_media_id": theirs}).
		ExpectError(http.StatusUnprocessableEntity, "unknown_picture")
	me.Patch("/v1/mgmt/agents/"+agent.ID, map[string]any{
		"avatar_media_id": domain.FormatID(domain.PrefixMedia, domain.NewID()),
	}).ExpectError(http.StatusUnprocessableEntity, "unknown_picture")
	me.Patch("/v1/mgmt/agents/"+agent.ID, map[string]any{"avatar_media_id": "med_nonsense"}).
		ExpectError(http.StatusUnprocessableEntity, "invalid_id")
}

// A face has to be a picture, and one already sent in a message has been
// spoken for.
func TestAFaceIsAnUnsentPicture(t *testing.T) {
	srv, owner, _ := setup(t)
	me := srv.AsUser(t, owner)
	agent := create(t, me, "helper")

	var doc struct {
		Media struct {
			ID string `json:"id"`
		} `json:"media"`
	}
	me.UploadRaw("/v1/client/media?name=notes.pdf", []byte("%PDF-1.7 not a picture")).
		ExpectStatus(http.StatusCreated).Decode(&doc)
	me.Patch("/v1/mgmt/agents/"+agent.ID, map[string]any{"avatar_media_id": doc.Media.ID}).
		ExpectError(http.StatusUnprocessableEntity, "not_a_picture")

	// One that went into a chat belongs to that message now.
	picture := uploadPicture(t, me, "sent.png")
	var conv struct {
		Conversations []struct {
			ID string `json:"id"`
		} `json:"conversations"`
	}
	me.Get("/v1/client/conversations").ExpectStatus(http.StatusOK).Decode(&conv)
	me.Post("/v1/client/conversations/"+conv.Conversations[0].ID+"/messages", map[string]any{
		"attachments": []string{picture},
	}).ExpectStatus(http.StatusCreated)
	me.Patch("/v1/mgmt/agents/"+agent.ID, map[string]any{"avatar_media_id": picture}).
		ExpectError(http.StatusUnprocessableEntity, "picture_already_sent")
}

// A person's photo is theirs: on their account, and not on a public route.
func TestAPersonsPhotoIsNotPublic(t *testing.T) {
	srv, owner, other := setup(t)
	me := srv.AsUser(t, owner)
	picture := uploadPicture(t, me, "me.png")

	var env struct {
		User struct {
			AvatarMediaID *string `json:"avatar_media_id"`
		} `json:"user"`
	}
	me.Patch("/v1/client/me", map[string]any{"avatar_media_id": picture}).
		ExpectStatus(http.StatusOK).Decode(&env)
	if env.User.AvatarMediaID == nil || *env.User.AvatarMediaID != picture {
		t.Fatalf("photo = %v, want the picture just uploaded", env.User.AvatarMediaID)
	}

	// Theirs to see; nobody else's, and on no public address.
	me.Get("/v1/client/media/" + picture).ExpectStatus(http.StatusOK)
	srv.AsUser(t, other).Get("/v1/client/media/"+picture).
		ExpectError(http.StatusNotFound, "media_not_found")
	srv.Anonymous(t).Get("/v1/client/media/" + picture).ExpectStatus(http.StatusUnauthorized)
}

// A face is not swept away as an abandoned upload.
func TestFacesSurviveTheSweep(t *testing.T) {
	srv, owner, _ := setup(t)
	me := srv.AsUser(t, owner)
	picture := uploadPicture(t, me, "logo.png")
	agent := create(t, me, "helper")
	me.Patch("/v1/mgmt/agents/"+agent.ID, map[string]any{"avatar_media_id": picture}).
		ExpectStatus(http.StatusOK)

	if _, err := srv.Media.SweepUnclaimed(t.Context(), 0); err != nil {
		t.Fatalf("SweepUnclaimed: %v", err)
	}
	srv.Anonymous(t).Get("/a/" + agent.ID + "/avatar").ExpectStatus(http.StatusOK)
	me.Get("/v1/client/media/" + picture).ExpectStatus(http.StatusOK)
}

// A company's servers publish an agent with its logo using an API key.
//
// The picture goes to the management API's own upload, because a key is
// refused on the client API by design, and it would be a strange rule that
// let a key create a bank's agent but not give it a face.
func TestAKeyCanPublishAnAgentWithItsLogo(t *testing.T) {
	srv, owner, _ := setup(t)
	var minted struct {
		Key string `json:"key"`
	}
	srv.AsUser(t, owner).Post("/v1/client/api-keys", map[string]any{"name": "deploy"}).
		ExpectStatus(http.StatusCreated).Decode(&minted)
	company := srv.AsKey(t, minted.Key)

	var uploaded struct {
		Media struct {
			ID   string `json:"id"`
			Kind string `json:"kind"`
		} `json:"media"`
	}
	company.Upload("/v1/mgmt/media", "logo.png", logo(t)).ExpectStatus(http.StatusCreated).Decode(&uploaded)
	if uploaded.Media.Kind != "image" {
		t.Fatalf("uploaded = %+v", uploaded.Media)
	}

	var env agentEnvelope
	company.Post("/v1/mgmt/agents", map[string]any{
		"handle": "sbi-cards", "display_name": "SBI Cards", "avatar_media_id": uploaded.Media.ID,
	}).ExpectStatus(http.StatusCreated).Decode(&env)
	if !env.Agent.HasAvatar {
		t.Fatal("the key could not publish a picture")
	}
	srv.Anonymous(t).Get("/a/" + env.Agent.ID + "/avatar").ExpectStatus(http.StatusOK)

	// The key is still refused where it always was.
	company.Upload("/v1/client/media", "logo.png", logo(t)).ExpectStatus(http.StatusUnauthorized)
}
