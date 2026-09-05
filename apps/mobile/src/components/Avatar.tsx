import React from 'react';
import { StyleSheet, Text, View } from 'react-native';

import type { AgentStatus } from '@/api/types';
import { colors, type } from '@/theme/tokens';
import { initials } from '@/util/time';

interface Props {
  name: string;
  size?: number;
  // Present for agents. The dot is the one thing WhatsApp does not have
  // and a person talking to an agent needs at a glance.
  status?: AgentStatus | null;
}

const dotColor: Record<AgentStatus, string> = {
  connected: colors.statusConnected,
  idle: colors.statusIdle,
  unreachable: colors.statusUnreachable,
};

export function Avatar({ name, size = 48, status }: Props) {
  const dot = size * 0.28;
  return (
    <View style={{ width: size, height: size }}>
      <View style={[styles.circle, { width: size, height: size, borderRadius: size / 2 }]}>
        <Text style={[styles.initials, { fontSize: size * 0.4 }]}>{initials(name)}</Text>
      </View>
      {status !== undefined ? (
        <View
          testID="status-dot"
          style={[
            styles.dot,
            { width: dot, height: dot, borderRadius: dot / 2 },
            status ? { backgroundColor: dotColor[status] } : styles.dotNone,
          ]}
        />
      ) : null}
    </View>
  );
}

const styles = StyleSheet.create({
  circle: { backgroundColor: colors.surfaceStrong, alignItems: 'center', justifyContent: 'center' },
  initials: { ...type.body, color: colors.textSecondary, fontWeight: '600' },
  dot: {
    position: 'absolute',
    right: -1,
    bottom: -1,
    borderWidth: 2,
    borderColor: colors.ground,
  },
  // No backend connected: a ring only.
  dotNone: { backgroundColor: colors.ground, borderColor: colors.statusIdle },
});
