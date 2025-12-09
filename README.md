# zima_cron

一个可在本地与 CasaOS 上运行的轻量任务调度器，支持定时执行真实命令、查看日志、暂停/恢复、删除任务，以及中英双语界面。

## 功能
- 定时任务：支持 `间隔分钟` 与 `cron` 表达式（5 字段）
- 真实命令执行：通过 `/bin/sh -lc` 执行，2 分钟超时，输出采集与截断保护
- 日志查看与清空：每次执行都会产生一条日志记录
- 任务管理：运行一次、暂停/恢复、删除任务
- 多语言界面：中文与英文，可在页面右上角切换

## 目录结构
- 后端：`cmd/zimaos-cron/main.go`
- 前端：`raw/usr/share/casaos/www/modules/zimaos_cron/`
- CasaOS 模块：`raw/usr/share/casaos/modules/zimaos_cron.json`
- Systemd 服务：`raw/usr/lib/systemd/system/zimaos-cron.service`
- RAW 打包：`.github/workflows/release-raw.yml`

## 本地快速开始
- 构建并运行后端：
  ```bash
  go build -o ./zimaos-cron ./cmd/zimaos-cron && ./zimaos-cron
  ```
- 启动静态文件服务并打开前端：
  ```bash
  python3 -m http.server 8000
  # 浏览器打开
  http://localhost:8000/raw/usr/share/casaos/www/modules/zimaos_cron/index.html
  ```

## HTTP API（CasaOS 路由前缀）
- 路由前缀：`/zimaos_cron`
- 列出任务：`GET /zimaos_cron/tasks`
- 创建任务：`POST /zimaos_cron/tasks`
  - Body：`{"name":"…","command":"…","type":"interval|cron","interval_min":60,"cron_expr":"*/5 * * * *"}`
- 获取任务：`GET /zimaos_cron/tasks/{id}`
- 运行一次：`POST /zimaos_cron/tasks/{id}/run`
- 暂停/恢复：`POST /zimaos_cron/tasks/{id}/toggle`
- 查看日志：`GET /zimaos_cron/tasks/{id}/logs`
- 清空日志：`POST /zimaos_cron/tasks/{id}/logs/clear`
- 删除任务：`DELETE /zimaos_cron/tasks/{id}`

示例：
```bash
curl -sX POST http://localhost:8989/zimaos_cron/tasks \
  -H 'Content-Type: application/json' \
  -d '{"name":"echo","command":"echo hello","type":"interval","interval_min":1}'

curl -sX POST http://localhost:8989/zimaos_cron/tasks/{id}/run
curl -s http://localhost:8989/zimaos_cron/tasks/{id}/logs
```

## 前端要点
- 语言切换与回退：英文下若翻译缺失，状态字样统一显示 `Success/Fail`
- 字段映射：
  - 下次执行：`next_run_at`
  - 上次结果：`last_result`（包含 `success` 与 `message`）
- 文件位置：`raw/usr/share/casaos/www/modules/zimaos_cron/app.js`

## RAW 打包与发布
- 手动打包：
  ```bash
  mksquashfs raw/ zimaos_cron.raw -noappend -comp gzip
  ```
- GitHub Actions：见 `.github/workflows/release-raw.yml`
  - 构建二进制：`GOOS=linux GOARCH=amd64 go build -o ./raw/usr/bin/zimaos-cron ./cmd/zimaos-cron`
  - 打包产物：`zimaos_cron.raw`

## CasaOS 部署提示
- 将 RAW 安装到 CasaOS 后，模块入口位置：`/modules/zimaos_cron/index.html`
- 后端服务由平台随机端口监听，并通过网关路由前缀 `/zimaos_cron` 转发（前端同源调用）

## 行为与限制
- 命令执行：`/bin/sh -lc`，超时 2 分钟
- 输出截断：消息超过 4000 字符会被截断
- 暂停状态不显示“下次执行”时间；恢复后重新计算

## 许可证
MIT（如需调整请在根目录替换许可证文件）
