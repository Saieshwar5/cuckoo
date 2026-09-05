import { t } from '@/i18n';
import { avatarColors } from '@/theme/tokens';
import { colorFor, initials } from '@/util/avatar';
import { formatCountdown, formatListTime } from '@/util/time';

describe('strings', () => {
  it('fills placeholders and shows a missing key rather than nothing', () => {
    expect(t('verify.subtitle', { email: 'a@b.com' })).toContain('a@b.com');
    expect(t('no.such.key')).toBe('no.such.key');
  });
});

describe('list time', () => {
  const now = new Date('2026-09-05T15:30:00');
  it('shows the time today, yesterday as a word, and a date before that', () => {
    expect(formatListTime('2026-09-05T09:05:00', now)).toMatch(/09:05/);
    expect(formatListTime('2026-09-04T23:59:00', now)).toBe('Yesterday');
    expect(formatListTime('2026-08-30T10:00:00', now)).toMatch(/30\/08\/26/);
    expect(formatListTime('garbage', now)).toBe('');
  });
});

describe('countdown', () => {
  it('reads as minutes and seconds and never goes below zero', () => {
    expect(formatCountdown(30)).toBe('0:30');
    expect(formatCountdown(65)).toBe('1:05');
    expect(formatCountdown(0)).toBe('0:00');
    expect(formatCountdown(-3)).toBe('0:00');
  });
});

describe('avatars', () => {
  it('takes the first letters of the first and last words', () => {
    expect(initials('SBI Support')).toBe('SS');
    expect(initials('priya')).toBe('P');
    expect(initials('  ')).toBe('?');
  });

  it('gives a name the same colour every time, from the palette', () => {
    expect(colorFor('Echo')).toBe(colorFor('echo '));
    expect(avatarColors).toContain(colorFor('Echo'));
    const seen = new Set(
      ['Echo', 'SBI Support', 'Priya', 'Helper', 'Weather', 'IRCTC', 'Ravi'].map(colorFor),
    );
    expect(seen.size).toBeGreaterThan(3);
  });
});
