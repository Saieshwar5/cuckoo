import { Ionicons } from '@expo/vector-icons';
import { useRouter } from 'expo-router';
import React, { useState } from 'react';
import { ActivityIndicator, Image, Pressable, StyleSheet, Text, View } from 'react-native';

import type { Attachment } from '@/api/types';
import { t } from '@/i18n';
import { formatDuration } from '@/util/time';
import { formatBytes } from '@/media/format';
import { openAttachment } from '@/media/save';
import { useMediaSource } from '@/media/source';
import { useSession } from '@/session/SessionProvider';
import { radius, spacing, type, useTheme, type Palette } from '@/theme';

import { VoiceNote } from './VoiceNote';

// What a message carries, drawn inside its bubble.
//
// A picture is the message: it fills the bubble's width and the words sit
// under it, which is what every messenger does because it is what people
// look at. Anything else is a card — an icon, a name, a size — because a
// document has nothing to show until it is opened.
//
// A file the hub has swept keeps its place and its shape, and says so in
// the same grey as the time under it. It is not an error; it is what a
// retention window looks like from inside a chat.

// The width a picture is drawn at. Fixed rather than proportional so a
// column of photos has one edge, and small enough to leave the bubble's
// shape recognisable.
const PHOTO_WIDTH = 232;

interface Props {
  attachments: Attachment[];
  mine: boolean;
  // Our own message, still on its way to the hub.
  sending: boolean;
}

export function Attachments({ attachments, mine, sending }: Props) {
  const { colors } = useTheme();
  return (
    <View style={styles.list}>
      {attachments.map((a, i) => {
        switch (a.kind) {
          case 'image':
            return <Photo key={a.media_id || i} attachment={a} sending={sending} colors={colors} />;
          case 'audio':
            return (
              <VoiceNote key={a.media_id || i} attachment={a} mine={mine} sending={sending} colors={colors} />
            );
          case 'video':
            return <VideoCard key={a.media_id || i} attachment={a} sending={sending} colors={colors} />;
          default:
            return <FileCard key={a.media_id || i} attachment={a} mine={mine} colors={colors} />;
        }
      })}
    </View>
  );
}

function Photo({
  attachment,
  sending,
  colors,
}: {
  attachment: Attachment;
  sending: boolean;
  colors: Palette;
}) {
  const router = useRouter();
  // The small copy is what a bubble needs; the full one waits until the
  // picture is opened. On a slow connection that is the whole difference.
  const { source, gone } = useMediaSource(attachment.media_id, {
    thumb: attachment.has_thumbnail,
    localUri: attachment.local_uri,
  });
  const ratio = attachment.width && attachment.height ? attachment.width / attachment.height : 1;
  const height = Math.round(PHOTO_WIDTH / Math.min(Math.max(ratio, 0.6), 1.9));

  return (
    <Pressable
      accessibilityRole="imagebutton"
      accessibilityLabel={gone ? t('chat.media.gone') : attachment.file_name}
      disabled={sending || gone}
      onPress={() =>
        router.push({
          pathname: '/media/[id]',
          params: { id: attachment.media_id, name: attachment.file_name },
        })
      }
      testID={`photo-${attachment.media_id}`}
      style={[styles.photo, { width: PHOTO_WIDTH, height, backgroundColor: colors.surfaceStrong }]}
    >
      {source ? (
        <Image source={source} style={styles.image} resizeMode="cover" accessibilityIgnoresInvertColors />
      ) : (
        <View style={styles.centre}>
          {gone ? (
            <Text
              style={[styles.gone, { color: colors.textSecondary }]}
              testID={`gone-${attachment.media_id}`}
            >
              {t('chat.media.gone')}
            </Text>
          ) : (
            <ActivityIndicator color={colors.textSecondary} />
          )}
        </View>
      )}
      {sending ? (
        <View style={[styles.veil, { backgroundColor: colors.scrim }]}>
          <ActivityIndicator color={colors.onAccent} />
        </View>
      ) : null}
    </Pressable>
  );
}

