import { render } from '@testing-library/react-native';
import React from 'react';

import type { Conversation } from '@/api/types';
import { rowActivity } from '@/chat/activity';
import { badge, ChatRow, preview } from '@/components/ChatRow';

const dm: Conversation = {
  id: 'cnv_1',
  kind: 'dm',
  created_at: '2026-09-01T00:00:00Z',
  participants: [
    { kind: 'user', id: 'usr_1', display_name: 'Priya' },
    { kind: 'agent', id: 'agt_1', display_name: 'SBI Support', handle: 'sbi', status: 'connected' },
  ],
  last_message: {
    id: 'msg_1',
    conversation_id: 'cnv_1',
    sender: { kind: 'user', id: 'usr_1' },
    body: { text: 'my payment failed' },
    reply_to: null,
    status: 'complete',
    truncated: false,
    delivery_status: 'delivered',
    created_at: new Date().toISOString(),
  },
};

describe('ChatRow', () => {
  it('shows the agent, the last message with its ticks, and the status dot', async () => {
    const view = await render(<ChatRow conversation={dm} onPress={() => {}} />);
    expect(view.getByText('SBI Support')).toBeTruthy();
    expect(view.getByText('my payment failed')).toBeTruthy();
    expect(view.getByTestId('ticks-delivered')).toBeTruthy();
    expect(view.getByTestId('status-dot')).toBeTruthy();
  });

  it('shows no ticks on what the agent said', async () => {
    const theirs = {
      ...dm,
      last_message: { ...dm.last_message!, sender: { kind: 'agent' as const, id: 'agt_1' } },
    };
    const view = await render(<ChatRow conversation={theirs} onPress={() => {}} />);
    expect(view.queryByTestId('ticks-delivered')).toBeNull();
  });

  it('previews a stream in progress as typing', () => {
    expect(preview({ ...dm.last_message!, status: 'streaming', body: {} })).toBe('Typing…');
    expect(preview(null)).toBe('No messages yet');
  });

  it('says what a busy agent is doing in place of the last message', async () => {
    const view = await render(
      <ChatRow conversation={dm} onPress={() => {}} activity="Checking the weather…" />,
    );
    expect(view.getByText('Checking the weather…')).toBeTruthy();
    expect(view.getByTestId('working-dots')).toBeTruthy();
    expect(view.queryByText('my payment failed')).toBeNull();
    expect(view.queryByTestId('ticks-delivered')).toBeNull();
  });

  it('carries an unread badge, written short past 99', async () => {
    const view = await render(<ChatRow conversation={{ ...dm, unread_count: 3 }} onPress={() => {}} />);
    expect(view.getByTestId('row-unread-cnv_1')).toBeTruthy();
    expect(view.getByText('3')).toBeTruthy();
    expect(badge(100)).toBe('99+');
    const none = await render(<ChatRow conversation={dm} onPress={() => {}} />);
    expect(none.queryByTestId('row-unread-cnv_1')).toBeNull();
  });

  it('reads a busy line from a reply being written, then from what the agent said it was doing', () => {
    const now = Date.parse('2026-09-05T00:00:00Z');
    const writingNow = {
      ...dm,
      last_message: {
        ...dm.last_message!,
        sender: { kind: 'agent' as const, id: 'agt_1' },
        status: 'streaming' as const,
      },
    };
    expect(rowActivity(writingNow, undefined)).toBe('writing…');
    expect(rowActivity(dm, { state: 'working', label: 'Searching flights', until: now + 5000 })).toBe(
      'Searching flights…',
    );
    expect(rowActivity(dm, { state: 'working', label: null, until: now + 5000 })).toBe('working…');
    expect(rowActivity(dm, { state: 'thinking', label: null, until: now + 5000 })).toBe('thinking…');
    expect(rowActivity(dm, undefined)).toBeNull();
  });
});
