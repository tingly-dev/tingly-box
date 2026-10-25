import React, { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Box, Typography } from '@mui/material';
import ConfigRow from '@/components/ConfigRow';
import { ChoiceToggle } from '@/components/ChoiceToggle';
import { CopyIconButton } from '@/components/CopyIconButton';
import { getApiBaseUrl } from '@/utils/protocol';
import { copyableTextStyle } from '@/styles/textStyles';

export type AgentRuntime = 'claude' | 'codex' | 'opencode';

const TOKEN_CMD = "cat ~/.tingly-box/config.json | jq -r '.user_token'";

// The token is read from local config at run time, so the commands only need to
// be right about where the gateway is bound; the endpoint is built from the
// resolved API base URL rather than hardcoded.
export const registerCommands = (baseUrl: string, profileId: string, serverName: string): Record<AgentRuntime, string> => {
    const endpoint = `${baseUrl}/api/v1/mcp/${encodeURIComponent(profileId)}`;
    return {
        claude: `claude mcp add --transport http ${serverName} "${endpoint}" --header "Authorization: Bearer $(${TOKEN_CMD})"`,
        codex: `codex mcp add ${serverName} --url "${endpoint}" --bearer-token-env-var TINGLY_MCP_TOKEN`,
        opencode: `"mcp": {\n  "${serverName}": {\n    "type": "remote",\n    "url": "${endpoint}",\n    "oauth": false,\n    "headers": {\n      "Authorization": "Bearer {env:MY_API_KEY}"\n    }\n  }\n}`,
    };
};

// Before getApiBaseUrl() resolves: the gateway's documented default port.
const DEFAULT_BASE_URL = 'http://localhost:12580';

// How to register a profile with a coding client, in the shape of the Claude Code
// page's Quick Start: one ConfigRow with the command and copy, and the client
// choice as a toggle next to it (like npx | global there).
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
    const [runtime, setRuntime] = useState<AgentRuntime>(defaultRuntime);
    const [baseUrl, setBaseUrl] = useState(DEFAULT_BASE_URL);
    useEffect(() => {
        let mounted = true;
        void getApiBaseUrl().then((url) => mounted && setBaseUrl(url));
        return () => {
            mounted = false;
        };
    }, []);
    const command = registerCommands(baseUrl, profileId, serverName)[runtime];
    // One line in the row; copy always takes the full text (OpenCode's is a JSON block).
    const preview = command.replace(/\s*\n\s*/g, ' ');
    return (
        <Box>
            <ConfigRow
                tabs={[
                    {
                        key: 'connect',
                        label: t('mcp.center.installHeading'),
                        content: (
                            <Typography variant="subtitle2" sx={copyableTextStyle} title={command}>
                                {preview}
                            </Typography>
                        ),
                        actions: (
                            <>
                                <CopyIconButton value={command} label={t('mcp.workspace.copyCommand', { defaultValue: 'Copy command' })} />
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
                activeTab="connect"
                onTabChange={() => {}}
            />
            <Typography variant="caption" color="text.secondary" component="p" sx={{ mt: 0.5, pl: { sm: '192px' } }}>
                {runtime === 'claude' && t('mcp.center.localTokenHint')}
                {runtime === 'codex' && t('mcp.center.codexSetup')}
                {runtime === 'opencode' && (
                    <>
                        {t('mcp.center.openCodeSetup')} <code>{TOKEN_CMD}</code>
                        {' · '}
                        {t('mcp.workspace.openCodeFile', { defaultValue: 'Add the copied block to ~/.config/opencode/opencode.json.' })}
                    </>
                )}
            </Typography>
        </Box>
    );
}
