import { Box, ButtonBase, Divider, Drawer, List, ListItemButton, ListItemIcon, ListItemText, Typography } from '@mui/material';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link as RouterLink } from 'react-router-dom';
import { Check, ExpandMore } from '@/components/icons';
import { fontSizes } from '@tingly/ui/theme/fonts';
import { mobile, safeAreaBottom } from '@tingly/ui/theme/mobile';
import type { SubPage } from './mobileSubPages';

/**
 * The phone's stand-in for the desktop secondary sidebar: a bar naming the
 * page you are on, and a sheet listing every page of the activity to switch to.
 */
export function SubPageSwitcher({ title, groups }: { title: string; groups: SubPage[][] }) {
    const { t } = useTranslation();
    const [open, setOpen] = useState(false);

    const current = groups.flat().find((page) => page.active);

    return (
        <>
            <ButtonBase
                onClick={() => setOpen(true)}
                aria-haspopup="dialog"
                aria-label={t('mobile.subPages.switch', { title })}
                sx={{
                    flexShrink: 0, width: '100%', minHeight: mobile.tapMin, px: 2, gap: 1.25, justifyContent: 'flex-start',
                    borderBottom: 1, borderColor: 'divider', bgcolor: 'background.paper', textAlign: 'left',
                }}
            >
                <Box sx={{ display: 'flex', color: 'text.secondary', '& svg': { fontSize: 20 } }}>{current?.icon}</Box>
                <Typography noWrap sx={{ flex: 1, minWidth: 0, fontWeight: 600 }}>{current?.label ?? title}</Typography>
                <Typography variant="caption" color="text.secondary" sx={{ flexShrink: 0 }}>{title}</Typography>
                <ExpandMore sx={{ color: 'text.secondary', fontSize: 20 }} />
            </ButtonBase>
            <Drawer
                anchor="bottom"
                open={open}
                onClose={() => setOpen(false)}
                slotProps={{ paper: { 'aria-label': title, sx: { maxHeight: '80dvh', borderTopLeftRadius: (theme) => theme.spacing(mobile.sheetRadius), borderTopRightRadius: (theme) => theme.spacing(mobile.sheetRadius), pb: safeAreaBottom } } }}
            >
                <Typography variant="subtitle1" component="h2" sx={{ px: 2.5, pt: 2, pb: 1 }}>{title}</Typography>
                <List disablePadding sx={{ overflowY: 'auto', pb: 1 }}>
                    {groups.map((group, i) => (
                        <Box key={i} component="li" sx={{ listStyle: 'none' }}>
                            {i > 0 && <Divider sx={{ mx: 2, my: 0.5 }} />}
                            <List disablePadding>
                                {group.map((page) => (
                                    <ListItemButton
                                        key={page.path}
                                        component={RouterLink}
                                        to={page.path}
                                        onClick={() => setOpen(false)}
                                        selected={page.active}
                                        aria-current={page.active ? 'page' : undefined}
                                        sx={{ minHeight: mobile.rowMin, gap: 1.5, px: 2.5 }}
                                    >
                                        {page.icon && <ListItemIcon sx={{ minWidth: 0, color: 'text.secondary', '& svg': { fontSize: 22 } }}>{page.icon}</ListItemIcon>}
                                        <ListItemText
                                            primary={page.label}
                                            secondary={page.subtitle}
                                            slotProps={{ primary: { noWrap: true, sx: { fontWeight: 600 } }, secondary: { noWrap: true, sx: { fontSize: fontSizes.base } } }}
                                        />
                                        {page.active && <Check sx={{ color: 'primary.main' }} />}
                                    </ListItemButton>
                                ))}
                            </List>
                        </Box>
                    ))}
                </List>
            </Drawer>
        </>
    );
}
