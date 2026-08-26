import { Box, Chip, Tooltip } from '@mui/material';
import React from 'react';
import { useTranslation } from 'react-i18next';
import { SUPPORTED_LANGUAGES, resolveLanguage, setAppLanguage } from '@/i18n';

/**
 * Compact language switcher for guide dialogs (EntryGuideDialog, TierGuideDialog).
 *
 * These dialogs are modal (fullScreen on mobile) and sit on top of the
 * ActivityBar's language button, so users can get stuck mid-guide with the
 * wrong language and no way to switch without closing the dialog. This gives
 * the guide its own always-visible toggle, reusing the same setAppLanguage
 * helper (bundle load + persistence) as ActivityBar.
 */
export const GuideLanguageToggle: React.FC = () => {
    const { t, i18n } = useTranslation();
    const current = resolveLanguage(i18n.language);

    return (
        <Tooltip title={t('system.language.title')}>
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
                {SUPPORTED_LANGUAGES.map(({ code, labelKey }) => {
                    const selected = current === code;
                    return (
                        <Chip
                            key={code}
                            label={t(labelKey)}
                            onClick={() => void setAppLanguage(code)}
                            size="small"
                            sx={{
                                bgcolor: selected ? 'primary.main' : 'action.hover',
                                color: selected ? 'primary.contrastText' : 'text.primary',
                                fontWeight: selected ? 600 : 400,
                                border: selected ? 'none' : '1px solid',
                                borderColor: 'divider',
                                cursor: 'pointer',
                                '&:hover': {
                                    bgcolor: selected ? 'primary.dark' : 'action.selected',
                                },
                            }}
                        />
                    );
                })}
            </Box>
        </Tooltip>
    );
};

export default GuideLanguageToggle;
