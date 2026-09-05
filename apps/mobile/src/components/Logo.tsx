import React from 'react';
import { Image } from 'react-native';

import { useTheme } from '@/theme';

// The mark: a bird whose body is a speech bubble. Ink on paper in the
// light theme, paper on ink in the dark one, so it always stands on the
// ground it is drawn on.
const onLight = require('../../assets/logo-light.png');
const onDark = require('../../assets/logo-dark.png');

export function Logo({ size = 28 }: { size?: number }) {
  const { scheme } = useTheme();
  return (
    <Image
      source={scheme === 'dark' ? onDark : onLight}
      style={{ width: size, height: size, borderRadius: size * 0.22 }}
      accessible={false}
      testID="logo"
    />
  );
}
