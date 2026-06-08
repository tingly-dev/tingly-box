import {
    Dialog,
    DialogTitle,
    DialogContent,
    DialogContentText,
    DialogActions,
    Button,
    FormControlLabel,
    Checkbox,
    Box,
    Typography,
} from '@mui/material';
import React, { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { isClaudeCodeScenario } from './utils';

// sessionStorage key for the "don't ask again this session" preference.
const SUPPRESS_KEY = 'tb.oneM.reconfig.suppress';

export function isOneMReconfigSuppressed(): boolean {
    try {
        return sessionStorage.getItem(SUPPRESS_KEY) === '1';
    } catch {
        return false;
    }
}

function suppressOneMReconfig() {
    try {
        sessionStorage.setItem(SUPPRESS_KEY, '1');
    } catch {
        /* ignore */
    }
}

export interface OneMReconfigDialogProps {
    open: boolean;
    /** The rule's scenario, e.g. "claude_code" or "claude_code:p1". */
    scenario: string;
    /** Whether the toggle just enabled (true) or disabled (false) 1M. */
    enabled: boolean;
    onClose: () => void;
    /**
     * Optional one-click re-apply (default scenario only). When provided, a
     * "Re-apply config" button is shown; it should re-derive prefs from the
     * now-updated rules and call the Claude Code apply endpoint.
     */
    onReapply?: () => void;
}

/**
 * Shown after the 1M switch is toggled on a Claude Code rule. The rule
 * (routing) is already updated, but the materialized client env (settings.json
 * for the default scenario / launch-time env for a profile) is now stale, so
 * the change won't reach Claude Code until it is re-materialized + the session
 * restarted. See .design/one-m-context.md §5c.
 */
const OneMReconfigDialog: React.FC<OneMReconfigDialogProps> = ({
    open,
    scenario,
    enabled,
    onClose,
    onReapply,
}) => {
    const { i18n } = useTranslation();
    const zh = i18n.language === 'zh';
    const [dontAsk, setDontAsk] = useState(false);

    const isProfile = isClaudeCodeScenario(scenario) && scenario.includes(':');
    const profileId = isProfile ? scenario.split(':')[1] : '';

    const handleClose = () => {
        if (dontAsk) suppressOneMReconfig();
        onClose();
    };

    const handleReapply = () => {
        if (dontAsk) suppressOneMReconfig();
        onReapply?.();
        onClose();
    };

    const title = zh
        ? enabled ? '已启用 1M 上下文' : '已关闭 1M 上下文'
        : enabled ? '1M context enabled' : '1M context disabled';

    return (
        <Dialog open={open} onClose={handleClose} maxWidth="sm" fullWidth>
            <DialogTitle>{title}</DialogTitle>
            <DialogContent>
                <DialogContentText component="div">
                    {zh ? (
                        <>
                            <Typography variant="body2" sx={{ mb: 1 }}>
                                路由规则已更新。但 Claude Code 是通过<strong>本地配置里的模型名</strong>
                                感知 1M 的,需要重新落盘配置才会生效。
                            </Typography>
                            {isProfile ? (
                                <Typography variant="body2">
                                    该 profile 的 env 在启动时生成 —— 重启会话即可:
                                    <Box component="code" sx={{ display: 'block', mt: 0.5, p: 1, bgcolor: 'action.hover', borderRadius: 1 }}>
                                        tingly-box cc --profile {profileId}
                                    </Box>
                                </Typography>
                            ) : (
                                <Typography variant="body2">
                                    请重新应用 Claude Code 配置(写入 settings.json),
                                    并<strong>重启正在运行的 Claude Code 会话</strong> —— env 变量在进程启动时读取。
                                </Typography>
                            )}
                        </>
                    ) : (
                        <>
                            <Typography variant="body2" sx={{ mb: 1 }}>
                                The routing rule is updated. But Claude Code perceives 1M from the
                                <strong> model name in its local config</strong>, so the change only
                                takes effect once that config is re-materialized.
                            </Typography>
                            {isProfile ? (
                                <Typography variant="body2">
                                    This profile&apos;s env is generated at launch — just restart the session:
                                    <Box component="code" sx={{ display: 'block', mt: 0.5, p: 1, bgcolor: 'action.hover', borderRadius: 1 }}>
                                        tingly-box cc --profile {profileId}
                                    </Box>
                                </Typography>
                            ) : (
                                <Typography variant="body2">
                                    Re-apply your Claude Code config (writes settings.json) and
                                    <strong> restart any running Claude Code session</strong> — env vars are
                                    read at process start.
                                </Typography>
                            )}
                        </>
                    )}
                </DialogContentText>
                <FormControlLabel
                    sx={{ mt: 1.5 }}
                    control={<Checkbox size="small" checked={dontAsk} onChange={(_, c) => setDontAsk(c)} />}
                    label={zh ? '本次会话不再提示' : "Don't ask again this session"}
                />
            </DialogContent>
            <DialogActions>
                <Button onClick={handleClose} color="inherit">
                    {zh ? '知道了' : 'Got it'}
                </Button>
                {!isProfile && onReapply && (
                    <Button onClick={handleReapply} variant="contained">
                        {zh ? '立即重新应用' : 'Re-apply config'}
                    </Button>
                )}
            </DialogActions>
        </Dialog>
    );
};

export default OneMReconfigDialog;
