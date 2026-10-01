// The rail's single bottom button opens this: language, theme, Power-ups
// (a submenu), feedback and the version live together here instead of as four stacked rail buttons
// (.design/ui-redesign.md §3.2 — they are set-once preferences, and the rail
// ran out of room at 900px). The same controls stay on System › General.
import { AppRegistration as IconApps, ChevronRight as IconChevronRight, MessageReport as IconMessageReport, OpenInNew as IconOpenInNew, Settings as IconSettings } from '@/components/icons';
import { displayVersion } from '@/utils/version';
import { Box, Divider, MenuItem, MenuList, Popover, Stack, ToggleButton, ToggleButtonGroup, Tooltip, Typography } from '@mui/material';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link as RouterLink } from 'react-router-dom';
import { SUPPORTED_LANGUAGES, resolveLanguage } from '@/i18n';
import type { ThemeMode } from '@/theme';
import { getThemeOptions } from '@/theme/options';
import { useThemeMode } from '../contexts/ThemeContext';
import { useVersion as useAppVersion } from '../contexts/VersionContext';
import { Z_INDEX } from '../constants/zIndex';
import { PowerUpsMenu } from './PowerUpsMenu';

const FEEDBACK_URL = 'https://github.com/tingly-dev/tingly-box/issues/new/choose';

interface PreferencesMenuProps {
    anchorEl: HTMLElement | null;
    onClose: () => void;
}

const SectionLabel = ({ children }: { children: React.ReactNode }) => (
    <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mb: 0.75 }}>
        {children}
    </Typography>
);

export const PreferencesMenu: React.FC<PreferencesMenuProps> = ({ anchorEl, onClose }) => {
    const { t, i18n } = useTranslation();
    const { currentVersion } = useAppVersion();
    const { mode: themeMode, setTheme } = useThemeMode();
    const themeOptions = useMemo(() => getThemeOptions(t), [t]);
    const currentLanguage = resolveLanguage(i18n.language);
    const [powerUpsAnchorEl, setPowerUpsAnchorEl] = useState<HTMLElement | null>(null);
    // Hover opens the Power-ups submenu; leaving the row or the submenu
    // closes it after a short grace period, so the pointer can cross the gap.
    const closeTimer = useRef<number | undefined>(undefined);
    const cancelClose = useCallback(() => window.clearTimeout(closeTimer.current), []);
    const scheduleClose = useCallback(() => {
        window.clearTimeout(closeTimer.current);
        closeTimer.current = window.setTimeout(() => setPowerUpsAnchorEl(null), 200);
    }, []);
    useEffect(() => () => window.clearTimeout(closeTimer.current), []);
    useEffect(() => {
        if (!anchorEl) setPowerUpsAnchorEl(null);
    }, [anchorEl]);

    return (
        <Popover
            open={Boolean(anchorEl)}
            anchorEl={anchorEl}
            onClose={onClose}
            anchorOrigin={{ vertical: 'bottom', horizontal: 'right' }}
            transformOrigin={{ vertical: 'bottom', horizontal: 'left' }}
            sx={{ zIndex: Z_INDEX.popover }}
            // The Power-ups submenu is portaled outside this popover; without
            // this the focus trap would pull focus back from its switches.
            disableEnforceFocus
            slotProps={{ paper: { sx: { width: 300, ml: 1, borderRadius: 2 } } }}
        >
            <Box sx={{ px: 2, pt: 1.75, pb: 1.25 }}>
                <Typography variant="subtitle2" sx={{ fontWeight: 600 }}>
                    Tingly-Box <Typography component="span" variant="caption" color="text.secondary">{displayVersion(currentVersion)}</Typography>
                </Typography>
                <Typography variant="caption" color="text.secondary">{t('layout.easterEgg')}</Typography>
            </Box>
            <Divider />
            <Stack spacing={1.75} sx={{ px: 2, py: 1.5 }}>
                <Box>
                    <SectionLabel>{t('system.language.title')}</SectionLabel>
                    <ToggleButtonGroup
                        exclusive
                        size="small"
                        value={currentLanguage}
                        onChange={(_, code: string | null) => {
                            if (!code) return;
                            i18n.changeLanguage(code);
                            localStorage.setItem('i18nextLng', code);
                        }}
                        sx={{ flexWrap: 'wrap' }}
                    >
                        {SUPPORTED_LANGUAGES.map(({ code, labelKey }) => (
                            <ToggleButton key={code} value={code} sx={{ px: 1.25, py: 0.25, textTransform: 'none' }}>
                                {t(labelKey)}
                            </ToggleButton>
                        ))}
                    </ToggleButtonGroup>
                </Box>
                <Box>
                    <SectionLabel>{t('layout.activityBar.theme')}</SectionLabel>
                    <ToggleButtonGroup
                        exclusive
                        size="small"
                        value={themeMode}
                        onChange={(_, mode: ThemeMode | null) => mode && setTheme(mode)}
                        sx={{ flexWrap: 'wrap' }}
                    >
                        {themeOptions.map(({ value, label, renderIcon }) => (
                            <Tooltip key={value} title={label} arrow>
                                <ToggleButton value={value} aria-label={label} sx={{ px: 1, py: 0.5 }}>
                                    {renderIcon({ size: 18 })}
                                </ToggleButton>
                            </Tooltip>
                        ))}
                    </ToggleButtonGroup>
                </Box>
            </Stack>
            <Divider />
            <MenuList dense sx={{ py: 0.5 }}>
                {/* Power-ups: which optional items the rail shows (Team,
                    Image, Remote, Bench, …). A submenu, since it is a list
                    of switches rather than one choice. */}
                <MenuItem
                    onMouseEnter={(e) => { cancelClose(); setPowerUpsAnchorEl(e.currentTarget); }}
                    onMouseLeave={scheduleClose}
                    // Click/keyboard/touch still open it.
                    onClick={(e) => setPowerUpsAnchorEl(e.currentTarget)}
                    selected={Boolean(powerUpsAnchorEl)}
                    aria-haspopup="true"
                    aria-expanded={Boolean(powerUpsAnchorEl)}
                    sx={{ gap: 1.5 }}
                >
                    <IconApps sx={{ fontSize: 18 }} />
                    <Typography variant="body2" sx={{ flex: 1 }}>{t('layout.powerUps')}</Typography>
                    <IconChevronRight sx={{ fontSize: 16, color: 'text.disabled' }} />
                </MenuItem>
                <MenuItem component={RouterLink} to="/system" onClick={onClose} sx={{ gap: 1.5 }}>
                    <IconSettings sx={{ fontSize: 18 }} />
                    <Typography variant="body2">{t('layout.activityBar.allSettings')}</Typography>
                </MenuItem>
                <MenuItem component="a" href={FEEDBACK_URL} target="_blank" rel="noopener noreferrer" onClick={onClose} sx={{ gap: 1.5 }}>
                    <IconMessageReport sx={{ fontSize: 18 }} />
                    <Typography variant="body2" sx={{ flex: 1 }}>{t('layout.activityBar.feedback')}</Typography>
                    <IconOpenInNew sx={{ fontSize: 14, color: 'text.disabled' }} />
                </MenuItem>
            </MenuList>
            <PowerUpsMenu
                anchorEl={powerUpsAnchorEl}
                onClose={() => setPowerUpsAnchorEl(null)}
                onNavigate={onClose}
                onMouseEnter={cancelClose}
                onMouseLeave={scheduleClose}
            />
        </Popover>
    );
};
