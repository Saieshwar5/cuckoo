import type { Schedule } from '@/api/types';
import { cadenceLabel, clock, statusLine, whenLabel } from '@/schedules/format';
import { applyScheduleFrame } from '@/schedules/store';

function schedule(over: Partial<Schedule> = {}): Schedule {
  return {
    id: 'sch_1',
    conversation_id: 'cnv_1',
    agent_id: 'agt_1',
    title: 'Morning weather',
    instruction: 'Tell me the weather in Hyderabad',
    cadence: { repeat: 'daily', time: '07:00', timezone: 'Asia/Kolkata' },
    status: 'active',
    created_by: 'user',
    next_run_at: null,
    last_run_at: null,
    missed_at: null,
    created_at: '2026-09-11T00:00:00Z',
    updated_at: '2026-09-11T00:00:00Z',
    ...over,
  };
}

describe('schedule rows', () => {
  const now = new Date(2026, 8, 11, 8, 0);

  it('says when, in words', () => {
    expect(clock('07:00')).toBe('7:00 AM');
    expect(clock('18:30')).toBe('6:30 PM');
    expect(cadenceLabel({ repeat: 'daily', time: '07:00', timezone: 'Asia/Kolkata' })).toBe(
      'Every day · 7:00 AM',
    );
    expect(
      cadenceLabel({ repeat: 'weekly', time: '18:00', days: ['mon', 'fri'], timezone: 'Asia/Kolkata' }),
    ).toBe('Mon, Fri · 6:00 PM');
    expect(
      cadenceLabel({ repeat: 'once', time: '09:00', date: '2026-09-12', timezone: 'Asia/Kolkata' }),
    ).toBe('Once · 12 Sep, 9:00 AM');
    expect(whenLabel(new Date(2026, 8, 12, 7, 0).toISOString(), now)).toBe('tomorrow 7:00 AM');
  });

  it('puts a miss first, and in red', () => {
    const next = new Date(2026, 8, 12, 7, 0).toISOString();
    expect(statusLine(schedule({ next_run_at: next }), 'Weather', now)).toEqual({
      text: 'Next: tomorrow 7:00 AM',
      tone: 'normal',
    });
    const missed = new Date(2026, 8, 11, 7, 0).toISOString();
    expect(statusLine(schedule({ next_run_at: next, missed_at: missed }), 'Weather', now)).toEqual({
      text: "Didn't run today 7:00 AM",
      tone: 'warn',
    });
    expect(statusLine(schedule({ status: 'pending' }), 'Weather', now).text).toBe(
      'Waiting for Weather to confirm…',
    );
    expect(statusLine(schedule({ status: 'paused', missed_at: missed }), 'Weather', now).text).toBe('Paused');
  });

  it('follows live changes to this chat only', () => {
    let list: Schedule[] = [];
    list = applyScheduleFrame(
      list,
      { type: 'schedule.changed', data: { conversation_id: 'cnv_1', schedule: schedule(), deleted: false } },
      'cnv_1',
    );
    expect(list).toHaveLength(1);
    list = applyScheduleFrame(
      list,
      {
        type: 'schedule.changed',
        data: { conversation_id: 'cnv_1', schedule: schedule({ status: 'paused' }), deleted: false },
      },
      'cnv_1',
    );
    expect(list[0]?.status).toBe('paused');
    const other = applyScheduleFrame(
      list,
      {
        type: 'schedule.changed',
        data: { conversation_id: 'cnv_2', schedule: schedule({ id: 'sch_9' }), deleted: false },
      },
      'cnv_1',
    );
    expect(other).toBe(list);
    list = applyScheduleFrame(
      list,
      { type: 'schedule.changed', data: { conversation_id: 'cnv_1', schedule: schedule(), deleted: true } },
      'cnv_1',
    );
    expect(list).toHaveLength(0);
  });
});
