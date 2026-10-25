import { Box, Button, FormControlLabel, Stack, Switch, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import MCPToolParameters from './MCPToolParameters';
import { isAdvisorTool, isToolRestricted, type MCPToolPatch } from './toolPresentation';
import type { MCPCatalogTool, MCPSourceConfig } from './types';

// One tool and its global switch. Off here means off in every profile.
export default function MCPToolCard({
    tool,
    source,
    enabled,
    busy,
    onChange,
    onTest,
    inWorkspace = false,
}: {
    tool: MCPCatalogTool;
    source?: MCPSourceConfig;
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
    return (
        <Box
            component={inWorkspace ? 'div' : 'article'}
            role={inWorkspace ? 'group' : undefined}
            aria-label={inWorkspace ? tool.name : `${source?.name || tool.source_id} / ${tool.name}`}
            sx={{ p: 1.75, border: '1px solid', borderColor: 'divider', borderRadius: 1.5 }}
        >
            <Stack spacing={1}>
                <Stack direction="row" sx={{ alignItems: 'center', justifyContent: 'space-between', gap: 1 }}>
                    <Typography variant="subtitle2" sx={{ fontWeight: 600 }}>
                        {tool.name}
                    </Typography>
                    <Stack direction="row" sx={{ alignItems: 'center', gap: 1 }}>
                        {onTest && (
                            <Button
                                size="small"
                                disabled={busy || !enabled || !tool.enabled || restricted || advisor}
                                onClick={onTest}
                            >
                                {label('testTool', 'Test tool')}
                            </Button>
                        )}
                        <FormControlLabel
                            sx={{ m: 0 }}
                            control={
                                <Switch
                                    size="small"
                                    disabled={busy || restricted}
                                    checked={tool.enabled}
                                    onChange={(e) => onChange({ enabled: e.target.checked })}
                                />
                            }
                            label={label('toolEnabled', 'On')}
                        />
                    </Stack>
                </Stack>
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
                <MCPToolParameters tool={tool} />
            </Stack>
        </Box>
    );
}
