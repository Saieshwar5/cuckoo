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
export { HubClient, type ActivityState, type ClientOptions, type SendOptions } from "./client.ts";
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
export { ProtocolError, StoppedError } from "./errors.ts";
export {
  EVENT_CONVERSATION_JOINED,
  EVENT_CONVERSATION_LEFT,
  EVENT_MESSAGE_CREATED,
  EVENT_SCHEDULE_DELETED,
  EVENT_SCHEDULE_REQUESTED,
  EVENT_SCHEDULE_UPDATED,
  EVENT_STOP_REQUESTED,
  InFlight,
  SeenEvents,
  dispatch,
  parseEnvelope,
  type Envelope,
  type Handlers,
  type JoinHandler,
  type LeaveHandler,
  type MessageHandler,
  type ScheduleHandler,
  type StopHandler,
  type StopRequest,
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
export { Schedules, parseSchedule, type Cadence, type Schedule, type ScheduleChange } from "./schedules.ts";
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
