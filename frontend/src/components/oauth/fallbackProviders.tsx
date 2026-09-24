import {Box} from '@mui/material';
import {Claude, Gemini, Google, Kimi, OpenAI, Qwen, XAI, Zai, Zhipu} from '../BrandIcons';

export interface OAuthProvider {
    id: string;
    name: string;
    displayName: string;
    description: string;
    icon: React.ReactNode;
    color: string;
    enabled?: boolean;
    dev?: boolean;
    deviceCodeFlow?: boolean;
    // Some upstream OAuth apps (Anthropic's Claude Code CLI client, OpenAI's Codex
    // CLI client) only accept a redirect to this exact loopback port. Tingly-Box
    // starts a temporary local listener on it for the callback — see
    // ai/oauth/registry.go `CallbackPorts`.
    callbackPort?: number;
}

// This list lives in its own dialog-free module on purpose: it carries JSX icon
// nodes AND is imported by ConnectProviderDialog. When it was exported from
// OAuthDialog.tsx, that single import dragged the whole dialog component into
// the picker's eager chunk (see frontend/CLAUDE.md on module-level imports).

// Fallback hardcoded providers for development or when API is unavailable
export const FALLBACK_OAUTH_PROVIDERS: OAuthProvider[] = [
    {
        id: 'claude_code',
        name: 'Claude Code',
        displayName: 'Anthropic Claude Code',
        description: 'Access Claude Code models via OAuth',
        icon: <Claude size={32}/>,
        color: '#D97757',
        enabled: true,
        callbackPort: 54545,
    },
    {
        id: 'gemini',
        name: 'Google Gemini CLI',
        displayName: 'Google Gemini CLI',
        description: 'Access Gemini CLI models via OAuth',
        icon: <Gemini size={32}/>,
        color: '#4285F4',
        enabled: true,
    },
    {
        id: 'antigravity',
        name: 'Antigravity',
        displayName: 'Antigravity (Experimental)',
        description: 'Access Antigravity services via Google OAuth',
        icon: <Google size={32}/>,
        color: '#7B1FA2',
        enabled: true,
    },
    {
        id: 'qwen_code',
        name: 'Qwen Code',
        displayName: 'Qwen Code',
        description: 'Access Qwen Code via device code flow',
        icon: <Qwen size={32}/>,
        color: '#00A8E1',
        enabled: false,  // DISABLED: Aliyun WAF blocking OAuth requests
        deviceCodeFlow: true,
    },
    {
        id: 'codex',
        name: 'Codex',
        displayName: 'OpenAI Codex',
        description: 'Access OpenAI Codex via OAuth',
        icon: <OpenAI size={32}/>,
        color: '#10A37F',
        enabled: true,
        callbackPort: 1455,
    },
    {
        id: 'kimi_code',
        name: 'Kimi Code',
        displayName: 'Kimi Code',
        description: 'Access Kimi Code via device code flow',
        icon: <Kimi size={32}/>,
        color: '#6366F1',
        enabled: true,
        deviceCodeFlow: true,
    },
    {
        id: 'xai',
        name: 'xAI Grok',
        displayName: 'xAI Grok',
        description: 'Access Grok via SuperGrok / X Premium+ OAuth login',
        icon: <XAI size={32}/>,
        color: '#000000',
        enabled: true,
    },
    // ZCode / GLM Coding Plan. The sign-in is a login link: zcode.z.ai owns the
    // callback, so the URL can be opened on any device and the backend polls for
    // the outcome — the standard auth_url + session status path, no user code.
    {
        id: 'zcode_cn',
        name: 'ZCode CN',
        displayName: 'ZCode CN (智谱 BigModel)',
        description: 'Sign in with your BigModel account to use ZCode CN',
        icon: <Zhipu size={32}/>,
        color: '#3859FF',
        enabled: true,
    },
    {
        id: 'zcode',
        name: 'ZCode International',
        displayName: 'ZCode International (Z.ai)',
        description: 'Sign in with your Z.ai account to use ZCode International',
        icon: <Zai size={32}/>,
        color: '#3859FF',
        enabled: true,
    },
    {
        id: 'mock',
        name: 'Mock',
        displayName: 'Mock OAuth',
        description: 'Test OAuth flow with mock provider',
        icon: <Box sx={{fontSize: 32}}>🧪</Box>,
        color: '#9E9E9E',
        enabled: true,
        dev: true,
    },
    // Add more providers as needed
];
