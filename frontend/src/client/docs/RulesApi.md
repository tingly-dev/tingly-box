# RulesApi

All URIs are relative to *http://localhost:8080*

|Method | HTTP request | Description|
|------------- | ------------- | -------------|
|[**apiV1RuleUuidDelete**](#apiv1ruleuuiddelete) | **DELETE** /api/v1/rule/:uuid | Delete a rule configuration|
|[**apiV1RuleUuidGet**](#apiv1ruleuuidget) | **GET** /api/v1/rule/:uuid | Get specific rule by UUID|
|[**apiV1RuleUuidPost**](#apiv1ruleuuidpost) | **POST** /api/v1/rule/:uuid | Create or update a rule configuration|
|[**apiV1RulesGet**](#apiv1rulesget) | **GET** /api/v1/rules | Get all configured rules|

# **apiV1RuleUuidDelete**
> DeleteRuleResponse apiV1RuleUuidDelete()

Delete a rule configuration

### Example

```typescript
import {
    RulesApi,
    Configuration
} from './api';

const configuration = new Configuration();
const apiInstance = new RulesApi(configuration);

const { status, data } = await apiInstance.apiV1RuleUuidDelete();
```

### Parameters
This endpoint does not have any parameters.


### Return type

**DeleteRuleResponse**

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
|**200** | Successful response |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **apiV1RuleUuidGet**
> RuleResponse apiV1RuleUuidGet()

Get specific rule by UUID

### Example

```typescript
import {
    RulesApi,
    Configuration
} from './api';

const configuration = new Configuration();
const apiInstance = new RulesApi(configuration);

const { status, data } = await apiInstance.apiV1RuleUuidGet();
```

### Parameters
This endpoint does not have any parameters.


### Return type

**RuleResponse**

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
|**200** | Successful response |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **apiV1RuleUuidPost**
> SetRuleResponse apiV1RuleUuidPost(request)

Create or update a rule configuration

### Example

```typescript
import {
    RulesApi,
    Configuration,
    SetRuleRequest
} from './api';

const configuration = new Configuration();
const apiInstance = new RulesApi(configuration);

let request: SetRuleRequest; //Request body

const { status, data } = await apiInstance.apiV1RuleUuidPost(
    request
);
```

### Parameters

|Name | Type | Description  | Notes|
|------------- | ------------- | ------------- | -------------|
| **request** | **SetRuleRequest**| Request body | |


### Return type

**SetRuleResponse**

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: application/json
 - **Accept**: application/json


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
|**200** | Successful response |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **apiV1RulesGet**
> RulesResponse apiV1RulesGet()

Get all configured rules

### Example

```typescript
import {
    RulesApi,
    Configuration
} from './api';

const configuration = new Configuration();
const apiInstance = new RulesApi(configuration);

const { status, data } = await apiInstance.apiV1RulesGet();
```

### Parameters
This endpoint does not have any parameters.


### Return type

**RulesResponse**

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
|**200** | Successful response |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

