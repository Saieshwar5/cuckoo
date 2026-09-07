import type { Device } from '@/api/types';
import { t } from '@/i18n';
import { timeSince, type Since } from '@/util/time';

// The hub stamps last_seen_at on every request a device carries, so a
// device somebody is holding is always within a minute or two. Inside that
// window it is called active rather than given a time.
const ACTIVE_MS = 2 * 60 * 1000;

// deviceName is what to call a device on screen. The name is whatever it
// gave when it signed in, and older devices gave nothing.
export function deviceName(device: Device): string {
  return device.name.trim() || t('devices.unnamed');
}

// deviceLine is the muted second line under a device: when it was last
// heard from, or — for one that has not been heard from at all — when it
// signed in.
export function deviceLine(device: Device, now: Date = new Date()): string {
  if (device.last_seen_at) {
    if (now.getTime() - Date.parse(device.last_seen_at) < ACTIVE_MS) return t('devices.active');
    const when = phrase(timeSince(device.last_seen_at, now));
    if (when) return t('devices.lastseen', { when });
  }
  const when = phrase(timeSince(device.created_at, now));
  return when ? t('devices.signedin', { when }) : '';
}

// orderDevices puts this device at the top and leaves the rest in the
// order the hub gave them, newest first. Sorting is stable, so the two
// groups keep their own order.
export function orderDevices(devices: Device[]): Device[] {
  return [...devices].sort((a, b) => Number(b.current) - Number(a.current));
}

function phrase(since: Since | null): string | null {
  if (!since) return null;
  switch (since.unit) {
    case 'now':
      return t('devices.when.now');
    case 'minutes':
      return since.count === 1 ? t('devices.when.minute') : t('devices.when.minutes', { count: since.count });
    case 'hours':
      return since.count === 1 ? t('devices.when.hour') : t('devices.when.hours', { count: since.count });
    case 'yesterday':
      return t('devices.when.yesterday');
    case 'days':
      return t('devices.when.days', { count: since.count });
    case 'date':
      return t('devices.when.date', { date: since.date });
  }
}
