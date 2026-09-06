import { useLocalSearchParams, useRouter } from 'expo-router';
import React, { useState } from 'react';
import { ActivityIndicator, FlatList, StyleSheet, Text, View } from 'react-native';

import type { Button as ButtonSpec } from '@/api/types';
import type { ChatMessage } from '@/chat/store';
import { useAgents, useContact } from '@/agents/AgentsProvider';
import { useChat } from '@/chat/useChat';
import { useConversation } from '@/chats/useChats';
import { counterpart } from '@/components/ChatRow';
import { Button } from '@/components/Button';
import { Screen } from '@/components/Screen';
import { Bubble } from '@/components/chat/Bubble';
import { ChatHeader } from '@/components/chat/ChatHeader';
import { Composer } from '@/components/chat/Composer';
import { DayDivider } from '@/components/chat/DayDivider';
import { QuickReplies } from '@/components/chat/QuickReplies';
import { TypingBubble } from '@/components/chat/TypingBubble';
import { t } from '@/i18n';
import type { PickedFile } from '@/media/pick';
import { spacing, type, useTheme } from '@/theme';
import { formatDay, sameDay } from '@/util/time';

// The conversation: history newest at the bottom, live as it happens, and a
// composer. The list is inverted, so new messages appear where the eye is
// and older pages load above without moving what is on screen.
export default function ChatScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const router = useRouter();
  const { colors } = useTheme();
  const conversation = useConversation(id);
  const who = conversation ? counterpart(conversation) : undefined;
  const name = who?.display_name ?? '';
  const contact = useContact(who?.kind === 'agent' ? who.id : '');
  const { controller: agentsController } = useAgents();
  const chat = useChat(id);
  const [replyTo, setReplyTo] = useState<ChatMessage | null>(null);

  const back = () => (router.canGoBack() ? router.back() : router.replace('/(tabs)/chats'));
  const dayLabels = { today: t('chat.day.today'), yesterday: t('chat.day.yesterday') };

  const send = (text: string, files: PickedFile[]) => {
    void chat.send({ text, files, replyTo: replyTo ?? undefined });
    setReplyTo(null);
  };
  const tap = (m: ChatMessage, b: ButtonSpec) =>
    void chat.send({ action: { button_id: b.id, source_message_id: m.id, label: b.label } });

  const messages = chat.messages;
  return (
    <Screen padded={false}>
      <ChatHeader
        name={name}
        status={who?.kind === 'agent' ? (who.status ?? null) : undefined}
        typing={chat.typing}
        onBack={back}
      />
      <View style={[styles.wall, { backgroundColor: colors.wallpaper }]}>
        {chat.loading ? (
          <ActivityIndicator color={colors.accent} style={styles.center} />
        ) : messages.length === 0 && !chat.typing ? (
          <View style={styles.center}>
            <Text style={[styles.empty, { color: colors.textSecondary }]}>{t('chat.empty', { name })}</Text>
            <Text style={[styles.hint, { color: colors.textSecondary }]}>{t('chat.reply.hint')}</Text>
          </View>
        ) : (
          <FlatList
            inverted
            data={messages}
            keyExtractor={(m) => m.id}
            renderItem={({ item, index }) => {
              // The list runs newest first, so the one before in time is next.
              const older = messages[index + 1];
              const newDay = !older || !sameDay(older.created_at, item.created_at);
              const first = newDay || older.sender.kind !== item.sender.kind;
              // One view per row: on the web an inverted list lays a fragment's
              // children out as separate rows, and reversed.
              return (
                <View>
                  {newDay ? <DayDivider label={formatDay(item.created_at, new Date(), dayLabels)} /> : null}
                  <Bubble
                    message={item}
                    mine={item.sender.kind === 'user'}
                    first={first}
                    agentName={name}
                    onReply={setReplyTo}
                    onButton={tap}
                    onRetry={(key) => void chat.retry(key)}
                  />
                </View>
              );
            }}
            ListHeaderComponent={chat.typing ? <TypingBubble /> : null}
            ListFooterComponent={
              chat.loadingOlder ? <ActivityIndicator color={colors.accent} style={styles.older} /> : null
            }
            onEndReached={() => void chat.loadOlder()}
            onEndReachedThreshold={0.6}
            contentContainerStyle={styles.list}
            keyboardShouldPersistTaps="handled"
          />
        )}
      </View>
      {contact?.blocked ? (
        <View style={[styles.blocked, { backgroundColor: colors.surface }]} testID="blocked-banner">
          <Text style={[styles.blockedText, { color: colors.textSecondary }]}>{t('chat.blocked')}</Text>
          <Button
            title={t('chat.blocked.action')}
            onPress={() => void agentsController?.unblock(contact.agent.id)}
            variant="outline"
            compact
            testID="chat-unblock"
          />
        </View>
      ) : (
        <>
          {chat.quickReplies.length ? (
            <QuickReplies labels={chat.quickReplies} onPick={(label) => send(label, [])} />
          ) : null}
          <Composer replyTo={replyTo} agentName={name} onCancelReply={() => setReplyTo(null)} onSend={send} />
        </>
      )}
    </Screen>
  );
}

const styles = StyleSheet.create({
  wall: { flex: 1 },
  list: { paddingVertical: spacing.sm },
  center: { flex: 1, alignItems: 'center', justifyContent: 'center', padding: spacing.xl, gap: spacing.sm },
  empty: { ...type.body, textAlign: 'center' },
  hint: { ...type.caption, textAlign: 'center' },
  older: { paddingVertical: spacing.md },
  blocked: { padding: spacing.lg, gap: spacing.md, alignItems: 'center' },
  blockedText: { ...type.secondary, textAlign: 'center', lineHeight: 20 },
});
