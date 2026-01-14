/**
 * Shared types for RuleGraphV2 and TabTemplatePage
 */

export interface ConfigProvider {
    uuid: string;
    provider: string;
    model: string;
    isManualInput?: boolean;
    weight?: number;
    active?: boolean;
    time_window?: number;
}

export interface ConfigRecord {
    uuid: string;
    requestModel: string;
    responseModel: string;
    active: boolean;
    providers: ConfigProvider[];
    description?: string;
}

export interface Rule {
    uuid: string;
    scenario: string;
    request_model: string;
    response_model?: string;
    active?: boolean;
    description?: string;
    services?: Array<{
        id?: string;
        uuid?: string;
        provider: string;
        model: string;
        weight?: number;
        active?: boolean;
        time_window?: number;
    }>;
}
