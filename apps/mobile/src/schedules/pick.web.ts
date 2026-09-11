// In a browser there is no native dialog to borrow; a prompt does the job
// for the development build that runs there.

export function pickTime(current: string, onPick: (time: string) => void): void {
  const answer = window.prompt('Time (24-hour, HH:MM)', current)?.trim();
  if (answer && /^([01]\d|2[0-3]):[0-5]\d$/.test(answer)) onPick(answer);
}

export function pickDate(current: string, onPick: (date: string) => void): void {
  const answer = window.prompt('Date (YYYY-MM-DD)', current)?.trim();
  if (answer && /^\d{4}-\d{2}-\d{2}$/.test(answer)) onPick(answer);
}
