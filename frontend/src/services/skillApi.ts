// Skill management control-plane API: skill locations (add/remove/refresh/
// discover/import) and skill content reads.
import {controlApi} from './openapi';

export const skillApi = {
    // ============================================
    // Skill Management API
    // ============================================

    // Get all skill locations
    getSkillLocations: async (): Promise<any> =>
        controlApi((client, headers) => client.GET('/api/v2/skill-locations', {headers})),

    // Add a new skill location
    addSkillLocation: async (data: {
        name: string;
        path: string;
        ide_source: string;
    }): Promise<any> =>
        controlApi((client, headers) => client.POST('/api/v2/skill-locations', {
            headers,
            body: data
        })),

    // Remove a skill location
    removeSkillLocation: async (id: string): Promise<any> =>
        controlApi((client, headers) => client.DELETE('/api/v2/skill-locations/{id}', {
            headers,
            params: {path: {id}}
        })),

    // Refresh/scan a skill location
    refreshSkillLocation: async (id: string): Promise<any> =>
        controlApi((client, headers) => client.POST('/api/v2/skill-locations/{id}/refresh', {
            headers,
            params: {path: {id}}
        })),

    // Discover IDEs with skills
    discoverIdes: async (): Promise<any> =>
        controlApi((client, headers) => client.GET('/api/v2/skill-locations/discover', {headers})),

    // Import discovered skill locations
    importSkillLocations: async (locations: any[]): Promise<any> =>
        controlApi((client, headers) => client.POST('/api/v2/skill-locations/import', {
            headers,
            body: {locations}
        })),

    // Get skill content with file content
    // NOTE: query params (location_id, skill_id, skill_path) are not yet documented in the OpenAPI spec.
    getSkillContent: async (locationId: string, skillId: string, skillPath?: string): Promise<any> =>
        controlApi((client, headers) => client.GET('/api/v2/skill-content', {
            headers,
            params: {query: {
                location_id: locationId,
                ...(skillId && {skill_id: skillId}),
                ...(skillPath && {skill_path: skillPath}),
            } as any},
        })),
};
