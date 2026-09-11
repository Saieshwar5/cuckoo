import * as Clipboard from 'expo-clipboard';
import { useFocusEffect, useLocalSearchParams, useRouter } from 'expo-router';
import React, { useCallback, useEffect, useRef, useState } from 'react';
import { ActivityIndicator, AppState, FlatList, Pressable, StyleSheet, Text, View } from 'react-native';
import { Ionicons } from '@expo/vector-icons';

import type { Button as ButtonSpec } from '@/api/types';
import type { ChatMessage } from '@/chat/store';
import { useAgents, useContact } from '@/agents/AgentsProvider';
import { keys } from '@/cache/cache';
import { useMemory } from '@/cache/CacheProvider';
import { activityLine } from '@/chat/activity';
import { unseenSince } from '@/chat/store';
import { useChat } from '@/chat/useChat';
import { useChats, useConversation } from '@/chats/useChats';
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
  const { open } = useChats();
  const [replyTo, setReplyTo] = useState<ChatMessage | null>(null);
  // The message a long-press opened the menu for.
  const [menu, setMenu] = useState<ChatMessage | null>(null);
  // The header's menu, which is about the agent rather than about a message.
  const [chatMenu, setChatMenu] = useState(false);
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

  // On screen and the app in front: the person can see this chat, so what
  // lands in it is read, and the list keeps no badge for it.
  const { setVisible } = chat;
  useFocusEffect(
    useCallback(() => {
      open(id);
      setVisible(AppState.currentState === 'active');
      const sub = AppState.addEventListener('change', (next) => setVisible(next === 'active'));
      return () => {
        sub.remove();
        setVisible(false);
        open(null);
      };
    }, [id, open, setVisible]),
  );

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
  // What somebody reaches for while they are in the conversation and mildly
  // annoyed. Muting first, because a mute that is three screens away is a
  // block: people do not go looking, they end it.
  const agentActions = (): SheetAction[] => {
    if (who?.kind !== 'agent') return [];
    const agentId = who.id;
    const muted = !!contact?.muted_until && new Date(contact.muted_until) > new Date();
    return [
      muted
        ? {
            label: t('chat.menu.unmute'),
            icon: 'notifications-outline' as const,
            onPress: () => void agentsController?.settings(agentId, { muted_until: null }),
            testID: 'chat-unmute',
          }
        : {
            label: t('chat.menu.mute'),
            icon: 'notifications-off-outline' as const,
            onPress: () =>
              void agentsController?.settings(agentId, {
                muted_until: new Date(Date.now() + 8 * 60 * 60 * 1000).toISOString(),
              }),
            testID: 'chat-mute',
          },
      {
        label: t('chat.menu.profile'),
        icon: 'person-circle-outline' as const,
        onPress: () => router.push({ pathname: '/agent/[id]', params: { id: agentId } }),
        testID: 'chat-profile',
      },
      {
        label: t('chat.menu.block'),
        icon: 'ban-outline' as const,
        onPress: () => void agentsController?.block(agentId),
        testID: 'chat-block',
      },
    ];
  };

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
  const busyLine = activityLine(chat.activity, chat.writing);
  return (
    <Screen padded={false}>
      <ChatHeader
        name={name}
        status={who?.kind === 'agent' ? (who.status ?? null) : undefined}
        avatar={who?.kind === 'agent' ? agentAvatar(who.id, who.has_avatar) : null}
        activity={busyLine}
        onBack={back}
        onOpenProfile={
          who?.kind === 'agent'
            ? () => router.push({ pathname: '/agent/[id]', params: { id: who.id } })
            : undefined
        }
        onMore={who?.kind === 'agent' ? () => setChatMenu(true) : undefined}
      />
      <View style={[styles.wall, { backgroundColor: colors.wallpaper }]}>
        {chat.loading ? (
          <ActivityIndicator color={colors.accent} style={styles.center} />
        ) : messages.length === 0 && !chat.activity ? (
          <View style={styles.center}>
            <Text style={[styles.empty, { color: colors.textSecondary }]}>
              {chat.trimmed ? t('chat.history.trimmed') : t('chat.empty', { name })}
            </Text>
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
            ListHeaderComponent={chat.activity && !chat.writing ? <TypingBubble /> : null}
            ListFooterComponent={
              chat.loadingOlder ? (
                <ActivityIndicator color={colors.accent} style={styles.older} />
              ) : chat.trimmed ? (
                <Text style={[styles.trimmed, { color: colors.textSecondary }]} testID="history-trimmed">
                  {t('chat.history.trimmed')}
                </Text>
              ) : null
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
      ) : chat.noReply && !contact?.blocked ? (
        // Silence after a message the agent did receive. Said once, quietly,
        // with the one thing worth doing about it.
        <View style={[styles.noReply, { backgroundColor: colors.surface }]} testID="no-reply">
          <Text style={[styles.noReplyText, { color: colors.textSecondary }]}>{t('chat.noReply')}</Text>
          {chat.lastWords ? (
            <Pressable
              accessibilityRole="button"
              onPress={() => void chat.sendAgain()}
              hitSlop={8}
              testID="send-again"
            >
              <Text style={[styles.noReplyText, styles.noReplyAction, { color: colors.accentStrong }]}>
                {t('chat.noReply.again')}
              </Text>
            </Pressable>
          ) : null}
        </View>
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
            busy={chat.busy}
            onStop={() => void chat.stopAgent()}
          />
          <ActionSheet
            visible={!!menu}
            onClose={() => setMenu(null)}
            actions={menu ? actionsFor(menu) : []}
          />
          <ActionSheet visible={chatMenu} onClose={() => setChatMenu(false)} actions={agentActions()} />
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
  // Where the hub stopped keeping, said once above the oldest message in
  // the same grey as a day's pill, and not dressed up as a warning.
  trimmed: {
    ...type.caption,
    textAlign: 'center',
    paddingVertical: spacing.md,
    paddingHorizontal: spacing.xl,
  },
  blocked: { padding: spacing.lg, gap: spacing.md, alignItems: 'center' },
  blockedText: { ...type.secondary, textAlign: 'center', lineHeight: 20 },
  // A line between the chat and the composer, quiet but not hidden.
  offline: {
    ...type.caption,
    textAlign: 'center',
    paddingVertical: spacing.xs + 2,
    paddingHorizontal: spacing.lg,
  },
  noReply: {
    flexDirection: 'row',
    justifyContent: 'center',
    alignItems: 'center',
    gap: spacing.md,
    paddingVertical: spacing.xs + 2,
    paddingHorizontal: spacing.lg,
  },
  noReplyText: type.caption,
  noReplyAction: { fontWeight: '700' },
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
