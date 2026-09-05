import { render } from '@testing-library/react-native';
import React from 'react';

import type { Conversation } from '@/api/types';
import { ChatRow, preview } from '@/components/ChatRow';

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
    expect(view.getByText(/✓✓ my payment failed/)).toBeTruthy();
    expect(view.getByTestId('status-dot')).toBeTruthy();
  });

  it('previews a stream in progress as typing', () => {
    expect(preview({ ...dm.last_message!, status: 'streaming', body: {} })).toBe('Typing…');
    expect(preview(null)).toBe('No messages yet');
  });
});
