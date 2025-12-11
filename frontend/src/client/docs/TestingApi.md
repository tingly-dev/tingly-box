# TestingApi

All URIs are relative to *http://localhost:8080*

|Method | HTTP request | Description|
|------------- | ------------- | -------------|
|[**apiV1ProbePost**](#apiv1probepost) | **POST** /api/v1/probe | Test a rule configuration by sending a sample request|

# **apiV1ProbePost**
> RuleResponse apiV1ProbePost(request)

Test a rule configuration by sending a sample request

### Example

```typescript
import {
    TestingApi,
    Configuration,
    ProbeRequest
} from './api';

const configuration = new Configuration();
const apiInstance = new TestingApi(configuration);

let request: ProbeRequest; //Request body

const { status, data } = await apiInstance.apiV1ProbePost(
    request
);
```

### Parameters

|Name | Type | Description  | Notes|
|------------- | ------------- | ------------- | -------------|
| **request** | **ProbeRequest**| Request body | |


### Return type

**RuleResponse**

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

