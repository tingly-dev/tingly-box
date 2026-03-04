import type { SxProps, Theme } from '@mui/material';
import { Box } from '@mui/material';

// Import SVG files as URLs
import AnthropicSvg from '@lobehub/icons-static-svg/icons/anthropic.svg?url';
import ClaudeSvg from '@lobehub/icons-static-svg/icons/claude.svg?url';
import ClaudeCodeSvg from '@lobehub/icons-static-svg/icons/claudecode.svg?url';
import GeminiSvg from '@lobehub/icons-static-svg/icons/gemini.svg?url';
import GoogleSvg from '@lobehub/icons-static-svg/icons/google.svg?url';
import OpenAISvg from '@lobehub/icons-static-svg/icons/openai.svg?url';
import QwenSvg from '@lobehub/icons-static-svg/icons/qwen.svg?url';

interface BrandIconProps {
    size?: number;
    sx?: SxProps<Theme>;
    style?: React.CSSProperties;
}

// Box 作为容器控制大小，img 填充整个 Box
const createBrandIcon = (src: string, alt: string) => {
    const Icon = ({ size = 24, sx, style }: BrandIconProps) => (
        <Box
            sx={{
                width: size,
                height: size,
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                flexShrink: 0,
                ...sx,
            }}
            style={style}
        >
            <Box
                component="img"
                src={src}
                alt={alt}
                sx={{
                    width: '100%',
                    height: '100%',
                    objectFit: 'contain',
                }}
            />
        </Box>
    );
    return Icon;
};

export const OpenAI = createBrandIcon(OpenAISvg, 'OpenAI');
export const Anthropic = createBrandIcon(AnthropicSvg, 'Anthropic');
export const Claude = createBrandIcon(ClaudeSvg, 'Claude');
export const ClaudeCode = createBrandIcon(ClaudeCodeSvg, 'Claude Code');
export const Gemini = createBrandIcon(GeminiSvg, 'Gemini');
export const Google = createBrandIcon(GoogleSvg, 'Google');
export const Qwen = createBrandIcon(QwenSvg, 'Qwen');
