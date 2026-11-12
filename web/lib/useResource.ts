'use client';

import { useCallback, useEffect, useState } from 'react';
import { ApiError, errorMessage } from './api';

/**
 * Loads data with loading and error state, and exposes reload().
 * A 401 means the session ended, so the user is sent back to /login; losing
 * the organization (removed by an administrator) leads to /onboarding.
 */
export function useResource<T>(load: () => Promise<T>) {
  const [data, setData] = useState<T | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  const reload = useCallback(async () => {
    try {
      setData(await load());
      setError(null);
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        window.location.assign('/login');
        return;
      }
      // Removed from the organization while the page was open.
      if (err instanceof ApiError && err.code === 'no_organization') {
        window.location.assign('/onboarding');
        return;
      }
      setError(errorMessage(err));
    } finally {
      setLoading(false);
    }
    // `load` is expected to be a stable module-level function.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    reload();
  }, [reload]);

  return { data, error, loading, reload };
}
