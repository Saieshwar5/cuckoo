import type { StorageUsage } from '@/api/types';
import { t } from '@/i18n';

// How big a file is, in the words a person uses. Two significant figures
// is what anyone reads off a file card: "1.4 MB", not "1,468,006 bytes".
export function formatBytes(bytes: number): string {
  if (!bytes || bytes < 0) return '';
  if (bytes < 1024) return `${bytes} B`;
  const units = ['KB', 'MB', 'GB'];
  let value = bytes / 1024;
  let unit = 0;
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024;
    unit++;
  }
  return `${value < 10 ? value.toFixed(1) : Math.round(value)} ${units[unit]}`;
}

// storageLine is the sentence in Settings about what the hub is holding
// for this person: how much of their room for files, and for how long it
// keeps what they say. A hub that never sweeps messages says so without
// a number of days; one with no room for files at all has no fraction to
// show, and the line is only about messages.
export function storageLine(usage: StorageUsage): string {
  const parts: string[] = [];
  const { used_bytes: used, budget_bytes: budget } = usage.media;
  if (budget > 0) {
    const of = { used: formatBytes(used), budget: formatBytes(budget) };
    parts.push(used > 0 ? t('settings.storage.hub', of) : t('settings.storage.hub.none', of));
  }
  const days = usage.messages.kept_days;
  parts.push(
    days <= 0
      ? t('settings.storage.kept.forever')
      : days === 1
        ? t('settings.storage.kept.one')
        : t('settings.storage.kept', { days }),
  );
  return parts.join(' ');
}
