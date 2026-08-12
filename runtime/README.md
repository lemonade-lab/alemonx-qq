# NapCat Linux 受管兼容运行环境

这不是 Docker 镜像，也不会在用户机器执行安装脚本。发布产物是两个受管
`tar.zst` 文件：

- `alemonx-qq-runtime-linux-amd64-glibc.tar.zst`
- `alemonx-qq-runtime-linux-arm64-glibc.tar.zst`

每个压缩包的根目录必须包含 `alx-runtime.json`、可执行的 `bin/Xvfb`、
可执行的 glibc 动态加载器和 `lib/`。运行器会在解压前验证 Release SHA-256，
解压时拒绝路径逃逸和链接文件，随后再验证描述文件、入口、加载器和内容指纹。

`alx-runtime.json` 示例：

```json
{
  "id": "linux-amd64-glibc-v1",
  "platform": "linux-amd64",
  "xvfb": "bin/Xvfb",
  "loader": "lib/ld-linux-x86-64.so.2",
  "libraryPath": "lib"
}
```

运行时构建在 GitHub 原生 Linux x64 / ARM64 工作机自动完成：工作流安装 Xvfb，
再调用仓库 Go 工具收集其动态库、生成 SBOM、SHA-256 并实际启动验证。两个运行时资产与 QQ 插件包一起附加到同一个正式 `v*` Release，避免插件发布后再寻找不存在的第二个 Release。它必须把 Xvfb 与运行 QQ 所需的用户态图形库一同打包；不得
写入系统目录、调用 `sudo`、依赖 Docker 或要求用户安装系统包。
