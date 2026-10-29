import { Box, Collapse, Typography } from '@mui/material';
import React from 'react';
import { useTranslation } from 'react-i18next';
import { ExpandMore } from '@/components/icons';

interface AdvancedSectionProps {
    /** Number of settings currently differing from their defaults. */
    customizedCount?: number;
    children: React.ReactNode;
}

// Collapsed disclosure for the settings most users never touch. Opens itself
// once when something non-default is already saved, so a customised value is
// never hidden behind the fold.
export const AdvancedSection: React.FC<AdvancedSectionProps> = ({ customizedCount = 0, children }) => {
    const { t } = useTranslation();
    const [open, setOpen] = React.useState(customizedCount > 0);
    const autoOpened = React.useRef(customizedCount > 0);

    React.useEffect(() => {
        if (customizedCount > 0 && !autoOpened.current) {
            autoOpened.current = true;
            setOpen(true);
        }
    }, [customizedCount]);

    return (
        <Box sx={{ mt: 2 }}>
            <Box
                component="button"
                type="button"
                aria-expanded={open}
                onClick={() => setOpen(v => !v)}
                sx={{
                    display: 'flex', alignItems: 'center', gap: 0.5, width: '100%',
                    p: 0, py: 0.75, border: 0, background: 'none', cursor: 'pointer',
                    color: 'text.secondary', font: 'inherit', textAlign: 'left',
                }}
            >
                <ExpandMore fontSize="small" sx={{ transform: open ? 'none' : 'rotate(-90deg)', transition: 'transform .2s' }} />
                <Typography variant="subtitle2" sx={{ color: 'inherit' }}>{t('common.advanced')}</Typography>
                {customizedCount > 0 && (
                    <Typography variant="caption" sx={{ color: 'primary.main', ml: 0.5 }}>
                        {t('common.advancedCustomized', { count: customizedCount })}
                    </Typography>
                )}
            </Box>
            <Collapse in={open} timeout={200} unmountOnExit={false}>{children}</Collapse>
        </Box>
    );
};
