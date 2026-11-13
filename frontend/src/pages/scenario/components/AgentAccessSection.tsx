import { Box, ButtonBase, Card, Collapse, Tooltip, Typography } from '@mui/material';
import { ExpandMore as ExpandMoreIcon } from '@/components/icons';
import { useCallback, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { fontMono } from '@/theme/fonts';
import ConfigRow from '@/components/ConfigRow';
import { SCENARIO_HEADER_CONTENT_MAX_WIDTH } from './ScenarioCardHeader';
import { radius } from '@tingly/ui/theme/radius';

// Room kept at the card's right edge for the fold chevron, in both states, so
// the row's own actions (Local | Docker, copy) never run underneath it at narrow
// widths and the first row sits in the same place whether folded or open.
const CHEVRON_GUTTER = 6;

const PREF_KEY = (scenario: string) => `agent-access-expanded-${scenario}`;

const readPref = (scenario: string): boolean | null => {
    try {
        const v = localStorage.getItem(PREF_KEY(scenario));
        return v === 'true' ? true : v === 'false' ? false : null;
    } catch {
        return null; // storage unavailable: just follow the default
    }
};

/**
 * Whether the connection section is open. It always sits in the same place on
 * the page; what changes is how much of it shows. `defaultExpanded` (not
 * connected yet, or the client's config is out of date) decides until the user
 * opens or closes it themselves — that choice is remembered on this machine.
 */
export function useAccessExpanded(scenario: string, defaultExpanded: boolean) {
    const [pref, setPref] = useState<boolean | null>(() => readPref(scenario));
    const expanded = pref ?? defaultExpanded;
    const toggle = useCallback(() => {
        const next = !expanded;
        setPref(next);
        try {
            localStorage.setItem(PREF_KEY(scenario), String(next));
        } catch { /* best-effort */ }
    }, [expanded, scenario]);
    return { expanded, toggle };
}

export interface AgentAccessSectionProps {
    expanded: boolean;
    onToggle: () => void;
    /** The full Base URL as the client should use it (Local / Docker applied). */
    url: string;
    /** The open section: Base URL, API Key, model mode... */
    children: React.ReactNode;
    /**
     * Replaces the folded URL row's content — for pages whose folded line is
     * not the Base URL (a profile page folds to its launch command).
     */
    foldedContent?: React.ReactNode;
    /**
     * Shown under the folded URL row — only for state that deviates from the
     * default (a plugin switched on), so the folded card
     * stays one quiet line until something actually differs.
     */
    foldedExtras?: React.ReactNode;
}

/**
 * The Agent page's connection card, fixed between the status row and the
 * rules. Open: the original card (Base URL, API Key, ...). Folded: a single
 * line with the Base URL; the whole line opens it. The fold chevron sits at
 * the card's right edge, as on the rule cards.
 */
export const AgentAccessSection: React.FC<AgentAccessSectionProps> = ({ expanded, onToggle, url, children, foldedContent, foldedExtras }) => {
    const { t } = useTranslation();
    const label = t(expanded ? 'agentAccess.collapse' : 'agentAccess.expand');
    // Folded and open are two different buttons, so toggling would drop keyboard
    // focus to the page: hand it to the new one.
    const toggleEl = useRef<HTMLElement | null>(null);
    const toggleRef = useCallback((el: HTMLElement | null) => { toggleEl.current = el; }, []);
    const refocus = useRef(false);
    useEffect(() => {
        if (refocus.current) {
            refocus.current = false;
            toggleEl.current?.focus();
        }
    }, [expanded]);
    const handleToggle = () => {
        refocus.current = true;
        onToggle();
    };
    // The same fold control as the rule card's header (ModelRequestHeader): a
    // chevron, turned over while open, grey until hovered.
    const chevron = (
        <ExpandMoreIcon
            sx={{
                transition: 'transform 0.2s',
                transform: expanded ? 'rotate(180deg)' : 'rotate(0deg)',
                fontSize: '1.1rem',
                color: 'text.secondary',
                flexShrink: 0,
            }}
        />
    );
    return (
        <Card
            sx={{
                position: 'relative',
                borderRadius: radius.card,
                border: '1px solid',
                borderColor: 'divider',
                boxShadow: 'none',
                backgroundColor: 'background.paper',
                '& .agent-access-toggle:hover svg': { color: 'text.primary' },
            }}
        >
            {expanded ? (
                <Tooltip title={label}>
                    <ButtonBase
                        ref={toggleRef}
                        className="agent-access-toggle"
                        onClick={handleToggle}
                        aria-expanded
                        aria-label={label}
                        sx={{ position: 'absolute', top: 12, right: 22, zIndex: 1, p: 0.25, borderRadius: radius.row }}
                    >
                        {chevron}
                    </ButtonBase>
                </Tooltip>
            ) : (
                // The open card's first row, unchanged (same wrappers, same ConfigRow
                // grid) — only the actions on its right are left out — so folding and
                // unfolding never moves it. The whole row is the button.
                <ButtonBase
                    ref={toggleRef}
                    component="div"
                    className="agent-access-toggle"
                    onClick={handleToggle}
                    aria-expanded={false}
                    aria-label={label}
                    sx={{ display: 'block', width: '100%', textAlign: 'left', '&:hover': { backgroundColor: 'action.hover' } }}
                >
                    {/* With extras below, the bottom padding moves to the extras
                        wrapper so the URL→extras gap matches the card's own
                        top/bottom rhythm (8 + 4 on each side of the row). */}
                    <Box sx={{ pt: 1, pb: foldedExtras ? 0.5 : 1, pr: CHEVRON_GUTTER }}>
                        <Box sx={{ maxWidth: SCENARIO_HEADER_CONTENT_MAX_WIDTH }}>
                            <Box sx={{ px: 2, py: 0.5 }}>
                                {foldedContent ?? (
                                    <ConfigRow
                                        tabs={[{
                                            key: 'baseUrl',
                                            label: 'Base URL',
                                            content: (
                                                <Typography
                                                    variant="subtitle2"
                                                    sx={{ fontFamily: fontMono, color: 'primary.main', boxSizing: 'border-box', display: 'block', maxWidth: '100%', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', py: 1 }}
                                                >
                                                    {url}
                                                </Typography>
                                            ),
                                        }]}
                                        activeTab="baseUrl"
                                        onTabChange={() => {}}
                                    />
                                )}
                            </Box>
                        </Box>
                    </Box>
                    <Box sx={{ position: 'absolute', top: 12, right: 22, p: 0.25, display: 'flex' }}>{chevron}</Box>
                </ButtonBase>
            )}
            {!expanded && foldedExtras && (
                // Same row wrappers as the folded URL row above — just without
                // its ButtonBase, so the extras' own controls stay clickable.
                <Box sx={{ pt: 0.5, pb: 1, pr: CHEVRON_GUTTER }}>
                    <Box sx={{ maxWidth: SCENARIO_HEADER_CONTENT_MAX_WIDTH }}>
                        <Box sx={{ px: 2, py: 0.5 }}>{foldedExtras}</Box>
                    </Box>
                </Box>
            )}
            <Collapse in={expanded} timeout={0} unmountOnExit>
                <Box sx={{ pt: 1, pb: 1, pr: CHEVRON_GUTTER }}>{children}</Box>
            </Collapse>
        </Card>
    );
};

export default AgentAccessSection;
