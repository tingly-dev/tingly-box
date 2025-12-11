import { keysToCamelCase, toCamelCase } from './camelCase';

// Test data from the example
const testData = {
    request_model: "",
    response_model: "",
    services: null,
    current_service_index: 0,
    tactic: "round_robin",
    active: false,
    provider: "openai"
};

const nestedTestData = {
    data: [
        {
            request_model: "",
            response_model: "",
            services: [
                {
                    provider: "",
                    model: "",
                    weight: 1,
                    active: true,
                    time_window: 300,
                    stats: {
                        service_id: "",
                        request_count: 0,
                        last_used: "0001-01-01T00:00:00Z",
                        window_start: "0001-01-01T00:00:00Z",
                        window_request_count: 0,
                        window_tokens_consumed: 0,
                        window_input_tokens: 0,
                        window_output_tokens: 0,
                        time_window: 0
                    }
                }
            ],
            current_service_index: 0,
            tactic: "round_robin",
            active: true
        }
    ],
    success: true
};

// Test functions
console.log('Testing camelCase conversion:');
console.log('Single object:', keysToCamelCase(testData));
console.log('\nNested object:', JSON.stringify(keysToCamelCase(nestedTestData), null, 2));

// Test individual string conversion
console.log('\nTesting string conversion:');
console.log('request_model ->', toCamelCase('request_model'));
console.log('current_service_index ->', toCamelCase('current_service_index'));
console.log('window_request_count ->', toCamelCase('window_request_count'));