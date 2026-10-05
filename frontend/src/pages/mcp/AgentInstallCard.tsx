/**
 * AgentInstallCard
 *
 * A self-contained, reusable "Add to Agents" block.
 * Shows a runtime picker (Claude Code / Codex / OpenCode) and a dark code block
 * with the registration command for the selected runtime.
 *
 * Usage:
 *   <AgentInstallCard />
 *   <AgentInstallCard sectionNumber="02" />
 */

import React, { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Box, Chip, Typography } from '@mui/material';
import { Terminal as TerminalIcon } from '@/components/icons';
import { CopyIconButton } from '@/components/CopyIconButton';
import { getApiBaseUrl } from '@/utils/protocol';
import { fontMono, fontSizes } from '@/theme/fonts';

// ─── Types ────────────────────────────────────────────────────────────────────

export type AgentRuntime = 'claude' | 'codex' | 'opencode';

export interface RuntimeOption {
    /** Short label shown on the pill button */
    label: string;
    /** Filename shown in the code-block header bar */
    filename: string;
    /** The command / snippet text displayed in the code block */
    command: string;
}

export type RuntimeOptions = Record<AgentRuntime, RuntimeOption>;

// ─── Sub-components ──────────────────────────────────────────────────────────

interface RuntimeSelectorProps {
    options: RuntimeOptions;
    value: AgentRuntime;
    onChange: (runtime: AgentRuntime) => void;
}

/**
 * A row of pill-shaped toggle buttons, one per runtime.
 * Active button uses emerald/green fill; inactive is white with a grey border.
 */
const RuntimeSelector: React.FC<RuntimeSelectorProps> = ({ options, value, onChange }) => (
    <Box sx={{ display: 'flex', gap: 0.75, flexWrap: 'wrap' }}>
        {(Object.entries(options) as [AgentRuntime, RuntimeOption][]).map(([key, opt]) => {
            const active = value === key;
            return (
                <Box
                    key={key}
                    component="button"
                    aria-pressed={active}
                    onClick={() => onChange(key)}
                    sx={{
                        px: 1.5,
                        height: 28,
                        fontSize: fontSizes.sm,
                        fontWeight: 600,
                        fontFamily: 'inherit',
                        cursor: 'pointer',
                        borderRadius: '999px',
                        border: '1px solid',
                        borderColor: active ? 'rgb(10, 124, 90)' : 'rgb(229, 231, 236)',
                        bgcolor: active ? 'rgb(10, 124, 90)' : '#fff',
                        color: active ? '#fff' : 'rgb(13, 17, 23)',
                        transition: 'background-color 0.15s, color 0.15s, border-color 0.15s',
                        lineHeight: 1,
                        '&:hover': {
                            borderColor: active ? 'rgb(10, 124, 90)' : 'rgb(156, 163, 175)',
                            bgcolor: active ? 'rgb(10, 124, 90)' : 'rgb(249, 250, 251)',
                        },
                    }}
                >
                    {opt.label}
                </Box>
            );
        })}
    </Box>
);

interface CodeBlockProps {
    filename: string;
    runtimeLabel: string;
    command: string;
}

/**
 * Dark-themed code block: filename header bar + syntax-coloured command pre.
 * Includes a copy-to-clipboard button in the header.
 */
