import type {SxProps, Theme} from '@mui/material';
import {Box, useTheme} from '@mui/material';

// Import SVG files as URLs
import AnthropicSvg from '@lobehub/icons-static-svg/icons/anthropic.svg?url';
import ClaudeSvg from '@lobehub/icons-static-svg/icons/claude-color.svg?url';
import ClaudeCodeSvg from '@lobehub/icons-static-svg/icons/claudecode-color.svg?url';
import CodexSvg from '@lobehub/icons-static-svg/icons/codex-color.svg?url';
import GeminiSvg from '@lobehub/icons-static-svg/icons/gemini-color.svg?url';
import GoogleSvg from '@lobehub/icons-static-svg/icons/google-color.svg?url';
import KimiSvg from '@lobehub/icons-static-svg/icons/kimi.svg?url';
import OpenAISvg from '@lobehub/icons-static-svg/icons/openai.svg?url';
import OpenCodeSvg from '@lobehub/icons-static-svg/icons/opencode.svg?url';
import PiSvg from '@lobehub/icons-static-svg/icons/pi.svg?url';
import QwenSvg from '@lobehub/icons-static-svg/icons/qwen-color.svg?url';
import DeepSeekSvg from '@lobehub/icons-static-svg/icons/deepseek-color.svg?url';
import MinimaxSvg from '@lobehub/icons-static-svg/icons/minimax-color.svg?url';
import ZhipuSvg from '@lobehub/icons-static-svg/icons/zhipu-color.svg?url';
import ZaiSvg from '@lobehub/icons-static-svg/icons/zai.svg?url';
import XAISvg from '@lobehub/icons-static-svg/icons/xai.svg?url';
import MistralSvg from '@lobehub/icons-static-svg/icons/mistral-color.svg?url';
import OpenRouterSvg from '@lobehub/icons-static-svg/icons/openrouter-color.svg?url';
import CursorSvg from '@lobehub/icons-static-svg/icons/cursor.svg?url';
import GroqSvg from '@lobehub/icons-static-svg/icons/groq.svg?url';
import TogetherSvg from '@lobehub/icons-static-svg/icons/together-color.svg?url';
import FireworksSvg from '@lobehub/icons-static-svg/icons/fireworks-color.svg?url';
import CerebrasSvg from '@lobehub/icons-static-svg/icons/cerebras.svg?url';
import PerplexitySvg from '@lobehub/icons-static-svg/icons/perplexity-color.svg?url';
import CohereSvg from '@lobehub/icons-static-svg/icons/cohere-color.svg?url';
import NvidiaSvg from '@lobehub/icons-static-svg/icons/nvidia-color.svg?url';
import NovitaSvg from '@lobehub/icons-static-svg/icons/novita-color.svg?url';
import DeepInfraSvg from '@lobehub/icons-static-svg/icons/deepinfra-color.svg?url';
import HyperbolicSvg from '@lobehub/icons-static-svg/icons/hyperbolic-color.svg?url';
import ModelScopeSvg from '@lobehub/icons-static-svg/icons/modelscope-color.svg?url';
import SiliconFlowSvg from '@lobehub/icons-static-svg/icons/siliconcloud-color.svg?url';
import StepfunSvg from '@lobehub/icons-static-svg/icons/stepfun-color.svg?url';
import XiaomimimoSvg from '@lobehub/icons-static-svg/icons/xiaomimimo.svg?url';
import BaiduSvg from '@lobehub/icons-static-svg/icons/baidu-color.svg?url';
import TencentSvg from '@lobehub/icons-static-svg/icons/tencent-color.svg?url';
import IflytekCloudSvg from '@lobehub/icons-static-svg/icons/iflytekcloud-color.svg?url';
import BaichuanSvg from '@lobehub/icons-static-svg/icons/baichuan-color.svg?url';
import YiSvg from '@lobehub/icons-static-svg/icons/yi-color.svg?raw';
import DoubaoSvg from '@lobehub/icons-static-svg/icons/doubao-color.svg?url';
import AwsSvg from '@lobehub/icons-static-svg/icons/aws.svg?url';
import BedrockSvg from '@lobehub/icons-static-svg/icons/bedrock-color.svg?url';
import AzureSvg from '@lobehub/icons-static-svg/icons/azure-color.svg?url';
import VertexAISvg from '@lobehub/icons-static-svg/icons/vertexai-color.svg?url';
import OllamaSvg from '@lobehub/icons-static-svg/icons/ollama.svg?url';
import LMStudioSvg from '@lobehub/icons-static-svg/icons/lmstudio.svg?url';
import VllmSvg from '@lobehub/icons-static-svg/icons/vllm-color.svg?url';
import VercelSvg from '@lobehub/icons-static-svg/icons/vercel.svg?url';
import CloudflareSvg from '@lobehub/icons-static-svg/icons/cloudflare-color.svg?url';

