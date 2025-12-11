# DefaultsApi

All URIs are relative to *http://localhost:8080*

|Method | HTTP request | Description|
|------------- | ------------- | -------------|
|[**apiV1DefaultsGet**](#apiv1defaultsget) | **GET** /api/v1/defaults | Get default request configurations|
|[**apiV1DefaultsPost**](#apiv1defaultspost) | **POST** /api/v1/defaults | Set default request configurations|

# **apiV1DefaultsGet**
> DefaultsResponse apiV1DefaultsGet()

Get default request configurations

### Example

```typescript
import {
    DefaultsApi,
    Configuration
} from './api';

const configuration = new Configuration();
const apiInstance = new DefaultsApi(configuration);

const { status, data } = await apiInstance.apiV1DefaultsGet();
```

### Parameters
This endpoint does not have any parameters.


### Return type

**DefaultsResponse**

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

# **apiV1DefaultsPost**
> SetRuleResponse apiV1DefaultsPost(request)

Set default request configurations

### Example

```typescript
import {
    DefaultsApi,
    Configuration,
    SetDefaultsRequest
} from './api';

const configuration = new Configuration();
const apiInstance = new DefaultsApi(configuration);

let request: SetDefaultsRequest; //Request body

const { status, data } = await apiInstance.apiV1DefaultsPost(
    request
);
```

### Parameters

|Name | Type | Description  | Notes|
|------------- | ------------- | ------------- | -------------|
| **request** | **SetDefaultsRequest**| Request body | |


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

