package space

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"hephaestus-backend/pkg/types"

	"github.com/qiwentaidi/clients"
)

// ShodanIpReportHandler 获取 Shodan IP 报告
func ShodanIpReportHandler(c *gin.Context) {
	ip := c.Query("ip")
	if ip == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "IP 地址不能为空"})
		return
	}

	report, err := getShodanIpReport(ip)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "获取 Shodan 数据失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": report,
	})
}

// getShodanIpReport 获取 Shodan IP 报告
func getShodanIpReport(ip string) (*types.ShodanIpReport, error) {
	url := fmt.Sprintf("https://internetdb.shodan.io/%s", ip)
	resp, err := clients.SimpleGet(url, clients.NewRestyClient(nil, true))
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}

	var report types.ShodanIpReport
	if err := json.Unmarshal(resp.Body(), &report); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	// 设置 IP 字段（API 返回的数据中可能没有 IP 字段）
	if report.IP == "" {
		report.IP = ip
	}

	return &report, nil
}
