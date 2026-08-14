# ALemonX QQ

ALemonX 的 QQ 内核管理插件。NapCat、LuckyLillia 与 SnowLuma 使用独立的安装目录、状态、日志、进程组和 OneBot 配置；一个内核的操作不会修改另一个内核。

## 支持范围

| 内核 | 平台 | 安装模式 | 登录边界 |
| --- | --- | --- | --- |
| NapCat | 以插件状态页为准 | 工作台受管安装 | 由 NapCat 流程提供登录引导 |
| LuckyLillia | 以官方 CLI 资产为准 | 工作台受管或关联已有目录 | 官方 Auth Token 后进入登录流程 |
| SnowLuma | Windows x64、Linux x64、Linux ARM64 | 下载官方完整原生包；不使用 Docker | 使用已经运行的本机 QQ 窗口扫码 |

SnowLuma 不是 Linux QQ 的安装器，也不会创建 VNC/noVNC 桌面。启动 SnowLuma 前必须已有同一系统用户、同一权限级别的 QQ 进程；Linux 还必须具备可用的 X11/Xvfb `DISPLAY` 和允许 Hook 注入的 ptrace 条件。macOS 没有上游 Darwin native addon，因此不支持 SnowLuma 原生注入。

SnowLuma 启动后会验证 WebUI，并从已生成且启用的 `config/onebot.json` WebSocket 配置读取 Token。没有 QQ、图形会话、注入权限或 OneBot Token 时，插件会报告阻断原因，不会把端口可达误报成登录成功。

官方参考：[Windows 原生部署](https://snowluma.github.io/en/guide/deploy/windows.html)、[Linux 原生部署](https://snowluma.github.io/guide/deploy/linux-manual.html)、[配置参考](https://snowluma.github.io/guide/configuration.html)。

## 验证与 E2E

默认 CI 运行测试、静态检查、清单校验、前端构建和跨平台编译。SnowLuma 真机 E2E 只在手动触发 `snowluma_e2e` 时运行，目标为 Windows x64、Linux x64、Linux ARM64。

该 E2E 入口会验证官方完整包和当前架构 native addon，然后调用受控 self-hosted runner 上配置的执行器。执行器和 QQ/X11 环境不在本仓库中；未配置对应 runner 标签与 `SNOWLUMA_E2E_COMMAND_JSON_<PLATFORM>` 时，不能把 E2E 视为已完成。具体报告契约见 [cmd/alx-ci/README.md](cmd/alx-ci/README.md)。
