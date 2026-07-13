import { Box, Chip, Tooltip } from '@mui/material';
import React from 'react';
import { useTranslation } from 'react-i18next';

/**
 * Compact language switcher for guide dialogs (EntryGuideDialog, TierGuideDialog).
 *
 * These dialogs are modal (fullScreen on mobile) and sit on top of the
 * ActivityBar's language button, so users can get stuck mid-guide with the
 * wrong language and no way to switch without closing the dialog. This gives
 * the guide its own always-visible toggle, reusing the same
 * i18n.changeLanguage + localStorage persistence as ActivityBar.
 */
export const GuideLanguageToggle: React.FC = () => {
    const { t, i18n } = useTranslation();

    const changeLanguage = (lng: string) => {
        if (i18n.language === lng) return;
        i18n.changeLanguage(lng);
        localStorage.setItem('i18nextLng', lng);
    };

    return (
        <Tooltip title={t('system.language.title')}>
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
                <Chip
                    label={t('system.language.en')}
                    onClick={() => changeLanguage('en')}
                    size="small"
                    sx={{
                        bgcolor: i18n.language === 'en' ? 'primary.main' : 'action.hover',
                        color: i18n.language === 'en' ? 'primary.contrastText' : 'text.primary',
                        fontWeight: i18n.language === 'en' ? 600 : 400,
                        border: i18n.language === 'en' ? 'none' : '1px solid',
                        borderColor: 'divider',
                        cursor: 'pointer',
                        '&:hover': {
                            bgcolor: i18n.language === 'en' ? 'primary.dark' : 'action.selected',
                        },
                    }}
                />
                <Chip
                    label={t('system.language.zh')}
                    onClick={() => changeLanguage('zh')}
                    size="small"
                    sx={{
                        bgcolor: i18n.language === 'zh' ? 'primary.main' : 'action.hover',
                        color: i18n.language === 'zh' ? 'primary.contrastText' : 'text.primary',
                        fontWeight: i18n.language === 'zh' ? 600 : 400,
                        border: i18n.language === 'zh' ? 'none' : '1px solid',
                        borderColor: 'divider',
                        cursor: 'pointer',
                        '&:hover': {
                            bgcolor: i18n.language === 'zh' ? 'primary.dark' : 'action.selected',
                        },
                    }}
                />
            </Box>
        </Tooltip>
    );
};

export default GuideLanguageToggle;