// A video is a black card with a triangle on it and its length in the
// corner. There is no still frame from inside it: pulling one out means
// decoding video, which the hub will not do to a stranger's file.
function VideoCard({
  attachment,
  sending,
  colors,
}: {
  attachment: Attachment;
  sending: boolean;
  colors: Palette;
}) {
  const router = useRouter();
  return (
    <Pressable
      accessibilityRole="button"
      accessibilityLabel={attachment.file_name}
      disabled={sending}
      onPress={() =>
        router.push({
          pathname: '/media/[id]',
          params: { id: attachment.media_id, name: attachment.file_name, kind: 'video' },
        })
      }
      testID={`video-${attachment.media_id}`}
      style={styles.video}
    >
      <View style={[styles.playDisc, { backgroundColor: colors.surfaceStrong }]}>
        <Ionicons name="play" size={22} color={colors.text} style={styles.nudge} />
      </View>
      {attachment.duration_ms ? (
        <Text style={styles.length}>{formatDuration(attachment.duration_ms / 1000)}</Text>
      ) : null}
    </Pressable>
  );
}

function FileCard({ attachment, mine, colors }: { attachment: Attachment; mine: boolean; colors: Palette }) {
  const { api, token } = useSession();
  // A document is not fetched to draw its card, so whether the hub still
  // has it is only learned on a tap.
  const [gone, setGone] = useState(false);
  const ink = mine ? colors.bubbleTextMine : colors.bubbleTextTheirs;
  const meta = mine ? colors.bubbleMetaMine : colors.bubbleMetaTheirs;
  const size = formatBytes(attachment.byte_size);
  const open = async () => {
    const result = await openAttachment(api, attachment.media_id, attachment.file_name, token);
    if (result === 'gone') setGone(true);
  };

  return (
    <Pressable
      accessibilityRole="button"
      accessibilityState={{ disabled: gone }}
      disabled={gone}
      onPress={() => void open()}
      testID={`file-${attachment.media_id}`}
      style={[styles.card, { backgroundColor: mine ? colors.bubbleQuoteMine : colors.bubbleQuoteTheirs }]}
    >
      <View style={[styles.icon, { backgroundColor: colors.surfaceStrong }]}>
        <Ionicons name={iconFor(attachment.kind)} size={20} color={colors.textSecondary} />
      </View>
      <View style={styles.cardBody}>
        <Text style={[styles.name, { color: ink }]} numberOfLines={1}>
          {attachment.file_name}
        </Text>
        <Text
          style={[styles.size, { color: meta }]}
          numberOfLines={1}
          testID={gone ? `gone-${attachment.media_id}` : undefined}
        >
          {gone ? t('chat.media.gone') : size ? `${size} · ${t('chat.file.open')}` : t('chat.file.open')}
        </Text>
      </View>
    </Pressable>
  );
}

function iconFor(kind: Attachment['kind']): keyof typeof Ionicons.glyphMap {
  switch (kind) {
    case 'video':
      return 'videocam';
    case 'audio':
      return 'musical-notes';
    default:
      return 'document-text';
  }
}

const styles = StyleSheet.create({
  list: { gap: 3, marginBottom: spacing.xs },
  photo: { borderRadius: radius.sm, overflow: 'hidden' },
  video: {
    width: PHOTO_WIDTH,
    height: Math.round((PHOTO_WIDTH * 9) / 16),
    // Black in either theme: a video plate is what a film is watched
    // against, not part of the bubble it sits in.
    backgroundColor: '#0B0B0B',
    borderRadius: radius.sm,
    alignItems: 'center',
    justifyContent: 'center',
  },
  playDisc: {
    width: 48,
    height: 48,
    borderRadius: radius.pill,
    alignItems: 'center',
    justifyContent: 'center',
  },
  nudge: { marginLeft: 3 },
  length: {
    ...type.caption,
    color: '#F2F2F2',
    backgroundColor: 'rgba(0, 0, 0, 0.55)',
    position: 'absolute',
    right: spacing.sm,
    bottom: spacing.sm,
    borderRadius: radius.sm,
    paddingHorizontal: 5,
    paddingVertical: 1,
    overflow: 'hidden',
  },
  image: { width: '100%', height: '100%' },
  centre: { flex: 1, alignItems: 'center', justifyContent: 'center' },
  gone: { ...type.caption, textAlign: 'center', paddingHorizontal: spacing.md },
  veil: {
    position: 'absolute',
    top: 0,
    left: 0,
    right: 0,
    bottom: 0,
    alignItems: 'center',
    justifyContent: 'center',
  },
  card: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: spacing.sm,
    borderRadius: radius.sm,
    padding: spacing.sm,
    minWidth: 200,
  },
  icon: { width: 38, height: 38, borderRadius: radius.sm, alignItems: 'center', justifyContent: 'center' },
  cardBody: { flex: 1, gap: 2 },
  name: { ...type.secondary, fontWeight: '600' },
  size: type.caption,
});
