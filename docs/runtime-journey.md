# QQ 核心运行旅程

本插件把 NapCat 与 LuckyLillia 的安装和扫码流程视为一个可观测的运行旅程，而不是“进程存在即成功”。`status` 会返回 `journey`，其中的 `phase`、`detail` 和 `nextAction` 是前端的唯一引导依据。

```
环境预检 → 下载并验证 → 原子安装 → 必填凭据 → 启动进程 → WebUI/二维码 → OneBot 就绪
```

`NapCat` 在 Linux 上会先验证 glibc、包管理器、Xvfb、XKB 和 QQ Electron 动态库；依赖不足时不得启动 QQ。`LuckyLillia` 在当前官方 CLI 要求 Auth Token 时，会在“安装完成”后进入 `needs-auth-token`，保留已验证的安装目录和私有配置，不会伪装成 WebUI 超时或回滚安装。

每个启动、安装、重装、更新和 Token 保存操作都有独立的操作日志。界面在任务运行期间轮询该日志，因此“当前执行详情”显示的是本次任务的真实阶段；核心长期日志只用于进程输出和故障排查。

阶段契约：

| 阶段 | 含义 | 下一步 |
| --- | --- | --- |
| `install` | 尚未安装 | 安装 |
| `repair` | 安装目录不完整 | 重新安装 |
| `needs-auth-token` | LuckyLillia 官方 CLI 尚未授权 | 填写 Auth Token |
| `start` / `starting` | 已具备启动条件，正在等待真实 WebUI 监听 | 启动 / 查看实时日志 |
| `scan-qq` | WebUI 已就绪，尚未完成 QQ 登录 | 扫码或打开管理页 |
| `connecting` | 已登录，OneBot 尚未监听 | 查看实时日志 |
| `ready` | QQ 与 OneBot 均就绪 | 配置并同步机器人 |

不复制移动端/proot 的实现细节：移动端的内嵌 Ubuntu、固定 ARM64 包和其历史 LLBot 启动方式不构成 CentOS 原生部署的契约。桌面/服务器插件只依据当前官方资产、实际进程退出状态、监听端口和可读取的日志推进阶段。
