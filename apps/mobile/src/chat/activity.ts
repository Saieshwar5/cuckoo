import type { Conversation, Frame } from '../api/types';
import { t } from '../i18n';

// What an agent is doing, as the hub last said: thinking, or working at
// something it named. Idle is no activity at all. It lasts until the time
// the hub gave, unless the agent says it again.
export interface Activity {
  state: 'thinking' | 'working';
  label: string | null;
  until: number;
}

type ActivityFrame = Extract<Frame, { type: 'activity' }>;

// activityOf reads an activity frame. Idle, or one already over, is null.
export function activityOf(data: ActivityFrame['data']): Activity | null {
  if (data.state === 'idle') return null;
  return { state: data.state, label: data.label?.trim() || null, until: Date.parse(data.expires_at) };
}

export function isLive(a: Activity | null | undefined, now: number): a is Activity {
  return !!a && a.until > now;
}

// activityLine is the line under a name while an agent is busy, or null.
// The activity given is one still live: the controllers drop what has run
// out, so a screen never has to ask the clock while it draws.
//
// What the agent says it is doing comes first, even with a reply open: a
// model will often write a line, then stop to call a tool, and "writing…"
// would be wrong for as long as the tool runs. An agent clears it when the
// words start again. Otherwise a reply appearing says "writing…".
export function activityLine(a: Activity | null | undefined, writing: boolean): string | null {
  if (a) {
    if (a.state === 'working')
      return a.label ? t('activity.label', { label: a.label }) : t('activity.working');
    return t('activity.thinking');
  }
  return writing ? t('activity.writing') : null;
}

// rowActivity is a chat-list row's busy line: a reply being written into
// it, or what its agent says it is doing.
export function rowActivity(c: Conversation, a: Activity | undefined): string | null {
  const last = c.last_message;
  return activityLine(a, !!last && last.status === 'streaming' && last.sender.kind === 'agent');
}

// unreadChats counts the chats with something unread that the person has
// asked to hear about: the number on the Chats tab.
export function unreadChats(list: Conversation[], muted: (c: Conversation) => boolean): number {
  return list.filter((c) => (c.unread_count ?? 0) > 0 && !muted(c)).length;
}
