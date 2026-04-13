import React from 'react';
import {
    OpenAI,
    Anthropic,
    Qwen,
    DeepSeek,
    Minimax,
    Zhipu,
    XAI,
    Mistral,
    Gemini,
    Kimi,
    OpenRouter,
    ClaudeCode,
    OpenCode,
    Google,
} from './BrandIcons';
import type {SxProps, Theme} from '@mui/material';
import {Box} from '@mui/material';

interface BrandIconProps {
    size?: number;
    sx?: SxProps<Theme>;
    style?: React.CSSProperties;
}

// Icon mapping table: maps icon identifier from provider templates to React components
const iconMap: Record<string, React.FC<BrandIconProps>> = {
    'openai': OpenAI,
    'anthropic': Anthropic,
    'qwen': Qwen,
    'deepseek': DeepSeek,
    'minimax': Minimax,
    'zhipu': Zhipu,
    'xai': XAI,
    'mistral': Mistral,
    'gemini': Gemini,
    'google': Google,
    'kimi': Kimi,
    'openrouter': OpenRouter,
    'claudecode': ClaudeCode,
    'opencode': OpenCode,
    // Aliases
    'zai': Zhipu,
    'bigmodel': Zhipu,
};

// Fallback icon component for unknown providers
const FallbackIcon: React.FC<BrandIconProps> = ({size = 20, sx, style}) => {
    return (
        <Box
            sx={{
                width: size,
                height: size,
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                flexShrink: 0,
                bgcolor: 'action.hover',
                borderRadius: 1,
                ...sx,
            }}
            style={style}
        >
            <Box
                sx={{
                    width: size * 0.5,
                    height: size * 0.5,
                    borderRadius: '50%',
                    bgcolor: 'text.disabled',
                }}
            />
        </Box>
    );
};

// Provider icon component that looks up the icon from the mapping table
const ProviderIcon = ({identifier, size = 20, sx, style}: {identifier: string} & BrandIconProps) => {
    const iconKey = identifier.toLowerCase();
    const IconComponent = iconMap[iconKey];

    if (!IconComponent) {
        return <FallbackIcon size={size} sx={sx} style={style} />;
    }

    return <IconComponent size={size} sx={sx} style={style} />;
};

export default ProviderIcon;
