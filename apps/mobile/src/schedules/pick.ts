import { DateTimePickerAndroid } from '@react-native-community/datetimepicker';

// The phone's own clock and calendar dialogs, for a schedule's time and
// date: the pickers every Android thumb already knows, rather than a
// wheel of ours.

function pad(n: number): string {
  return String(n).padStart(2, '0');
}

// pickTime opens the clock at "07:00" and hands back what was chosen.
export function pickTime(current: string, onPick: (time: string) => void): void {
  const [h = 7, m = 0] = current.split(':').map((part) => Number(part));
  const value = new Date();
  value.setHours(h, m, 0, 0);
  DateTimePickerAndroid.open({
    value,
    mode: 'time',
    is24Hour: false,
    onChange: (event, date) => {
      if (event.type === 'set' && date) onPick(`${pad(date.getHours())}:${pad(date.getMinutes())}`);
    },
  });
}

// pickDate opens the calendar at "2026-09-12", from today on.
export function pickDate(current: string, onPick: (date: string) => void): void {
  const [y, mo, d] = current.split('-').map((part) => Number(part));
  const value = y && mo && d ? new Date(y, mo - 1, d) : new Date();
  DateTimePickerAndroid.open({
    value,
    mode: 'date',
    minimumDate: new Date(),
    onChange: (event, date) => {
      if (event.type === 'set' && date) {
        onPick(`${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`);
      }
    },
  });
}
