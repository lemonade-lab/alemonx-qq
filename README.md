# ALemonX QQ

ALemonX 的 QQ 内核管理插件。NapCat 与 LuckyLillia 使用独立的安装目录、状态、日志、进程组和 OneBot 配置；一个内核的操作不会修改另一个内核。

## 当前自动化能力

| 内核 | 平台 | 安装与运行 |
| --- | --- | --- |
| NapCat | Windows x64 | 下载官方 OneKey Release，校验 SHA-256，受管安装与运行 |
| NapCat | Linux x64 / ARM64 | 纯 Go 下载、校验和解压 NapCat Shell 与官方 QQ DEB/RPM；运行时依赖由工作台先提示并按需授权安装 |
| NapCat | macOS | 仅关联现有 QQ 注入实例；工作台不修改 QQ、不会停止 QQ 或删除文件 |
| LuckyLillia | Windows x64、macOS Apple Silicon、Linux x64 / ARM64 | 下载官方 CLI Release，校验 SHA-256，受管安装与运行 |
| LuckyLillia | macOS Intel 与其他平台 | 不支持自动安装 |

Linux QQ 运行时只接受工作台内置的腾讯官方包 URL 和已审核 SHA-256。DEB、RPM、ZIP 与 TAR.XZ 的解压、入口校验、原子替换和失败回滚均由 Go 实现，不会下载或执行第三方安装脚本。

## 使用流程

1. 在工作台的系统插件页安装 QQ 插件。
2. 选择 NapCat 或 LuckyLillia，点击页面给出的下一步操作。
3. Linux 首次安装 NapCat 时，如缺少系统依赖，先确认影响并在工作台密码框输入一次 sudo 密码；密码只用于宿主固定的依赖命令，不保存、不传给插件或下载内容。
4. 启动后在内嵌 WebUI 中扫码登录，端口通过工作台的认证网关访问，不需要对外暴露 6099 或 3080。
5. OneBot 就绪后，选择一个已受工作台管理的 AlemonJS 机器人并同步 URL 与 Token。同步不自动重启机器人。

外部关联实例只可查看状态、二维码与 WebUI；取消关联不会删除外部目录。仅工作台创建且身份完整的受管安装可以启动、更新或卸载。

关联已有目录时点击“选择目录”即可调起主应用的系统原生目录选择器并自动回填。该能力由主应用的受限 `/api/v1/system/picker` 提供：插件只能请求其 `alx.json` 中声明的目录或文件选择器，远程或反代访问不会打开本机窗口。

## 开发与发布

- 运行器：`runner/`；共享 Linux QQ 发布契约：`internal/qqruntime/`；Web：`frontend/`。
- `go test ./...`：运行全部 Go 测试。
- `make build`：构建五个平台的插件执行器；`make web`：构建 Web。
- `cmd/alx-ci` 代替原有 Python / Bash 发布脚本，负责清单、版本、证据和 E2E 记录校验。
- Release 与 E2E 证据必须绑定具体官方 Asset 与 SHA-256；Linux NapCat 额外绑定 QQ 运行包的名称与 SHA-256。
