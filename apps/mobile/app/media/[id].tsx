import { Ionicons } from '@expo/vector-icons';
import { useLocalSearchParams, useRouter } from 'expo-router';
import { useVideoPlayer, VideoView } from 'expo-video';
import React from 'react';
import { ActivityIndicator, Image, Pressable, StyleSheet, Text, View } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';

import { t } from '@/i18n';
import { openAttachment } from '@/media/save';
import { useMediaSource } from '@/media/source';
import { useSession } from '@/session/SessionProvider';
import { spacing, type } from '@/theme';
import { dark } from '@/theme/tokens';

// One picture, as large as the screen allows.
//
// Always on black, in either theme: a photograph is looked at against
// nothing, and the chrome steps back to the corners so it is not part of
// what you are looking at.
export default function MediaScreen() {
  const router = useRouter();
  const insets = useSafeAreaInsets();
  const { api, token } = useSession();
  const { id, name, kind } = useLocalSearchParams<{ id: string; name?: string; kind?: string }>();
  const { source, gone } = useMediaSource(id);
  const fileName = name ?? '';
  const isVideo = kind === 'video';
  // A video opened full-screen plays at once, the way tapping one anywhere
  // else does.
  const player = useVideoPlayer(isVideo ? (source ?? null) : null, (p) => {
    p.play();
  });

  return (
    <View style={styles.screen}>
      {gone ? (
        <Text style={styles.gone} testID="media-gone">
          {t('chat.media.gone')}
        </Text>
      ) : !source ? (
        <ActivityIndicator color={dark.text} />
      ) : isVideo ? (
        <VideoView
          player={player}
          style={styles.image}
          contentFit="contain"
          nativeControls
          testID="media-video"
        />
      ) : (
        <Image
          source={source}
          style={styles.image}
          resizeMode="contain"
          accessibilityLabel={fileName}
          accessibilityIgnoresInvertColors
          testID="media-image"
        />
      )}

      <View style={[styles.bar, { paddingTop: insets.top + spacing.sm }]}>
        <Pressable
          accessibilityRole="button"
          accessibilityLabel={t('chat.back')}
          onPress={() => router.back()}
          hitSlop={12}
          testID="media-close"
        >
          <Ionicons name="close" size={26} color={dark.text} />
        </Pressable>
        <Text style={styles.name} numberOfLines={1}>
          {fileName}
        </Text>
        {gone ? null : (
          <Pressable
            accessibilityRole="button"
            accessibilityLabel={t('chat.file.save')}
            onPress={() => void openAttachment(api, id, fileName || 'photo.jpg', token)}
            hitSlop={12}
            testID="media-save"
          >
            <Ionicons name="download-outline" size={24} color={dark.text} />
          </Pressable>
        )}
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  screen: { flex: 1, backgroundColor: '#000000', alignItems: 'center', justifyContent: 'center' },
  image: { width: '100%', height: '100%' },
  bar: {
    position: 'absolute',
    top: 0,
    left: 0,
    right: 0,
    flexDirection: 'row',
    alignItems: 'center',
    gap: spacing.md,
    paddingHorizontal: spacing.lg,
    paddingBottom: spacing.sm,
  },
  name: { ...type.secondary, color: dark.text, flex: 1 },
  gone: { ...type.secondary, color: dark.textSecondary, textAlign: 'center', padding: spacing.xl },
});
