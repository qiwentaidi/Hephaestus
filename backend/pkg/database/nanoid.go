package database

import "hephaestus-backend/pkg/nanoid"

// GenerateNanoid 生成唯一ID（简化版 nanoid）
func GenerateNanoid() string {
	return nanoid.Generate()
}
