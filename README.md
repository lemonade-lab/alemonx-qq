# ALemonX QQ

ALemonX 的 QQ 内核管理插件。NapCat 与 LuckyLillia 使用独立的安装目录、状态、日志、进程组和 OneBot 配置；一个内核的操作不会修改另一个内核。

## 当前自动化能力

| 内核 | 平台 | 安装与运行 |
| --- | --- | --- |
| NapCat | Windows x64 | 下载官方 OneKey Release，受管安装与运行 |
| NapCat | Linux x64 / ARM64 | 纯 Go 下载与解压 NapCat Shell、官方 QQ DEB/RPM；系统图形环境不可用时自动回退到插件发布的兼容运行时 |
| NapCat | macOS | 仅关联现有 QQ 注入实例；工作台不修改 QQ、不会停止 QQ 或删除文件 |
| LuckyLillia | Windows x64、macOS Apple Silicon、Linux x64 / ARM64 | 下载官方 CLI Release，受管安装与运行 |
| LuckyLillia | macOS Intel 与其他平台 | 不支持自动安装 |

DEB、RPM、ZIP 与 TAR.XZ 的解压、入口校验、原子替换和失败回滚均由 Go 实现，不会下载或执行第三方安装脚本。安装是否成功只以解包结构、真实进程启动和 WebUI 就绪为准。

## 使用流程

1. 在工作台的系统插件页安装 QQ 插件。
2. 选择 NapCat 或 LuckyLillia，点击页面给出的下一步操作。
3. Linux 安装 NapCat 时会自动准备运行环境；若系统需要管理员授权，工作台会弹出一次密码框。密码只用于本次宿主固定操作，不保存、不传给插件或下载内容；系统环境无法准备时自动改用兼容运行时。
4. 启动后在内嵌 WebUI 中扫码登录，端口通过工作台的认证网关访问，不需要对外暴露 6099 或 3080。
5. OneBot 就绪后，选择一个已受工作台管理的 AlemonJS 机器人并同步 URL 与 Token。同步不自动重启机器人。

外部关联实例只可查看状态、二维码与 WebUI；取消关联不会删除外部目录。仅工作台创建且身份完整的受管安装可以启动、更新或卸载。

关联已有目录时点击“选择目录”即可调起主应用的系统目录选择器并自动回填。插件通过 `finder.pick` 能力请求其 `alx.json` 中声明的目录或文件选择器。

## 开发与发布

- 运行器：`runner/`；共享 Linux QQ 发布契约：`internal/qqruntime/`；Web：`frontend/`。
- `go test ./...`：运行全部 Go 测试。
- `make build`：构建五个平台的插件执行器；`make web`：构建 Web。
- `cmd/alx-ci` 代替原有 Python / Bash 发布脚本，负责清单、版本、构建与 E2E 记录。
- Release 可额外生成校验文件供发布方审计；插件运行时不读取、保存或依赖这些信息。
