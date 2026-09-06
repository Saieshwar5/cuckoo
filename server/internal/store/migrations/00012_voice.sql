-- What a voice note needs beyond its bytes: how long it is, and its shape.
--
-- Neither can be read from the file without decoding the audio, which this
-- server will not do — decoding a stranger's media in the process that
-- serves everyone is how a media server becomes a security problem. Both
-- are recorded by the device that did the recording, which already had the
-- numbers: a phone measures loudness while it records, and so does a
-- browser.
--
-- They are therefore what the sender said, not what the hub verified. That
-- is safe because of what they are for: drawing a bubble. A wrong duration
-- draws a wrong number under a waveform. Neither decides who may read
-- anything.

-- +goose Up

ALTER TABLE media
    -- Milliseconds, for a recording or a video; 0 for anything else.
    ADD COLUMN duration_ms integer NOT NULL DEFAULT 0,
    -- Loudness over time, 0 to 100, a few dozen values: enough for a
    -- waveform a thumb can seek along, small enough to sit in the message
    -- body and be drawn with no audio decoded at all.
    ADD COLUMN waveform    integer[],

    ADD CONSTRAINT media_duration_check CHECK (duration_ms >= 0);

-- +goose Down

ALTER TABLE media
    DROP CONSTRAINT media_duration_check,
    DROP COLUMN waveform,
    DROP COLUMN duration_ms;
