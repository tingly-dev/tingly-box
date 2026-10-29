import { useTranslation } from 'react-i18next';
import MCPToolRow from './MCPToolRow';
import { isAdvisorTool, isServerToolSource, needsSearchKey, isToolRestricted, type MCPToolPatch } from './toolPresentation';
import type { MCPCatalogTool, MCPSourceConfig } from './types';

// One tool of a connection with its global switch. Off here means off in every profile.
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
    const serverTool = isServerToolSource(source);
    const missingSearchKey = tool.name === 'mcp_web_search' && needsSearchKey(source);
    const restricted = isToolRestricted(tool, source);
    const note = missingSearchKey
        ? label('missing_searchKey', 'Search needs a Serper API key; Fetch does not.')
        : restricted
          ? source?.enabled === false
              ? label('connectionOff', 'This connection is off. Enable it to discover and use its tools.')
              : label('restrictedTool', 'Excluded by the connection allow list. Adjust it in connection settings.')
          : '';
    return (
        <MCPToolRow
            name={tool.name}
            tool={tool}
            checked={tool.enabled}
            switchDisabled={busy || restricted}
            muted={!tool.enabled || restricted}
            note={note}
            groupRole={inWorkspace}
            ariaLabel={inWorkspace ? tool.name : `${source?.name || tool.source_id} / ${tool.name}`}
            onToggle={(on) => onChange({ enabled: on })}
            onTest={onTest}
            testDisabled={busy || (!enabled && !serverTool) || !tool.enabled || restricted || advisor || missingSearchKey}
        />
    );
}
