package collect

import (
	"hephaestus-backend/pkg/database"
	"hephaestus-backend/pkg/types"
)

// Collection callers and startup migrations use the same data codec.
func ValidateCollectData(result types.CollectResult) error {
	return database.ValidateCollectData(result)
}

func encodeCollectData(result types.CollectResult) (string, error) {
	return database.EncodeCollectData(result)
}

func decodeCollectData(content string, count int) (types.RowData, error) {
	return database.DecodeCollectData(content, count)
}
