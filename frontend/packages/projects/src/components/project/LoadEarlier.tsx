import { useState, type RefObject } from 'react';
import { Box, Button } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { useNotify } from '../../hooks/useNotify';

/**
 * At the top of a transcript that opened on its latest entries: loads the
 * page before them and keeps what the user was reading where it was.
 */
export function LoadEarlier({ scrollRef, onLoad }: { scrollRef: RefObject<HTMLDivElement | null>; onLoad: () => Promise<void> }) {
    const { t } = useTranslation('projects');
    const notify = useNotify();
    const [busy, setBusy] = useState(false);
    const load = async () => {
        const el = scrollRef.current;
        const fromBottom = el ? el.scrollHeight - el.scrollTop : 0;
        setBusy(true);
        try {
            await onLoad();
            requestAnimationFrame(() => {
                if (el) el.scrollTop = el.scrollHeight - fromBottom;
            });
        } catch (e) {
            notify((e as Error).message, 'error');
        } finally {
            setBusy(false);
        }
    };
    return (
        <Box sx={{ display: 'flex', justifyContent: 'center', pb: 1 }}>
            <Button size="small" onClick={() => void load()} disabled={busy}>{t('thread.loadEarlier')}</Button>
        </Box>
    );
}
