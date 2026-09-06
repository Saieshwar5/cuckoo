import * as Clipboard from 'expo-clipboard';
import { useLocalSearchParams, useRouter } from 'expo-router';
import React, { useEffect, useRef, useState } from 'react';
import { ActivityIndicator, FlatList, Pressable, StyleSheet, Text, View } from 'react-native';
import { Ionicons } from '@expo/vector-icons';

import type { Button as ButtonSpec } from '@/api/types';
import type { ChatMessage } from '@/chat/store';
import { useAgents, useContact } from '@/agents/AgentsProvider';
import { keys } from '@/cache/cache';
import { useMemory } from '@/cache/CacheProvider';
import { unseenSince } from '@/chat/store';
import { useChat } from '@/chat/useChat';
import { useConversation } from '@/chats/useChats';
import { counterpart } from '@/components/ChatRow';
import { Button } from '@/components/Button';
import { Screen } from '@/components/Screen';
import { ActionSheet, type SheetAction } from '@/components/ActionSheet';
import { Bubble } from '@/components/chat/Bubble';
import { ChatHeader } from '@/components/chat/ChatHeader';
import { Composer } from '@/components/chat/Composer';
import { DayDivider } from '@/components/chat/DayDivider';
import { QuickReplies } from '@/components/chat/QuickReplies';
import { TypingBubble } from '@/components/chat/TypingBubble';
import { t } from '@/i18n';
import { agentAvatar } from '@/media/avatar';
import type { PickedFile } from '@/media/pick';
import { spacing, type, useTheme } from '@/theme';
import { openLink } from '@/util/open';
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
  // The message a long-press opened the menu for.
  const [menu, setMenu] = useState<ChatMessage | null>(null);
  const memory = useMemory();

  // Drafts: what was typed here and not sent, kept on the device.
  const draftKey = memory && id ? keys.draft(memory.userId, id) : null;
  const [draft, setDraft] = useState<string | null>(null);
  const draftTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  useEffect(() => {
    if (!memory || !draftKey) return;
    let cancelled = false;
    void memory.cache.get<string>(draftKey).then((saved) => {
      if (!cancelled) setDraft(saved ?? '');
    });
    return () => {
      cancelled = true;
    };
  }, [memory, draftKey]);
  const keepDraft = (text: string) => {
    if (!memory || !draftKey) return;
    if (draftTimer.current) clearTimeout(draftTimer.current);
    draftTimer.current = setTimeout(() => {
      void (text.trim() ? memory.cache.set(draftKey, text) : memory.cache.remove(draftKey));
    }, 300);
  };

  // Scrolled away from the newest: a way back, with how much arrived since.
  const list = useRef<FlatList<ChatMessage>>(null);
  const [awayFrom, setAwayFrom] = useState<string | null>(null);

  const back = () => (router.canGoBack() ? router.back() : router.replace('/(tabs)/chats'));
  const dayLabels = { today: t('chat.day.today'), yesterday: t('chat.day.yesterday') };

  const send = (text: string, files: PickedFile[]) => {
    void chat.send({ text, files, replyTo: replyTo ?? undefined });
    setReplyTo(null);
    if (memory && draftKey) {
      if (draftTimer.current) clearTimeout(draftTimer.current);
      void memory.cache.remove(draftKey);
    }
  };
  const tap = (m: ChatMessage, b: ButtonSpec) =>
    void chat.send({
      action: { button_id: b.id as string, source_message_id: m.id, label: b.label },
    });

  // What can be done with one message. Copy needs words; delete-for-me
  // needs an id, which one of ours still waiting for the hub does not have.
  const actionsFor = (m: ChatMessage): SheetAction[] => [
    {
      icon: 'arrow-undo-outline',
      label: t('chat.action.reply'),
      onPress: () => setReplyTo(m),
      testID: 'menu-reply',
    },
    ...(m.body.text
      ? [
          {
            icon: 'copy-outline' as const,
            label: t('chat.action.copy'),
            onPress: () => void Clipboard.setStringAsync(m.body.text ?? ''),
            testID: 'menu-copy',
          },
        ]
      : []),
    ...(m.localKey
      ? []
      : [
          {
            icon: 'trash-outline' as const,
            label: t('chat.action.delete'),
            onPress: () => void chat.deleteForMe(m.id),
            testID: 'menu-delete',
          },
        ]),
  ];

  const messages = chat.messages;
  const unseen = unseenSince(messages, awayFrom);
  // The agent's backend, as the hub last saw it. A person sending into
  // silence deserves to be told the silence is not theirs.
  const offline = who?.kind === 'agent' && who.status !== 'connected' && !contact?.blocked;
  return (
    <Screen padded={false}>
      <ChatHeader
        name={name}
        status={who?.kind === 'agent' ? (who.status ?? null) : undefined}
        avatar={who?.kind === 'agent' ? agentAvatar(who.id, who.has_avatar) : null}
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
            {who?.kind === 'agent' && who.starters?.length ? (
              <View style={styles.starters}>
                <QuickReplies labels={who.starters} onPick={(label) => send(label, [])} />
              </View>
            ) : null}
          </View>
        ) : (
          <FlatList
            ref={list}
            inverted
            data={messages}
            onScroll={(e) => {
              const away = e.nativeEvent.contentOffset.y > 240;
              setAwayFrom((current) => (away ? (current ?? messages[0]?.id ?? null) : null));
            }}
            scrollEventThrottle={100}
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
                    onReply={setMenu}
                    onButton={tap}
                    onOpen={(url) => void openLink(url)}
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
        {awayFrom ? (
          <Pressable
            accessibilityRole="button"
            accessibilityLabel={t('chat.newer')}
            onPress={() => list.current?.scrollToOffset({ offset: 0, animated: true })}
            style={[styles.toBottom, { backgroundColor: colors.surfaceStrong }]}
            testID="scroll-bottom"
          >
            <Ionicons name="chevron-down" size={22} color={colors.accent} />
            {unseen > 0 ? (
              <View style={[styles.count, { backgroundColor: colors.accent }]}>
                <Text style={[styles.countText, { color: colors.onAccent }]} testID="unseen-count">
                  {unseen}
                </Text>
              </View>
            ) : null}
          </Pressable>
        ) : null}
      </View>
      {offline ? (
        <Text
          style={[styles.offline, { color: colors.textSecondary, backgroundColor: colors.surface }]}
          testID="offline-line"
        >
          {t('chat.offline', { name })}
        </Text>
      ) : null}
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
          <Composer
            replyTo={replyTo}
            agentName={name}
            onCancelReply={() => setReplyTo(null)}
            onSend={send}
            draft={draft}
            onDraft={keepDraft}
          />
          <ActionSheet
            visible={!!menu}
            onClose={() => setMenu(null)}
            actions={menu ? actionsFor(menu) : []}
          />
        </>
      )}
    </Screen>
  );
}

