import React, { useEffect, useState } from 'react';
import { Animated, StyleSheet, View } from 'react-native';

import { radius, spacing, useTheme } from '@/theme';

// TypingBubble is three dots taking turns, where the agent's next bubble
// will be.
export function TypingBubble() {
  const { colors } = useTheme();
  const [dots] = useState(() => [new Animated.Value(0.3), new Animated.Value(0.3), new Animated.Value(0.3)]);

  useEffect(() => {
    const pulse = (v: Animated.Value, delay: number) =>
      Animated.sequence([
        Animated.delay(delay),
        Animated.timing(v, { toValue: 1, duration: 300, useNativeDriver: false }),
        Animated.timing(v, { toValue: 0.3, duration: 300, useNativeDriver: false }),
      ]);
    const loop = Animated.loop(
      Animated.parallel(dots.map((v, i) => pulse(v, i * 160)).concat(Animated.delay(200))),
    );
    loop.start();
    return () => loop.stop();
  }, [dots]);

  return (
    <View style={styles.row} testID="typing">
      <View style={[styles.bubble, { backgroundColor: colors.bubbleTheirs }]}>
        {dots.map((opacity, i) => (
          <Animated.View key={i} style={[styles.dot, { backgroundColor: colors.textSecondary, opacity }]} />
        ))}
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  row: { alignItems: 'flex-start', paddingHorizontal: spacing.sm, marginVertical: 1.5 },
  bubble: {
    flexDirection: 'row',
    gap: 5,
    paddingHorizontal: spacing.md,
    paddingVertical: spacing.md,
    borderRadius: radius.md,
    borderTopLeftRadius: 4,
  },
  dot: { width: 8, height: 8, borderRadius: 4 },
});
