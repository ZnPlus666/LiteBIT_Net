# Linux 源码与构建 / Linux source and build

最终用户不需要本目录，也不需要安装 Go、Python 或 OpenSSL；使用两个 Linux 版本目录中附带的可执行程序即可。

End users do not need this directory, Go, Python or OpenSSL. Use the executables bundled in the two Linux release folders.

## 开发者 / Developers

仅编译开发时需要 Go 1.24+、POSIX shell 和 `sha256sum`。代码仅使用 Go 标准库，无外部模块。当前附带程序使用 Go 1.27.2 编译。

Building requires Go 1.24+, a POSIX shell and `sha256sum`. Only the Go standard library is used, with no external modules. The bundled binaries were built with Go 1.27.2.

在本目录运行 / Run in this directory:

```sh
go test -count=1 ./...
sh build.sh
```

构建脚本先运行主机可执行的测试，再使用 `CGO_ENABLED=0`、`GOAMD64=v1` 和 `GOARM64=v8.0` 为两种模式分别生成 Linux x64/ARM64 静态程序及校验清单。系统服务定义直接启动程序，不调用解释器、OpenSSL、curl 或浏览器。

The build script runs host-compatible tests, then builds Linux x64/ARM64 static executables and checksum lists for both modes using `CGO_ENABLED=0`, `GOAMD64=v1` and `GOARM64=v8.0`. Services start the executable directly, without an interpreter, OpenSSL, curl or a browser.

`crypto.go` 实现 SRun 参数和 AES-256-GCM 凭据加密；`portal.go` 实现认证、请求超时和凭据验证；`storage.go` 实现受限权限文件与日志；`systemd.go` 实现用户会话检查与服务管理；`app.go` 实现入口与安装回滚；`platform_linux.go` 实现 Linux 文件锁、禁止链接的文件打开和终端密码输入。

`crypto.go` implements SRun parameters and AES-256-GCM credential encryption; `portal.go` implements authentication, HTTP timeouts and credential verification; `storage.go` handles protected files and logs; `systemd.go` manages sessions/services; `app.go` implements commands and installation rollback; `platform_linux.go` implements Linux locking, no-follow file opening and terminal password input.

`platform_other.go` 只允许其他系统编译并运行离线开发测试，不提供 Windows/macOS 服务支持。`main_test.go` 使用虚构凭据、模拟服务与本机 HTTP 服务器；`platform_linux_test.go` 另验证真实 Linux 文件权限及只读状态。认证参考值位于 `../tests_linux/crypto_vectors.json`。

`platform_other.go` enables offline development tests on other hosts, not Windows/macOS service support. `main_test.go` uses synthetic credentials, mocked services and localhost HTTP servers. `platform_linux_test.go` additionally checks actual Linux permissions and read-only status. Authentication reference vectors are in `../tests_linux/crypto_vectors.json`.

真实 systemd 开机/用户登录启动和真实校园网认证需在 Linux 实机验证；单元测试通过不代表已完成这些验证。

Real systemd boot/user-login startup and campus authentication require Linux hardware validation; passing unit tests does not establish those results.
