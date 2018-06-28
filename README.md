# Steam 2018 夏促刷分辅助 - golang

【已下架0.1.2～0.1.4， 请使用0.1.2～0.1.4的童鞋更新为0.2.0】
【0.2.0 - 重新从0.1.1分支出来的版本，仅仅增加了退出星球修复】 鉴于0.1.2～0.1.4都没有修复切换星球问题，还是还原回简单粗暴的把。管用的才是最好的。

- 自动恢复现有游戏
- 超精简, 单文件, 无依赖
- 占领进程信息输出
- 支持热切换星球, 网页切换星球后脚本将重新寻找适合战区
- 星球占领自动更换, 无星球自动加入目前占领进度最低的星球
- [0.0.6] STEAM_TOKEN环境变量(或者-token参数值) 支持多token, 以英文逗号(`,`)分隔(注意不能有空格)
- [0.1.0] 加入星域问题处理修复, 现在加入星域失败3次后将标记该星域进黑名单
- [0.1.0] 优化星球选择逻辑, 现在将每轮都将自动选择最佳星球的最佳星域加入
- [0.1.0] 修复了切换星球多次失败的bug

创意来源于 MapleRecall https://steamcn.com/t399390-1-1

# 使用指南
- 浏览器打开 https://steamcommunity.com/saliengame/gettoken 登陆并获取 token
- 用以下任一启动方式启动脚本

# 注意事项
- 0.1.0 修复了多个稳定性问题, 建议所有用户更新到该版本
- 仅供学习参考, 作者不对任何使用此工具造成的任何问题负责

## 0.0.6 多账号单实例示范
直接将环境变量`STEAM_TOKEN`设置为以英文逗号(`,`)分隔的多个token值即可(注意不能有空格)

docker
```bash
docker run --name salienbot --log-opt max-size=10m -d -e "STEAM_TOKEN=TOKEN1,TOKEN2,TOKEN3" azusa0127/salienbot-go
```
替换`TOKEN1,TOKEN2,TOKEN3`为需要设置的token值, 数量没有上限(但是过多可能会影响性能)
加入docker的`--log-opt max-size=10m`参数以限制日志文件大小为10m
单实例所以加入`--name salienbot`标记容器名以便获取日志

获取docker特定用户实时日志 (Linux/MacOS),
```
docker logs salienbot -f | grep 用户TOKEN前6位
```
`-f`选项跟随滚动, ctrl+c退出


Windows可执行文件
```bash
.\可执行文件名 -token TOKEN1,TOKEN2,TOKEN3
```

## Docker 启动 (推荐)
*更新*
直接在停止并删除现有docker`容器`和`镜像`后重新执行运行指令即可

*停止并删除docker容器和镜像*
```bash
docker rm -f $(docker ps -a -q --filter="ancestor=azusa0127/salienbot-go") && docker rmi -f azusa0127/salienbot-go
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


## 可执行文件
到 https://github.com/azusa0127/salienbot-go/releases 下载对应平台的可执行文件。

替换以下命令中的`你的TOKEN`和`可执行文件名`
Linux/Mac
```bash
./可执行文件名 -token 你的token
```

Windows
```bash
.\可执行文件名 -token 你的token
```


## 源码执行, 需要go环境
Linux/Mac
```bash
go run main.go -token 你的TOKEN
```

Windows
```bash
go run .\main.go -token 你的TOKEN
```

## 设置HTTPS代理
go使用`HTTP_PROXY`环境变量设置http代理, 所以只需要添加`HTTP_PROXY`环境变量值即可
docker
```bash
docker run -d -e "STEAM_TOKEN=你的TOKEN" -e "HTTP_PROXY=代理服务器地址和端口" azusa0127/salienbot-go
```

Mac/Linux
```bash
HTTP_PROXY=代理服务器地址和端口 ./可执行文件名 -token 你的token
```

Windows
```bash
cmd /C "set HTTP_PROXY=代理服务器地址和端口 && .\可执行文件名 -token 你的token"
```
