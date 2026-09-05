import { Ionicons } from '@expo/vector-icons';
import * as Clipboard from 'expo-clipboard';
import { useLocalSearchParams, useRouter } from 'expo-router';
import React, { useState } from 'react';
import { Pressable, ScrollView, StyleSheet, Text, View } from 'react-native';

import { useAgent, useAgents } from '@/agents/AgentsProvider';
import { describeAgentError } from '@/agents/form';
import type { AgentStatus, BindingMode } from '@/api/types';
import { Button } from '@/components/Button';
import { ConfirmSheet } from '@/components/ConfirmSheet';
import { Screen } from '@/components/Screen';
import { TextField } from '@/components/TextField';
import { TopBar } from '@/components/TopBar';
import { hubUrl } from '@/config';
import { t } from '@/i18n';
import { radius, spacing, type, useStyles, useTheme, type Palette, type Theme } from '@/theme';

// snippet is the whole of a backend, with this agent's secret and this
// hub's address already in it. Pasting it into a terminal is the setup.
function snippet(secret: string): string {
  return [
    'pip install cuckoo-agent',
    '',
    'from cuckoo import Agent',
    '',
    `agent = Agent(secret="${secret}", hub="${hubUrl}")`,
    '',
    '@agent.on_message',
    'async def handle(msg, conv):',
    '    await conv.send(f"You said: {msg.text}")',
    '',
    'agent.run()',
  ].join('\n');
}