const CodeBlock: React.FC<CodeBlockProps> = ({ filename, runtimeLabel, command }) => {
    return (
        <Box
            sx={{
                bgcolor: 'rgb(13, 17, 23)',
                borderRadius: '10px',
                border: '1px solid rgb(31, 37, 48)',
                overflow: 'hidden',
            }}
        >
            {/* Header bar: filename + runtime chip + copy */}
            <Box
                sx={{
                    display: 'flex',
                    alignItems: 'center',
                    gap: 1,
                    px: 2,
                    py: 1,
                    borderBottom: '1px solid rgb(31, 37, 48)',
                }}
            >
                <Typography
                    sx={{
                        flex: 1,
                        fontSize: fontSizes.xs,
                        fontFamily: fontMono,
                        color: 'rgb(125, 133, 144)',
                    }}
                >
                    {filename}
                </Typography>
                <Chip
                    label={runtimeLabel}
                    size="small"
                    sx={{
                        height: 18,
                        fontSize: fontSizes.micro,
                        fontWeight: 600,
                        bgcolor: 'rgb(31, 37, 48)',
                        color: 'rgb(154, 161, 172)',
                        border: '1px solid rgb(48, 54, 61)',
                    }}
                />
                <CopyIconButton
                    value={command.replace(/\\\n\s*/g, ' ')}
                    color="rgb(125, 133, 144)"
                    copiedColor="rgb(16, 185, 129)"
                    iconSize="0.85rem"
                    tooltipArrow
                    sx={{ p: 0.5, '&:hover': { color: 'rgb(201, 209, 217)' } }}
                />
            </Box>

            {/* Command text */}
            <Box
                component="pre"
                sx={{
                    m: 0,
                    px: 2,
                    py: 1.75,
                    fontFamily: fontMono,
                    fontSize: fontSizes.md,
                    lineHeight: 1.7,
                    whiteSpace: 'pre-wrap',
                    wordBreak: 'break-all',
                    color: 'rgb(230, 237, 243)',
                }}
            >
                {command}
            </Box>
        </Box>
    );
};

// ─── Main component ──────────────────────────────────────────────────────────

export interface AgentInstallCardProps {
    /** Initial command format; users can still choose any supported client. */
    defaultRuntime?: AgentRuntime;
    /** Overrides the default runtime options (commands/filenames) */
    runtimeOptions?: RuntimeOptions;
    clientId?: string;
    /** Section number badge shown to the left of the heading (default: "01") */
    sectionNumber?: string;
    /** Heading text (default: "Add to agents") */
    heading?: string;
    /** Subtitle below heading (default: "Register the gateway…") */
    subtitle?: string;
    /** Extra content rendered below the code block inside the card */
    footer?: React.ReactNode;
}

// The token is read from local config at run time (`~/.tingly-box/config.json`),
// so these commands only need to be correct about *where* the gateway is
// bound — which is why the endpoint is built from the resolved API base URL
// (buildDefaultRuntimeOptions) rather than hardcoded, so it stays correct
// when the server isn't on the default port.
const buildDefaultRuntimeOptions = (baseUrl: string, clientId: string): RuntimeOptions => {
    const endpoint = `${baseUrl}/api/v1/mcp/${encodeURIComponent(clientId)}`;
    return {
        claude: {
            label: 'Claude Code',
            filename: 'register-tb.sh',
            command: `claude mcp add --transport http tb "${endpoint}" --header "Authorization: Bearer $(cat ~/.tingly-box/config.json | jq -r '.user_token')"`,
        },
        codex: {
            label: 'Codex',
            filename: 'register-tb.sh',
            command: `codex mcp add tb --url "${endpoint}" --bearer-token-env-var TINGLY_MCP_TOKEN`,
        },
        opencode: {
            label: 'OpenCode',
            filename: '~/.config/opencode/opencode.json',
            command: `"mcp": {\n  "${endpoint}": {\n    "type": "remote",\n    "url": "${endpoint}",\n    "oauth": false,\n    "headers": {\n      "Authorization": "Bearer {env:MY_API_KEY}"\n    }\n  }\n}`,
        },
    };
};

// Fallback shown before getApiBaseUrl() resolves — matches the gateway's
// documented default port, same as every other "not yet loaded" placeholder
// in the scenario pages.
const DEFAULT_BASE_URL = 'http://localhost:12580';

