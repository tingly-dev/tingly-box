import { Box, Button, CircularProgress, Dialog, DialogTitle, Tab, Tabs, Typography } from '@mui/material';
import React from 'react';
import { useTranslation } from 'react-i18next';
import { shouldIgnoreDialogClose } from '@/components/dialogClose';

export interface ConfigModalTabsProps {
    value: string;
    onChange: (value: string) => void;
    items: { value: string; label: React.ReactNode }[];
}

interface ConfigModalShellProps {
    open: boolean;
    onClose: () => void;
    /** h6 title line. */
    title: React.ReactNode;
    /** Muted subtitle line under the title. */
    subtitle?: React.ReactNode;
    /** Top-right header action (e.g. the reset-to-defaults IconButton),
     * absolutely positioned inside the title row. */
    headerAction?: React.ReactNode;
    /** Quick/Manual tab strip under the title. */
    tabs?: ConfigModalTabsProps;
    /** Dialog body — the caller renders DialogContent / DialogActions. */
    children: React.ReactNode;
}

// Shared skeleton for the big config modals (Codex / Dsh / OpenCode): lg
// always-fullwidth Dialog with rounded paper, a close-guarded onClose
// (backdrop clicks never close — Escape does), and the standard DialogTitle
// block (title + subtitle + optional header action + optional Quick/Manual
// tabs). Modal bodies and action rows stay per-tool.
export const ConfigModalShell: React.FC<ConfigModalShellProps> = ({
    open,
    onClose,
    title,
    subtitle,
    headerAction,
    tabs,
    children,
}) => (
    <Dialog
        open={open}
        onClose={(_event, reason) => {
            if (shouldIgnoreDialogClose(reason)) {
                return;
            }
            onClose();
        }}
        maxWidth="lg"
        fullWidth
        slotProps={{
            paper: {
                sx: {
                    borderRadius: 3,
                    maxHeight: '90vh',
                },
            }
        }}
    >
        <DialogTitle sx={{ pb: 1, borderBottom: 1, borderColor: 'divider', position: 'relative' }}>
            <Typography variant="h6" sx={{
                fontWeight: 600
            }}>
                {title}
            </Typography>
            {subtitle != null && (
                <Typography
                    variant="body2"
                    sx={{
                        color: "text.secondary",
                        mt: 0.5
                    }}>
                    {subtitle}
                </Typography>
            )}
            {headerAction}
            {tabs && (
                <Tabs
                    value={tabs.value}
                    onChange={(_, value) => tabs.onChange(value)}
                    sx={{ mt: 1, minHeight: 40, '& .MuiTabs-indicator': { height: 3 } }}
                >
                    {tabs.items.map((item) => (
                        <Tab key={item.value} label={item.label} value={item.value} sx={{ minHeight: 40, textTransform: 'none' }} />
                    ))}
                </Tabs>
            )}
        </DialogTitle>
        {children}
    </Dialog>
);

// Close + Auto-Config action row shared by the apply-capable config modals
// (Codex / Dsh). Rendered inside a DialogActions.
export const QuickApplyActions: React.FC<{
    onClose: () => void;
    onApply: () => void;
    applying: boolean;
}> = ({ onClose, onApply, applying }) => {
    const { t } = useTranslation();
    return (
        <Box sx={{ display: 'flex', justifyContent: 'flex-end', gap: 1, width: '100%' }}>
            <Button onClick={onClose} variant="outlined">
                {t('common.close')}
            </Button>
            <Button
                onClick={onApply}
                variant="contained"
                disabled={applying}
                startIcon={applying ? <CircularProgress size={16} color="inherit" /> : null}
            >
                {applying ? t('common.applying') : t('scenarioPage.autoConfig')}
            </Button>
        </Box>
    );
};
