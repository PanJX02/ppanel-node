# PPanel-node

A PPanel node server based on xray-core, modified from v2node.  
一个基于xray内核的PPanel节点服务端，修改自v2node

## 二改功能说明 (Custom Features)

本项目在原版 [ppanel-node](https://github.com/perfect-panel/ppanel-node) 的基础上进行了增强：

1. **多后端并发支持**: 配置文件中的 `Nodes` 支持配置多个面板地址。程序启动后会同时连接多个后端并独立运行。
2. **配置目录隔离**: 根据每个后端的 `ApiHost` 自动在 `/etc/PPanel-node/` 下创建独立的子目录，隔离存放各自的 `node.json` 和 `core.json` 运行时配置。
3. **本地配置锁定**: 支持 `LocalConfig` 选项。开启后，若本地已存在配置，将优先使用本地配置启动，方便本地修改。
4. **端口冲突检测**: 自动检测不同后端之间是否存在监听端口冲突，并提供预警。
5. **内核增强**: 针对 Hysteria2 等协议的特殊配置（如 ALPN）进行了优化处理。

## 软件安装

### 一键安装

```
wget -N https://raw.githubusercontent.com/PanJX02/ppanel-node/master/scripts/install.sh && bash install.sh
```

## 构建
``` bash
GOEXPERIMENT=jsonv2 go build -v -o ./node -trimpath -ldflags "-s -w -buildid="
```

