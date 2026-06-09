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

// Effect class derived from the rule's scenario. This is the honest core of the
// dialog: 1M means different things per agent.
//   - codex: only takes effect via the model catalog → needs re-apply + codex
//     restart. The dialog offers a one-click apply.
//   - gateway: Claude Code / Anthropic — the gateway honors the rule flag on
//     the next request immediately, no apply / restart. The dialog just
//     confirms and educates.
export type OneMEffect = 'codex' | 'gateway';

export function oneMEffectForScenario(scenario?: string): OneMEffect {
    return (scenario || '').startsWith('codex') ? 'codex' : 'gateway';
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

const TEXT: Record<Lang, {
    titleOn: string;
    titleOff: string;
    what: string;
    codexBody: string;
    codexRestart: string;
    gatewayBody: string;
    gatewayRestart: string;
    appliedTitle: string;
    cancel: string;
    apply: string;
    enable: string;
    disable: string;
    done: string;
    restartLabel: string;
}> = {
    zh: {
        titleOn: '为该 rule 启用 1M 上下文',
        titleOff: '为该 rule 关闭 1M 上下文',
        what: '1M 指 1,000,000-token 的上下文窗口。它是该 rule 的开关，目标模型需支持，否则上游会拒绝。',
        codexBody: 'Codex 通过模型 catalog 感知上下文窗口。要让 codex 用上 1M，需要重新写入 Codex 配置并重启 codex。点击 “应用 Codex 配置” 会立即写盘（合并保留你的其它设置）。',
        codexRestart: '配置已写入。请退出并重新运行 `codex` 使其生效。',
        gatewayBody: '该改动在网关侧对此 rule 即时生效（下一条请求起），无需重启。仅当你想把 settings.json 里的 [1m] 后缀也同步给客户端时，才需到 Claude Code Quick Config 重新 Apply 并重启 Claude Code。',
        gatewayRestart: '已启用，网关即时生效。',
        appliedTitle: '生效就差最后一步',
        cancel: '取消',
        apply: '应用 Codex 配置',
        enable: '启用',
        disable: '关闭',
        done: '知道了',
        restartLabel: '下一步',
    },
    en: {
        titleOn: 'Enable 1M context for this rule',
        titleOff: 'Disable 1M context for this rule',
        what: '1M is the 1,000,000-token context window. It is a per-rule switch — the routed model must support it or the upstream rejects the request.',
        codexBody: 'Codex learns the context window from its model catalog. For codex to use 1M, its config must be re-applied and codex restarted. “Apply Codex config” writes it now (merged, your other settings are preserved).',
        codexRestart: 'Config written. Quit and re-run `codex` for it to take effect.',
        gatewayBody: 'This takes effect at the gateway for this rule immediately (from the next request) — no restart. Only if you also want the settings.json [1m] suffix synced should you re-apply Claude Code Quick Config and restart Claude Code.',
        gatewayRestart: 'Enabled — effective immediately at the gateway.',
        appliedTitle: 'One last step to take effect',
        cancel: 'Cancel',
        apply: 'Apply Codex config',
        enable: 'Enable',
        disable: 'Disable',
        done: 'Got it',
        restartLabel: 'Next',
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

    const isCodex = effect === 'codex';
    const title = enabling ? t.titleOn : t.titleOff;

    // Primary action label: codex applies config; gateway just commits the flag.
    const confirmLabel = isCodex ? t.apply : enabling ? t.enable : t.disable;
    const restartText = isCodex ? t.codexRestart : t.gatewayRestart;

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
                        <Typography variant="body2">
                            {isCodex ? t.codexBody : t.gatewayBody}
                        </Typography>
                        {error && <Alert severity="error">{error}</Alert>}
                    </Stack>
                ) : (
                    <Stack spacing={1.5}>
                        <Alert
                            icon={<RestartAltIcon fontSize="inherit" />}
                            severity={isCodex ? 'warning' : 'success'}
                        >
                            <Typography variant="body2" sx={{ fontWeight: 600 }}>
                                {t.appliedTitle}
                            </Typography>
                        </Alert>
                        <Box>
                            <Typography variant="caption" color="text.secondary">
                                {t.restartLabel}
                            </Typography>
                            <Typography variant="body2" sx={{ fontFamily: isCodex ? fontMono : undefined }}>
                                {restartText}
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
                            {confirmLabel}
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
