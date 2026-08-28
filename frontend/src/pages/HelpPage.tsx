import CardGrid from '@/components/CardGrid.tsx';
import { PageLayout } from '@/components/PageLayout.tsx';
import { ShortcutCard } from '@/components/ShortcutCard.tsx';
import { ProvidersCard } from '@/components/ProvidersCard.tsx';
import { useTranslation } from 'react-i18next';

// The Shortcut card spans full width like every other card; only its content
// is capped, matching System settings cards. ProvidersCard is left uncapped —
// its provider grid wants the room.
const SHORTCUT_CONTENT_MAX_WIDTH = 720;

/**
 * HelpPage — the lightbulb entry in the activity bar, replacing the old
 * standalone "Quick Add Provider" wand in that exact nav slot. This *is* the
 * product's onboarding front door now; ProvidersCard (the old Onboarding
 * page's content, unchanged, just no longer a full page of its own) is one
 * card among a small, growing set of easy-to-miss useful actions — Shortcut
 * first, Providers second. Each card is a standalone, re-entrant action (not
 * a step in a linear tour) — the page has no "done" state and nothing to
 * complete in order.
 */
const HelpPage = () => {
    const { t } = useTranslation();

    return (
        <PageLayout loading={false} title={t('help.title')} subtitle={t('help.description')}>
            <CardGrid>
                <ShortcutCard contentMaxWidth={SHORTCUT_CONTENT_MAX_WIDTH} />
                <ProvidersCard />
            </CardGrid>
        </PageLayout>
    );
};

export default HelpPage;
