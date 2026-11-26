import { Alert, Box, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { ShieldCheck, ShieldOff } from '../icons';

/**
 * The two lines a chat always shows: what it can do (the boundary must be
 * visible, not only enforced), and, when there is no OS sandbox here, that
 * the folder is only read-only. Neither is a one-time toast.
 */
export function ChatNotices({ sandboxed, compact = false }: { sandboxed: boolean; compact?: boolean }) {
    const { t } = useTranslation('projects');
    if (compact) {
        return (
            <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 1, color: 'text.secondary', fontSize: '0.8rem' }}>
                <Box component="span" sx={{ display: 'inline-flex', alignItems: 'center', gap: 0.5 }}>
                    <ShieldCheck sx={{ fontSize: 15 }} />{t('readonlyChats.canRead')}
                </Box>
                {!sandboxed && (
                    <Box component="span" sx={{ display: 'inline-flex', alignItems: 'center', gap: 0.5 }}>
                        <ShieldOff sx={{ fontSize: 15 }} />{t('readonlyChats.unsandboxed')}
                    </Box>
                )}
            </Box>
        );
    }
    return (
        <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1 }}>
            <Alert severity="info" icon={<ShieldCheck fontSize="small" />} sx={{ py: 0.25 }}>
                <Typography variant="body2">{t('readonlyChats.capability')}</Typography>
            </Alert>
            {!sandboxed && (
                <Alert severity="warning" icon={<ShieldOff fontSize="small" />} sx={{ py: 0.25 }}>
                    <Typography variant="body2">{t('readonlyChats.unsandboxed')}</Typography>
                </Alert>
            )}
        </Box>
    );
}
