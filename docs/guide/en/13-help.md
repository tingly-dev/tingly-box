# Tips & Help

Path: `/help`

![Tips & Help](../images/help-page.png)

The **Help** lightbulb in the rail, just above **Star** (a link to the project on GitHub — the one place the app asks for a star). It is not the first-run landing: new installs open the last-used agent page, whose [Quick Start](./03-scenario-claude-code.md#quick-start) walks provider → model → install → configure. Help is where the same material stays reachable afterwards (the old `/onboarding` URL redirects here). Subtitle: *"A few easy-to-miss but useful things."*

## Quick Start

Help collects the easy-to-miss helpers. The most common errand: add a provider, or reopen a guide.

1. Click the **Help** lightbulb in the rail (just above **Star**). Every card starts collapsed — click a card's header to open it.
2. Need a provider? Open **Provider Catalog**, search for it (or paste a config snippet) and click it; the form that opens is the same as **Connect AI** ([Credentials Quick Start](./08-credentials.md#quick-start)).
3. Want a refresher on routing? Open **Routing & Tier Guides** and click **Direct Routing Guide**, **Smart Routing Guide** or **Tier Guide**.
4. Found a bug or have an idea? Open **Feedback** → **Open GitHub Issues**.

**Verify:** the guide dialog or provider form opens. Nothing here changes any setting by itself.

Five collapsible cards, **all collapsed when the page opens**, each a standalone action rather than a step in a linear tour — opening one doesn't close the others:

## Feedback

**Open GitHub Issues** opens the GitHub issue chooser — report a problem or share an idea. The same link is **Feedback** in the rail gear's Preferences menu.

## Desktop Shortcut

Hidden in the [desktop app](./23-desktop-app.md) (the app itself is the shortcut). In a browser, one click creates a desktop shortcut that restarts Tingly-Box and opens the web UI — it never auto-updates or auto-restarts the server on its own; creating the shortcut is the only thing this does. **Create Shortcut** / **Recreate** button inside.

## Provider Catalog

![Provider Catalog card](../images/onboarding.png)

Collapsed like the others until you open it. Embeds the same provider catalog as the **Connect AI** picker (search box + grouped cards by connection type — Custom, OAuth sign-in, Self-hosted, API key providers) — see [Credentials § Adding a Provider](./08-credentials.md#adding-a-provider-the-connect-ai-flow) for the full picker walkthrough. The card scrolls internally past a fixed height so it can't push the other cards off screen.

## Routing & Tier Guides

Three buttons that reopen the same diagrammed guides embedded elsewhere in the product, so there's one diagram to keep in sync, not several:
- **Direct routing guide**: the same [first-run Direct Routing Guide](./20-routing-rules.md#first-run-guide) that auto-opens once per user on any scenario page
- **Smart routing guide**: the Smart-mode counterpart
- **Tier guide**: explains tier-based failover (T0/T1/T2 priority and circuit breaking)

## Team Guide

A **How Team works** button that reopens the same [Team Guide dialog](./07-team.md#team-guide-dialog) available from any Team workspace page's help icon — what a Team is for, how it's isolated, and how to configure a Team Key.

---

## Related Pages

- [Credentials](./08-credentials.md)
- [Routing Rules & Plugins](./20-routing-rules.md)
- [Team](./07-team.md)
