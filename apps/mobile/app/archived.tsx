import { useRouter } from 'expo-router';
import React from 'react';
import { FlatList } from 'react-native';

import { useAgents } from '@/agents/AgentsProvider';
import { isMuted } from '@/agents/store';
import { rowActivity } from '@/chat/activity';
import { arrange } from '@/chats/store';
import { useChats } from '@/chats/useChats';
import { ChatRow } from '@/components/ChatRow';
import { EmptyState } from '@/components/EmptyState';
import { Screen } from '@/components/Screen';
import { TopBar } from '@/components/TopBar';
import { t } from '@/i18n';

// Archived: the chats the person set aside. Same rows, same liveness, one
// step out of the way. Archiving is undone from the agent's profile, which
// is also where it was done.
export default function ArchivedScreen() {
  const { conversations, activity } = useChats();
  const { contacts } = useAgents();
  const router = useRouter();
  const { archived } = arrange(conversations, contacts);
  const byAgent = new Map(contacts.map((c) => [c.agent.id, c]));
  const back = () => (router.canGoBack() ? router.back() : router.replace('/(tabs)/chats'));

  return (
    <Screen padded={false}>
      <TopBar title={t('chats.archived')} onBack={back} />
      <FlatList
        data={archived}
        extraData={activity}
        keyExtractor={(c) => c.id}
        renderItem={({ item }) => {
          const agent = item.participants.find((p) => p.kind === 'agent');
          const contact = agent ? byAgent.get(agent.id) : undefined;
          return (
            <ChatRow
              conversation={item}
              muted={contact ? isMuted(contact) : false}
              activity={rowActivity(item, activity[item.id])}
              onPress={() => router.push({ pathname: '/chat/[id]', params: { id: item.id } })}
            />
          );
        }}
        ListEmptyComponent={
          <EmptyState
            icon="archive-outline"
            title={t('chats.archived.empty.title')}
            subtitle={t('chats.archived.empty.subtitle')}
          />
        }
        contentContainerStyle={archived.length === 0 ? grow : undefined}
      />
    </Screen>
  );
}

const grow = { flexGrow: 1 } as const;
