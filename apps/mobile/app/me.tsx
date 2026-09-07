import { useRouter } from 'expo-router';
import React, { useState } from 'react';
import { ScrollView, StyleSheet, Text } from 'react-native';

import { describeAgentError } from '@/agents/form';
import { AvatarPicker } from '@/components/AvatarPicker';
import { Button } from '@/components/Button';
import { Screen } from '@/components/Screen';
import { TextField } from '@/components/TextField';
import { TopBar } from '@/components/TopBar';
import { t } from '@/i18n';
import type { PickedFile } from '@/media/pick';
import { useMediaSource } from '@/media/source';
import { useSession } from '@/session/SessionProvider';
import { spacing, type, useStyles, type Theme } from '@/theme';

// You: your name and your photo.
//
// Your photo is not public the way an agent's logo is. It is fetched with
// your session, and today only you ever see it — every conversation is you
// and an agent, and an agent is told a display name and nothing else. When
// groups arrive, the people in them will see it; nobody else ever will.
export default function ProfileScreen() {
  const router = useRouter();
  const { api, user, setUser } = useSession();
  const styles = useStyles(makeStyles);
  const [name, setName] = useState(user?.display_name ?? '');
  const [picture, setPicture] = useState<PickedFile | null>(null);
  const [avatarMediaID, setAvatarMediaID] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  // Loaded once, because the session remembers only the name and the id.
  React.useEffect(() => {
    let cancelled = false;
    void api.me().then(
      (me) => {
        if (!cancelled) setAvatarMediaID(me.avatar_media_id ?? null);
      },
      () => {},
    );
    return () => {
      cancelled = true;
    };
  }, [api]);

  const { source: current } = useMediaSource(avatarMediaID ?? undefined, { thumb: true });
  const changed = name.trim() !== (user?.display_name ?? '') || !!picture;

  const save = async () => {
    if (!changed || !name.trim()) return;
    setBusy(true);
    setError(null);
    try {
      let mediaID: string | undefined;
      if (picture) {
        const media = await api.uploadMedia({
          uri: picture.uri,
          name: picture.name,
          mimeType: picture.mimeType,
        });
        mediaID = media.id;
      }
      const me = await api.updateMe({ display_name: name.trim(), avatar_media_id: mediaID });
      await setUser({ id: me.id, display_name: me.display_name });
      router.back();
    } catch (err) {
      setError(describeAgentError(err).message);
      setBusy(false);
    }
  };

  return (
    <Screen padded={false}>
      <TopBar title={t('profile.title')} onBack={() => router.back()} />
      <ScrollView contentContainerStyle={styles.body} keyboardShouldPersistTaps="handled">
        <AvatarPicker
          name={name.trim() || '?'}
          source={picture ? { uri: picture.uri } : current}
          onPicked={setPicture}
          testID="pick-my-photo"
        />
        <TextField
          label={t('profile.name')}
          value={name}
          onChangeText={(v) => {
            setName(v);
            setError(null);
          }}
          autoCapitalize="words"
          error={error}
          testID="my-name"
        />
        <Text style={styles.note}>{t('profile.note')}</Text>
        <Button
          title={t('profile.save')}
          onPress={() => void save()}
          busy={busy}
          disabled={!changed || !name.trim()}
          testID="save-profile"
        />
      </ScrollView>
    </Screen>
  );
}

const makeStyles = ({ colors }: Theme) =>
  StyleSheet.create({
    body: { padding: spacing.lg, gap: spacing.lg },
    note: { ...type.caption, color: colors.textSecondary, lineHeight: 18 },
  });
