import { useNavigate, useParams } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { useEffect, useMemo, useState } from 'react';
import { Telegram, Feishu, Lark, DingTalk, Weixin, WeCom, QQ, Discord, Slack } from '@/components/BrandIcons';
import { PlatformSideNav } from '@/components/bot';
import SecondaryNavLayout from '@/components/SecondaryNavLayout';
import { BOT_PLATFORM_IDS, platformDisplayName, usePlatformGuide } from '@/constants/platformGuides';
import { api } from '@/services/api';
import PlatformRemoteAgentPage from './PlatformRemoteAgentPage';

const PLATFORM_BRAND_ICONS: Record<string, React.ComponentType<{ size?: number }>> = {
    telegram: Telegram,
    feishu: Feishu,
    lark: Lark,
    dingtalk: DingTalk,
    weixin: Weixin,
    wecom: WeCom,
    qq: QQ,
    discord: Discord,
    slack: Slack,
};

// RemoteAgentPage is the nav-facing entry for the Remote purpose: ONE sidebar
// row (under the "Bots" rail icon, alongside Overview and Notify), with
// platform selection moved in-page — a vertical picker on the left, the same
// interaction and visual language as the platform rows the global Sidebar
// used to show, just scoped to this page instead of the app-wide nav. The
// routes it switches between (/remote-agent/:platform) are unchanged — deep
// links and the BotCard purpose chip still work. PlatformRemoteAgentPage
// itself is untouched: same guide, add, and pairing behavior it already had.
const RemoteAgentPage = () => {
    const { platform = 'weixin' } = useParams<{ platform: string }>();
    const navigate = useNavigate();
    const { t } = useTranslation();
    const platformName = usePlatformGuide(platform)?.name || platform;

    // Active/total per platform, for the side nav subtitles — mirrors what
    // the old per-platform sidebar rows showed.
    const [counts, setCounts] = useState<Record<string, { active: number; total: number }>>({});
    useEffect(() => {
        let cancelled = false;
        api.getImBotSettingsList().then((data) => {
            if (cancelled || !data?.success || !Array.isArray(data.settings)) return;
            const map: Record<string, { active: number; total: number }> = {};
            for (const bot of data.settings) {
                if (!bot?.platform) continue;
                const slot = map[bot.platform] ?? (map[bot.platform] = { active: 0, total: 0 });
                slot.total++;
                if (bot.enabled) slot.active++;
            }
            setCounts(map);
        }).catch(() => {});
        return () => { cancelled = true; };
    }, [platform]);

    const sideNavItems = useMemo(() => BOT_PLATFORM_IDS.map((id) => {
        const BrandIcon = PLATFORM_BRAND_ICONS[id];
        const c = counts[id];
        return {
            id,
            label: platformDisplayName(id, t),
            icon: <BrandIcon size={20} />,
            subtitle: c && c.total > 0 ? `active ${c.active} / ${c.total}` : undefined,
        };
    }), [t, counts]);

    return (
        <SecondaryNavLayout
            nav={
                <PlatformSideNav
                    items={sideNavItems}
                    value={BOT_PLATFORM_IDS.includes(platform as typeof BOT_PLATFORM_IDS[number]) ? platform : ''}
                    onChange={(next) => navigate(`/remote-agent/${next}`)}
                />
            }
        >
            <PlatformRemoteAgentPage platformId={platform} platformName={platformName} />
        </SecondaryNavLayout>
    );
};

export default RemoteAgentPage;
