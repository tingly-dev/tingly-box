import { Telegram, Feishu, Lark, DingTalk, Weixin, WeCom, QQ, Discord, Slack } from '@/components/BrandIcons';
import { OpenInNew } from '@/components/icons';
import { Box, Link, Stack, Typography } from '@mui/material';
import type { ComponentType } from 'react';
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import type { TFunction } from 'i18next';

export interface PlatformGuideConfig {
    id: string;
    name: string;
    description: string;
    icon: string;
    /** BrandIcon component for consistent SVG rendering */
    BrandIcon?: ComponentType<{ size?: number; sx?: any }>;
    status: 'available' | 'coming-soon' | 'beta';
    path: string;
    color: string;
    guide: React.ReactNode;
}

// Feishu and Lark are the same product (China vs. international release) -
// same one-click QR flow, same manual App ID/App Secret fallback, different
// domain and mobile app name. Shared here so the two guides can't drift.
const buildFeishuFamilyGuide = (t: TFunction, opts: {
    tip: string;
    step1TextAfter: string;
    manualUrl: string;
    manualLinkLabel: string;
}) => (
    <Stack spacing={2}>
        <Box sx={{ bgcolor: 'info.lighter', p: 1.5, borderRadius: 1, border: '1px solid', borderColor: 'info.light' }}>
            <Typography variant="body2" sx={{
                color: "info.dark"
            }}>
                {opts.tip}
            </Typography>
        </Box>
        <Box>
            <Typography variant="subtitle2" sx={{ fontWeight: 600, mb: 1 }}>
                {t('remoteControl.guides.feishuFamily.step1Title', { defaultValue: '1. Scan to create (recommended)' })}
            </Typography>
            <Typography variant="body2" sx={{
                color: "text.secondary"
            }}>
                {t('remoteControl.guides.feishuFamily.step1TextBefore', { defaultValue: 'Click "Add Bot" above, choose' })}{' '}
                <strong>{t('remoteControl.guides.feishuFamily.oneClickOption', { defaultValue: 'One-click (scan QR)' })}</strong>
                {opts.step1TextAfter}
            </Typography>
        </Box>
        <Box>
            <Typography variant="subtitle2" sx={{ fontWeight: 600, mb: 1 }}>
                {t('remoteControl.guides.feishuFamily.step2Title', { defaultValue: '2. Or create manually' })}
            </Typography>
            <Typography variant="body2" component="div" sx={{
                color: "text.secondary"
            }}>
                <Box component="ul" sx={{ pl: 2, m: 0 }}>
                    <li>
                        {t('remoteControl.guides.feishu.step2Open', { defaultValue: 'Open' })}{' '}
                        <Link href={opts.manualUrl} target="_blank">
                            {opts.manualLinkLabel} <OpenInNew sx={{ fontSize: 10 }} />
                        </Link>
                        {' '}{t('remoteControl.guides.feishuFamily.step2LogIn', { defaultValue: 'and log in' })}
                    </li>
                    <li>{t('remoteControl.guides.feishuFamily.step2Configure', { defaultValue: 'Enter an app name and confirm — bot capability, permissions, events and Long Connection mode are pre-configured for you' })}</li>
                    <li>
                        {t('remoteControl.guides.feishuFamily.step2CopyBefore', { defaultValue: 'Copy the generated' })}{' '}
                        <strong>App ID</strong> {t('common.and', { defaultValue: 'and' })} <strong>App Secret</strong>
                        {t('remoteControl.guides.feishuFamily.step2CopyAfterBefore', { defaultValue: ', then enter them via "Add Bot" →' })}{' '}
                        <strong>{t('remoteControl.authForm.manualOption', { defaultValue: 'Enter manually' })}</strong>
                    </li>
                </Box>
            </Typography>
        </Box>
    </Stack>
);

const buildComingSoonGuide = (t: TFunction, platformName: string) => (
    <Stack spacing={2}>
        <Box>
            <Typography variant="body2" sx={{
                color: "text.secondary"
            }}>
                {t('remoteControl.guides.comingSoon', { defaultValue: '{{platform}} bot integration is currently under development. Stay tuned for updates!', platform: platformName })}
            </Typography>
        </Box>
    </Stack>
);

// Guide prose is built per-render from `t` so it re-localizes immediately on
// language switch (the guide JSX can't call the useTranslation hook itself,
// since it's assembled outside a component). Technical field names a user
// has to locate verbatim in a third-party console (App ID/App Secret/App
// Key/Client ID/Client Secret/Bot ID/Bot Token) are intentionally left in
// English in both locales.

