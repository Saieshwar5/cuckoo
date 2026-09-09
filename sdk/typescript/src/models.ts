/**
 * What a handler is given, and what the hub is told: the wire shapes of the
 * agent protocol, in the names TypeScript would have chosen.
 *
 * The hub speaks snake_case; this package speaks camelCase and converts at the
 * edge, so nothing above has to remember which side of the wire it is on.
 */

/** Where a hub lives unless told otherwise. A development hub is http://localhost:8080. */
export const DEFAULT_HUB = "https://cuckoo.onl";

/** Who wrote a message. A backend never learns more about a person than this. */
export interface Sender {
  kind: string;
  id: string;
  displayName: string;
}

/** A current member of a conversation. `isMe` marks your own agent. */
export interface Participant {
  kind: string;
  id: string;
  displayName: string;
  handle?: string;
  isMe: boolean;
}

/** A tap: which button, on which of your messages. */
export interface Action {
  buttonId: string;
  sourceMessageId: string;
}

/** The message a message answers, with enough to draw the quote. */
export interface ReplyRef {
  id: string;
  senderKind: string;
  textPreview: string;
}

/**
 * A file on a message. The bytes are not here: fetch them with
 * `agent.download(mediaId)` when you want them, which matters when the message
 * is a 40 MB video and your handler only wanted the caption.
 */
export interface Attachment {
  mediaId: string;
  kind: string;
  mimeType: string;
  fileName: string;
  byteSize: number;
  width: number;
  height: number;
  hasThumbnail: boolean;
  /** For a recording or video: how long it runs, and its loudness over time. */
  durationMs: number;
  waveform: number[];
}

/** One thing said in a conversation. */
export interface Message {
  id: string;
  conversationId: string;
  text: string;
  sender: Sender;
  createdAt: string;
  status: string;
  truncated: boolean;
  /** Set when the person tapped one of your buttons; `text` is then its label. */
  action?: Action;
  replyTo?: ReplyRef;
  /**
   * The hub's own seal over the message, made with a key only the hub holds.
   * Keep it with the message and leave it unchanged: it is what lets a copy
   * you kept past the hub's window be served back later.
   */
  signature?: string;
  /** The event that carried this message, when it arrived as one. */
  eventId?: string;
  attachments: Attachment[];
}

/** The code someone scanned to reach the agent, with whatever the owner put in it. */
export interface PairToken {
  id: string;
  payload?: unknown;
}

/** A button that opens something instead of answering. The tap tells you nothing. */
export interface Link {
  label: string;
  /** Starts with `https`, `http`, `upi` or `tel`. */
  url: string;
  style?: string;
}

/** A button that answers: the id comes back when it is tapped. */
export interface Choice {
  id: string;
  label: string;
  style?: string;
}

/** Rows of buttons. Three to a row, three rows, is the hub's limit. */
export type Buttons = (Choice | Link)[][];

// -- parsing ---------------------------------------------------------------

type Wire = Record<string, unknown>;

const str = (v: unknown): string => (typeof v === "string" ? v : "");
const num = (v: unknown): number => (typeof v === "number" ? v : Number(v) || 0);
const obj = (v: unknown): Wire => (v && typeof v === "object" ? (v as Wire) : {});
const arr = (v: unknown): unknown[] => (Array.isArray(v) ? v : []);

export function parseSender(data: unknown): Sender {
  const w = obj(data);
  return { kind: str(w.kind), id: str(w.id), displayName: str(w.display_name) };
}

export function parseParticipant(data: unknown): Participant {
  const w = obj(data);
  const handle = str(w.handle);
  return {
    kind: str(w.kind),
    id: str(w.id),
    displayName: str(w.display_name),
    ...(handle ? { handle } : {}),
    isMe: Boolean(w.is_me),
  };
}

export function parseAttachment(data: unknown): Attachment {
  const w = obj(data);
  return {
    // An upload calls it id; a message calls it media_id. Same file.
    mediaId: str(w.media_id) || str(w.id),
    kind: str(w.kind),
    mimeType: str(w.mime_type),
    fileName: str(w.file_name),
    byteSize: num(w.byte_size),
    width: num(w.width),
    height: num(w.height),
    hasThumbnail: Boolean(w.has_thumbnail),
    durationMs: num(w.duration_ms),
    waveform: arr(w.waveform).map(num),
  };
}

/**
 * Read a message. The same shape arrives in an event, in a history page and
 * as the answer to a send, so this is the only parser there is.
 */
export function parseMessage(data: unknown, conversationId: string, eventId?: string): Message {
  const w = obj(data);
  const body = obj(w.body);
  const action = obj(body.action);
  const reply = obj(w.reply_to);
  const signature = str(w.signature);
  return {
    id: str(w.id),
    conversationId,
    text: str(body.text),
    sender: parseSender(w.sender),
    createdAt: str(w.created_at),
    status: str(w.status) || "complete",
    truncated: Boolean(w.truncated),
    ...(action.button_id
      ? { action: { buttonId: str(action.button_id), sourceMessageId: str(action.source_message_id) } }
      : {}),
    ...(reply.id
      ? {
          replyTo: {
            id: str(reply.id),
            senderKind: str(reply.sender_kind),
            textPreview: str(reply.text_preview),
          },
        }
      : {}),
    ...(signature ? { signature } : {}),
    ...(eventId ? { eventId } : {}),
    attachments: arr(body.attachments).map(parseAttachment),
  };
}

export function parsePairToken(data: unknown): PairToken | undefined {
  const w = obj(data);
  if (!w.id) return undefined;
  return { id: str(w.id), payload: w.payload };
}

// -- writing ---------------------------------------------------------------

function isLink(button: Choice | Link): button is Link {
  return "url" in button;
}

/**
 * Rows of buttons as the hub wants them. A button carries an id or a url and
 * never both — the hub refuses one that carries two.
 */
export function buttonsToWire(buttons?: Buttons): Record<string, string>[][] | undefined {
  if (!buttons?.length) return undefined;
  return buttons.map((row) =>
    row.map((button) => ({
      ...(isLink(button) ? { url: button.url } : { id: button.id }),
      label: button.label,
      style: button.style ?? "default",
    })),
  );
}

export function quickRepliesToWire(labels?: string[]): { label: string }[] | undefined {
  if (!labels?.length) return undefined;
  return labels.map((label) => ({ label }));
}
