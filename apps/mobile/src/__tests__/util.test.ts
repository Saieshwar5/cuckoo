import { t } from '@/i18n';
import { avatarIndex, initials } from '@/util/avatar';
import { formatClock, formatCountdown, formatDay, formatListTime, sameDay, timeSince } from '@/util/time';

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

describe('conversation time', () => {
  const now = new Date('2026-09-05T15:30:00');
  it('labels days and shows the clock inside a bubble', () => {
    expect(formatDay('2026-09-05T09:05:00', now)).toBe('Today');
    expect(formatDay('2026-09-04T23:59:00', now)).toBe('Yesterday');
    expect(formatDay('2026-08-15T10:00:00', now)).toBe('15 August 2026');
    expect(formatClock('2026-09-05T09:05:00')).toMatch(/09:05/);
    expect(sameDay('2026-09-05T00:10:00', '2026-09-05T23:50:00')).toBe(true);
    expect(sameDay('2026-09-05T23:50:00', '2026-09-06T00:10:00')).toBe(false);
  });
});

describe('time since', () => {
  const now = new Date('2026-09-07T15:30:00');
  it('picks the largest unit that still says something', () => {
    expect(timeSince('2026-09-07T15:29:40', now)).toEqual({ unit: 'now' });
    expect(timeSince('2026-09-07T15:20:00', now)).toEqual({ unit: 'minutes', count: 10 });
    expect(timeSince('2026-09-07T12:30:00', now)).toEqual({ unit: 'hours', count: 3 });
    expect(timeSince('2026-09-06T23:00:00', now)).toEqual({ unit: 'yesterday' });
    expect(timeSince('2026-09-04T10:00:00', now)).toEqual({ unit: 'days', count: 3 });
    expect(timeSince('2026-08-20T10:00:00', now)).toEqual({ unit: 'date', date: '20 August' });
    expect(timeSince('2025-08-20T10:00:00', now)).toEqual({ unit: 'date', date: '20 August 2025' });
  });

  it('reads a clock that runs ahead as now, and an unreadable time as nothing', () => {
    expect(timeSince('2026-09-07T15:40:00', now)).toEqual({ unit: 'now' });
    expect(timeSince('garbage', now)).toBeNull();
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

  it('gives a name the same shade every time, spread across the shades', () => {
    expect(avatarIndex('Echo', 5)).toBe(avatarIndex('echo ', 5));
    expect(avatarIndex('Echo', 5)).toBeLessThan(5);
    const seen = new Set(
      ['Echo', 'SBI Support', 'Priya', 'Helper', 'Weather', 'IRCTC', 'Ravi'].map((n) => avatarIndex(n, 5)),
    );
    expect(seen.size).toBeGreaterThan(2);
    expect(avatarIndex('Echo', 0)).toBe(0);
  });
});
