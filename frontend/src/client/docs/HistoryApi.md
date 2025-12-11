# HistoryApi

All URIs are relative to *http://localhost:8080*

|Method | HTTP request | Description|
|------------- | ------------- | -------------|
|[**apiV1HistoryGet**](#apiv1historyget) | **GET** /api/v1/history | Get request history|

# **apiV1HistoryGet**
> HistoryResponse apiV1HistoryGet()

Get request history

### Example

```typescript
import {
    HistoryApi,
    Configuration
} from './api';

const configuration = new Configuration();
const apiInstance = new HistoryApi(configuration);

const { status, data } = await apiInstance.apiV1HistoryGet();
```

### Parameters
This endpoint does not have any parameters.


### Return type

**HistoryResponse**

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

