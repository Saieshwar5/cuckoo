/**
 * @cuckoo/agent — connect an agent backend to a Cuckoo hub.
 *
 * Two ways to receive, one way to answer:
 *
 *   - `Agent` holds a socket open. One agent, no public address, no deploy.
 *   - `WebhookReceiver` verifies what the hub posts. Any number of agents
 *     behind one URL, which is what a service answering for thousands needs.
 *
 * Both hand your handler the same `Message` and `Conversation`.
 */

export { Agent, type AgentOptions } from "./agent.ts";
export { HubClient, type ClientOptions, type SendOptions } from "./client.ts";
export { Conversation, Stream } from "./conversation.ts";
export {
  Management,
  type AgentInfo,
  type BindingInfo,
  type Code,
  type CodeSummary,
  type CreateAgentInput,
  type ManagementOptions,
  type UpdateAgentInput,
} from "./management.ts";
export { ProtocolError } from "./errors.ts";
export {
  EVENT_CONVERSATION_JOINED,
  EVENT_CONVERSATION_LEFT,
  EVENT_MESSAGE_CREATED,
  SeenEvents,
  dispatch,
  parseEnvelope,
  type Envelope,
  type Handlers,
  type JoinHandler,
  type LeaveHandler,
  type MessageHandler,
} from "./events.ts";
export {
  DEFAULT_HUB,
  type Action,
  type Attachment,
  type Buttons,
  type Choice,
  type Link,
  type Message,
  type PairToken,
  type Participant,
  type ReplyRef,
  type Sender,
} from "./models.ts";
export { CLOSE_BINDING_GONE, CLOSE_REPLACED, SocketSession } from "./socket.ts";
export {
  HEADER_EVENT,
  HEADER_EVENT_ID,
  HEADER_SIGNATURE,
  HEADER_TIMESTAMP,
  SignatureError,
  WebhookReceiver,
  sign,
  signingKey,
  verify,
  type ReceiverOptions,
  type SecretLookup,
} from "./webhook.ts";
