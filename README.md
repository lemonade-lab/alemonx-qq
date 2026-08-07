# QQ 机器人

[![CI](https://github.com/lemonade-lab/alemonx-qq/actions/workflows/ci.yml/badge.svg)](https://github.com/lemonade-lab/alemonx-qq/actions/workflows/ci.yml)

这是 ALemonX 的一个插件，帮你在 alx 界面里**安装和运行 NapCat**——NapCat 是让 QQ 号变成一个机器人的底层程序。装好后，在插件界面里点几下就能：

- **安装 / 卸载** NapCat（自动下载、校验、解压）。
- **启动 / 停止 / 重启** NapCat。
- **看状态**：装没装、跑没跑、能不能连。
- **看日志**：NapCat 运行时的输出。
- **网络配置**：直接改 OneBot 的 HTTP / WebSocket 端口与 Token（改完重启生效）。
- **检查 / 升级**：对比最新版本，一键更新 NapCat。
- **扫码登录**：内嵌 NapCat 管理面板，直接在插件页扫码登录 QQ。
- **守护模式**：开启后 NapCat 意外退出会自动拉起（约 15 秒）。

支持 Windows、Linux 和 macOS。

## 安装插件

把 `alemonx-qq` 文件夹放进 ALemonX 的插件目录（同 ALemonX 其它插件），打开 ALemonX → 插件 → 点「QQ 机器人」。

## 第一次使用

1. 打开插件，点「安装」→ 等待下载和解压（首次较大，请耐心）。
2. 点「启动」→ 状态变成运行中。
3. 切到「管理面板」页签，用手机 QQ **扫码登录**（建议用机器人专用小号）。
4. 登录后，NapCat 就跑起来了。
5. 想让别的程序（如 NoneBot、Koishi）接入这个 QQ 机器人？切到「网络配置」页签，设置 HTTP / WebSocket 的端口和 Token，重启 NapCat 生效，再把地址和 Token 填到你的程序里。
6. 想让它 7×24 在线？点「开启守护」——NapCat 意外退出会自动拉起。

## 平台限制

- Windows：下载内置 QQ 的 Shell 包，解压即用。
- Linux：下载 Shell 包，需要 `unzip` 与 Node.js 18+。
- macOS：原生支持（无需 Docker）。NapCat 通过注入 QQ 应用运行，需先装 QQ（Mac App Store）与 Node.js 18+；安装按插件内引导手动完成（需在「系统设置 → 隐私与安全性 → App 管理」授权一次）。启动 = 打开 QQ，停止 = 退出 QQ。

## 给开发者

- 逻辑在 `runner/`（Go），界面在 `frontend/`（React + Vite + Tailwind，对齐 alx 设计）。
- `make check`（测试）、`make web`（构建界面）、`make build`（构建二进制）、`make dist`（打包）。

## 安全

- 安装仅从 NapCat 官方 GitHub Release 下载并校验。
- 启动/停止只操作 NapCat 自身进程，不触碰系统其它部分。