// Single-colour twins of the brand-colour icons, for surfaces that stay
// black and white (the routing graph).
import ClaudeCodeMonoSvg from '@lobehub/icons-static-svg/icons/claudecode.svg?url';
import CodexMonoSvg from '@lobehub/icons-static-svg/icons/codex.svg?url';
import GeminiMonoSvg from '@lobehub/icons-static-svg/icons/gemini.svg?url';
import GoogleMonoSvg from '@lobehub/icons-static-svg/icons/google.svg?url';
import QwenMonoSvg from '@lobehub/icons-static-svg/icons/qwen.svg?url';
import DeepSeekMonoSvg from '@lobehub/icons-static-svg/icons/deepseek.svg?url';
import MinimaxMonoSvg from '@lobehub/icons-static-svg/icons/minimax.svg?url';
import ZhipuMonoSvg from '@lobehub/icons-static-svg/icons/zhipu.svg?url';
import MistralMonoSvg from '@lobehub/icons-static-svg/icons/mistral.svg?url';
import OpenRouterMonoSvg from '@lobehub/icons-static-svg/icons/openrouter.svg?url';
import TogetherMonoSvg from '@lobehub/icons-static-svg/icons/together.svg?url';
import FireworksMonoSvg from '@lobehub/icons-static-svg/icons/fireworks.svg?url';
import PerplexityMonoSvg from '@lobehub/icons-static-svg/icons/perplexity.svg?url';
import CohereMonoSvg from '@lobehub/icons-static-svg/icons/cohere.svg?url';
import NvidiaMonoSvg from '@lobehub/icons-static-svg/icons/nvidia.svg?url';
import NovitaMonoSvg from '@lobehub/icons-static-svg/icons/novita.svg?url';
import DeepInfraMonoSvg from '@lobehub/icons-static-svg/icons/deepinfra.svg?url';
import HyperbolicMonoSvg from '@lobehub/icons-static-svg/icons/hyperbolic.svg?url';
import ModelScopeMonoSvg from '@lobehub/icons-static-svg/icons/modelscope.svg?url';
import SiliconFlowMonoSvg from '@lobehub/icons-static-svg/icons/siliconcloud.svg?url';
import StepfunMonoSvg from '@lobehub/icons-static-svg/icons/stepfun.svg?url';
import BaiduMonoSvg from '@lobehub/icons-static-svg/icons/baidu.svg?url';
import TencentMonoSvg from '@lobehub/icons-static-svg/icons/tencent.svg?url';
import IflytekCloudMonoSvg from '@lobehub/icons-static-svg/icons/iflytekcloud.svg?url';
import BaichuanMonoSvg from '@lobehub/icons-static-svg/icons/baichuan.svg?url';
import YiMonoSvg from '@lobehub/icons-static-svg/icons/yi.svg?url';
import DoubaoMonoSvg from '@lobehub/icons-static-svg/icons/doubao.svg?url';
import BedrockMonoSvg from '@lobehub/icons-static-svg/icons/bedrock.svg?url';
import AzureMonoSvg from '@lobehub/icons-static-svg/icons/azure.svg?url';
import VertexAIMonoSvg from '@lobehub/icons-static-svg/icons/vertexai.svg?url';
import VllmMonoSvg from '@lobehub/icons-static-svg/icons/vllm.svg?url';
import ClaudeMonoSvg from '@lobehub/icons-static-svg/icons/claude.svg?url';
import CloudflareMonoSvg from '@lobehub/icons-static-svg/icons/cloudflare.svg?url';

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
// Not in @lobehub/icons yet; taken from @zenmux/icons (MIT, brand artwork
// belongs to its owners).
import WorkBuddySvg from '@/assets/icons/workbuddy.svg?url';
import ZCodeSvg from '@/assets/icons/zcode.svg?url';
import LocalAIPng from '@/assets/icons/localai.png?url';
import JanSvg from '@/assets/icons/jan.svg?url';
import SGLangSvg from '@/assets/icons/sglang.svg?url';
import AmdSvg from '@/assets/icons/amd.svg?url';
import LemonadeSvg from '@/assets/icons/lemonade.svg?url';
import TypeSafePng from '@/assets/icons/typesafe.png?url';
import LlamaCppSvg from '@/assets/icons/llamacpp.svg?url';
import OmlxSvg from '@/assets/icons/omlx.svg?url';
import OmlxDarkSvg from '@/assets/icons/omlx-dark.svg?url';