// The full set of bot platform ids, in display order. Single source of
// truth for anything that needs to list "every platform" (e.g. the Remote
// page's in-page platform tabs) — keeps that list from drifting out of sync
// with the guides below or with the routes in App.tsx. Discord and Slack
// aren't supported yet — their guide entries and routes stay (so nothing
// 404s if something still links to them), just left out of this list so
// they don't show up as pickable platforms.
export const BOT_PLATFORM_IDS = ['telegram', 'feishu', 'lark', 'dingtalk', 'weixin', 'wecom', 'qq'] as const;

const buildPlatformGuides = (t: TFunction): Record<string, PlatformGuideConfig> => ({
    telegram: {
        id: 'telegram',
        name: 'Telegram',
        description: t('remoteControl.guides.telegram.description', { defaultValue: 'Popular cloud-based instant messaging service' }),
        icon: '📱',
        BrandIcon: Telegram,
        status: 'available',
        path: '/bots/telegram',
        color: '#0088cc',
        guide: (
            <Stack spacing={2}>
                <Box sx={{ bgcolor: 'info.lighter', p: 1.5, borderRadius: 1, border: '1px solid', borderColor: 'info.light' }}>
                    <Typography variant="body2" sx={{
                        color: "info.dark"
                    }}>
                        {t('remoteControl.guides.telegram.tip', { defaultValue: 'Tip: Configure traffic proxy as needed for network access.' })}
                    </Typography>
                </Box>
                <Box>
                    <Typography variant="subtitle2" sx={{ fontWeight: 600, mb: 1 }}>
                        {t('remoteControl.guides.telegram.step1Title', { defaultValue: '1. Create a bot' })}
                    </Typography>
                    <Typography variant="body2" sx={{
                        color: "text.secondary"
                    }}>
                        {t('remoteControl.guides.telegram.step1Open', { defaultValue: 'Open Telegram, search' })}{' '}
                        <Link href="https://t.me/BotFather" target="_blank">
                            @BotFather <OpenInNew sx={{ fontSize: 10 }} />
                        </Link>
                    </Typography>
                    <Typography
                        variant="body2"
                        sx={{
                            color: "text.secondary",
                            mt: 0.5
                        }}>
                        {t('remoteControl.guides.telegram.step1Send', { defaultValue: 'Send' })} <code>/newbot</code> {t('remoteControl.guides.telegram.step1SendTail', { defaultValue: ', follow the prompts, and copy the token' })}
                    </Typography>
                </Box>
                <Box>
                    <Typography variant="subtitle2" sx={{ fontWeight: 600, mb: 1 }}>
                        {t('remoteControl.guides.telegram.step2Title', { defaultValue: '2. Add bot' })}
                    </Typography>
                    <Typography variant="body2" sx={{
                        color: "text.secondary"
                    }}>
                        {t('remoteControl.guides.telegram.step2Text', { defaultValue: 'Click "Add Bot" button above and paste the token to create your bot.' })}
                    </Typography>
                </Box>
            </Stack>
        ),
    },
    feishu: {
        id: 'feishu',
        name: 'Feishu (飞书)',
        description: t('remoteControl.guides.feishu.description', { defaultValue: 'Enterprise collaboration platform' }),
        icon: '🚀',
        BrandIcon: Feishu,
        status: 'available',
        path: '/bots/feishu',
        color: '#00d6b9',
        guide: buildFeishuFamilyGuide(t, {
            tip: t('remoteControl.guides.feishu.tip', { defaultValue: 'Tip: Feishu uses WebSocket - no public IP needed. Configure traffic proxy as needed.' }),
            step1TextAfter: t('remoteControl.guides.feishu.step1TextAfter', { defaultValue: ', and scan the QR code with the Feishu mobile app. The app, permissions and events are created automatically and the credentials are saved for you.' }),
            manualUrl: 'https://open.feishu.cn/page/launcher?from=backend_oneclick',
            manualLinkLabel: t('remoteControl.guides.feishu.step2LinkLabel', { defaultValue: 'Feishu one-click app creation' }),
        }),
    },
    lark: {
        id: 'lark',
        name: 'Lark',
        description: t('remoteControl.guides.lark.description', { defaultValue: 'Global version of Feishu' }),
        icon: '🐦',
        BrandIcon: Lark,
        status: 'available',
        path: '/bots/lark',
        color: '#00d6b9',
        guide: buildFeishuFamilyGuide(t, {
            tip: t('remoteControl.guides.lark.tip', { defaultValue: 'Tip: Lark uses WebSocket - no public IP needed. Configure traffic proxy as needed.' }),
            step1TextAfter: t('remoteControl.guides.lark.step1TextAfter', { defaultValue: ', and scan the QR code with the Lark mobile app. The app, permissions and events are created automatically and the credentials are saved for you.' }),
            manualUrl: 'https://open.larksuite.com/page/launcher?from=backend_oneclick',
            manualLinkLabel: t('remoteControl.guides.lark.step2LinkLabel', { defaultValue: 'Lark one-click app creation' }),
        }),
    },
    dingtalk: {
        id: 'dingtalk',
        name: 'DingTalk (钉钉)',
        description: t('remoteControl.guides.dingtalk.description', { defaultValue: 'Enterprise communication and collaboration' }),
        icon: '💬',
        BrandIcon: DingTalk,
        status: 'available',
        path: '/bots/dingtalk',
        color: '#0089ff',
        guide: (
            <Stack spacing={2}>
                <Box sx={{ bgcolor: 'info.lighter', p: 1.5, borderRadius: 1, border: '1px solid', borderColor: 'info.light' }}>
                    <Typography variant="body2" sx={{
                        color: "info.dark"
                    }}>
                        {t('remoteControl.guides.dingtalk.tip', { defaultValue: 'Tip: DingTalk uses Stream Mode - no public IP required. Configure traffic proxy as needed.' })}
                    </Typography>
                </Box>
                <Box>
                    <Typography variant="subtitle2" sx={{ fontWeight: 600, mb: 1 }}>
                        {t('remoteControl.guides.dingtalk.step1Title', { defaultValue: '1. Create a DingTalk bot' })}
                    </Typography>
                    <Typography variant="body2" component="div" sx={{
                        color: "text.secondary"
                    }}>
                        <Box component="ul" sx={{ pl: 2, m: 0 }}>
                            <li>
                                {t('remoteControl.guides.dingtalk.step1Visit', { defaultValue: 'Visit' })}{' '}
                                <Link href="https://open.dingtalk.com/" target="_blank">
                                    {t('remoteControl.guides.dingtalk.step1LinkLabel', { defaultValue: 'DingTalk Open Platform' })} <OpenInNew sx={{ fontSize: 10 }} />
                                </Link>
                            </li>
                            <li>{t('remoteControl.guides.dingtalk.step1CreateApp', { defaultValue: 'Create a new app - Add Robot capability' })}</li>
                            <li>{t('remoteControl.guides.dingtalk.step1Config', { defaultValue: 'Configuration:' })}</li>
                            <Box component="ul" sx={{ pl: 2 }}>
                                <li>{t('remoteControl.guides.dingtalk.step1StreamMode', { defaultValue: 'Toggle' })} <strong>Stream Mode</strong> {t('common.on', { defaultValue: 'On' })}</li>
                                <li>{t('remoteControl.guides.dingtalk.step1Permissions', { defaultValue: 'Permissions: Add necessary permissions for sending messages' })}</li>
                            </Box>
                            <li>{t('remoteControl.guides.dingtalk.step1GetKeys', { defaultValue: 'Get AppKey (Client ID) and AppSecret (Client Secret) from "Credentials"' })}</li>
                            <li>{t('remoteControl.guides.dingtalk.step1Publish', { defaultValue: 'Publish the app' })}</li>
                        </Box>
                    </Typography>
                </Box>
                <Box>
                    <Typography variant="subtitle2" sx={{ fontWeight: 600, mb: 1 }}>
                        {t('remoteControl.guides.telegram.step2Title', { defaultValue: '2. Add bot' })}
                    </Typography>
                    <Typography variant="body2" sx={{
                        color: "text.secondary"
                    }}>
                        {t('remoteControl.guides.dingtalk.step2Text', { defaultValue: 'Click "Add Bot" button above and fill in App Key and App Secret to create your bot.' })}
                    </Typography>
                </Box>
            </Stack>
        ),
    },
    weixin: {
        id: 'weixin',
        name: 'Weixin (微信)',
        description: t('remoteControl.guides.weixin.description', { defaultValue: 'China\'s most popular messaging platform' }),
        icon: '💚',
        BrandIcon: Weixin,
        status: 'available',
        path: '/bots/weixin',
        color: '#07c160',
        guide: (
            <Stack spacing={2}>
                <Box sx={{ bgcolor: 'info.lighter', p: 1.5, borderRadius: 1, border: '1px solid', borderColor: 'info.light' }}>
                    <Typography variant="body2" sx={{
                        color: "info.dark"
                    }}>
                        <strong>{t('remoteControl.guides.weixin.betaLabel', { defaultValue: 'Beta:' })}</strong> {t('remoteControl.guides.weixin.betaText', { defaultValue: 'Weixin integration is in beta. Please provide feedback for any issues.' })}
                    </Typography>
                </Box>
                <Box>
                    <Typography variant="subtitle2" sx={{ fontWeight: 600, mb: 1 }}>
                        {t('remoteControl.guides.weixin.step1Title', { defaultValue: '1. Install latest Weixin' })}
                    </Typography>
                    <Typography variant="body2" sx={{
                        color: "text.secondary"
                    }}>
                        {t('remoteControl.guides.weixin.step1TextBefore', { defaultValue: 'Make sure you have the latest version of' })}{' '}
                        <Link href="https://weixin.qq.com/" target="_blank">
                            Weixin <OpenInNew sx={{ fontSize: 10 }} />
                        </Link>{' '}
                        {t('remoteControl.guides.weixin.step1TextAfter', { defaultValue: 'installed on your device.' })}
                    </Typography>
                </Box>
                <Box>
                    <Typography variant="subtitle2" sx={{ fontWeight: 600, mb: 1 }}>
                        {t('remoteControl.guides.telegram.step2Title', { defaultValue: '2. Add bot' })}
                    </Typography>
                    <Typography variant="body2" sx={{
                        color: "text.secondary"
                    }}>
                        {t('remoteControl.guides.weixin.step2Text', { defaultValue: 'Click "Add Bot" button above and scan the QR code with Weixin to bind your account.' })}
                    </Typography>
                </Box>
            </Stack>
        ),
    },
    wecom: {
        id: 'wecom',
        name: 'WeCom (企业微信)',
        description: t('remoteControl.guides.wecom.description', { defaultValue: 'Enterprise Weixin communication platform' }),
        icon: '💼',
        BrandIcon: WeCom,
        status: 'available',
        path: '/bots/wecom',
        color: '#10a800',
        guide: (
            <Stack spacing={2}>
                <Box sx={{ bgcolor: 'info.lighter', p: 1.5, borderRadius: 1, border: '1px solid', borderColor: 'info.light' }}>
                    <Typography variant="body2" sx={{
                        color: "info.dark"
                    }}>
                        {t('remoteControl.guides.wecom.tip', { defaultValue: 'Tip: WeCom AI Bot uses WebSocket long connection — no public IP required.' })}
                    </Typography>
                </Box>
                <Box>
                    <Typography variant="subtitle2" sx={{ fontWeight: 600, mb: 1 }}>
                        {t('remoteControl.guides.wecom.step1Title', { defaultValue: '1. Open WeCom Admin Console' })}
                    </Typography>
                    <Typography variant="body2" sx={{
                        color: "text.secondary"
                    }}>
                        {t('remoteControl.guides.wecom.step1GoTo', { defaultValue: 'Go to' })}{' '}
                        <Link href="https://work.weixin.qq.com/wework_admin/frame#/aiHelper/list" target="_blank">
                            {t('remoteControl.guides.wecom.step1LinkLabel', { defaultValue: 'WeCom Admin → AI Assistant' })} <OpenInNew sx={{ fontSize: 10 }} />
                        </Link>
                        {' '}{t('remoteControl.guides.wecom.step1AndClick', { defaultValue: 'and click' })} <strong>{t('remoteControl.guides.wecom.createBot', { defaultValue: 'Create Bot' })}</strong> → <strong>{t('remoteControl.guides.wecom.createManually', { defaultValue: 'Create Manually' })}</strong>.
                    </Typography>
                </Box>
                <Box>
                    <Typography variant="subtitle2" sx={{ fontWeight: 600, mb: 1 }}>
                        {t('remoteControl.guides.wecom.step2Title', { defaultValue: '2. Create via API mode' })}
                    </Typography>
                    <Typography variant="body2" sx={{
                        color: "text.secondary"
                    }}>
                        {t('remoteControl.guides.wecom.step2TextBefore', { defaultValue: 'Scroll to the bottom of the page and click' })} <strong>{t('remoteControl.guides.wecom.step2LinkLabel', { defaultValue: 'Create via API Mode' })}</strong>.
                    </Typography>
                </Box>
                <Box>
                    <Typography variant="subtitle2" sx={{ fontWeight: 600, mb: 1 }}>
                        {t('remoteControl.guides.wecom.step3Title', { defaultValue: '3. Configure the bot' })}
                    </Typography>
                    <Typography variant="body2" component="div" sx={{
                        color: "text.secondary"
                    }}>
                        <Box component="ul" sx={{ pl: 2, m: 0 }}>
                            <li><strong>{t('remoteControl.guides.wecom.step3VisibleScope', { defaultValue: 'Visible Scope:' })}</strong> {t('remoteControl.guides.wecom.step3VisibleScopeText', { defaultValue: 'Set who can use the bot' })}</li>
                            <li><strong>{t('remoteControl.guides.wecom.step3ApiConfig', { defaultValue: 'API Config:' })}</strong> {t('remoteControl.guides.wecom.step3ApiConfigTextBefore', { defaultValue: 'Under Connection Method, select' })} <strong>{t('remoteControl.guides.wecom.step3LongConnection', { defaultValue: 'Long Connection' })}</strong></li>
                            <li>
                                {t('remoteControl.guides.wecom.step3SecretBefore', { defaultValue: 'In the Secret section, click' })} <strong>{t('remoteControl.guides.wecom.step3ClickToRetrieve', { defaultValue: 'Click to Retrieve' })}</strong>
                                {' '}{t('remoteControl.guides.wecom.step3SecretAfter', { defaultValue: '— save the' })} <strong>Bot ID</strong> {t('common.and', { defaultValue: 'and' })} <strong>Secret</strong>
                            </li>
                            <li><strong>{t('remoteControl.guides.wecom.step3Permissions', { defaultValue: 'Permissions:' })}</strong> {t('remoteControl.guides.wecom.step3PermissionsTextBefore', { defaultValue: 'Configure as needed, then click' })} <strong>{t('remoteControl.guides.wecom.step3Save', { defaultValue: 'Save' })}</strong></li>
                        </Box>
                    </Typography>
                </Box>
                <Box>
                    <Typography variant="subtitle2" sx={{ fontWeight: 600, mb: 1 }}>
                        {t('remoteControl.guides.wecom.step4Title', { defaultValue: '4. Add bot' })}
                    </Typography>
                    <Typography variant="body2" sx={{
                        color: "text.secondary"
                    }}>
                        {t('remoteControl.guides.wecom.step4Text', { defaultValue: 'Click "Add Bot" above and enter the Bot ID and Secret to connect.' })}
                    </Typography>
                </Box>
            </Stack>
        ),
    },
    qq: {
        id: 'qq',
        name: 'QQ',
        description: t('remoteControl.guides.qq.description', { defaultValue: 'Tencent instant messaging platform' }),
        icon: '🐧',
        BrandIcon: QQ,
        status: 'coming-soon',
        path: '/bots/qq',
        color: '#888',
        guide: buildComingSoonGuide(t, 'QQ'),
    },
    discord: {
        id: 'discord',
        name: 'Discord',
        description: t('remoteControl.guides.discord.description', { defaultValue: 'Voice, video, and text communication' }),
        icon: '🎮',
        BrandIcon: Discord,
        status: 'coming-soon',
        path: '/bots/discord',
        color: '#888',
        guide: buildComingSoonGuide(t, 'Discord'),
    },
    slack: {
        id: 'slack',
        name: 'Slack',
        description: t('remoteControl.guides.slack.description', { defaultValue: 'Business communication platform' }),
        icon: '💳',
        BrandIcon: Slack,
        status: 'coming-soon',
        path: '/bots/slack',
        color: '#888',
        guide: buildComingSoonGuide(t, 'Slack'),
    },
});

/** Localized platform guide for a single platform, recomputed on language switch. */
export function usePlatformGuide(platformId: string): PlatformGuideConfig | undefined {
    const { t, i18n } = useTranslation();
    return useMemo(() => buildPlatformGuides(t)[platformId], [t, i18n.language, platformId]);
}

/**
 * Plain-function platform display name lookup — for call sites that need a
 * name per id inside a loop (e.g. building tabs), where calling the
 * `usePlatformGuide` hook per-iteration would break the rules of hooks.
 * Callers get `t` from their own single `useTranslation()` call.
 */
export function platformDisplayName(platformId: string, t: TFunction): string {
    return buildPlatformGuides(t)[platformId]?.name || platformId;
}
