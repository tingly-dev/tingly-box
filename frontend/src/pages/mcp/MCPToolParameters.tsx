import { Box, Stack, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import CopyIconButton from '@/components/CopyIconButton';
import type { MCPCatalogTool } from './types';

const isEmpty = (schema: unknown) =>
    schema == null || (typeof schema === 'object' && Object.keys(schema as object).length === 0);

function SchemaBlock({ title, schema }: { title: string; schema: unknown }) {
    const json = JSON.stringify(schema, null, 2);
    return (
        <Box sx={{ minWidth: 0 }}>
            <Typography variant="caption" color="text.secondary">
                {title}
            </Typography>
            <Box
                sx={{
                    position: 'relative',
                    mt: 0.5,
                    border: '1px solid',
                    borderColor: 'divider',
                    borderRadius: 1,
                    bgcolor: 'background.paper',
                }}
            >
                <Box
                    component="pre"
                    sx={{
                        m: 0,
                        p: 1.25,
                        pr: 5,
                        height: 200,
                        overflow: 'auto',
                        whiteSpace: 'pre-wrap',
                        overflowWrap: 'anywhere',
                        fontSize: 12,
                        fontFamily: 'monospace',
                    }}
                >
                    {json}
                </Box>
                <CopyIconButton value={json} label={`Copy ${title}`} sx={{ position: 'absolute', top: 4, right: 4 }} />
            </Box>
        </Box>
    );
}

// The tool's schemas as plain JSON, each copyable as-is.
export default function MCPToolParameters({ tool }: { tool: MCPCatalogTool }) {
    const { t } = useTranslation();
    return (
        <Stack spacing={1.5}>
            <SchemaBlock
                title={t('mcp.center.inputSchema', { defaultValue: 'Parameters' })}
                schema={isEmpty(tool.input_schema) ? {} : tool.input_schema}
            />
            {!isEmpty(tool.output_schema) && (
                <SchemaBlock
                    title={t('mcp.center.outputSchema', { defaultValue: 'Output schema' })}
                    schema={tool.output_schema}
                />
            )}
        </Stack>
    );
}
