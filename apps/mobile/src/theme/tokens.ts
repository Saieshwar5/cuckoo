// The visual language, as tokens. White ground, ash surfaces, near-black
// text, one accent. No gradients, no shadows beyond a hairline. Dark mode is
// not in v1, but everything goes through these so it can be.
export const colors = {
  ground: '#FFFFFF',
  surface: '#F5F6F7',
  surfaceStrong: '#ECEEF0',
  hairline: '#DDE0E3',
  text: '#1A1D21',
  textSecondary: '#6B7280',
  accent: '#0F766E',
  accentTint: '#E6F2F0',
  onAccent: '#FFFFFF',
  danger: '#B42318',
  // The dot on every agent avatar.
  statusConnected: '#16A34A',
  statusIdle: '#9CA3AF',
  statusUnreachable: '#D97706',
} as const;

export const spacing = { xs: 4, sm: 8, md: 12, lg: 16, xl: 24, xxl: 32 } as const;

export const radius = { sm: 6, md: 10, lg: 16, pill: 999 } as const;

export const type = {
  title: { fontSize: 22, fontWeight: '600' as const },
  body: { fontSize: 16 },
  secondary: { fontSize: 14 },
  caption: { fontSize: 12 },
} as const;

// Minimum tap target, in points.
export const tapTarget = 44;
