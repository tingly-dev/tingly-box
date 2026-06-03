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

import React, { useCallback, useState } from 'react';
import { Alert, Box, Chip, IconButton, Tooltip, Typography } from '@mui/material';
import {
    ContentCopy as CopyIcon,
    Terminal as TerminalIcon,
} from '@/components/icons';

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
export const RuntimeSelector: React.FC<RuntimeSelectorProps> = ({ options, value, onChange }) => (
    <Box sx={{ display: 'flex', gap: 0.75, flexWrap: 'wrap' }}>
        {(Object.entries(options) as [AgentRuntime, RuntimeOption][]).map(([key, opt]) => {
            const active = value === key;
            return (
                <Box
                    key={key}
                    component="button"
                    onClick={() => onChange(key)}
                    sx={{
                        px: 1.5,
                        height: 28,
                        fontSize: '0.75rem',
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
export const CodeBlock: React.FC<CodeBlockProps> = ({ filename, runtimeLabel, command }) => {
    const [copied, setCopied] = useState(false);

    const handleCopy = useCallback(() => {
        void navigator.clipboard.writeText(command.replace(/\\\n\s*/g, ' '));
        setCopied(true);
        setTimeout(() => setCopied(false), 2000);
    }, [command]);

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
                        fontSize: '0.72rem',
                        fontFamily: 'monospace',
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
                        fontSize: '0.65rem',
                        fontWeight: 600,
                        bgcolor: 'rgb(31, 37, 48)',
                        color: 'rgb(154, 161, 172)',
                        border: '1px solid rgb(48, 54, 61)',
                    }}
                />
                <Tooltip title={copied ? 'Copied!' : 'Copy'} arrow>
                    <IconButton
                        size="small"
                        onClick={handleCopy}
                        sx={{
                            p: 0.5,
                            color: copied ? 'rgb(16, 185, 129)' : 'rgb(125, 133, 144)',
                            '&:hover': { color: 'rgb(201, 209, 217)' },
                        }}
                    >
                        <CopyIcon sx={{ fontSize: '0.85rem' }} />
                    </IconButton>
                </Tooltip>
            </Box>

            {/* Command text */}
            <Box
                component="pre"
                sx={{
                    m: 0,
                    px: 2,
                    py: 1.75,
                    fontFamily: 'monospace',
                    fontSize: '0.78rem',
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
    /** Overrides the default runtime options (commands/filenames) */
    runtimeOptions?: RuntimeOptions;
    /** Section number badge shown to the left of the heading (default: "01") */
    sectionNumber?: string;
    /** Heading text (default: "Add to agents") */
    heading?: string;
    /** Subtitle below heading (default: "Register the gateway…") */
    subtitle?: string;
    /** Extra content rendered below the code block inside the card */
    footer?: React.ReactNode;
}

const DEFAULT_MCP_COMMAND = `claude mcp add --transport http tb "http://localhost:12580/api/v1/mcp/tb" --header "Authorization: Bearer $(cat ~/.tingly-box/config.json | jq -r '.user_token')"`;

const DEFAULT_CODEX_COMMAND = `codex mcp add tb -- http http://localhost:12580/api/v1/mcp/tb \\\n  --header "Authorization: Bearer $(cat ~/.tingly-box/config.json | jq -r '.user_token')"`;

const DEFAULT_OPENCODE_COMMAND = `"mcp": {\n  "http://localhost:12580/api/v1/mcp/tb": {\n    "type": "remote",\n    "url": "http://localhost:12580/api/v1/mcp/tb",\n    "oauth": false,\n    "headers": {\n      "Authorization": "Bearer {MY_API_KEY}"\n    }\n  }\n}`;

const DEFAULT_RUNTIME_OPTIONS: RuntimeOptions = {
    claude: {
        label: 'Claude Code',
        filename: 'register-tb.sh',
        command: DEFAULT_MCP_COMMAND,
    },
    codex: {
        label: 'Codex',
        filename: 'register-tb.sh',
        command: DEFAULT_CODEX_COMMAND,
    },
    opencode: {
        label: 'OpenCode',
        filename: '~/.config/opencode/opencode.json',
        command: DEFAULT_OPENCODE_COMMAND,
    },
};

export const AgentInstallCard: React.FC<AgentInstallCardProps> = ({
    runtimeOptions = DEFAULT_RUNTIME_OPTIONS,
    sectionNumber = '01',
    heading = 'Add to agents',
    subtitle = 'Register the gateway with your coding agent. Run once per machine.',
    footer,
}) => {
    const [runtime, setRuntime] = useState<AgentRuntime>('claude');
    const config = runtimeOptions[runtime];

    return (
        <Box>
            {/* ── Section header ── */}
            <Box sx={{ display: 'flex', alignItems: 'flex-start', gap: 2, mb: 2.5 }}>
                <Typography
                    sx={{
                        fontFamily: 'monospace',
                        fontSize: '0.85rem',
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
                        {heading}
                    </Typography>
                    <Typography variant="body2" color="text.secondary">
                        {subtitle}
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
                            Pick your runtime
                        </Typography>
                        <Typography variant="caption" color="text.secondary" sx={{ lineHeight: 1.4 }}>
                            The token is read from your local config — no copy/paste required.
                        </Typography>
                    </Box>

                    {/* Runtime pills */}
                    <RuntimeSelector
                        options={runtimeOptions}
                        value={runtime}
                        onChange={setRuntime}
                    />
                </Box>

                {/* Code block */}
                <Box sx={{ p: 2 }}>
                    <CodeBlock
                        filename={config.filename}
                        runtimeLabel={config.label}
                        command={config.command}
                    />
                </Box>

                {/* OpenCode extra note */}
                {runtime === 'opencode' && (
                    <Alert
                        severity="info"
                        sx={{
                            borderRadius: 0,
                            borderTop: '1px solid',
                            borderColor: 'divider',
                            mx: 0,
                            '& .MuiAlert-message': { fontSize: '0.8rem' },
                        }}
                    >
                        Set <code>MY_API_KEY</code> to your token. Run{' '}
                        <code>{'cat ~/.tingly-box/config.json | jq -r \'.user_token\''}</code> to get it.
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