interface BrandIconProps {
    size?: number;
    sx?: SxProps<Theme>;
    style?: React.CSSProperties;
    grayscale?: boolean;
    /** Single-colour rendering, black on light and white on dark. */
    mono?: boolean;
}

// Lobehub 的品牌 SVG 用 fill="currentColor" 是单色设计，但 <img> 加载是独立文档，
// currentColor 没有外部颜色继承，会落到默认黑色，导致暗色主题下黑图不可见。
// 解决：暗色主题下对单色图标加 invert(1)（黑→白），亮色主题保持原样。
// 高亮态由 Sidebar 处理（白底 + 强制清零 filter，保持黑图在白底上的效果）。
const createBrandIcon = (src: string, alt: string, defaultGrayscale = false, monochrome = false, monoSrc?: string, darkSrc?: string) => {
    return ({size = 24, sx, style, grayscale = defaultGrayscale, mono = false}: BrandIconProps) => {
        const theme = useTheme();
        const isDark = theme.palette.mode === 'dark';

        const useMono = mono && !!monoSrc;
        let filter = 'none';
        if (useMono) {
            filter = isDark ? 'invert(1)' : 'none';
        } else if (mono && !monochrome) {
            // Collected colour logos may not have a monochrome twin. Keep
            // their light shades visible against a light routing-graph node.
            filter = isDark ? 'grayscale(100%) brightness(1.8)' : 'grayscale(100%) brightness(0.55)';
        } else if (grayscale) {
            filter = isDark
                ? 'grayscale(100%) brightness(1.6)'
                : 'grayscale(100%) brightness(1.15) contrast(1.1)';
        } else if (monochrome && isDark) {
            filter = 'invert(1)';
        }

        return (
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
                    src={useMono ? monoSrc : isDark && darkSrc ? darkSrc : src}
                    alt={alt}
                    sx={{
                        display: 'block',
                        width: '100%',
                        height: '100%',
                        objectFit: 'contain',
                        filter,
                        transition: 'filter 0.2s',
                    }}
                />
            </Box>
        );
    };
};

export const OpenAI = createBrandIcon(OpenAISvg, 'OpenAI', false, true);
export const Anthropic = createBrandIcon(AnthropicSvg, 'Anthropic', false, true);
export const Claude = createBrandIcon(ClaudeSvg, 'Claude', true, false, ClaudeMonoSvg);
export const ClaudeCode = createBrandIcon(ClaudeCodeSvg, 'Claude Code', false, false, ClaudeCodeMonoSvg);
export const Codex = createBrandIcon(CodexSvg, 'Codex', true, false, CodexMonoSvg);
export const Gemini = createBrandIcon(GeminiSvg, 'Gemini', false, false, GeminiMonoSvg);
export const Google = createBrandIcon(GoogleSvg, 'Google', false, false, GoogleMonoSvg);
export const Kimi = createBrandIcon(KimiSvg, 'Kimi', false, true);
export const Qwen = createBrandIcon(QwenSvg, 'Qwen', false, false, QwenMonoSvg);
export const OpenCode = createBrandIcon(OpenCodeSvg, 'OpenCode', false, true);
export const DeepSeek = createBrandIcon(DeepSeekSvg, 'DeepSeek', true, false, DeepSeekMonoSvg);
export const Minimax = createBrandIcon(MinimaxSvg, 'Minimax', false, false, MinimaxMonoSvg);
export const Zhipu = createBrandIcon(ZhipuSvg, 'Zhipu', false, false, ZhipuMonoSvg);
export const Zai = createBrandIcon(ZaiSvg, 'Z.ai', false, true);
export const ZCode = createBrandIcon(ZCodeSvg, 'ZCode', false, true);
export const WorkBuddy = createBrandIcon(WorkBuddySvg, 'WorkBuddy');
export const XAI = createBrandIcon(XAISvg, 'xAI', false, true);
export const Mistral = createBrandIcon(MistralSvg, 'Mistral', false, false, MistralMonoSvg);
export const OpenRouter = createBrandIcon(OpenRouterSvg, 'OpenRouter', false, false, OpenRouterMonoSvg);
export const Xcode = createBrandIcon(XcodeSvg, 'Xcode', true);
export const VSCode = createBrandIcon(VSCodeSvg, 'VS Code', true);
export const Cursor = createBrandIcon(CursorSvg, 'Cursor', false, true);
export const Pi = createBrandIcon(PiSvg, 'Pi', false, true);

// Tingly-Box's own mark (full colour, so no dark-mode inversion). Marks a
// provider that is itself another Tingly-Box gateway.
export const TinglyBox = createBrandIcon('/assets/icon.svg', 'Tingly-Box');

