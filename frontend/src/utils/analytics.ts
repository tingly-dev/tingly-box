/**
 * Analytics Utility
 *
 * Privacy-preserving Google Analytics 4 integration
 *
 * Data Collected (ONLY):
 * - Page views: Which pages you visit
 * - System info: App version, OS type (detected automatically)
 *
 * Privacy Features:
 * - No IP addresses (anonymizeIp enabled)
 * - No personal identifiers
 * - No request content or API keys
 * - No provider names or model names
 * - No error messages
 * - Opt-in only
 * - Users can see exactly what data is collected
 */

interface AnalyticsConfig {
    measurementId: string;
    enabled: boolean;
    debug?: boolean;
}

interface AnalyticsEvent {
    name: string;
    parameters?: Record<string, string | number | boolean>;
}

/**
 * Detect OS from user agent
 * Returns a simple OS identifier for analytics
 */
function detectOS(): string {
    const userAgent = navigator.userAgent.toLowerCase();

    if (userAgent.includes('mac os x') || userAgent.includes('macintosh')) {
        return 'macOS';
    }
    if (userAgent.includes('windows')) {
        return 'Windows';
    }
    if (userAgent.includes('linux')) {
        return 'Linux';
    }
    if (userAgent.includes('android')) {
        return 'Android';
    }
    if (userAgent.includes('iphone') || userAgent.includes('ipad') || userAgent.includes('ios')) {
        return 'iOS';
    }

    return 'Unknown';
}

class Analytics {
    private config: AnalyticsConfig | null = null;
    private isInitialized = false;
    private consentGiven = false;
    private os: string = detectOS();
    private version: string = 'unknown';
    private systemInfoSent = false;

    /**
     * Initialize analytics with configuration
     * Only actually loads GA script if consent is given
     */
    initialize(config: AnalyticsConfig): void {
        this.config = config;

        // Check if user has given consent
        const storedConsent = localStorage.getItem('analytics_consent');
        this.consentGiven = storedConsent === 'true';

        if (this.config.enabled && this.consentGiven) {
            this.loadGA();
        }
    }

    /**
     * Set application version
     * Call this when version info is available
     */
    setVersion(version: string): void {
        this.version = this.sanitizeVersion(version);

        if (this.config?.debug) {
            console.log('[Analytics] Version set:', this.version);
        }
    }

    /**
     * Load Google Analytics script
     * Only called after user consent
     */
    private loadGA(): void {
        if (this.isInitialized || !this.config) {
            return;
        }

        try {
            // Load gtag.js
            const script = document.createElement('script');
            script.async = true;
            script.src = `https://www.googletagmanager.com/gtag/js?id=${this.config.measurementId}`;
            document.head.appendChild(script);

            // Initialize gtag
            const initScript = document.createElement('script');
            initScript.textContent = `
                window.dataLayer = window.dataLayer || [];
                function gtag(){dataLayer.push(arguments);}
                gtag('js', new Date());
                gtag('config', '${this.config.measurementId}', {
                    'anonymize_ip': true,
                    'allow_google_signals': false,
                    'send_page_view': false
                });
            `;
            document.head.appendChild(initScript);

            this.isInitialized = true;

            if (this.config.debug) {
                console.log('[Analytics] GA4 initialized with measurement ID:', this.config.measurementId);
            }
        } catch (error) {
            console.error('[Analytics] Failed to load GA:', error);
        }
    }

    /**
     * Grant consent and load analytics
     */
    grantConsent(): void {
        this.consentGiven = true;
        localStorage.setItem('analytics_consent', 'true');

        if (this.config?.enabled && !this.isInitialized) {
            this.loadGA();
        }

        // Update GA consent
        if (this.isInitialized) {
            this.updateConsent(true);
            // Send system info when consent is granted
            this.sendSystemInfo();
        }

        this.trackEvent('analytics_consent_granted');
    }

    /**
     * Revoke consent
     * Note: This only stops future tracking. Data already sent to GA cannot be deleted from client-side.
     */
    revokeConsent(): void {
        this.consentGiven = false;
        localStorage.setItem('analytics_consent', 'false');

        if (this.isInitialized) {
            this.updateConsent(false);
        }

        if (this.config?.debug) {
            console.log('[Analytics] Consent revoked');
        }
    }

    /**
     * Check if user has given consent
     */
    hasConsent(): boolean {
        return this.consentGiven;
    }

