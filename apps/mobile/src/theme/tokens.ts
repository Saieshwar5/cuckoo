import { Platform, type TextStyle } from 'react-native';

// The visual language, as tokens. Two palettes with the same shape, dark
// first because that is how the phones this is built for are set. The look
// is the one people already know from WhatsApp on Android: near-black
// ground, a green accent, ash surfaces, one hairline, no shadows.

export interface Palette {
  // Backgrounds, from the page up.
  ground: string;
  surface: string;
  surfaceStrong: string;
  hairline: string;
  // Text.
  text: string;
  textSecondary: string;
  // The one accent, and what sits on it.
  accent: string;
  accentStrong: string;
  accentTint: string;
  onAccent: string;
  danger: string;
  // The conversation: its wallpaper and the two kinds of bubble.
  wallpaper: string;
  bubbleMine: string;
  bubbleTheirs: string;
  bubbleText: string;
  bubbleMetaMine: string;
  bubbleMetaTheirs: string;
  bubbleQuote: string;
  quoteBar: string;
  tickRead: string;
  // The dot on every agent avatar.
  statusConnected: string;
  statusIdle: string;
  statusUnreachable: string;
}

export const dark: Palette = {
  ground: '#0B141A',
  surface: '#1F2C34',
  surfaceStrong: '#2A3942',
  hairline: '#222E35',
  text: '#E9EDEF',
  textSecondary: '#8696A0',
  accent: '#00A884',
  accentStrong: '#25D366',
  accentTint: '#103529',
  onAccent: '#0B141A',
  danger: '#F15C6D',
  wallpaper: '#0B141A',
  bubbleMine: '#005C4B',
  bubbleTheirs: '#202C33',
  bubbleText: '#E9EDEF',
  bubbleMetaMine: 'rgba(233, 237, 239, 0.65)',
  bubbleMetaTheirs: '#8696A0',
  bubbleQuote: 'rgba(0, 0, 0, 0.25)',
  quoteBar: '#25D366',
  tickRead: '#53BDEB',
  statusConnected: '#25D366',
  statusIdle: '#8696A0',
  statusUnreachable: '#F5A524',
};

export const light: Palette = {
  ground: '#FFFFFF',
  surface: '#F0F2F5',
  surfaceStrong: '#E4E8EB',
  hairline: '#E9EDEF',
  text: '#111B21',
  textSecondary: '#667781',
  accent: '#008069',
  accentStrong: '#25D366',
  accentTint: '#D9FDD3',
  onAccent: '#FFFFFF',
  danger: '#D23B47',
  wallpaper: '#EFEAE2',
  bubbleMine: '#D9FDD3',
  bubbleTheirs: '#FFFFFF',
  bubbleText: '#111B21',
  bubbleMetaMine: '#667781',
  bubbleMetaTheirs: '#667781',
  bubbleQuote: 'rgba(0, 0, 0, 0.06)',
  quoteBar: '#008069',
  tickRead: '#53BDEB',
  statusConnected: '#25D366',
  statusIdle: '#8696A0',
  statusUnreachable: '#D97706',
};

// Avatar colours, Telegram-style: a name always gets the same one.
export const avatarColors = [
  '#E17076',
  '#FAA774',
  '#A695E7',
  '#7BC862',
  '#6EC9CB',
  '#65AADD',
  '#EE7AAE',
] as const;

export const spacing = { xs: 4, sm: 8, md: 12, lg: 16, xl: 24, xxl: 32 } as const;

export const radius = { sm: 8, md: 12, lg: 18, xl: 24, pill: 999 } as const;

// The platform's own face, at these sizes. Roboto on Android, San Francisco
// on iOS, the system font on the web: what every other chat app on the
// phone uses, so this one feels native from the first screen.
export const type = {
  display: { fontSize: 28, fontWeight: '700' as const, letterSpacing: -0.3 },
  title: { fontSize: 22, fontWeight: '600' as const },
  headline: { fontSize: 17, fontWeight: '600' as const },
  body: { fontSize: 16, fontWeight: '400' as const },
  secondary: { fontSize: 14, fontWeight: '400' as const },
  label: { fontSize: 13, fontWeight: '500' as const },
  caption: { fontSize: 12, fontWeight: '400' as const },
} as const;

// Minimum tap target, in points.
export const tapTarget = 44;

// Sizes that recur across screens.
export const sizes = { avatar: 52, avatarSmall: 40, avatarLarge: 96, fab: 56, control: 48 } as const;

// Browsers draw their own focus ring around an input; the field's border
// already says which one has focus, so the ring is turned off on the web.
export const inputReset: TextStyle =
  Platform.OS === 'web' ? ({ outlineStyle: 'none' } as unknown as TextStyle) : {};
