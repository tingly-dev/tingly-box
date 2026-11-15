import { Box, Divider, List, ListItemButton, ListItemIcon, ListItemText, Paper, Typography } from '@mui/material';
import { useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { Link as RouterLink, Navigate, useNavigate } from 'react-router-dom';
import { ChevronRight, Computer, Help, Palette } from '@/components/icons';
import { PreferencesMenu } from '@/layout/PreferencesMenu';
import { useActivityItems } from '@/layout/useActivityItems';
import { fontSizes } from '@tingly/ui/theme/fonts';
import { mobile } from '@tingly/ui/theme/mobile';
import { radius } from '@tingly/ui/theme/radius';
import { setShellPref, useUiShell } from './uiShell';

interface Row {
    key: string;
    icon: ReactNode;
    label: string;
    hint?: string;
    to?: string;
    onClick?: (event: React.MouseEvent<HTMLElement>) => void;
}

function Group({ title, rows }: { title?: string; rows: Row[] }) {
    if (rows.length === 0) return null;
    return (
        <Box sx={{ mt: 2 }}>
            {title && <Typography variant="caption" color="text.secondary" sx={{ display: 'block', px: 0.5, mb: 0.75, fontWeight: 600 }}>{title}</Typography>}
            <Paper variant="outlined" sx={{ borderRadius: radius.card, overflow: 'hidden' }}>
                <List disablePadding>
                    {rows.map((row, i) => (
                        <Box key={row.key}>
                            {i > 0 && <Divider component="li" />}
                            <ListItemButton
                                {...(row.to ? { component: RouterLink, to: row.to } : { onClick: row.onClick })}
                                sx={{ minHeight: mobile.rowMin, gap: 1.5 }}
                            >
                                <ListItemIcon sx={{ minWidth: 0, color: 'text.secondary' }}>{row.icon}</ListItemIcon>
                                <ListItemText primary={row.label} secondary={row.hint} slotProps={{ primary: { sx: { fontWeight: 600 } }, secondary: { sx: { fontSize: fontSizes.base } } }} />
                                <ChevronRight sx={{ color: 'text.disabled' }} />
                            </ListItemButton>
                        </Box>
                    ))}
                </List>
            </Paper>
        </Box>
    );
}

/** The management entry: everything that is not the workspace or an agent, two taps from anywhere. */
export default function MorePage() {
    const { t } = useTranslation();
    const navigate = useNavigate();
    const { shell } = useUiShell();
    const items = useActivityItems();
    const [prefsAnchor, setPrefsAnchor] = useState<HTMLElement | null>(null);

    if (shell !== 'mobile') return <Navigate to="/agent" replace />;

    const manage: Row[] = items
        .filter((item) => !item.railHidden && item.key !== 'scenario' && item.key !== 'projects')
        .map((item) => ({ key: item.key, icon: item.icon, label: item.label, to: item.defaultPath ?? item.path }))
        .filter((row): row is Row & { to: string } => !!row.to);
    const system = items.find((item) => item.key === 'system');
    const systemRows: Row[] = [
        ...(system ? [{ key: 'system', icon: system.icon, label: system.label, to: system.defaultPath ?? system.path }] : []),
        { key: 'prefs', icon: <Palette />, label: t('mobile.more.preferences'), onClick: (event) => setPrefsAnchor(event.currentTarget) },
        { key: 'help', icon: <Help />, label: t('layout.help'), to: '/help' },
    ];
    const interfaceRows: Row[] = [
        { key: 'desktop', icon: <Computer />, label: t('mobile.more.useDesktop'), hint: t('mobile.more.useDesktopHint'), onClick: () => { setShellPref('desktop'); navigate('/agent'); } },
    ];

    return (
        <Box>
            <Typography variant="h4" component="h1" sx={{ mb: 0.5 }}>{t('mobile.more.title')}</Typography>
            <Group title={t('mobile.more.manage')} rows={manage} />
            <Group title={t('mobile.more.system')} rows={systemRows} />
            <Group title={t('mobile.more.interface')} rows={interfaceRows} />
            <PreferencesMenu anchorEl={prefsAnchor} onClose={() => setPrefsAnchor(null)} />
        </Box>
    );
}