// Connect: how a backend comes to answer for an agent. Pick socket or
// webhook, generate the secret, paste it into your code, and watch the
// status line flip to Connected the moment the code speaks.
export default function ConnectScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const router = useRouter();
  const { colors } = useTheme();
  const styles = useStyles(makeStyles);
  const agent = useAgent(id);
  const { controller } = useAgents();
  const [mode, setMode] = useState<BindingMode>('socket');
  const [url, setUrl] = useState('');
  const [secret, setSecret] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [confirm, setConfirm] = useState<'regenerate' | 'disconnect' | null>(null);
  const [copied, setCopied] = useState<'secret' | 'snippet' | null>(null);

  const binding = agent?.binding ?? null;
  const status = binding?.status ?? null;
  // The form shows when there is nothing to show yet, or when the person
  // asked for a new secret.
  const [replacing, setReplacing] = useState(false);
  const showForm = !secret && (!binding || replacing);

  const generate = async () => {
    if (!agent || !controller) return;
    setBusy(true);
    setError(null);
    try {
      const result = await controller.setBinding(agent.id, {
        mode,
        webhook_url: mode === 'webhook' ? url.trim() : undefined,
      });
      setSecret(result.secret);
      setReplacing(false);
    } catch (err) {
      setError(describeAgentError(err).message);
    } finally {
      setBusy(false);
    }
  };

  const disconnect = async () => {
    if (!agent || !controller) return;
    setBusy(true);
    try {
      await controller.revokeBinding(agent.id);
      setConfirm(null);
      setSecret(null);
      setReplacing(false);
    } catch (err) {
      setError(describeAgentError(err).message);
    } finally {
      setBusy(false);
    }
  };

  const copy = async (what: 'secret' | 'snippet') => {
    if (!secret) return;
    await Clipboard.setStringAsync(what === 'secret' ? secret : snippet(secret));
    setCopied(what);
  };

  return (
    <Screen padded={false}>
      <TopBar title={t('agent.connect.title')} onBack={() => router.back()} />
      <ScrollView contentContainerStyle={styles.body} keyboardShouldPersistTaps="handled">
        {agent ? (
          <Text style={styles.subtitle}>{t('agent.connect.subtitle', { name: agent.display_name })}</Text>
        ) : null}

        {showForm ? (
          <>
            <Text style={styles.section}>{t('agent.connect.mode.title')}</Text>
            <ModeCard
              selected={mode === 'socket'}
              title={t('agent.connect.mode.socket')}
              hint={t('agent.connect.mode.socket.hint')}
              onPress={() => setMode('socket')}
              colors={colors}
              testID="mode-socket"
            />
            <ModeCard
              selected={mode === 'webhook'}
              title={t('agent.connect.mode.webhook')}
              hint={t('agent.connect.mode.webhook.hint')}
              onPress={() => setMode('webhook')}
              colors={colors}
              testID="mode-webhook"
            />
            {mode === 'webhook' ? (
              <View style={styles.urlField}>
                <TextField
                  label={t('agent.connect.url.label')}
                  placeholder={t('agent.connect.url.placeholder')}
                  value={url}
                  onChangeText={setUrl}
                  autoCapitalize="none"
                  autoCorrect={false}
                  keyboardType="url"
                  testID="webhook-url"
                />
              </View>
            ) : null}
            {error ? <Text style={styles.error}>{error}</Text> : null}
            <Button
              title={binding ? t('agent.connect.regenerate') : t('agent.connect.generate')}
              onPress={() => void generate()}
              busy={busy}
              disabled={mode === 'webhook' && !url.trim()}
              testID="generate"
            />
            {replacing ? (
              <Button title={t('common.cancel')} onPress={() => setReplacing(false)} variant="plain" />
            ) : null}
          </>
        ) : null}

        {binding && !showForm && !secret ? (
          <View style={[styles.card, { backgroundColor: colors.surface }]}>
            <Text style={styles.cardLabel}>{t('agent.connect.current')}</Text>
            <StatusLine status={status} colors={colors} />
            <Text style={styles.hint}>
              {binding.mode === 'webhook' ? (binding.webhook_url ?? '') : t('agent.connect.mode.socket')}
            </Text>
            <View style={styles.row}>
              <Button
                title={t('agent.connect.regenerate')}
                onPress={() => setConfirm('regenerate')}
                variant="outline"
                compact
                testID="regenerate"
              />
              <Button
                title={t('agent.connect.disconnect')}
                onPress={() => setConfirm('disconnect')}
                variant="plain"
                tone="danger"
                compact
                testID="disconnect"
              />
            </View>
          </View>
        ) : null}

        {secret ? (
          <>
            <View style={[styles.card, { backgroundColor: colors.surface }]}>
              <Text style={styles.cardLabel}>{t('agent.connect.secret.title')}</Text>
              <Text style={styles.secret} selectable testID="secret">
                {secret}
              </Text>
              <Text style={styles.once}>{t('agent.connect.secret.once')}</Text>
              <Button
                title={copied === 'secret' ? t('agent.connect.copied') : t('agent.connect.copy')}
                onPress={() => void copy('secret')}
                variant="outline"
                icon={copied === 'secret' ? 'checkmark' : 'copy-outline'}
                compact
              />
            </View>
            <View style={[styles.card, { backgroundColor: colors.surface }]}>
              <Text style={styles.cardLabel}>{t('agent.connect.current')}</Text>
              <StatusLine status={status} colors={colors} />
              <Button title={t('agent.connect.done')} onPress={() => router.back()} testID="done" />
            </View>
            <View style={[styles.card, { backgroundColor: colors.surface }]}>
              <Text style={styles.cardLabel}>{t('agent.connect.snippet.title')}</Text>
              <Text style={styles.hint}>{t('agent.connect.snippet.hint')}</Text>
              <View style={[styles.code, { backgroundColor: colors.ground, borderColor: colors.hairline }]}>
                <Text style={styles.codeText} selectable>
                  {snippet(secret)}
                </Text>
              </View>
              <Button
                title={copied === 'snippet' ? t('agent.connect.copied') : t('agent.connect.copy')}
                onPress={() => void copy('snippet')}
                variant="outline"
                icon={copied === 'snippet' ? 'checkmark' : 'copy-outline'}
                compact
              />
            </View>
          </>
        ) : null}
      </ScrollView>
      <ConfirmSheet
        visible={confirm === 'regenerate'}
        title={t('agent.connect.regenerate.title')}
        body={t('agent.connect.regenerate.body')}
        confirmLabel={t('agent.connect.regenerate')}
        onConfirm={() => {
          setConfirm(null);
          setReplacing(true);
        }}
        onCancel={() => setConfirm(null)}
      />
      <ConfirmSheet
        visible={confirm === 'disconnect'}
        title={t('agent.connect.disconnect.title')}
        body={t('agent.connect.disconnect.body')}
        confirmLabel={t('agent.connect.disconnect')}
        destructive
        busy={busy}
        onConfirm={() => void disconnect()}
        onCancel={() => setConfirm(null)}
      />
    </Screen>
  );
}