const styles = StyleSheet.create({
  // The agent's suggestions, under the words that say the chat is new.
  starters: { marginTop: spacing.md, alignSelf: 'stretch' },
  wall: { flex: 1 },
  list: { paddingVertical: spacing.sm },
  center: { flex: 1, alignItems: 'center', justifyContent: 'center', padding: spacing.xl, gap: spacing.sm },
  empty: { ...type.body, textAlign: 'center' },
  hint: { ...type.caption, textAlign: 'center' },
  older: { paddingVertical: spacing.md },
  blocked: { padding: spacing.lg, gap: spacing.md, alignItems: 'center' },
  blockedText: { ...type.secondary, textAlign: 'center', lineHeight: 20 },
  // A line between the chat and the composer, quiet but not hidden.
  offline: {
    ...type.caption,
    textAlign: 'center',
    paddingVertical: spacing.xs + 2,
    paddingHorizontal: spacing.lg,
  },
  toBottom: {
    position: 'absolute',
    right: spacing.md,
    bottom: spacing.md,
    width: 40,
    height: 40,
    borderRadius: 20,
    alignItems: 'center',
    justifyContent: 'center',
  },
  count: {
    position: 'absolute',
    top: -6,
    right: -4,
    minWidth: 18,
    height: 18,
    borderRadius: 9,
    paddingHorizontal: 4,
    alignItems: 'center',
    justifyContent: 'center',
  },
  countText: { fontSize: 11, fontWeight: '700' },
});