    /**
     * Update GA consent settings
     */
    private updateConsent(granted: boolean): void {
        if (typeof window !== 'undefined' && (window as any).gtag) {
            (window as any).gtag('consent', 'update', {
                analytics_storage: granted ? 'granted' : 'denied',
            });
        }
    }

    /**
     * Track a page view
     * Only sends if consent is given
     */
    trackPageView(path: string, title?: string): void {
        if (!this.consentGiven || !this.isInitialized) {
            return;
        }

        try {
            if (typeof window !== 'undefined' && (window as any).gtag) {
                (window as any).gtag('event', 'page_view', {
                    page_path: path,
                    page_title: title || document.title,
                    os: this.os,
                    app_version: this.version,
                    // Don't send location or other PII
                });

                // Send system info once if not yet sent
                if (!this.systemInfoSent) {
                    this.sendSystemInfo();
                }
            }

            if (this.config?.debug) {
                console.log('[Analytics] Page view:', { path, title, os: this.os, app_version: this.version });
            }
        } catch (error) {
            console.error('[Analytics] Failed to track page view:', error);
        }
    }

    /**
     * Send system info event (sent once per session)
     */
    private sendSystemInfo(): void {
        if (this.systemInfoSent || !this.consentGiven || !this.isInitialized) {
            return;
        }

        this.trackEvent('system_info', {
            os: this.os,
            app_version: this.version,
        });

        this.systemInfoSent = true;

        if (this.config?.debug) {
            console.log('[Analytics] System info sent:', { os: this.os, app_version: this.version });
        }
    }

    /**
     * Track a custom event
     * Only sends if consent is given
     *
     * @param eventName - The name of the event
     * @param parameters - Event parameters (no PII)
     */
    trackEvent(eventName: string, parameters?: AnalyticsEvent['parameters']): void {
        if (!this.consentGiven || !this.isInitialized) {
            return;
        }

        try {
            if (typeof window !== 'undefined' && (window as any).gtag) {
                (window as any).gtag('event', eventName, parameters || {});
            }

            if (this.config?.debug) {
                console.log('[Analytics] Event:', { eventName, parameters });
            }
        } catch (error) {
            console.error('[Analytics] Failed to track event:', error);
        }
    }

    /**
     * Track application start
     */
    trackAppStart(version: string, mode: 'cli' | 'webui' | 'gui' | 'slim'): void {
        this.trackEvent('app_start', {
            version: this.sanitizeVersion(version),
            mode: mode,
        });
    }

    /**
     * Track API call (without sensitive data)
     */
    trackAPICall(scenario: 'openai' | 'anthropic' | 'claude_code', apiStyle: string): void {
        this.trackEvent('api_call', {
            scenario: scenario,
            api_style: apiStyle,
        });
    }

    /**
     * Track error (without sensitive error messages)
     */
    trackError(category: string, component: string): void {
        this.trackEvent('error', {
            error_category: category,
            component: component,
        });
    }

    /**
     * Track feature usage
     */
    trackFeature(feature: string, action: string): void {
        this.trackEvent('feature_usage', {
            feature: feature,
            action: action,
        });
    }

    /**
     * Sanitize version to remove any potentially sensitive info
     */
    private sanitizeVersion(version: string): string {
        // Just keep major.minor.patch format if possible
        const match = version.match(/^v?\d+\.\d+\.\d+/);
        return match ? match[0] : 'unknown';
    }

    /**
     * Get preview of what data would be sent
     * Shows actual detected OS and version
     */
    getDataPreview(): string {
        const actualOS = this.os;
        const actualVersion = this.version;
        return JSON.stringify([
            {
                event: 'page_view',
                parameters: {
                    page_path: '/dashboard',
                    page_title: 'Tingly Box - Usage Dashboard',
                    os: actualOS,
                    app_version: actualVersion,
                },
                note: 'Sent when you navigate to a page',
            },
            {
                event: 'page_view',
                parameters: {
                    page_path: '/system',
                    page_title: 'Tingly Box - System',
                    os: actualOS,
                    app_version: actualVersion,
                },
                note: 'Sent when you navigate to a page',
            },
            {
                event: 'system_info',
                parameters: {
                    os: actualOS,
                    app_version: actualVersion,
                },
                note: 'Sent once when you grant consent',
            },
        ], null, 2);
    }
}

// Export singleton instance
export const analytics = new Analytics();

// Export types
export type { AnalyticsConfig, AnalyticsEvent };
