# Steam 2018 夏促刷分辅助 - golang

[更新 0.0.2 - 星球热切换支持]

- 自动恢复现有游戏
- 超精简, 单文件, 无依赖
- 占领进程信息输出
- [0.0.2] 支持热切换星球, 网页切换星球后脚本将重新寻找适合星球

创意的方法来源于 MapleRecall https://steamcn.com/t399390-1-1

# 使用指南
- 浏览器打开 https://steamcommunity.com/saliengame/play/ 加入一个星球
- 浏览器打开 https://steamcommunity.com/saliengame/gettoken 获取 token
- 用以下任一启动方式启动脚本

# 注意事项
- 0.0.1 版本热切换星球会报错, 请更新0.0.2版本或者切换星球后重新运行工具
- 仅供学习参考, 作者不对任何使用此工具造成的任何问题负责

## Docker 启动 (推荐)
*更新*
0.0.1版本网页热切换星球后会报错,请用以下方法更新docker image后重新运行
```bash
docker pull azusa0127/salienbot-go
```

*前台*
```bash
docker run -e "STEAM_TOKEN=你的TOKEN" azusa0127/salienbot-go
```
记得替换`你的TOKEN`为之前获取的token值

*后台运行*
```bash
docker run -d -e "STEAM_TOKEN=你的TOKEN" azusa0127/salienbot-go
```

可以运行多个实例对应多个token, 重复执行以上命令即可

*停止并删除docker容器*
```bash
docker rm -f $(docker ps -a -q --filter="ancestor=azusa0127/salienbot-go")
```

## 可执行文件
到 https://github.com/azusa0127/salienbot-go/releases 下载对应平台的可执行文件。

替换以下命令中的`你的TOKEN`和`可执行文件名`
Linux/Mac
```bash
STEAM_TOKEN=你的TOKEN ./可执行文件名
```

Windows
```bash
cmd /C "set STEAM_TOKEN=你的TOKEN && .\可执行文件名"
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

## 设置HTTPS代理
go使用`HTTP_PROXY`环境变量设置http代理, 所以只需要添加`HTTP_PROXY`环境变量值即可
docker
```bash
docker run -d -e "STEAM_TOKEN=你的TOKEN" -e "HTTP_PROXY=代理服务器地址和端口" azusa0127/salienbot-go
```

Mac/Linux
```bash
STEAM_TOKEN=你的TOKEN HTTP_PROXY=代理服务器地址和端口 ./可执行文件名
```

Windows
```bash
cmd /C "set STEAM_TOKEN=你的TOKEN && HTTP_PROXY=代理服务器地址和端口 && 可执行文件名"
```
