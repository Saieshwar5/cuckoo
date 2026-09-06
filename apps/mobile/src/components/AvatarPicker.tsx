import { Ionicons } from '@expo/vector-icons';
import React, { useState } from 'react';
import { ActivityIndicator, Pressable, StyleSheet, Text, View } from 'react-native';

import { t } from '@/i18n';
import { pickOnePhoto, type PickedFile } from '@/media/pick';
import type { MediaSource } from '@/media/source';
import { radius, sizes, spacing, type, useTheme } from '@/theme';

import { Avatar } from './Avatar';

interface Props {
  name: string;
  // The picture already published, when there is one.
  source?: MediaSource | null;
  onPicked: (file: PickedFile) => void;
  size?: number;
  testID?: string;
}

// AvatarPicker is the disc with a camera on it: tap to choose a picture.
//
// What it hands back is the picked file, not an uploaded one. Uploading
// belongs with saving, so a person who changes their mind after choosing a
// photo and leaves the screen has not left a stray file on the hub.
export function AvatarPicker({ name, source, onPicked, size = sizes.avatarLarge, testID }: Props) {
  const { colors } = useTheme();
  const [picked, setPicked] = useState<PickedFile | null>(null);
  const [busy, setBusy] = useState(false);
  const shown: MediaSource | null = picked ? { uri: picked.uri } : (source ?? null);

  const choose = async () => {
    setBusy(true);
    try {
      const file = await pickOnePhoto();
      if (file) {
        setPicked(file);
        onPicked(file);
      }
    } finally {
      setBusy(false);
    }
  };

  return (
    <View style={styles.wrap}>
      <Pressable
        accessibilityRole="button"
        accessibilityLabel={t('profile.photo.change')}
        onPress={() => void choose()}
        testID={testID ?? 'pick-avatar'}
      >
        <Avatar name={name || '?'} size={size} source={shown} />
        <View
          style={[styles.badge, { backgroundColor: colors.accent, borderColor: colors.ground }]}
          pointerEvents="none"
        >
          {busy ? (
            <ActivityIndicator size="small" color={colors.onAccent} />
          ) : (
            <Ionicons name="camera" size={16} color={colors.onAccent} />
          )}
        </View>
      </Pressable>
      <Text style={[styles.hint, { color: colors.textSecondary }]}>
        {shown ? t('profile.photo.change') : t('profile.photo.add')}
      </Text>
    </View>
  );
}

const styles = StyleSheet.create({
  wrap: { alignItems: 'center', gap: spacing.sm },
  badge: {
    position: 'absolute',
    right: -2,
    bottom: -2,
    width: 32,
    height: 32,
    borderRadius: radius.pill,
    borderWidth: 2,
    alignItems: 'center',
    justifyContent: 'center',
  },
  hint: type.caption,
});
