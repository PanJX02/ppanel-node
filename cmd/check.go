package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/perfect-panel/ppanel-node/api/panel"
	"github.com/perfect-panel/ppanel-node/common/portmap"
	"github.com/perfect-panel/ppanel-node/conf"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "检查端口冲突",
	Run: func(cmd *cobra.Command, args []string) {
		configPath, _ := cmd.Flags().GetString("config")
		c := conf.New()
		if err := c.LoadFromPath(configPath); err != nil {
			fmt.Printf("读取配置文件失败: %s\n", err)
			os.Exit(1)
		}

		if c.Nodes == nil || len(c.Nodes) == 0 {
			fmt.Println("未配置任何后端节点")
			return
		}

		usedRanges := make([]portmap.PortRangeRecord, 0)
		hasConflict := false

		for _, apiConf := range c.Nodes {
			apiDir := apiConf.ApiDir()
			nodeConfigPath := filepath.Join(apiDir, "node.json")
			
			nodeData, err := os.ReadFile(nodeConfigPath)
			if err != nil {
				// 如果文件不存在, 可能是还没启动过, 跳过检测
				continue
			}

			var serverconfig panel.ServerConfigResponse
			if err := json.Unmarshal(nodeData, &serverconfig); err != nil {
				continue
			}

			if serverconfig.Data == nil || serverconfig.Data.Protocols == nil {
				continue
			}

			for _, proto := range *serverconfig.Data.Protocols {
				if !proto.Enable {
					continue
				}

				// 检查主端口冲突
				if err := portmap.CheckPortRangeConflict(usedRanges, proto.Port, proto.Port, apiConf.ApiHost, fmt.Sprintf("%s/port", proto.Type)); err != nil {
					fmt.Printf("[冲突] %s\n", err)
					hasConflict = true
				} else {
					usedRanges = append(usedRanges, portmap.PortRangeRecord{
						Start: proto.Port,
						End:   proto.Port,
						Host:  apiConf.ApiHost,
						Label: fmt.Sprintf("%s/port", proto.Type),
					})
				}

				// 检查跳跃端口冲突
				if proto.HopPorts != "" {
					start, end, err := portmap.ParseHopPorts(proto.HopPorts)
					if err == nil {
						if err := portmap.CheckPortRangeConflict(usedRanges, start, end, apiConf.ApiHost, fmt.Sprintf("%s/hop", proto.Type)); err != nil {
							fmt.Printf("[冲突] %s\n", err)
							hasConflict = true
						} else {
							usedRanges = append(usedRanges, portmap.PortRangeRecord{
								Start: start,
								End:   end,
								Host:  apiConf.ApiHost,
								Label: fmt.Sprintf("%s/hop", proto.Type),
							})
						}
					}
				}
			}
		}

		if hasConflict {
			os.Exit(1)
		}
		fmt.Println("未检测到端口冲突")
	},
}

func init() {
	checkCmd.Flags().StringP("config", "c", "/etc/PPanel-node/config.yml", "config file path")
	command.AddCommand(checkCmd)
}
