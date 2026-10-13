# LiteBIT_Net：北京理工大学校园网自动登录工具

适用于Windows系统的北京理工大学校园网自动登录工具：

创建后台任务，定期查询深澜SRun门户的认证状态，并在未认证时自动重新登录。

Beijing Institute of Technology Campus Network Automatic Login Tool for Windows System:


Create backend tasks to regularly check the authentication status of the Shenlan SRun portal and automatically log in again if not authenticated.

**适用于防止校园网意外断联导致的远程控制无法连接**。

**Suitable for preventing remote control from being unable to connect due to unexpected disconnection of campus network.**

如果该工具对你有用，请不要忘记**点亮star**⭐和**将该工具推荐给朋友**！

If the tool is useful to you, don't forget to **light up the star**⭐ and **recommend this tool to friends**！

## 如何使用 / How to use

###  运行前提 / Requirements

- 电脑操作系统为 Windows 10/11。

  The computer must run Windows 10 or 11.

- 可用的校园网账号和密码。

  A valid campus-network account and password are required.

- 网卡必须已通过有线网或 Wi-Fi 连接至校园网。但注意，**电脑处于睡眠、休眠及关机时工具无法正常工作**，如有需要请调整设备的自动睡眠、休眠及关机情况。

  The network adapter must already be connected to the campus network through Ethernet or Wi-Fi. **The tool cannot operate while the computer is asleep, hibernating or shut down.** Adjust automatic sleep, hibernation and shutdown settings if necessary.

- 注意：
当前版本针对已核对的 SRun V1.18/B2022·0802 门户，默认服务器为 `http://10.0.0.55`，参数为 `ac_id=8`、`n=200`、`type=1`、`double_stack=0`、`ignore=2`。它不是直接适用于其他学校的通用客户端；其他部署需先核对并调整对应参数。

- Note:
This version targets the verified SRun V1.18/B2022·0802 portal at `http://10.0.0.55`, with `ac_id=8`, `n=200`, `type=1`, `double_stack=0` and `ignore=2`. It is not a universal client for other schools; verify and adjust the parameters before using it with another deployment.

### 版本选择 / Choosing a version

项目包含两个可独立安装的版本，请根据需求选择。

The project includes two independently installable versions. Choose the one that fits your needs.

- **关联用户版**：设备由多人共享使用且你不想他人在使用时也自动登录你的校园网。

  **User-linked version:** for a shared computer when you do not want your campus account to be used automatically during other users' sessions.

- **关联电脑版**：设备仅自己使用或不介意他人在使用时自动登录你的校园网；需要开机后无需操作也可以自动登录校园网。

  **PC-linked version:** for a computer you use alone, or when you accept automatic use of your campus account across Windows users and want unattended login after boot.

具体区别如下：

The differences are listed below:


| 项目 / Item | 关联用户版 / User-linked `LiteBIT_Net_link2user` | 关联电脑版 / PC-linked `LiteBIT_Net_link2PC` |
| --- | --- | --- |
| 启动条件 / Starts when | 配置任务的 Windows 用户登录后 / After the configuring Windows user signs in | Windows 开机约 60 秒后 / About 60 seconds after Windows boots |
| 未登录 Windows 账户时 / At the Windows sign-in screen | 不运行 / Does not run | 运行 / Runs |
| 执行账户 / Runs as | 配置任务的 Windows 用户 / The configuring Windows user | 内置 SYSTEM 服务账户 / The built-in SYSTEM service account |
| 中止条件 / Stops when | 创建任务的 Windows 账户注销 / The Windows account that created the task logs off | 设备关闭 / The PC is shut down |
| 安装权限 / Installation permissions | 通常无需管理员，系统策略可能限制 / Usually no administrator privileges required; system policy may restrict installation | 需要管理员权限 / Administrator privileges required |
| 登录凭据 / Login credentials | 为配置任务的 Windows 用户保存一组 / One set for the configuring Windows user | 为整台电脑保存一组，不为每个 Windows 用户分别保存 / One set for the entire PC, not a separate set for each Windows user |
| 密码保护 / Password protection | 当前用户范围的 Windows DPAPI / Current-user Windows DPAPI | 当前电脑范围的 Windows DPAPI，目录限 SYSTEM 和管理员访问 / Local-machine Windows DPAPI; directories restricted to SYSTEM and administrators |

### 工具安装 / Installation

1.选择需要的版本，下载并打开对应文件夹：关联用户版打开`LiteBIT_Net_link2user`，关联电脑版打开`LiteBIT_Net_link2PC`。

Choose a version and open its folder: `LiteBIT_Net_link2user` for the user-linked version, or `LiteBIT_Net_link2PC` for the PC-linked version.

2.关联用户版双击运行`install_link2user.cmd`，关联电脑版右键`install_link2PC.cmd`并选择“以管理员身份运行”。

For link2user, double-click `install_link2user.cmd`. For link2PC, right-click `install_link2PC.cmd` and choose **Run as administrator**.

3.根据提示输入账号、密码和检查间隔。

Enter your campus-network account, password and check interval when prompted.