// StatusLine is the sentence that changes on its own: waiting, connected,
// or unreachable, from the hub's live announcement.
function StatusLine({ status, colors }: { status: AgentStatus | null; colors: Palette }) {
  const connected = status === 'connected';
  const unreachable = status === 'unreachable';
  const color = connected
    ? colors.statusConnected
    : unreachable
      ? colors.statusUnreachable
      : colors.textSecondary;
  const text = connected
    ? t('agent.connect.connected')
    : unreachable
      ? t('agent.connect.unreachable')
      : t('agent.connect.waiting');
  return (
    <View style={line.row}>
      <Ionicons
        name={connected ? 'checkmark-circle' : unreachable ? 'alert-circle' : 'ellipse-outline'}
        size={20}
        color={color}
      />
      <Text style={[line.text, { color: connected ? colors.text : color }]} testID="connect-status">
        {text}
      </Text>
    </View>
  );
}

function ModeCard({
  selected,
  title,
  hint,
  onPress,
  colors,
  testID,
}: {
  selected: boolean;
  title: string;
  hint: string;
  onPress: () => void;
  colors: Palette;
  testID: string;
}) {
  return (
    <Pressable
      accessibilityRole="radio"
      accessibilityState={{ selected }}
      onPress={onPress}
      testID={testID}
      style={[
        mode.card,
        { backgroundColor: colors.surface, borderColor: selected ? colors.accent : colors.surface },
      ]}
    >
      <Ionicons
        name={selected ? 'radio-button-on' : 'radio-button-off'}
        size={22}
        color={selected ? colors.accent : colors.textSecondary}
      />
      <View style={mode.body}>
        <Text style={[mode.title, { color: colors.text }]}>{title}</Text>
        <Text style={[mode.hint, { color: colors.textSecondary }]}>{hint}</Text>
      </View>
    </Pressable>
  );
}

const line = StyleSheet.create({
  row: { flexDirection: 'row', alignItems: 'center', gap: spacing.sm },
  text: type.headline,
});

const mode = StyleSheet.create({
  card: {
    flexDirection: 'row',
    alignItems: 'flex-start',
    gap: spacing.md,
    padding: spacing.lg,
    borderRadius: radius.lg,
    borderWidth: 1.5,
    marginBottom: spacing.sm,
  },
  body: { flex: 1, gap: 2 },
  title: type.headline,
  hint: { ...type.secondary, lineHeight: 20 },
});

const makeStyles = ({ colors }: Theme) =>
  StyleSheet.create({
    body: { padding: spacing.lg, paddingBottom: spacing.xxl, gap: spacing.md },
    subtitle: { ...type.body, color: colors.textSecondary, lineHeight: 22 },
    section: { ...type.label, color: colors.textSecondary, textTransform: 'uppercase', letterSpacing: 0.4 },
    urlField: { marginTop: spacing.sm },
    error: { ...type.secondary, color: colors.danger },
    card: { borderRadius: radius.lg, padding: spacing.lg, gap: spacing.sm },
    cardLabel: { ...type.label, color: colors.textSecondary, textTransform: 'uppercase', letterSpacing: 0.4 },
    secret: { ...type.body, color: colors.text, fontFamily: 'monospace', lineHeight: 22 },
    once: { ...type.caption, color: colors.textSecondary },
    hint: { ...type.secondary, color: colors.textSecondary },
    code: { borderRadius: radius.md, borderWidth: StyleSheet.hairlineWidth, padding: spacing.md },
    codeText: { ...type.secondary, color: colors.text, fontFamily: 'monospace', lineHeight: 20 },
    row: { flexDirection: 'row', flexWrap: 'wrap', gap: spacing.sm, marginTop: spacing.xs },
  });
