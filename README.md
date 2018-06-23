# Steam 2018 夏促刷分辅助 - golang

- 自动恢复现有游戏
- 超精简, 单文件, 无依赖
- 占领进程信息输出

创意的方法来源于 MapleRecall https://steamcn.com/t399390-1-1

# 使用指南
- 浏览器打开 https://steamcommunity.com/saliengame/play/ 加入一个星球
- 浏览器打开 https://steamcommunity.com/saliengame/gettoken 获取 token
- 用以下任一启动方式启动脚本

## Docker 启动 (推荐)
### 前台
```bash
docker run -e "STEAM_TOKEN=你的TOKEN" azusa0127/salienbot-go
```
记得替换`你的TOKEN`为之前获取的token值

### 后台运行
```bash
docker run -d -e "STEAM_TOKEN=你的TOKEN" azusa0127/salienbot-go
```

可以运行多个实例对应多个token, 重复执行以上命令即可

### 停止并删除docker容器
```bash
docker rm -f $(docker ps -a -q --filter="ancestor=azusa0127/salienbot-go")
```

## 源码执行, 需要go环境
Linux/Mac
```bash
STEAM_TOKEN=你的TOKEN go run main.go
```

Windows
```bash
cmd /C "set STEAM_TOKEN=你的TOKEN && go run .\main.go"
```
