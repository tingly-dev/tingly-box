import { Visibility as VisibilityIcon, ContentCopy as CopyIcon } from '@/components/icons';
import { IconButton, Tooltip, Typography } from '@mui/material';
import React, { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ConfigRow } from './ConfigRow';
import { EnvironmentModeSwitcher, type EnvironmentMode } from './EnvironmentModeSwitcher';
import { useScenarioPageModal } from '@/pages/scenario/context/ScenarioPageContext';
import { copyableTextStyle } from '@/styles/textStyles';
import { maskToken, transformUrlByMode } from '@/utils/tokenUtils';

// ============================================================================
// Types
// ============================================================================

export type ConfigTabType = 'baseUrl' | 'apiKey';

interface CompactConfigCardProps {
    /** Base URL path (e.g., "/tingly/anthropic") */
    baseUrlPath: string;
    /** Full base URL from getBaseUrl() */
    baseUrl: string;
    /** Copy handler */
    onCopy: (text: string, label: string) => Promise<void>;
    /** Optional: custom label for base URL */
    baseUrlLabel?: string;
    /** Optional: custom label for API key */
    apiKeyLabel?: string;
    /** Optional: card title */
    title?: string;
    /** Optional: available environment modes (defaults to local + docker) */
    environmentModes?: EnvironmentMode[];
    /** Controlled environment mode (a page that also shows the URL elsewhere); omit to keep it local. */
    envMode?: EnvironmentMode;
    onEnvModeChange?: (mode: EnvironmentMode) => void;
    /** Show the API Key row (default true). Off where clients bring their own key, e.g. Team (Team Keys). */
    showApiKeyRow?: boolean;
}

// ============================================================================
// Component
// ============================================================================

/**
 * Compact configuration card: a Base URL row and an API Key row.
 * Uses ConfigRow component internally.
 *
 * Modal state (token, showTokenModal, setShowTokenModal) is obtained from
 * ScenarioPageModalProvider context.
 *
 * Layout:
 * ┌─────────────────────────────────────────────────────────────────┐
 * │ Base URL    http://localhost:8080/tingly/anthropic   [⧉][💻][🐳] │
 * │ API Key     tingly-••••••                            [⧉][👁]    │
 * └─────────────────────────────────────────────────────────────────┘
 *
 * Features:
 * - Click URL/token to copy (no separate copy button)
 * - Environment mode switcher (Local/Docker/etc) for URL transformation
 * - View token modal for API Key tab
 */
export const CompactConfigCard: React.FC<CompactConfigCardProps> = ({
    baseUrlPath,
    baseUrl,
    onCopy,
    baseUrlLabel = 'Base URL',
    apiKeyLabel = 'API Key',
    title,
    environmentModes,
    envMode: envModeProp,
    onEnvModeChange,
    showApiKeyRow = true,
}) => {
    const { t } = useTranslation();
    // Get modal state from context
    const { token, setShowTokenModal } = useScenarioPageModal();
    const [localEnvMode, setLocalEnvMode] = useState<EnvironmentMode>('local');
    const envMode = envModeProp ?? localEnvMode;
    const setEnvMode = onEnvModeChange ?? setLocalEnvMode;

    // Build full URL based on environment mode
    const fullUrl = React.useMemo(() => {
        const url = `${baseUrl}${baseUrlPath}`;
        return transformUrlByMode(url, envMode);
    }, [baseUrl, baseUrlPath, envMode]);

    // Define tabs
    const tabs = [
        {
            key: 'baseUrl',
            label: 'Base URL',
            content: (
                <Tooltip title={`Click to copy ${baseUrlLabel}`} arrow placement="top">
                    <Typography
                        variant="subtitle2"
                        onClick={() => onCopy(fullUrl, title || baseUrlLabel)}
                        sx={copyableTextStyle}
                    >
                        {fullUrl}
                    </Typography>
                </Tooltip>
            ),
            actions: (
                <>
                    <Tooltip title={`Copy ${baseUrlLabel}`} arrow>
                        <IconButton onClick={() => onCopy(fullUrl, title || baseUrlLabel)} size="small">
                            <CopyIcon fontSize="small" />
                        </IconButton>
                    </Tooltip>
                    <EnvironmentModeSwitcher
                        value={envMode}
                        onChange={setEnvMode}
                        modes={environmentModes}
                    />
                </>
            ),
        },
        {
            key: 'apiKey',
            label: 'API Key',
            content: (
                <Tooltip title={`Click to copy ${apiKeyLabel}`} arrow placement="top">
                    <Typography
                        variant="subtitle2"
                        onClick={() => onCopy(token, apiKeyLabel)}
                        sx={copyableTextStyle}
                    >
                        {maskToken(token)}
                    </Typography>
                </Tooltip>
            ),
            actions: (
                <>
                    <Tooltip title={`Copy ${apiKeyLabel}`} arrow>
                        <IconButton onClick={() => onCopy(token, apiKeyLabel)} size="small">
                            <CopyIcon fontSize="small" />
                        </IconButton>
                    </Tooltip>
                    <Tooltip title="View Full Token">
                        <IconButton onClick={() => setShowTokenModal(true)} size="small">
                            <VisibilityIcon fontSize="small" />
                        </IconButton>
                    </Tooltip>
                </>
            ),
        },
    ];

    // Both values the user copies into their tool are shown as rows of their
    // own — the key used to sit behind a "Base URL | API Key" tab switch.
    return (
        <>
            {tabs.filter((tab) => showApiKeyRow || tab.key !== 'apiKey').map((tab) => (
                <ConfigRow key={tab.key} tabs={[tab]} activeTab={tab.key} onTabChange={() => {}} />
            ))}
        </>
    );
};

export default CompactConfigCard;
