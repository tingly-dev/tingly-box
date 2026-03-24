import type {SxProps, Theme} from '@mui/material';
import {Box} from '@mui/material';

// Import SVG files as URLs
import AnthropicSvg from '@lobehub/icons-static-svg/icons/anthropic.svg?url';
import ClaudeSvg from '@lobehub/icons-static-svg/icons/claude.svg?url';
import ClaudeCodeSvg from '@lobehub/icons-static-svg/icons/claudecode.svg?url';
import GeminiSvg from '@lobehub/icons-static-svg/icons/gemini.svg?url';
import GoogleSvg from '@lobehub/icons-static-svg/icons/google.svg?url';
import OpenAISvg from '@lobehub/icons-static-svg/icons/openai.svg?url';
import QwenSvg from '@lobehub/icons-static-svg/icons/qwen.svg?url';

import DingTalkSvg from '@/assets/icons/dingtalk.svg?url';
import DiscordSvg from '@/assets/icons/discord.svg?url';
import FeishuSvg from '@/assets/icons/feishu.svg?url';
import LarkSvg from '@/assets/icons/feishu.svg?url';
import QQSvg from '@/assets/icons/qq.svg?url';
import SlackSvg from '@/assets/icons/slack.svg?url';
import TelegramSvg from '@/assets/icons/telegram.svg?url';
import WeComSvg from '@/assets/icons/wecom.svg?url';
import WeixinSvg from '@/assets/icons/weixin.svg?url';

interface BrandIconProps {
    size?: number;
    sx?: SxProps<Theme>;
    style?: React.CSSProperties;
    grayscale?: boolean;
}

// Box 作为容器控制大小，img 填充整个 Box
const createBrandIcon = (src: string, alt: string, defaultGrayscale = false) => {
    return ({size = 24, sx, style, grayscale = defaultGrayscale}: BrandIconProps) => (
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
                    display: 'block',
                    width: '100%',
                    height: '100%',
                    objectFit: 'contain',
                    filter: grayscale ? 'grayscale(100%)' : 'none',
                    transition: 'filter 0.2s',
                }}
            />
        </Box>
    );
};

export const OpenAI = createBrandIcon(OpenAISvg, 'OpenAI');
export const Anthropic = createBrandIcon(AnthropicSvg, 'Anthropic');
export const Claude = createBrandIcon(ClaudeSvg, 'Claude');
export const ClaudeCode = createBrandIcon(ClaudeCodeSvg, 'Claude Code');
export const Gemini = createBrandIcon(GeminiSvg, 'Gemini');
export const Google = createBrandIcon(GoogleSvg, 'Google');
export const Qwen = createBrandIcon(QwenSvg, 'Qwen');

export const Telegram = createBrandIcon(TelegramSvg, 'Telegram', true);
export const Feishu = createBrandIcon(FeishuSvg, 'Feishu', true);
export const Lark = createBrandIcon(LarkSvg, 'Lark', true);
export const DingTalk = createBrandIcon(DingTalkSvg, 'DingTalk', true);
export const Weixin = createBrandIcon(WeixinSvg, 'Weixin', true);
export const WeCom = createBrandIcon(WeComSvg, 'WeCom', true);
export const QQ = createBrandIcon(QQSvg, 'QQ', true);
export const Discord = createBrandIcon(DiscordSvg, 'Discord', true);
export const Slack = createBrandIcon(SlackSvg, 'Slack', true);
