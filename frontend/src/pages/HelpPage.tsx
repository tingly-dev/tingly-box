import { useState } from 'react';
import { Button, Stack } from '@mui/material';
import { useTranslation } from 'react-i18next';
import CardGrid from '@/components/CardGrid.tsx';
import { PageLayout } from '@/components/PageLayout.tsx';
import { CollapsibleCard } from '@/components/CollapsibleCard.tsx';
import { ShortcutCard, shouldShowShortcutCard } from '@/components/ShortcutCard.tsx';
import { ProvidersCard } from '@/components/ProvidersCard.tsx';
import { EntryGuideDialog } from '@/components/tier/EntryGuideDialog';
import { TierGuideDialog } from '@/components/tier/TierGuideDialog';

// The Shortcut card spans full width like every other card; only its content
// is capped, matching System settings cards. ProvidersCard is left uncapped —
// its provider grid wants the room.
const SHORTCUT_CONTENT_MAX_WIDTH = 720;

// The provider catalog is unbounded (many key + OAuth providers) — cap it so
// this one section can't push Shortcut/Routing off screen; it scrolls
// internally past this height instead of growing the page.
const PROVIDERS_CONTENT_MAX_HEIGHT = 480;

type HelpSectionId = 'shortcut' | 'providers' | 'routing';

/**
 * HelpPage — the lightbulb entry in the activity bar, replacing the old
 * standalone "Quick Add Provider" wand in that exact nav slot. This *is* the
 * product's onboarding front door now.
 *
 * Each section is an accordion (CollapsibleCard): title + one-line summary
 * always visible, body behind a chevron. Cards here vary wildly in shape —
 * a two-line shortcut action next to a full provider catalog next to a set
 * of guide launchers — so keeping them all open at once made the page look
 * like several unrelated tools glued together. Providers is expanded by
 * default (OnboardingGate sends brand-new, provider-less installs straight
 * here to add one); every other section is a click away. Sections toggle
 * independently — each is a standalone, re-entrant action, not a step in a
 * linear tour, so opening one doesn't imply closing another.
 *
 * The routing/tier section doesn't redraw anything: it just surfaces the
 * same EntryGuideDialog / TierGuideDialog already embedded on the routing
 * and tier pages, so there's one diagram to keep in sync, not two.
 */
const HelpPage = () => {
    const { t } = useTranslation();
    const showShortcut = shouldShowShortcutCard();

    const [expanded, setExpanded] = useState<Set<HelpSectionId>>(new Set(['providers']));
    const toggle = (id: HelpSectionId) => setExpanded((prev) => {
        const next = new Set(prev);
        if (next.has(id)) next.delete(id); else next.add(id);
        return next;
    });

    const [entryGuideOpen, setEntryGuideOpen] = useState(false);
    const [entryGuideMode, setEntryGuideMode] = useState<'direct' | 'smart'>('direct');
    const [tierGuideOpen, setTierGuideOpen] = useState(false);
    const openEntryGuide = (mode: 'direct' | 'smart') => {
        setEntryGuideMode(mode);
        setEntryGuideOpen(true);
    };

    return (
        <PageLayout loading={false} title={t('help.title')} subtitle={t('help.description')}>
            <CardGrid>
                {showShortcut && (
                    <CollapsibleCard
                        title={t('help.shortcut.title')}
                        description={t('help.shortcut.description')}
                        expanded={expanded.has('shortcut')}
                        onToggle={() => toggle('shortcut')}
                        contentMaxWidth={SHORTCUT_CONTENT_MAX_WIDTH}
                    >
                        <ShortcutCard />
                    </CollapsibleCard>
                )}

                <CollapsibleCard
                    title={t('help.providers.title')}
                    description={t('help.providers.description')}
                    expanded={expanded.has('providers')}
                    onToggle={() => toggle('providers')}
                    contentMaxHeight={PROVIDERS_CONTENT_MAX_HEIGHT}
                >
                    <ProvidersCard />
                </CollapsibleCard>

                <CollapsibleCard
                    title={t('help.routing.title')}
                    description={t('help.routing.description')}
                    expanded={expanded.has('routing')}
                    onToggle={() => toggle('routing')}
                    contentMaxWidth={SHORTCUT_CONTENT_MAX_WIDTH}
                >
                    <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1.5}>
                        <Button variant="outlined" size="small" onClick={() => openEntryGuide('direct')}>
                            {t('help.routing.direct')}
                        </Button>
                        <Button variant="outlined" size="small" onClick={() => openEntryGuide('smart')}>
                            {t('help.routing.smart')}
                        </Button>
                        <Button variant="outlined" size="small" onClick={() => setTierGuideOpen(true)}>
                            {t('help.routing.tier')}
                        </Button>
                    </Stack>
                </CollapsibleCard>
            </CardGrid>

            <EntryGuideDialog open={entryGuideOpen} onClose={() => setEntryGuideOpen(false)} mode={entryGuideMode} />
            <TierGuideDialog open={tierGuideOpen} onClose={() => setTierGuideOpen(false)} />
        </PageLayout>
    );
};

export default HelpPage;
