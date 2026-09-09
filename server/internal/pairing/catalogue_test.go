package pairing_test

import (
	"context"
	"testing"

	"github.com/Saieshwar5/cuckoo/server/internal/agents"
	"github.com/Saieshwar5/cuckoo/server/internal/pairing"
)

// The catalogue is what the app offers somebody who has nothing yet. Two
// things matter: it holds only what was published to it, and a private agent
// can never be handed out however it is asked for.

func TestCatalogueHoldsOnlyWhatWasListed(t *testing.T) {
	ctx := context.Background()
	f := setup(t)
	svc := agents.New(f.db)

	listed, err := svc.Create(ctx, f.owner.ID, agents.CreateInput{
		Handle: "weather", DisplayName: "Weather", Description: "The forecast.", Listed: true,
	})
	if err != nil {
		t.Fatalf("create listed: %v", err)
	}
	// f.agent already exists with a poster code and is not listed: handing out
	// a QR and wanting to appear in a directory are different wishes.

	cards, err := f.svc.Catalogue(ctx, f.priya.ID, 0)
	if err != nil {
		t.Fatalf("Catalogue: %v", err)
	}
	if len(cards) != 1 || cards[0].Agent.ID != listed.ID {
		t.Fatalf("catalogue holds %d agents, want only the one that was listed", len(cards))
	}
	if cards[0].AlreadyAdded {
		t.Error("a stranger has not added it")
	}
	// The row is the card a scanned code resolves to, owner line and all.
	if cards[0].OwnerName != "SBI" {
		t.Errorf("owner = %q, want the account that published it", cards[0].OwnerName)
	}
}

func TestCatalogueSaysWhatIsAlreadyAdded(t *testing.T) {
	ctx := context.Background()
	f := setup(t)
	svc := agents.New(f.db)

	if _, err := svc.Create(ctx, f.owner.ID, agents.CreateInput{
		Handle: "weather", DisplayName: "Weather", Listed: true,
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	// Its owner has it by definition: creating an agent opens the DM.
	cards, err := f.svc.Catalogue(ctx, f.owner.ID, 0)
	if err != nil {
		t.Fatalf("Catalogue: %v", err)
	}
	if len(cards) != 1 {
		t.Fatalf("catalogue holds %d, want 1", len(cards))
	}
	if !cards[0].AlreadyAdded || cards[0].ConversationID == nil {
		t.Errorf("card = %+v, want it marked added with the chat it opened", cards[0])
	}
}

// The rule that makes an agent safe to run on somebody's behalf: there is no
// code for it, and no way to ask for one.
func TestAPrivateAgentCannotBeHandedOut(t *testing.T) {
	ctx := context.Background()
	f := setup(t)
	svc := agents.New(f.db)

	private, err := svc.Create(ctx, f.priya.ID, agents.CreateInput{
		Handle: "priyas-mail", DisplayName: "Mail", Private: true,
	})
	if err != nil {
		t.Fatalf("create private: %v", err)
	}

	_, _, err = f.svc.CreateToken(ctx, f.priya.ID, private.ID, pairing.CreateTokenInput{})
	if got := code(t, err); got != "agent_is_private" {
		t.Fatalf("minting a code for a private agent got %q, want agent_is_private", got)
	}

	cards, err := f.svc.Catalogue(ctx, f.priya.ID, 0)
	if err != nil {
		t.Fatalf("Catalogue: %v", err)
	}
	for _, c := range cards {
		if c.Agent.ID == private.ID {
			t.Error("a private agent appeared in the catalogue")
		}
	}
}

// Adding from the catalogue is the same act as scanning, reached another way:
// the same contact, the same chat, and the backend told the same thing.
func TestAddingFromTheCatalogueNeedsNoCode(t *testing.T) {
	ctx := context.Background()
	f := setup(t)
	svc := agents.New(f.db)

	listed, err := svc.Create(ctx, f.owner.ID, agents.CreateInput{
		Handle: "weather", DisplayName: "Weather", Listed: true,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	accepted, err := f.svc.AddFromCatalogue(ctx, f.priya.ID, listed.ID)
	if err != nil {
		t.Fatalf("AddFromCatalogue: %v", err)
	}
	if !accepted.New {
		t.Error("the first add is new")
	}

	// It is in her list now, and the catalogue says so rather than offering
	// it again.
	cards, err := f.svc.Catalogue(ctx, f.priya.ID, 0)
	if err != nil {
		t.Fatalf("Catalogue: %v", err)
	}
	if len(cards) != 1 || !cards[0].AlreadyAdded {
		t.Fatalf("card = %+v, want it marked as added", cards[0])
	}
	// Adding twice is not two chats.
	again, err := f.svc.AddFromCatalogue(ctx, f.priya.ID, listed.ID)
	if err != nil {
		t.Fatalf("second add: %v", err)
	}
	if again.New || again.Conversation.ID != accepted.Conversation.ID {
		t.Error("adding twice should find the same chat, not open another")
	}
}

// The listing is checked when adding, not only when the list was drawn.
// Otherwise taking an agent out of the catalogue leaves it addable forever by
// anyone who kept the identifier.
func TestAnAgentTakenOutOfTheCatalogueCannotBeAdded(t *testing.T) {
	ctx := context.Background()
	f := setup(t)
	svc := agents.New(f.db)

	listed, err := svc.Create(ctx, f.owner.ID, agents.CreateInput{
		Handle: "weather", DisplayName: "Weather", Listed: true,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	no := false
	if _, err := svc.Update(ctx, f.owner.ID, listed.ID, agents.UpdateInput{Listed: &no}); err != nil {
		t.Fatalf("unlist: %v", err)
	}

	if _, err := f.svc.AddFromCatalogue(ctx, f.priya.ID, listed.ID); code(t, err) != "agent_not_listed" {
		t.Fatalf("adding an unlisted agent got %v, want agent_not_listed", err)
	}
}

// An agent that was never on offer is not addable merely because somebody
// knows its identifier.
func TestAPrivateAgentCannotBeAddedFromTheCatalogue(t *testing.T) {
	ctx := context.Background()
	f := setup(t)
	svc := agents.New(f.db)

	private, err := svc.Create(ctx, f.priya.ID, agents.CreateInput{
		Handle: "priyas-mail", DisplayName: "Mail", Private: true,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	stranger := f.owner.ID
	if _, err := f.svc.AddFromCatalogue(ctx, stranger, private.ID); code(t, err) != "agent_not_listed" {
		t.Fatalf("adding a private agent got %v, want agent_not_listed", err)
	}
}

// The schema refuses the contradiction rather than trusting every caller to
// notice it: an agent cannot be hidden from everyone and offered to everyone.
func TestAnAgentCannotBeBothPrivateAndListed(t *testing.T) {
	ctx := context.Background()
	f := setup(t)
	svc := agents.New(f.db)

	if _, err := svc.Create(ctx, f.priya.ID, agents.CreateInput{
		Handle: "impossible", DisplayName: "Both", Private: true, Listed: true,
	}); err == nil {
		t.Fatal("creating an agent that is both private and listed was allowed")
	}
}
