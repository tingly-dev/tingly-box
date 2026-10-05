import { PageLayout } from '@/components/PageLayout';
import UnifiedCard from '@/components/UnifiedCard';
import {
    Alert,
    Box,
    Chip,
    Divider,
    Paper,
    Stack,
    Typography,
} from '@mui/material';
import {
    CheckCircle as CheckCircleIcon,
    Terminal as TerminalIcon,
} from '@/components/icons';
import { CopyIconButton } from '@/components/CopyIconButton';
import { useEffect, useState } from 'react';
import { useNotify } from '@/hooks/useNotify';
import { getApiBaseUrl } from '@/utils/protocol';
import { fontMono } from '@/theme/fonts';

const MCPLocalMode = () => {
    const notify = useNotify();
    const [baseUrl, setBaseUrl] = useState('');

    useEffect(() => {
        let isMounted = true;
        // getApiBaseUrl() (not window.location.origin) so this also resolves
        // correctly in GUI/Wails mode, where the frontend's own origin does
        // not necessarily match the backend's bound port.
        getApiBaseUrl().then(url => {
            if (isMounted) setBaseUrl(url);
        });
        return () => {
            isMounted = false;
        };
    }, []);

    const getEndpointUrl = () => {
        return `${baseUrl}/api/v1/mcp/tb`;
    };

    const getClaudeCodeCommand = () => {
        return `claude mcp add --transport http tb "${getEndpointUrl()}" --header "Authorization: Bearer \$(cat ~/.tingly-box/config.json | jq -r '.user_token')"`;
    };

    const getClaudeDesktopConfig = () => {
        return JSON.stringify({
            mcpServers: {
                tb: {
                    url: getEndpointUrl(),
                    headers: {
                        Authorization: 'Bearer YOUR_TOKEN'
                    }
                }
            }
        }, null, 2);
    };

    const notifyCopied = () => notify.success('Copied to clipboard');

    return (
        <PageLayout loading={false}>
            <Stack spacing={3}>
                <UnifiedCard title="MCP Local Mode" titleHeadingLevel={1} size="full">
                    <Stack spacing={2}>
                        <Stack direction="row" spacing={1} sx={{
                            alignItems: "center"
                        }}>
                            <Chip
                                icon={<CheckCircleIcon />}
                                label="Active"
                                color="success"
                                size="small"
                            />
                            <Typography variant="body1">
                                External MCP clients can connect to Tingly-Box and use its tools.
                            </Typography>
                        </Stack>
                        <Alert severity="info">
                            Tingly-Box is running in Client Tool mode. Register MCP sources in the Sources page, then connect your MCP client using the instructions below.
                        </Alert>
                    </Stack>
                </UnifiedCard>

                <UnifiedCard title="Connection Information" size="full">
                    <Stack spacing={3}>
                        <Box>
                            <Typography variant="subtitle2" gutterBottom sx={{
                                color: "text.secondary"
                            }}>
                                MCP Endpoint URL
                            </Typography>
                            <Paper
                                variant="outlined"
                                sx={{
                                    p: 2,
                                    bgcolor: 'background.paper',
                                    fontFamily: fontMono,
                                    fontSize: '0.875rem',
                                    wordBreak: 'break-all',
                                }}
                            >
                                {getEndpointUrl()}
                            </Paper>
                        </Box>
                    </Stack>
                </UnifiedCard>

                <UnifiedCard
                    title="Connect Claude Code"
                    size="full"
                >
                    <Stack spacing={3}>
                        <Typography variant="body1">
                            Configure Claude Code to use Tingly-Box as an MCP server.
                        </Typography>

                        <Divider />

                        <Box>
                            <Typography variant="subtitle2" gutterBottom sx={{
                                fontWeight: 600
                            }}>
                                Method 1: Using Claude CLI
                            </Typography>
                            <Typography
                                variant="body2"
                                sx={{
                                    color: "text.secondary",
                                    mb: 1
                                }}>
                                Run this command in your terminal:
                            </Typography>
                            <Paper
                                variant="outlined"
                                sx={{
                                    p: 2,
                                    bgcolor: 'background.paper',
                                    border: '1px solid',
                                    borderColor: 'divider',
                                    position: 'relative',
                                }}
                            >
                                <Typography
                                    component="pre"
                                    sx={{
                                        fontFamily: fontMono,
                                        fontSize: '0.875rem',
                                        margin: 0,
                                        pr: 4,
                                        whiteSpace: 'pre-wrap',
                                        wordBreak: 'break-word',
                                    }}
                                >
                                    {getClaudeCodeCommand()}
                                </Typography>
                                <CopyIconButton
                                    value={getClaudeCodeCommand()}
                                    aria-label="Copy Claude Code command"
                                    sx={{ position: 'absolute', right: 8, top: 8 }}
                                    onCopied={notifyCopied}
                                />
                            </Paper>
                        </Box>

                        <Divider />

                        <Box>
                            <Typography variant="subtitle2" gutterBottom sx={{
                                fontWeight: 600
                            }}>
                                Method 2: Manual Config File
                            </Typography>
                            <Typography
                                variant="body2"
                                sx={{
                                    color: "text.secondary",
                                    mb: 1
                                }}>
                                Add to your Claude Desktop config file:
                            </Typography>
                            <Paper
                                variant="outlined"
                                sx={{
                                    p: 2,
                                    bgcolor: 'background.paper',
                                    border: '1px solid',
                                    borderColor: 'divider',
                                    position: 'relative',
                                }}
                            >
                                <Typography
                                    component="pre"
                                    sx={{
                                        fontFamily: fontMono,
                                        fontSize: '0.875rem',
                                        margin: 0,
                                        pr: 4,
                                        whiteSpace: 'pre-wrap',
                                    }}
                                >
                                    {getClaudeDesktopConfig()}
                                </Typography>
                                <CopyIconButton
                                    value={getClaudeDesktopConfig()}
                                    aria-label="Copy Claude Desktop configuration"
                                    sx={{ position: 'absolute', right: 8, top: 8 }}
                                    onCopied={notifyCopied}
                                />
                            </Paper>
                            <Typography
                                variant="caption"
                                sx={{
                                    color: "text.secondary",
                                    mt: 0.5,
                                    display: 'block'
                                }}>
                                Config location: ~/Library/Application Support/Claude/claude_desktop_config.json (macOS)
                            </Typography>
                        </Box>

                        <Alert severity="success" icon={<TerminalIcon />}>
                            <Typography variant="body2">
                                After configuration, restart Claude Code. You should see Tingly-Box tools available in your conversations.
                            </Typography>
                        </Alert>
                    </Stack>
                </UnifiedCard>
            </Stack>
        </PageLayout>
    );
};

export default MCPLocalMode;
