import { Box, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import type { MCPCatalogTool } from './types';

export default function MCPToolParameters({ tool, expanded = false }: { tool: MCPCatalogTool; expanded?: boolean }) {
    const { t } = useTranslation();
    return (
        <Box component="details" open={expanded}>
            <Typography component="summary" variant="caption" sx={{ cursor: 'pointer', color: 'text.secondary' }}>
                {t('mcp.center.schema', { defaultValue: 'Parameters and output schema' })}
            </Typography>
            <Box component="pre" sx={{ m: 0, mt: 1, whiteSpace: 'pre-wrap', overflowWrap: 'anywhere', fontSize: 12 }}>
                {JSON.stringify(
                    { input: tool.input_schema, output: tool.output_schema, annotations: tool.annotations },
                    null,
                    2
                )}
            </Box>
        </Box>
    );
}