export const AgentInstallCard: React.FC<AgentInstallCardProps> = ({
    defaultRuntime = 'claude',
    runtimeOptions,
    clientId = 'tb',
    sectionNumber = '01',
    heading,
    subtitle,
    footer,
}) => {
    const { t } = useTranslation();
    const [runtime, setRuntime] = useState<AgentRuntime>(defaultRuntime);
    const [baseUrl, setBaseUrl] = useState(DEFAULT_BASE_URL);

    useEffect(() => {
        // Only needed for the default commands; an explicit runtimeOptions
        // override means the caller already resolved its own base URL.
        if (runtimeOptions) return;
        let isMounted = true;
        getApiBaseUrl().then((url) => {
            if (isMounted) setBaseUrl(url);
        });
        return () => {
            isMounted = false;
        };
    }, [runtimeOptions]);

    const resolvedOptions = runtimeOptions ?? buildDefaultRuntimeOptions(baseUrl, clientId);
    const config = resolvedOptions[runtime];

    return (
        <Box>
            {/* ── Section header ── */}
            <Box sx={{ display: 'flex', alignItems: 'flex-start', gap: 2, mb: 2.5 }}>
                <Typography
                    sx={{
                        fontFamily: fontMono,
                        fontSize: fontSizes.lg,
                        fontWeight: 700,
                        color: 'text.primary',
                        mt: 0.35,
                        flexShrink: 0,
                        userSelect: 'none',
                        opacity: 0.35,
                        letterSpacing: '0.05em',
                    }}
                >
                    {sectionNumber}
                </Typography>
                <Box>
                    <Typography variant="h5" sx={{ fontWeight: 700, lineHeight: 1.2, mb: 0.5 }}>
                        {heading ?? t('mcp.center.installHeading')}
                    </Typography>
                    <Typography
                        variant="body2"
                        sx={{
                            color: 'text.secondary',
                        }}
                    >
                        {subtitle ?? t('mcp.center.installSubtitle')}
                    </Typography>
                </Box>
            </Box>
            {/* ── Card body ── */}
            <Box
                sx={{
                    border: '1px solid',
                    borderColor: 'divider',
                    borderRadius: '14px',
                    overflow: 'hidden',
                    bgcolor: 'background.paper',
                }}
            >
                {/* Card header: icon + title/caption + runtime selector */}
                <Box
                    sx={{
                        display: 'flex',
                        alignItems: 'center',
                        gap: 2,
                        px: 2.5,
                        py: 2,
                        borderBottom: '1px solid',
                        borderColor: 'divider',
                    }}
                >
                    {/* Icon */}
                    <Box
                        sx={{
                            width: 36,
                            height: 36,
                            borderRadius: 1.5,
                            bgcolor: 'action.selected',
                            display: 'flex',
                            alignItems: 'center',
                            justifyContent: 'center',
                            flexShrink: 0,
                        }}
                    >
                        <TerminalIcon sx={{ fontSize: '1.1rem', color: 'text.secondary' }} />
                    </Box>

                    {/* Title + caption */}
                    <Box sx={{ flex: 1, minWidth: 0 }}>
                        <Typography variant="subtitle2" sx={{ fontWeight: 600, lineHeight: 1.3 }}>
                            {t('mcp.center.pickRuntime')}
                        </Typography>
                        <Typography
                            variant="caption"
                            sx={{
                                color: 'text.secondary',
                                lineHeight: 1.4,
                            }}
                        >
                            {t(
                                runtime === 'codex'
                                    ? 'mcp.center.codexTokenHint'
                                    : runtime === 'opencode'
                                      ? 'mcp.center.openCodeTokenHint'
                                      : 'mcp.center.localTokenHint'
                            )}
                        </Typography>
                    </Box>

                    {/* Runtime pills */}
                    <RuntimeSelector options={resolvedOptions} value={runtime} onChange={setRuntime} />
                </Box>

                {/* Code block */}
                <Box sx={{ p: 2 }}>
                    <CodeBlock filename={config.filename} runtimeLabel={config.label} command={config.command} />
                </Box>

                {runtime === 'codex' && <Alert severity="info">{t('mcp.center.codexSetup')}</Alert>}
                {/* OpenCode extra note */}
                {runtime === 'opencode' && (
                    <Alert
                        severity="info"
                        sx={{
                            borderRadius: 0,
                            borderTop: '1px solid',
                            borderColor: 'divider',
                            mx: 0,
                            '& .MuiAlert-message': { fontSize: fontSizes.md },
                        }}
                    >
                        {t('mcp.center.openCodeSetup')}{' '}
                        <code>{"cat ~/.tingly-box/config.json | jq -r '.user_token'"}</code>
                    </Alert>
                )}

                {/* Optional footer slot */}
                {footer && (
                    <Box
                        sx={{
                            px: 2.5,
                            py: 1.5,
                            borderTop: '1px solid',
                            borderColor: 'divider',
                        }}
                    >
                        {footer}
                    </Box>
                )}
            </Box>
        </Box>
    );
};

export default AgentInstallCard;
