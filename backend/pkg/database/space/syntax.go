package space

import (
	"fmt"
	"hephaestus-backend/pkg/database"
)

// SaveSpaceEngineSyntax 保存空间引擎语法
func SaveSpaceEngineSyntax(source, name, content string) (*database.SpaceEngineSyntax, error) {
	db, err := database.GetDB()
	if err != nil {
		return nil, fmt.Errorf("获取数据库连接失败: %w", err)
	}

	syntax := &database.SpaceEngineSyntax{
		Source:  source,
		Name:    name,
		Content: content,
	}

	if err := db.Create(syntax).Error; err != nil {
		return nil, fmt.Errorf("保存语法失败: %w", err)
	}

	return syntax, nil
}

// GetSpaceEngineSyntaxList 获取指定来源的语法列表
func GetSpaceEngineSyntaxList(source string) ([]database.SpaceEngineSyntax, error) {
	db, err := database.GetDB()
	if err != nil {
		return nil, fmt.Errorf("获取数据库连接失败: %w", err)
	}

	var syntaxes []database.SpaceEngineSyntax
	query := db
	if source != "" {
		query = query.Where("source = ?", source)
	}

	if err := query.Find(&syntaxes).Error; err != nil {
		return nil, fmt.Errorf("查询语法列表失败: %w", err)
	}

	return syntaxes, nil
}

// DeleteSpaceEngineSyntax 删除空间引擎语法
func DeleteSpaceEngineSyntax(id uint) error {
	db, err := database.GetDB()
	if err != nil {
		return fmt.Errorf("获取数据库连接失败: %w", err)
	}

	result := db.Delete(&database.SpaceEngineSyntax{}, id)
	if result.Error != nil {
		return fmt.Errorf("删除语法失败: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("未找到指定的语法记录")
	}

	return nil
}

// DeleteSpaceEngineSyntaxByName 根据来源和名称删除语法
func DeleteSpaceEngineSyntaxByName(source, name string) error {
	db, err := database.GetDB()
	if err != nil {
		return fmt.Errorf("获取数据库连接失败: %w", err)
	}

	result := db.Where("source = ? AND name = ?", source, name).Delete(&database.SpaceEngineSyntax{})
	if result.Error != nil {
		return fmt.Errorf("删除语法失败: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("未找到指定的语法记录")
	}

	return nil
}
