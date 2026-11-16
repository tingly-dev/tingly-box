import React, { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Box, Typography } from '@mui/material';
import ConfigRow from '@/components/ConfigRow';
import { ChoiceToggle } from '@/components/ChoiceToggle';
import { CopyIconButton } from '@/components/CopyIconButton';
import { api } from '@/services/api';
import { getApiBaseUrl } from '@/utils/protocol';
import { copyableTextStyle } from '@/styles/textStyles';
import { fontMono } from '@/theme/fonts';

export type AgentRuntime = 'claude' | 'codex' | 'opencode';

// Shown until the token loads, so the command is never copied half-built by accident.
const TOKEN_PLACEHOLDER = 'YOUR_TOKEN';

export const profileEndpoint = (baseUrl: string, profileId: string) =>
    `${baseUrl}/api/v1/mcp/${encodeURIComponent(profileId)}`;

// Each client's registration with the real endpoint and token written in: the
// user copies and runs it, nothing is read from the machine or an environment.
export const registerCommands = (
    baseUrl: string,
    profileId: string,
    serverName: string,
    token: string
): Record<AgentRuntime, { target: string; command: string }> => {
    const endpoint = profileEndpoint(baseUrl, profileId);
    return {
        claude: {
            target: '',
            command: `claude mcp add --transport http ${serverName} "${endpoint}" --header "Authorization: Bearer ${token}"`,
        },
        codex: {
            target: '~/.codex/config.toml',
            command: `[mcp_servers.${serverName}]\nurl = "${endpoint}"\nhttp_headers = { Authorization = "Bearer ${token}" }`,
        },
        opencode: {
            target: '~/.config/opencode/opencode.json',
            command: `"mcp": {\n  "${serverName}": {\n    "type": "remote",\n    "url": "${endpoint}",\n    "oauth": false,\n    "headers": {\n      "Authorization": "Bearer ${token}"\n    }\n  }\n}`,
        },
    };
};

// Before getApiBaseUrl() resolves: the gateway's documented default port.
const DEFAULT_BASE_URL = 'http://localhost:12580';

// How to connect a client to a profile, in the shape of the Claude Code page's
// connection card: the URL is one row, the command another (the client is a
// toggle beside it), and the command is shown whole, as it will be copied.
export default function ProfileConnect({
    profileId,
    serverName,
    defaultRuntime = 'claude',
}: {
    profileId: string;
    serverName: string;
    defaultRuntime?: AgentRuntime;
}) {
    const { t } = useTranslation();
    const label = (key: string, fallback: string) => t(`mcp.workspace.${key}`, { defaultValue: fallback });
    const [runtime, setRuntime] = useState<AgentRuntime>(defaultRuntime);
    const [baseUrl, setBaseUrl] = useState(DEFAULT_BASE_URL);
    const [token, setToken] = useState('');
    useEffect(() => {
        let mounted = true;
        void getApiBaseUrl().then((url) => mounted && setBaseUrl(url));
        void api
            .getToken()
            .then((result) => mounted && result?.token && setToken(result.token))
            .catch(() => undefined);
        return () => {
            mounted = false;
        };
    }, []);
    const endpoint = profileEndpoint(baseUrl, profileId);
    const { command, target } = registerCommands(baseUrl, profileId, serverName, token || TOKEN_PLACEHOLDER)[runtime];
    return (
        <Box>
            <ConfigRow
                tabs={[
                    {
                        key: 'endpoint',
                        label: label('endpointLabel', 'Endpoint URL'),
                        content: (
                            <Typography variant="subtitle2" sx={copyableTextStyle}>
                                {endpoint}
                            </Typography>
                        ),
                        actions: <CopyIconButton value={endpoint} label={label('copyEndpoint', 'Copy URL')} />,
                    },
                ]}
                activeTab="endpoint"
                onTabChange={() => {}}
            />
            <ConfigRow
                tabs={[
                    {
                        key: 'command',
                        label: label('addToClient', 'Add to client'),
                        content: (
                            <Box sx={{ py: 1 }}>
                                {target && (
                                    <Typography variant="caption" color="text.secondary" component="div" sx={{ mb: 0.5 }}>
                                        {label('addToFile', 'Add to')} {target}
                                    </Typography>
                                )}
                                <Box
                                    component="pre"
                                    data-testid="mcp-connect-command"
                                    sx={{
                                        m: 0,
                                        fontFamily: fontMono,
                                        fontSize: 13,
                                        lineHeight: 1.5,
                                        whiteSpace: 'pre-wrap',
                                        overflowWrap: 'anywhere',
                                        userSelect: 'all',
                                    }}
                                >
                                    {command}
                                </Box>
                            </Box>
                        ),
                        actions: (
                            <>
                                <CopyIconButton value={command} label={label('copyCommand', 'Copy command')} />
                                <ChoiceToggle
                                    value={runtime}
                                    onChange={setRuntime}
                                    optionWidth={104}
                                    options={[
                                        { value: 'claude', label: 'Claude Code' },
                                        { value: 'codex', label: 'Codex' },
                                        { value: 'opencode', label: 'OpenCode' },
                                    ]}
                                />
                            </>
                        ),
                    },
                ]}
                activeTab="command"
                onTabChange={() => {}}
            />
        </Box>
    );
}
