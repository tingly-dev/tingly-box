package catalog

import (
	_ "embed"
	"encoding/json"
)

// These files describe native model semantics, not deployment or routing state.
// Sources: https://huggingface.co/tencent/Hy-MT2-1.8B
// and https://www.alibabacloud.com/help/en/model-studio/qwen-mt-api
//
//go:embed hunyuan.models.json
var hunyuanModelsJSON []byte

//go:embed qwen.models.json
var qwenModelsJSON []byte

var translationAdapters = loadTranslationAdapters()

func loadTranslationAdapters() map[string]string {
	adapters := make(map[string]string)
	for _, data := range [][]byte{hunyuanModelsJSON, qwenModelsJSON} {
		var models []struct {
			ID          string `json:"id"`
			Translation struct {
				Adapter string `json:"adapter"`
			} `json:"translation"`
		}
		if err := json.Unmarshal(data, &models); err != nil {
			panic(err)
		}
		for _, model := range models {
			adapters[model.ID] = model.Translation.Adapter
		}
	}
	return adapters
}

// TranslationAdapter returns verified native translation semantics for an exact
// upstream model ID. Unknown deployment aliases remain unsupported; never infer
// capabilities from a substring, provider name or a load-balancing setting.
func TranslationAdapter(model string) string { return translationAdapters[model] }
