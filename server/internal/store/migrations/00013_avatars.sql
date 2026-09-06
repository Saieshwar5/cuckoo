-- Faces: a photo for a person, a logo for an agent.
--
-- Both are ordinary media rows — uploaded, sniffed and thumbnailed by the
-- same path as a photo in a chat — pointed at from the profile that uses
-- them. Nothing about a picture changes because it is a face; only who is
-- allowed to see it does, and that is decided where it is served.
--
-- An agent's logo is the one picture in Cuckoo that is public. The card a
-- stranger opens from a QR code, in a browser, before they have an account,
-- has to show it: a company asking for trust with a grey disc and the word
-- "Unverified" is asking for too much. Its owner published it deliberately,
-- to be handed to strangers, which is what a logo is.

-- +goose Up

ALTER TABLE agents ADD COLUMN avatar_media_id uuid REFERENCES media (id);
ALTER TABLE users  ADD COLUMN avatar_media_id uuid REFERENCES media (id);

-- +goose Down

ALTER TABLE users  DROP COLUMN avatar_media_id;
ALTER TABLE agents DROP COLUMN avatar_media_id;