4.根据提示选择是否立即尝试一次登录来验证账号和密码的正确性。请注意，若验证时已登录，则会**注销**当前登录后再尝试新登录。

Follow the prompts to choose whether to attempt a login immediately to verify your account and password. If already authenticated, verification will **sign out** of the current campus-network session before attempting a new login.

5.如需修改账号、密码或检查间隔，重复第2步重新安装即可。

To change the account, password or check interval, repeat step 2 and reinstall.

6.如需验证是否安装成功或查看日志，关联用户版双击运行`status_link2user.cmd`，关联电脑版右键`status_link2PC.cmd`并选择“以管理员身份运行”。

To check installation status or view logs, double-click `status_link2user.cmd` for link2user, or right-click `status_link2PC.cmd` and choose **Run as administrator** for link2PC.

7.如需卸载该工具，关联用户版双击运行`uninstall_link2user.cmd`，关联电脑版右键`uninstall_link2PC.cmd`并选择“以管理员身份运行”，并根据提示清理对应路径残留文件。

To uninstall, double-click `uninstall_link2user.cmd` for link2user, or right-click `uninstall_link2PC.cmd` and choose **Run as administrator** for link2PC. Remove the retained files at the indicated paths if no longer needed.

## 安全性说明 / Security and privacy

本工具不向开发者或无关第三方上传个人数据，也不提供云端存储。为实现自动登录，仅在本机保存校园网账号（明文）、加密密码及运行日志；日志可能包含 Windows 用户名和 IP。认证所需的信息会发送给配置的校园网门户。

The tool does not upload personal data to its developer or unrelated third parties, and provides no cloud storage. Automatic login requires local storage of the campus account (plaintext), an encrypted password and runtime logs. Logs may include the Windows account name and IP address. Information required for authentication is sent to the configured campus-network portal.

- 关联用户版 / link2user：配置和日志位于 `%LOCALAPPDATA%\SrunAutoLogin`，密码使用 Windows DPAPI 当前用户范围加密（CurrentUser）。

  Configuration and logs are stored in `%LOCALAPPDATA%\SrunAutoLogin`. The password uses current-user Windows DPAPI encryption (CurrentUser).

- 关联电脑版 / link2PC：配置和日志位于 `%ProgramData%\SrunMachineLogin`，密码使用 Windows DPAPI 当前电脑范围加密（LocalMachine）。程序和数据目录仅限 SYSTEM 和管理员访问。

  Configuration and logs are stored in `%ProgramData%\SrunMachineLogin`. The password uses local-machine Windows DPAPI encryption (LocalMachine). Program and data directories are restricted to SYSTEM and administrators.

账号和日志不加密，DPAPI 也不防范本机管理员；默认 HTTP 门户不提供 HTTPS 传输保护。

Accounts and logs are not encrypted. DPAPI does not protect against local administrators, and the default HTTP portal does not provide HTTPS transport protection.

- 不要上传或公开运行目录、`config.json`、日志、账号密码、Cookie、HAR 或抓包文件；仅上传源码和说明。`.gitignore` 不保护 GitHub 网页手动上传。

  Do not upload or publish runtime directories, `config.json`, logs, credentials, cookies, HAR files or packet captures. Publish only source code and documentation. `.gitignore` does not protect GitHub's manual web uploads.

- 不要将账号密码写入源码、共享加密配置，或放宽关联电脑版的程序、数据目录权限（`%ProgramFiles%\SrunMachineLogin`、`%ProgramData%\SrunMachineLogin`）。

  Do not hard-code credentials, share encrypted configuration, or relax permissions on link2PC's program and data directories (`%ProgramFiles%\SrunMachineLogin`, `%ProgramData%\SrunMachineLogin`).

- 不要在无法现场处理断网的远程会话中贸然注销验证；跳过验证或结果未知后继续安装，不代表账号密码正确。

  Do not initiate sign-out verification remotely unless you can handle a lost connection at the computer. Skipping verification or continuing after an unknown result does not prove the credentials are correct.

## 其他 / Miscellaneous

### 联系方式 / Contact

如果你有任何问题、建议或反馈，欢迎通过以下方式联系我：

If you have any questions, suggestions or feedback, feel free to contact me:

- **电子邮箱 / Email**: [HarryZinc666@outlook.com](mailto:HarryZinc666@outlook.com)

### 碎碎念 / A few thoughts

本工具是之前自己为了自己远程控制电脑方便，vibe coding的一个小工具吧，希望能帮助到有需要的人。如果该工具对你有帮助，请不要忘记点亮收藏star，这对我很重要！

This is a little tool I vibe-coded to make remote access to my own computer easier. I hope it can help others who need it.

### 赞助支持 / Sponsor and Support

如果该工具对你有帮助，欢迎通过以下方式给作者补个🥚补充一下蛋白质。

If this tool has helped you, you're welcome to buy the author an egg 🥚 for a little extra protein:

- 点亮收藏⭐  / star⭐
- 推荐给朋友  /  recommend to friends
- [「微信赞赏码」 / 「WeChat donation code」](sponsor/wechat.png)
