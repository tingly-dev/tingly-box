import React from 'react';
import { Button, Dialog, DialogActions, DialogContent, DialogTitle, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { Bolt as BoltIcon } from '@/components/icons';

// oneMAgentForScenario maps a rule scenario to the restartable agent whose
// config/runtime materializes 1M. Returns null for non-agent scenarios (plain
// anthropic/openai), where the switch just saves directly with no dialog.
// The copy is generic — only the agent name differs — so there is no per-agent
// special-casing.
export function oneMAgentForScenario(scenario?: string): string | null {
    switch ((scenario || '').split(':')[0]) {
        case 'codex': return 'Codex';
        case 'claude_code': return 'Claude Code';
        case 'claude_desktop': return 'Claude Desktop';
        default: return null;
    }
}

export interface OneMConfirmDialogProps {
    open: boolean;
    enabling: boolean;
    agent: string;
    busy: boolean;
    onConfirm: () => void;
    onCancel: () => void;
}

const copy = (lang: 'zh' | 'en', agent: string, enabling: boolean) =>
    lang === 'zh'
        ? {
            title: enabling ? '为该 rule 启用 1M 上下文' : '为该 rule 关闭 1M 上下文',
            body: `保存后需重启 ${agent} 才会生效。`,
            cancel: '取消',
            confirm: '应用',
        }
        : {
            title: enabling ? 'Enable 1M context for this rule' : 'Disable 1M context for this rule',
            body: `Restart ${agent} for the change to take effect.`,
            cancel: 'Cancel',
            confirm: 'Apply',
        };

export const OneMConfirmDialog: React.FC<OneMConfirmDialogProps> = ({ open, enabling, agent, busy, onConfirm, onCancel }) => {
    const { i18n } = useTranslation();
    const t = copy(i18n.language === 'zh' ? 'zh' : 'en', agent, enabling);
    return (
        <Dialog open={open} onClose={busy ? undefined : onCancel} maxWidth="xs" fullWidth>
            <DialogTitle sx={{ display: 'flex', alignItems: 'center', gap: 1, pb: 1 }}>
                <BoltIcon fontSize="small" color="primary" />
                {t.title}
            </DialogTitle>
            <DialogContent>
                <Typography variant="body2" color="text.secondary">{t.body}</Typography>
            </DialogContent>
            <DialogActions>
                <Button onClick={onCancel} disabled={busy} color="inherit">{t.cancel}</Button>
                <Button onClick={onConfirm} disabled={busy} variant="contained">{t.confirm}</Button>
            </DialogActions>
        </Dialog>
    );
};

export default OneMConfirmDialog;
