import type {SxProps, Theme} from '@mui/material';
import {Box} from '@mui/material';

// Import SVG files as URLs
import AnthropicSvg from '@lobehub/icons-static-svg/icons/anthropic.svg?url';
import ClaudeSvg from '@lobehub/icons-static-svg/icons/claude.svg?url';
import ClaudeCodeSvg from '@lobehub/icons-static-svg/icons/claudecode.svg?url';
import CodexSvg from '@lobehub/icons-static-svg/icons/codex.svg?url';
import GeminiSvg from '@lobehub/icons-static-svg/icons/gemini.svg?url';
import GoogleSvg from '@lobehub/icons-static-svg/icons/google.svg?url';
import KimiSvg from '@lobehub/icons-static-svg/icons/kimi.svg?url';
import OpenClawSvg from '@lobehub/icons-static-svg/icons/openclaw.svg?url';
import OpenAISvg from '@lobehub/icons-static-svg/icons/openai.svg?url';
import OpenCodeSvg from '@lobehub/icons-static-svg/icons/opencode.svg?url';
import QwenSvg from '@lobehub/icons-static-svg/icons/qwen.svg?url';
import DeepSeekSvg from '@lobehub/icons-static-svg/icons/deepseek.svg?url';
import MinimaxSvg from '@lobehub/icons-static-svg/icons/minimax.svg?url';
import ZhipuSvg from '@lobehub/icons-static-svg/icons/zhipu.svg?url';
import XAISvg from '@lobehub/icons-static-svg/icons/xai.svg?url';
import MistralSvg from '@lobehub/icons-static-svg/icons/mistral.svg?url';
import OpenRouterSvg from '@lobehub/icons-static-svg/icons/openrouter.svg?url';
import GroqSvg from '@lobehub/icons-static-svg/icons/groq.svg?url';
import TogetherSvg from '@lobehub/icons-static-svg/icons/together-color.svg?url';
import FireworksSvg from '@lobehub/icons-static-svg/icons/fireworks.svg?url';
import CerebrasSvg from '@lobehub/icons-static-svg/icons/cerebras.svg?url';
import PerplexitySvg from '@lobehub/icons-static-svg/icons/perplexity.svg?url';
import CohereSvg from '@lobehub/icons-static-svg/icons/cohere.svg?url';
import NvidiaSvg from '@lobehub/icons-static-svg/icons/nvidia.svg?url';
import NovitaSvg from '@lobehub/icons-static-svg/icons/novita.svg?url';
import DeepInfraSvg from '@lobehub/icons-static-svg/icons/deepinfra.svg?url';
import HyperbolicSvg from '@lobehub/icons-static-svg/icons/hyperbolic.svg?url';
import ModelScopeSvg from '@lobehub/icons-static-svg/icons/modelscope.svg?url';
import SiliconFlowSvg from '@lobehub/icons-static-svg/icons/siliconcloud.svg?url';
import StepfunSvg from '@lobehub/icons-static-svg/icons/stepfun.svg?url';
import XiaomimimoSvg from '@lobehub/icons-static-svg/icons/xiaomimimo.svg?url';
import BaiduSvg from '@lobehub/icons-static-svg/icons/baidu-color.svg?url';
import TencentSvg from '@lobehub/icons-static-svg/icons/tencent.svg?url';
import IflytekCloudSvg from '@lobehub/icons-static-svg/icons/iflytekcloud.svg?url';
import BaichuanSvg from '@lobehub/icons-static-svg/icons/baichuan.svg?url';
import YiSvg from '@lobehub/icons-static-svg/icons/yi-color.svg?url';
import DoubaoSvg from '@lobehub/icons-static-svg/icons/doubao.svg?url';

