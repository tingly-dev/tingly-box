import { useMediaQuery } from '@mui/material';
import { PHONE_QUERY } from '../theme/mobile';

/** Whether this is a phone-sized touch screen (see `PHONE_QUERY`). Read synchronously, so there is no desktop flash. */
export function useIsPhone(): boolean {
    return useMediaQuery(PHONE_QUERY, { noSsr: true });
}