// Reuse Claude icon for Claude Desktop
export const ClaudeDesktop = Claude;

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
// Together / Baidu / Yi import the brand-color variants and stay <img>; the
// rest use lobehub's `fill="currentColor"` monochrome SVGs and need mask
// rendering so they follow text color in dark/light themes.
export const Groq = createBrandIcon(GroqSvg, 'Groq', false, true);
export const Together = createBrandIcon(TogetherSvg, 'Together', false, false, TogetherMonoSvg);
export const Fireworks = createBrandIcon(FireworksSvg, 'Fireworks', false, false, FireworksMonoSvg);
export const Cerebras = createBrandIcon(CerebrasSvg, 'Cerebras', false, true);
export const Perplexity = createBrandIcon(PerplexitySvg, 'Perplexity', false, false, PerplexityMonoSvg);
export const Cohere = createBrandIcon(CohereSvg, 'Cohere', false, false, CohereMonoSvg);
export const Nvidia = createBrandIcon(NvidiaSvg, 'NVIDIA', false, false, NvidiaMonoSvg);
export const Novita = createBrandIcon(NovitaSvg, 'Novita', false, false, NovitaMonoSvg);
export const DeepInfra = createBrandIcon(DeepInfraSvg, 'DeepInfra', false, false, DeepInfraMonoSvg);
export const Hyperbolic = createBrandIcon(HyperbolicSvg, 'Hyperbolic', false, false, HyperbolicMonoSvg);
export const ModelScope = createBrandIcon(ModelScopeSvg, 'ModelScope', false, false, ModelScopeMonoSvg);
export const SiliconFlow = createBrandIcon(SiliconFlowSvg, 'SiliconFlow', false, false, SiliconFlowMonoSvg);
export const Stepfun = createBrandIcon(StepfunSvg, 'Stepfun', false, false, StepfunMonoSvg);
export const Xiaomimimo = createBrandIcon(XiaomimimoSvg, 'Xiaomi Mimo', false, true);
export const Baidu = createBrandIcon(BaiduSvg, 'Baidu', false, false, BaiduMonoSvg);
export const Aws = createBrandIcon(AwsSvg, 'AWS', false, true);
export const Bedrock = createBrandIcon(BedrockSvg, 'Amazon Bedrock', false, false, BedrockMonoSvg);
export const Azure = createBrandIcon(AzureSvg, 'Azure', false, false, AzureMonoSvg);
export const VertexAI = createBrandIcon(VertexAISvg, 'Vertex AI', false, false, VertexAIMonoSvg);
export const Ollama = createBrandIcon(OllamaSvg, 'Ollama', false, true);
export const LMStudio = createBrandIcon(LMStudioSvg, 'LM Studio', false, true);
export const LocalAI = createBrandIcon(LocalAIPng, 'LocalAI');
export const Jan = createBrandIcon(JanSvg, 'Jan');
export const Vllm = createBrandIcon(VllmSvg, 'vLLM', false, false, VllmMonoSvg);
export const SGLang = createBrandIcon(SGLangSvg, 'SGLang');
export const Amd = createBrandIcon(AmdSvg, 'AMD', false, true);
export const Lemonade = createBrandIcon(LemonadeSvg, 'Lemonade');
export const Vercel = createBrandIcon(VercelSvg, 'Vercel', false, true);
export const Cloudflare = createBrandIcon(CloudflareSvg, 'Cloudflare', false, false, CloudflareMonoSvg);
export const TypeSafe = createBrandIcon(TypeSafePng, 'TypeSafe AI');
export const LlamaCpp = createBrandIcon(LlamaCppSvg, 'llama.cpp');
export const Omlx = createBrandIcon(OmlxSvg, 'oMLX', false, false, undefined, OmlxDarkSvg);
export const Tencent = createBrandIcon(TencentSvg, 'Tencent', false, false, TencentMonoSvg);
export const IflytekCloud = createBrandIcon(IflytekCloudSvg, 'iFlytek', false, false, IflytekCloudMonoSvg);
export const Baichuan = createBrandIcon(BaichuanSvg, 'Baichuan', false, false, BaichuanMonoSvg);
// Resolve Yi's currentColor strokes per theme while preserving its green accent.
export const Yi = createBrandIcon(
    `data:image/svg+xml,${encodeURIComponent(YiSvg.replaceAll('currentColor', '#000'))}`,
    'Lingyi Wanwu', false, false, YiMonoSvg,
    `data:image/svg+xml,${encodeURIComponent(YiSvg.replaceAll('currentColor', '#fff'))}`,
);
export const Doubao = createBrandIcon(DoubaoSvg, 'Doubao', false, false, DoubaoMonoSvg);
