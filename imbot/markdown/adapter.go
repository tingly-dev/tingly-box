package markdown

import (
	"github.com/tingly-dev/tingly-box/imbot/core"
)

// ToIMBotEntities converts markdown MessageEntity to imbot core.Entity
func ToIMBotEntities(entities []MessageEntity) []core.Entity {
	result := make([]core.Entity, len(entities))
	for i, ent := range entities {
		coreEnt := core.Entity{
			Type:   ent.Type,
			Offset: ent.Offset,
			Length: ent.Length,
			Data:   make(map[string]interface{}),
		}

		// Add optional fields to Data map
		if ent.URL != nil {
			coreEnt.Data["url"] = *ent.URL
		}
		if ent.Language != nil {
			coreEnt.Data["language"] = *ent.Language
		}

		result[i] = coreEnt
	}
	return result
}
