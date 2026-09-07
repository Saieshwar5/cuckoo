import type { Device } from '@/api/types';
import { deviceLine, deviceName, orderDevices } from '@/devices/format';

const now = new Date('2026-09-07T15:30:00');

const device = (over: Partial<Device> = {}): Device => ({
  id: 'ses_1',
  name: 'laptop',
  last_seen_at: '2026-09-07T15:29:30',
  created_at: '2026-09-01T09:00:00',
  expires_at: '2026-12-01T09:00:00',
  current: false,
  ...over,
});

describe('device name', () => {
  it('falls back to a name for a device that never gave one', () => {
    expect(deviceName(device())).toBe('laptop');
    expect(deviceName(device({ name: '   ' }))).toBe('Unnamed device');
  });
});

describe('device line', () => {
  it('calls a device in use active, and dates the rest', () => {
    expect(deviceLine(device(), now)).toBe('Active now');
    expect(deviceLine(device({ last_seen_at: '2026-09-07T15:20:00' }), now)).toBe('Last seen 10 minutes ago');
    expect(deviceLine(device({ last_seen_at: '2026-09-07T12:30:00' }), now)).toBe('Last seen 3 hours ago');
    expect(deviceLine(device({ last_seen_at: '2026-09-06T23:00:00' }), now)).toBe('Last seen yesterday');
    expect(deviceLine(device({ last_seen_at: '2026-09-04T10:00:00' }), now)).toBe('Last seen 3 days ago');
    expect(deviceLine(device({ last_seen_at: '2026-08-20T10:00:00' }), now)).toBe('Last seen on 20 August');
  });

  it('says when a device signed in when it has not been heard from', () => {
    expect(deviceLine(device({ last_seen_at: null }), now)).toBe('Signed in 6 days ago');
    expect(deviceLine(device({ last_seen_at: null, created_at: '2026-09-07T15:29:50' }), now)).toBe(
      'Signed in just now',
    );
  });
});

describe('device order', () => {
  it("lifts this device above the rest and leaves the hub's order alone", () => {
    const list = [device({ id: 'a' }), device({ id: 'b' }), device({ id: 'c', current: true })];
    expect(orderDevices(list).map((d) => d.id)).toEqual(['c', 'a', 'b']);
    // The list it was given is not the one it changed.
    expect(list.map((d) => d.id)).toEqual(['a', 'b', 'c']);
  });
});
