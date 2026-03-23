import { OpenInNew } from '@mui/icons-material';
import { Box, Link, Stack, Typography } from '@mui/material';

export interface PlatformGuideConfig {
    id: string;
    name: string;
    description: string;
    icon: string;
    status: 'available' | 'coming-soon' | 'beta';
    path: string;
    color: string;
    guide: React.ReactNode;
}

export const platformGuides: Record<string, PlatformGuideConfig> = {
    telegram: {
        id: 'telegram',
        name: 'Telegram',
        description: 'Popular cloud-based instant messaging service',
        icon: '📱',
        status: 'available',
        path: '/remote-control/telegram',
        color: '#0088cc',
        guide: (
            <Stack spacing={2}>
                <Box>
                    <Typography variant="subtitle2" sx={{ fontWeight: 600, mb: 1 }}>
                        1. Create a bot
                    </Typography>
                    <Typography variant="body2" color="text.secondary">
                        Open Telegram, search{' '}
                        <Link href="https://t.me/BotFather" target="_blank">
                            @BotFather <OpenInNew sx={{ fontSize: 10 }} />
                        </Link>
                    </Typography>
                    <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
                        Send <code>/newbot</code>, follow the prompts, and copy the token
                    </Typography>
                </Box>
                <Box>
                    <Typography variant="subtitle2" sx={{ fontWeight: 600, mb: 1 }}>
                        2. Add bot
                    </Typography>
                    <Typography variant="body2" color="text.secondary">
                        Click "Add Bot" button above and paste the token to create your bot.
                    </Typography>
                </Box>
                <Box sx={{ bgcolor: 'info.lighter', p: 1.5, borderRadius: 1, border: '1px solid', borderColor: 'info.light' }}>
                    <Typography variant="body2" color="info.dark">
                        Tip: Configure traffic proxy as needed for network access.
                    </Typography>
                </Box>
            </Stack>
        ),
    },
    feishu: {
        id: 'feishu',
        name: 'Feishu (飞书)',
        description: 'Enterprise collaboration platform',
        icon: '🚀',
        status: 'available',
        path: '/remote-control/feishu',
        color: '#00d6b9',
        guide: (
            <Stack spacing={2}>
                <Box>
                    <Typography variant="subtitle2" sx={{ fontWeight: 600, mb: 1 }}>
                        1. Create a Feishu bot
                    </Typography>
                    <Typography variant="body2" color="text.secondary" component="div">
                        <Box component="ul" sx={{ pl: 2, m: 0 }}>
                            <li>Visit <Link href="https://open.feishu.cn/" target="_blank">Feishu Open Platform <OpenInNew sx={{ fontSize: 10 }} /></Link></li>
                            <li>Create a new app - Enable Bot capability</li>
                            <li>Permissions: Add <code>im:message</code> (send messages) and <code>im:message.p2p_msg:readonly</code> (receive messages)</li>
                            <li>Events: Add <code>im.message.receive_v1</code> (receive messages)</li>
                            <li>Select <strong>Long Connection</strong> mode (requires running nanobot first to establish connection)</li>
                            <li>Get App ID and App Secret from "Credentials & Basic Info"</li>
                            <li>Publish the app</li>
                        </Box>
                    </Typography>
                </Box>
                <Box>
                    <Typography variant="subtitle2" sx={{ fontWeight: 600, mb: 1 }}>
                        2. Add bot
                    </Typography>
                    <Typography variant="body2" color="text.secondary">
                        Click "Add Bot" button above and fill in App ID and App Secret to create your bot.
                    </Typography>
                </Box>
                <Box sx={{ bgcolor: 'info.lighter', p: 1.5, borderRadius: 1, border: '1px solid', borderColor: 'info.light' }}>
                    <Typography variant="body2" color="info.dark">
                        Tip: Feishu uses WebSocket - no public IP needed. Configure traffic proxy as needed.
                    </Typography>
                </Box>
            </Stack>
        ),
    },
    lark: {
        id: 'lark',
        name: 'Lark',
        description: 'Global version of Feishu',
        icon: '🐦',
        status: 'available',
        path: '/remote-control/lark',
        color: '#00d6b9',
        guide: (
            <Stack spacing={2}>
                <Box>
                    <Typography variant="subtitle2" sx={{ fontWeight: 600, mb: 1 }}>
                        1. Create a Lark bot
                    </Typography>
                    <Typography variant="body2" color="text.secondary" component="div">
                        <Box component="ul" sx={{ pl: 2, m: 0 }}>
                            <li>Visit <Link href="https://open.larksuite.com/" target="_blank">Lark Open Platform <OpenInNew sx={{ fontSize: 10 }} /></Link></li>
                            <li>Create a new app - Enable Bot capability</li>
                            <li>Permissions: Add <code>im:message</code> (send messages) and <code>im:message.p2p_msg:readonly</code> (receive messages)</li>
                            <li>Events: Add <code>im.message.receive_v1</code> (receive messages)</li>
                            <li>Select <strong>Long Connection</strong> mode (requires running nanobot first to establish connection)</li>
                            <li>Get App ID and App Secret from "Credentials & Basic Info"</li>
                            <li>Publish the app</li>
                        </Box>
                    </Typography>
                </Box>
                <Box>
                    <Typography variant="subtitle2" sx={{ fontWeight: 600, mb: 1 }}>
                        2. Add bot
                    </Typography>
                    <Typography variant="body2" color="text.secondary">
                        Click "Add Bot" button above and fill in App ID and App Secret to create your bot.
                    </Typography>
                </Box>
                <Box sx={{ bgcolor: 'info.lighter', p: 1.5, borderRadius: 1, border: '1px solid', borderColor: 'info.light' }}>
                    <Typography variant="body2" color="info.dark">
                        Tip: Lark uses WebSocket - no public IP needed. Configure traffic proxy as needed.
                    </Typography>
                </Box>
            </Stack>
        ),
    },
    dingtalk: {
        id: 'dingtalk',
        name: 'DingTalk (钉钉)',
        description: 'Enterprise communication and collaboration',
        icon: '💬',
        status: 'available',
        path: '/remote-control/dingtalk',
        color: '#0089ff',
        guide: (
            <Stack spacing={2}>
                <Box>
                    <Typography variant="subtitle2" sx={{ fontWeight: 600, mb: 1 }}>
                        1. Create a DingTalk bot
                    </Typography>
                    <Typography variant="body2" color="text.secondary" component="div">
                        <Box component="ul" sx={{ pl: 2, m: 0 }}>
                            <li>Visit <Link href="https://open.dingtalk.com/" target="_blank">DingTalk Open Platform <OpenInNew sx={{ fontSize: 10 }} /></Link></li>
                            <li>Create a new app - Add Robot capability</li>
                            <li>Configuration:</li>
                            <Box component="ul" sx={{ pl: 2 }}>
                                <li>Toggle <strong>Stream Mode</strong> ON</li>
                                <li>Permissions: Add necessary permissions for sending messages</li>
                            </Box>
                            <li>Get AppKey (Client ID) and AppSecret (Client Secret) from "Credentials"</li>
                            <li>Publish the app</li>
                        </Box>
                    </Typography>
                </Box>
                <Box>
                    <Typography variant="subtitle2" sx={{ fontWeight: 600, mb: 1 }}>
                        2. Add bot
                    </Typography>
                    <Typography variant="body2" color="text.secondary">
                        Click "Add Bot" button above and fill in App Key and App Secret to create your bot.
                    </Typography>
                </Box>
                <Box sx={{ bgcolor: 'info.lighter', p: 1.5, borderRadius: 1, border: '1px solid', borderColor: 'info.light' }}>
                    <Typography variant="body2" color="info.dark">
                        Tip: DingTalk uses Stream Mode - no public IP required. Configure traffic proxy as needed.
                    </Typography>
                </Box>
            </Stack>
        ),
    },
    weixin: {
        id: 'weixin',
        name: 'Weixin (微信)',
        description: 'China\'s most popular messaging platform',
        icon: '💚',
        status: 'beta',
        path: '/remote-control/weixin',
        color: '#07c160',
        guide: (
            <Stack spacing={2}>
                <Box>
                    <Typography variant="subtitle2" sx={{ fontWeight: 600, mb: 1 }}>
                        1. Create a Weixin bot
                    </Typography>
                    <Typography variant="body2" color="text.secondary" component="div">
                        <Box component="ul" sx={{ pl: 2, m: 0 }}>
                            <li>Visit <Link href="https://mp.weixin.qq.com/" target="_blank">WeChat MP Platform <OpenInNew sx={{ fontSize: 10 }} /></Link></li>
                            <li>Register a Mini Program or Service Account</li>
                            <li>Enable Message Push capability</li>
                            <li>Configure server URL and token</li>
                            <li>Get App ID and App Secret from "Development Settings"</li>
                        </Box>
                    </Typography>
                </Box>
                <Box>
                    <Typography variant="subtitle2" sx={{ fontWeight: 600, mb: 1 }}>
                        2. Add bot
                    </Typography>
                    <Typography variant="body2" color="text.secondary">
                        Click "Add Bot" button above and fill in App ID and App Secret to create your bot.
                    </Typography>
                </Box>
                <Box sx={{ bgcolor: 'info.lighter', p: 1.5, borderRadius: 1, border: '1px solid', borderColor: 'info.light' }}>
                    <Typography variant="body2" color="info.dark">
                        <strong>Beta:</strong> Weixin integration is in beta. Please provide feedback for any issues.
                    </Typography>
                </Box>
            </Stack>
        ),
    },
    wecom: {
        id: 'wecom',
        name: 'WeCom (企业微信)',
        description: 'Enterprise WeChat communication platform',
        icon: '💼',
        status: 'coming-soon',
        path: '/remote-control/wecom',
        color: '#888',
        guide: (
            <Stack spacing={2}>
                <Box>
                    <Typography variant="body2" color="text.secondary">
                        WeCom bot integration is currently under development. Stay tuned for updates!
                    </Typography>
                </Box>
            </Stack>
        ),
    },
    qq: {
        id: 'qq',
        name: 'QQ',
        description: 'Tencent instant messaging platform',
        icon: '🐧',
        status: 'coming-soon',
        path: '/remote-control/qq',
        color: '#888',
        guide: (
            <Stack spacing={2}>
                <Box>
                    <Typography variant="body2" color="text.secondary">
                        QQ bot integration is currently under development. Stay tuned for updates!
                    </Typography>
                </Box>
            </Stack>
        ),
    },
    discord: {
        id: 'discord',
        name: 'Discord',
        description: 'Voice, video, and text communication',
        icon: '🎮',
        status: 'coming-soon',
        path: '/remote-control/discord',
        color: '#888',
        guide: (
            <Stack spacing={2}>
                <Box>
                    <Typography variant="body2" color="text.secondary">
                        Discord bot integration is currently under development. Stay tuned for updates!
                    </Typography>
                </Box>
            </Stack>
        ),
    },
    slack: {
        id: 'slack',
        name: 'Slack',
        description: 'Business communication platform',
        icon: '💳',
        status: 'coming-soon',
        path: '/remote-control/slack',
        color: '#888',
        guide: (
            <Stack spacing={2}>
                <Box>
                    <Typography variant="body2" color="text.secondary">
                        Slack bot integration is currently under development. Stay tuned for updates!
                    </Typography>
                </Box>
            </Stack>
        ),
    },
};

export const getPlatformGuide = (platformId: string): PlatformGuideConfig | undefined => {
    return platformGuides[platformId];
};

export const getAvailablePlatforms = (): PlatformGuideConfig[] => {
    return Object.values(platformGuides).filter(p => p.status === 'available');
};

export const getAllPlatforms = (): PlatformGuideConfig[] => {
    // Return platforms in a specific order
    const order: (keyof typeof platformGuides)[] = [
        'telegram',
        'weixin',
        'feishu',
        'lark',
        'dingtalk',
        'wecom',
        'qq',
        'discord',
        'slack',
    ];
    return order.map(id => platformGuides[id]).filter(Boolean);
};
