import { ApiError } from '../api/client';
import { t } from '../i18n';

// describeAgentError turns a failure from the management API into the
// sentence the form shows, and says which field it belongs under.
export function describeAgentError(err: unknown): {
  message: string;
  field: 'handle' | 'name' | 'description' | null;
} {
  if (err instanceof ApiError) {
    const known = t(`agent.error.${err.code}`);
    const message = known !== `agent.error.${err.code}` ? known : err.message;
    const field =
      err.code === 'handle_taken' || err.field === 'handle'
        ? 'handle'
        : err.field === 'display_name'
          ? 'name'
          : err.field === 'description'
            ? 'description'
            : null;
    return { message, field };
  }
  if (err instanceof Error && err.name === 'NetworkError')
    return { message: t('common.error.network'), field: null };
  return { message: t('common.error.unknown'), field: null };
}
