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
    Groq,
    Together,
    Fireworks,
    Cerebras,
    Perplexity,
    Cohere,
    Nvidia,
    Novita,
    DeepInfra,
    Hyperbolic,
    ModelScope,
    SiliconFlow,
    Stepfun,
    Xiaomimimo,
    Baidu,
    Tencent,
    IflytekCloud,
    Baichuan,
    Yi,
    Doubao,
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
    // Core providers
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

    // Additional providers
    'groq': Groq,
    'together': Together,
    'fireworks': Fireworks,
    'cerebras': Cerebras,
    'perplexity': Perplexity,
    'cohere': Cohere,
    'nvidia': Nvidia,
    'novita': Novita,
    'deepinfra': DeepInfra,
    'hyperbolic': Hyperbolic,
    'modelscope': ModelScope,
    'siliconflow': SiliconFlow,
    'stepfun': Stepfun,
    'xiaomimimo': Xiaomimimo,
    'xiaomi': Xiaomimimo,  // Alias for xiaomimimo
    'baidu': Baidu,
    'tencent': Tencent,
    'iflytek': IflytekCloud,
    'baichuan': Baichuan,
    'yi': Yi,
    'doubao': Doubao,

    // Aliases (for backward compatibility and alternative names)
    'zai': Zhipu,
    'bigmodel': Zhipu,
    'z-ai': Zhipu,
    'dashscope': Qwen,
    'antigravity': Google,
    'googleapis': Gemini,
    'x-ai': XAI,
    'deepseek-com': DeepSeek,
    'mistral-ai': Mistral,
    'minimaxi-com': Minimax,
    'minimax-io': Minimax,
    'moonshot-ai': Kimi,
    'moonshot-cn': Kimi,
    'kimi-com-coding': Kimi,
    'volces-com': Doubao,
    'baidubce-com': Baidu,
    'xf-yun-com': IflytekCloud,
    'lingyiwanwu-com': Yi,
    'stepfun-com': Stepfun,
    'stepfun-ai': Stepfun,
    'siliconflow-cn': SiliconFlow,
    'siliconflow-com': SiliconFlow,
    'xiaomimimo-com': Xiaomimimo,
    'groq-com': Groq,
    'together-xyz': Together,
    'fireworks-ai': Fireworks,
    'cerebras-ai': Cerebras,
    'perplexity-ai': Perplexity,
    'cohere-com': Cohere,
    'nvidia-com': Nvidia,
    'novita-ai': Novita,
    'deepinfra-com': DeepInfra,
    'hyperbolic-xyz': Hyperbolic,
    'openrouter-ai': OpenRouter,
    'opencode-ai': OpenCode,
    'dashscope-cn': Qwen,
    'dashscope-intl': Qwen,
    'dashscope-cn-coding': Qwen,
    'dashscope-intl-coding': Qwen,
    'qwen-code': Qwen,
    'modelscope-cn': ModelScope,
    'z-ai-coding': Zhipu,
    'bigmodel-cn': Zhipu,
    'bigmodel-cn-coding': Zhipu,
    'volces-com-coding': Doubao,
    'baidubce-com-coding': Baidu,
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
