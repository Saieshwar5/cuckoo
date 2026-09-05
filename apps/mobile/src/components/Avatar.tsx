import React from 'react';
import { StyleSheet, Text, View } from 'react-native';

import type { AgentStatus } from '@/api/types';
import { sizes, useTheme, type Palette } from '@/theme';
import { avatarIndex, initials } from '@/util/avatar';

interface Props {
  name: string;
  size?: number;
  // Present for agents. The dot is the one thing WhatsApp does not have
  // and a person talking to an agent needs at a glance.
  status?: AgentStatus | null;
}

function dotColor(colors: Palette, status: AgentStatus): string {
  switch (status) {
    case 'connected':
      return colors.statusConnected;
    case 'idle':
      return colors.statusIdle;
    case 'unreachable':
      return colors.statusUnreachable;
  }
}

// Avatar is a grey disc with the name's initials. The shade comes from the
// name, so an agent looks the same everywhere it appears.
export function Avatar({ name, size = sizes.avatar, status }: Props) {
  const { colors } = useTheme();
  const dot = Math.round(size * 0.27);
  const shade = colors.avatars[avatarIndex(name, colors.avatars.length)] ?? colors.surfaceStrong;
  return (
    <View style={{ width: size, height: size }}>
      <View
        style={[styles.disc, { width: size, height: size, borderRadius: size / 2, backgroundColor: shade }]}
      >
        <Text style={[styles.initials, { fontSize: Math.round(size * 0.38), color: colors.onAvatar }]}>
          {initials(name)}
        </Text>
      </View>
      {status !== undefined ? (
        <View
          testID="status-dot"
          style={[
            styles.dot,
            { width: dot, height: dot, borderRadius: dot / 2, borderColor: colors.ground },
            status
              ? { backgroundColor: dotColor(colors, status) }
              : // No backend connected: a ring only.
                { backgroundColor: colors.ground, borderColor: colors.statusIdle },
          ]}
        />
      ) : null}
    </View>
  );
}

const styles = StyleSheet.create({
  disc: { alignItems: 'center', justifyContent: 'center' },
  initials: { fontWeight: '600' },
  dot: { position: 'absolute', right: -1, bottom: -1, borderWidth: 2.5 },
});
