import { Box, Button, Checkbox, FormControlLabel, Stack, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import MCPToolParameters from './MCPToolParameters';
import { isAdvisorTool, isToolRestricted, type MCPToolMode, type MCPToolPatch } from './toolPresentation';
import type { MCPCatalogTool, MCPSourceConfig } from './types';

export default function MCPToolCard({
    tool,
    source,
    mode,
    enabled,
    busy,
    onChange,
    onTest,
    inWorkspace = false,
}: {
    tool: MCPCatalogTool;
    source?: MCPSourceConfig;
    mode: MCPToolMode;
    enabled: boolean;
    busy: boolean;
    onChange: (patch: MCPToolPatch) => void;
    onTest?: () => void;
    inWorkspace?: boolean;
}) {
    const { t } = useTranslation();
    const label = (key: string, fallback: string) => t(`mcp.workspace.${key}`, { defaultValue: fallback });
    const advisor = isAdvisorTool(tool, source);
    const restricted = isToolRestricted(tool, source);
    const asset = mode === 'asset';
    return (
        <Box
            component={inWorkspace ? 'div' : 'article'}
            role={inWorkspace ? 'group' : undefined}
            aria-label={inWorkspace ? tool.name : `${source?.name || tool.source_id} / ${tool.name}`}
            sx={{ p: 1.75, border: '1px solid', borderColor: 'divider', borderRadius: 1.5 }}
        >
            <Stack spacing={1}>
                <Typography variant="subtitle2" sx={{ fontWeight: 600 }}>
                    {tool.name}
                </Typography>
                {tool.description && (
                    <Typography variant="body2" color="text.secondary">
                        {tool.description}
                    </Typography>
                )}
                {restricted && (
                    <Typography variant="caption" color="warning.main">
                        {source?.enabled === false
                            ? label('connectionOff', 'This connection is off. Enable it to discover and use its tools.')
                            : label(
                                  'restrictedTool',
                                  'Excluded by the connection allow list. Adjust it in connection settings.'
                              )}
                    </Typography>
                )}
                {!asset && !tool.enabled && !restricted && (
                    <Typography variant="caption" color="warning.main">
                        {label(
                            'sharedToolOff',
                            'This tool is disabled in the shared connection. Enable it from the Tool page.'
                        )}
                    </Typography>
                )}
                <Stack direction="row" sx={{ alignItems: 'center', flexWrap: 'wrap', gap: 1 }}>
                    <FormControlLabel
                        control={
                            <Checkbox
                                disabled={busy || restricted || (!asset && mode === 'client' && advisor)}
                                checked={
                                    asset
                                        ? tool.enabled
                                        : mode === 'client'
                                          ? tool.usage.client && !advisor
                                          : tool.usage.gateway
                                }
                                onChange={(e) =>
                                    onChange(
                                        asset
                                            ? { enabled: e.target.checked }
                                            : { usage: { ...tool.usage, [mode]: e.target.checked } }
                                    )
                                }
                            />
                        }
                        label={
                            asset
                                ? label('toolEnabled', 'Enabled')
                                : mode === 'client'
                                  ? label('publishUsage', 'Expose through MCP')
                                  : label('serverTools', 'Server Tools')
                        }
                    />
                    {asset && onTest && (
                        <Button
                            size="small"
                            disabled={busy || !enabled || !tool.enabled || restricted || advisor}
                            onClick={onTest}
                        >
                            {label('testTool', 'Test tool')}
                        </Button>
                    )}
                </Stack>
                <MCPToolParameters tool={tool} />
            </Stack>
        </Box>
    );
}
