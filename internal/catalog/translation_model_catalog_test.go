package catalog

import "testing"

func TestTranslationAdapterRequiresCatalogedModel(t *testing.T) {
	for model, want := range map[string]string{
		"tencent/Hy-MT2-1.8B": "hunyuan_mt", "qwen-mt-flash": "qwen_mt",
		"local-mt": "", "not-qwen-mt-flash": "", "qwen-max": "",
	} {
		if got := TranslationAdapter(model); got != want {
			t.Errorf("%s: got %q, want %q", model, got, want)
		}
	}
}
