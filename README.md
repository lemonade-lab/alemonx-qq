# QQ

[![CI](https://github.com/lemonade-lab/alemonx-qq/actions/workflows/ci.yml/badge.svg)](https://github.com/lemonade-lab/alemonx-qq/actions/workflows/ci.yml)

这是 ALemonX 的 QQ 内核管理插件，可在 alx 中分别管理 **NapCat** 与 **LuckyLillia**。两套内核的安装目录、进程、日志和配置彼此隔离。

- **安装 / 卸载** NapCat（自动下载、校验、解压）。
- **启动 / 停止 / 重启** NapCat。
- **看状态**：装没装、跑没跑、能不能连。
- **看日志**：NapCat 运行时的输出。
- **网络配置**：直接改 OneBot 的 HTTP / WebSocket 端口与 Token（改完重启生效）。
- **检查 / 升级**：对比最新版本，一键更新 NapCat。
- **扫码登录**：内嵌 NapCat 管理面板，直接在插件页扫码登录 QQ。
- **守护模式**：开启后 NapCat 意外退出会自动拉起（约 15 秒）。
- **LuckyLillia**：Linux ARM64 上从官方 Release 安装，启动前检查 Node.js 22+，并分别显示 WebUI、OneBot 与扫码登录状态。
- **机器人同步**：选择受工作台管理的 AlemonJS 机器人后，可同步 OneBot URL/Token；不会自动重启机器人。

NapCat 支持 Windows、Linux 和 macOS；LuckyLillia 自动安装仅支持 Linux ARM64。

## 安装插件

把 `alemonx-qq` 文件夹放进 ALemonX 的插件目录（同 ALemonX 其它插件），打开 ALemonX → 插件 → 点「QQ」。

## 第一次使用

1. 在右上角选择要管理的 QQ 内核；NapCat 与 LuckyLillia 的状态不会相互影响。
2. 打开插件，点「安装」→ 等待下载和解压（首次较大，请耐心）。
3. 点「启动」→ 状态变成运行中。
4. 切到「管理面板」页签，用手机 QQ **扫码登录**（建议用机器人专用小号）。
5. 登录后，OneBot 就绪；可在「网络配置」里设置端口和 Token，或同步到已选择的 AlemonJS 机器人。
6. 想让 NapCat 7×24 在线？点「开启守护」——仅在异常退出时自动拉起。

## 平台限制

- Windows：下载内置 QQ 的 Shell 包，解压即用。
- Linux：下载 Shell 包，需要 `unzip` 与 Node.js 18+。
- macOS：原生支持（无需 Docker）。NapCat 通过注入 QQ 应用运行，需先装 QQ（Mac App Store）与 Node.js 18+；安装按插件内引导手动完成（需在「系统设置 → 隐私与安全性 → App 管理」授权一次）。启动 = 打开 QQ，停止 = 退出 QQ。
- LuckyLillia：仅 Linux ARM64 自动安装，使用官方 `LLBot-CLI-linux-arm64.zip`；需要 Node.js 22+。其他平台可查看状态，但不会显示为可自动安装。

## 给开发者

- 逻辑在 `runner/`（Go），界面在 `frontend/`（React + Vite + Tailwind，对齐 alx 设计）。
- `make check`（测试）、`make web`（构建界面）、`make build`（构建二进制）、`make dist`（打包）。

## 安全

- 安装仅从对应内核的官方发布渠道下载；LuckyLillia 安装包会限制大小、校验 Zip 结构并拒绝越界路径。
- 启动/停止只操作当前选择的 QQ 内核进程，不触碰另一内核或系统其它部分。
