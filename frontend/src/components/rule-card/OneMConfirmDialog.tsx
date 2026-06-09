import React from 'react';
import {
    Alert,
    Box,
    Button,
    Dialog,
    DialogActions,
    DialogContent,
    DialogTitle,
    Stack,
    Typography,
} from '@mui/material';
import { useTranslation } from 'react-i18next';
import { Bolt as BoltIcon, RestartAlt as RestartAltIcon } from '@/components/icons';
import { fontMono } from '@/theme/fonts';

// Effect class derived from the rule's scenario. 1M only materializes into an
// agent-config artifact for the two CLI agents, and both read that config at
// startup — so both need a re-apply + restart:
//   - codex   → ~/.codex model catalog context_window
//   - claude  → ~/.claude/settings.json model slots ([1m] suffix)
// Everything else ('gateway') only affects the proxy and needs no dialog.
export type OneMEffect = 'codex' | 'claude' | 'gateway';

export function oneMEffectForScenario(scenario?: string): OneMEffect {
    const base = (scenario || '').split(':')[0];
    if (base === 'codex') return 'codex';
    if (base === 'claude_code') return 'claude';
    return 'gateway';
}

export interface OneMConfirmDialogProps {
    open: boolean;
    /** true = turning 1M on, false = turning it off. */
    enabling: boolean;
    effect: OneMEffect;
    /** Phase: 'confirm' asks; 'applied' shows the restart reminder after apply. */
    phase: 'confirm' | 'applied';
    busy: boolean;
    error?: string;
    onConfirm: () => void;
    onCancel: () => void;
    onClose: () => void;
}

type Lang = 'zh' | 'en';

interface AgentCopy {
    name: string;
    body: string;
    apply: string;
    restart: string;
    /** restart text is a shell instruction → render monospace. */
    restartMono?: boolean;
}

interface DialogCopy {
    titleOn: string;
    titleOff: string;
    what: string;
    appliedTitle: string;
    restartLabel: string;
    cancel: string;
    done: string;
    agents: Record<'codex' | 'claude', AgentCopy>;
}

const TEXT: Record<Lang, DialogCopy> = {
    zh: {
        titleOn: '为该 rule 启用 1M 上下文',
        titleOff: '为该 rule 关闭 1M 上下文',
        what: '1M 指 1,000,000-token 的上下文窗口。它是该 rule 的开关，目标模型需支持，否则上游会拒绝。',
        appliedTitle: '生效就差最后一步',
        restartLabel: '下一步',
        cancel: '取消',
        done: '知道了',
        agents: {
            codex: {
                name: 'codex',
                body: 'Codex 启动时从模型 catalog 读取上下文窗口。要让 codex 用上 1M，需要重新写入 Codex 配置并重启 codex。点击 “应用 Codex 配置” 会立即写盘（合并保留你的其它设置）。',
                apply: '应用 Codex 配置',
                restart: '配置已写入。请退出并重新运行 `codex` 使其生效。',
                restartMono: true,
            },
            claude: {
                name: 'Claude Code',
                body: 'Claude Code 启动时从 ~/.claude/settings.json 读取模型配置。要让 Claude Code 用上 1M，需要重新写入配置并重启 Claude Code。点击 “应用 Claude Code 配置” 会按你当前的路由规则立即写盘。',
                apply: '应用 Claude Code 配置',
                restart: '配置已写入。请退出并重新启动 Claude Code 使其生效。',
            },
        },
    },
    en: {
        titleOn: 'Enable 1M context for this rule',
        titleOff: 'Disable 1M context for this rule',
        what: '1M is the 1,000,000-token context window. It is a per-rule switch — the routed model must support it or the upstream rejects the request.',
        appliedTitle: 'One last step to take effect',
        restartLabel: 'Next',
        cancel: 'Cancel',
        done: 'Got it',
        agents: {
            codex: {
                name: 'codex',
                body: 'Codex reads the context window from its model catalog at startup. For codex to use 1M, its config must be re-applied and codex restarted. “Apply Codex config” writes it now (merged, your other settings are preserved).',
                apply: 'Apply Codex config',
                restart: 'Config written. Quit and re-run `codex` for it to take effect.',
                restartMono: true,
            },
            claude: {
                name: 'Claude Code',
                body: 'Claude Code reads its model config from ~/.claude/settings.json at startup. For Claude Code to use 1M, its config must be re-applied and Claude Code restarted. “Apply Claude Code config” writes it now from your current routing rules.',
                apply: 'Apply Claude Code config',
                restart: 'Config written. Quit and relaunch Claude Code for it to take effect.',
            },
        },
    },
};

export const OneMConfirmDialog: React.FC<OneMConfirmDialogProps> = ({
    open,
    enabling,
    effect,
    phase,
    busy,
    error,
    onConfirm,
    onCancel,
    onClose,
}) => {
    const { i18n } = useTranslation();
    const lang: Lang = i18n.language === 'zh' ? 'zh' : 'en';
    const t = TEXT[lang];

    // 'gateway' should never reach the dialog (RuleCard toggles those directly),
    // but guard against an undefined agent lookup just in case.
    const agent = effect === 'claude' ? t.agents.claude : t.agents.codex;
    const title = enabling ? t.titleOn : t.titleOff;

    return (
        <Dialog open={open} onClose={busy ? undefined : onClose} maxWidth="xs" fullWidth>
            <DialogTitle sx={{ display: 'flex', alignItems: 'center', gap: 1, pb: 1 }}>
                <BoltIcon fontSize="small" color="primary" />
                {title}
            </DialogTitle>
            <DialogContent>
                {phase === 'confirm' ? (
                    <Stack spacing={1.5}>
                        <Typography variant="body2" color="text.secondary">
                            {t.what}
                        </Typography>
                        <Typography variant="body2">{agent.body}</Typography>
                        {error && <Alert severity="error">{error}</Alert>}
                    </Stack>
                ) : (
                    <Stack spacing={1.5}>
                        <Alert icon={<RestartAltIcon fontSize="inherit" />} severity="warning">
                            <Typography variant="body2" sx={{ fontWeight: 600 }}>
                                {t.appliedTitle}
                            </Typography>
                        </Alert>
                        <Box>
                            <Typography variant="caption" color="text.secondary">
                                {t.restartLabel}
                            </Typography>
                            <Typography variant="body2" sx={{ fontFamily: agent.restartMono ? fontMono : undefined }}>
                                {agent.restart}
                            </Typography>
                        </Box>
                    </Stack>
                )}
            </DialogContent>
            <DialogActions>
                {phase === 'confirm' ? (
                    <>
                        <Button onClick={onCancel} disabled={busy} color="inherit">
                            {t.cancel}
                        </Button>
                        <Button onClick={onConfirm} disabled={busy} variant="contained">
                            {agent.apply}
                        </Button>
                    </>
                ) : (
                    <Button onClick={onClose} variant="contained">
                        {t.done}
                    </Button>
                )}
            </DialogActions>
        </Dialog>
    );
};

export default OneMConfirmDialog;
