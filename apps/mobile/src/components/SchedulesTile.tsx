import { useRouter } from 'expo-router';
import React from 'react';

import type { Contact } from '@/api/types';
import { t } from '@/i18n';
import type { Palette } from '@/theme';

import { ActionTile } from './ActionTile';

// SchedulesTile is the way into an agent's schedules from its profile, for
// an agent that keeps them, and nothing at all for one that does not.
export function SchedulesTile({ contact, colors }: { contact: Contact | null | undefined; colors: Palette }) {
  const router = useRouter();
  if (!contact?.agent.supports_schedules) return null;
  return (
    <ActionTile
      icon="alarm-outline"
      label={t('agent.schedules')}
      onPress={() => router.push({ pathname: '/schedules/[id]', params: { id: contact.conversation_id } })}
      colors={colors}
      testID="agent-schedules"
    />
  );
}
