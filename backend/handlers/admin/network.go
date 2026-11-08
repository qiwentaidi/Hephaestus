package admin

import (
	"net"
	"net/http"

	"github.com/gin-gonic/gin"
)

// NetworkCard 网卡信息
type NetworkCard struct {
	Name string `json:"name"`
	IP   string `json:"ip"`
}

// GetNetworkCardsHandler 获取本地网卡信息
func GetNetworkCardsHandler(c *gin.Context) {
	networks, err := getNetworkCards()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "获取网卡信息失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": networks,
	})
}

// getNetworkCards 获取本地网卡信息
func getNetworkCards() ([]NetworkCard, error) {
	var networks []NetworkCard

	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			switch v := addr.(type) {
			case *net.IPNet:
				if v.IP.To4() != nil {
					networks = append(networks, NetworkCard{
						Name: iface.Name,
						IP:   v.IP.String(),
					})
				}
			}
		}
	}

	return networks, nil
}