import DingTalkSvg from '@/assets/icons/dingtalk.svg?url';
import DiscordSvg from '@/assets/icons/discord.svg?url';
import FeishuSvg from '@/assets/icons/feishu.svg?url';
import LarkSvg from '@/assets/icons/feishu.svg?url';
import QQSvg from '@/assets/icons/qq.svg?url';
import SlackSvg from '@/assets/icons/slack.svg?url';
import TelegramSvg from '@/assets/icons/telegram.svg?url';
import WeComSvg from '@/assets/icons/wecom.svg?url';
import WeixinSvg from '@/assets/icons/weixin.svg?url';
import XcodeSvg from '@/assets/icons/xcode.svg?url';
import VSCodeSvg from '@/assets/icons/vscode.svg?url';

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
                    filter: grayscale ? 'grayscale(100%) brightness(1.15) contrast(1.1)' : 'none',
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
export const Codex = createBrandIcon(CodexSvg, 'Codex');
export const Gemini = createBrandIcon(GeminiSvg, 'Gemini');
export const Google = createBrandIcon(GoogleSvg, 'Google');
export const Kimi = createBrandIcon(KimiSvg, 'Kimi');
export const OpenClaw = createBrandIcon(OpenClawSvg, 'OpenClaw');
export const Qwen = createBrandIcon(QwenSvg, 'Qwen');
export const OpenCode = createBrandIcon(OpenCodeSvg, 'OpenCode');
export const DeepSeek = createBrandIcon(DeepSeekSvg, 'DeepSeek');
export const Minimax = createBrandIcon(MinimaxSvg, 'Minimax');
export const Zhipu = createBrandIcon(ZhipuSvg, 'Zhipu');
export const XAI = createBrandIcon(XAISvg, 'xAI');
export const Mistral = createBrandIcon(MistralSvg, 'Mistral');
export const OpenRouter = createBrandIcon(OpenRouterSvg, 'OpenRouter');
export const Xcode = createBrandIcon(XcodeSvg, 'Xcode', true);
export const VSCode = createBrandIcon(VSCodeSvg, 'VS Code', true);

export const Telegram = createBrandIcon(TelegramSvg, 'Telegram', true);
export const Feishu = createBrandIcon(FeishuSvg, 'Feishu', true);
export const Lark = createBrandIcon(LarkSvg, 'Lark', true);
export const DingTalk = createBrandIcon(DingTalkSvg, 'DingTalk', true);
export const Weixin = createBrandIcon(WeixinSvg, 'Weixin', true);
export const WeCom = createBrandIcon(WeComSvg, 'WeCom', true);
export const QQ = createBrandIcon(QQSvg, 'QQ', true);
export const Discord = createBrandIcon(DiscordSvg, 'Discord', true);
export const Slack = createBrandIcon(SlackSvg, 'Slack', true);

// New provider icons
export const Groq = createBrandIcon(GroqSvg, 'Groq');
export const Together = createBrandIcon(TogetherSvg, 'Together');
export const Fireworks = createBrandIcon(FireworksSvg, 'Fireworks');
export const Cerebras = createBrandIcon(CerebrasSvg, 'Cerebras');
export const Perplexity = createBrandIcon(PerplexitySvg, 'Perplexity');
export const Cohere = createBrandIcon(CohereSvg, 'Cohere');
export const Nvidia = createBrandIcon(NvidiaSvg, 'NVIDIA');
export const Novita = createBrandIcon(NovitaSvg, 'Novita');
export const DeepInfra = createBrandIcon(DeepInfraSvg, 'DeepInfra');
export const Hyperbolic = createBrandIcon(HyperbolicSvg, 'Hyperbolic');
export const ModelScope = createBrandIcon(ModelScopeSvg, 'ModelScope');
export const SiliconFlow = createBrandIcon(SiliconFlowSvg, 'SiliconFlow');
export const Stepfun = createBrandIcon(StepfunSvg, 'Stepfun');
export const Xiaomimimo = createBrandIcon(XiaomimimoSvg, 'Xiaomi Mimo');
export const Baidu = createBrandIcon(BaiduSvg, 'Baidu');
export const Tencent = createBrandIcon(TencentSvg, 'Tencent');
export const IflytekCloud = createBrandIcon(IflytekCloudSvg, 'iFlytek');
export const Baichuan = createBrandIcon(BaichuanSvg, 'Baichuan');
export const Yi = createBrandIcon(YiSvg, 'Lingyi Wanwu');
export const Doubao = createBrandIcon(DoubaoSvg, 'Doubao');
