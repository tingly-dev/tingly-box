# SetRuleRequest


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**active** | **boolean** | Field active | [default to undefined]
**current_service_index** | **number** | Field current_service_index | [default to undefined]
**default_model** | **string** | Field default_model | [optional] [default to undefined]
**provider** | **string** | Field provider | [optional] [default to undefined]
**request_model** | **string** | Field request_model | [default to undefined]
**response_model** | **string** | Field response_model | [default to undefined]
**services** | **Array&lt;object&gt;** | Field services | [default to undefined]
**tactic** | **string** | Field tactic | [default to undefined]
**tactic_params** | **object** | Field tactic_params | [optional] [default to undefined]
**uuid** | **string** | Field uuid | [default to undefined]

## Example

```typescript
import { SetRuleRequest } from './api';

const instance: SetRuleRequest = {
    active,
    current_service_index,
    default_model,
    provider,
    request_model,
    response_model,
    services,
    tactic,
    tactic_params,
    uuid,
};
```

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)
