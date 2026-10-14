# LiteBIT_Net：北京理工大学校园网自动登录工具

适用于Windows和Linux系统的北京理工大学校园网自动登录工具：

创建后台任务，定期查询深澜SRun门户的认证状态，并在未认证时自动重新登录。

Beijing Institute of Technology Campus Network Automatic Login Tool for Windows and Linux:


Create background tasks to regularly check the authentication status of the Shenlan SRun portal and automatically log in again if not authenticated.

**尤其适用于防止校园网意外断联导致的远程控制无法连接**。

**Especially useful for preventing remote access failures caused by unexpected campus-network disconnections.**

如果该工具对你有用，请不要忘记**点亮star**⭐和**将该工具推荐给朋友**！

If you find this tool useful, please don't forget to **star the project**⭐ and **recommend it to your friends**!

## 如何使用 / How to use

###  运行前提 / Requirements

- Windows系统为 Windows 10/11；Linux系统为 x64 或 ARM64 架构的 64 位系统，使用运行中的 systemd 245 或更新版本；Linux关联用户版还需要 systemd-logind 和正常的用户服务管理器。

  Windows requires Windows 10 or 11. Linux requires a 64-bit x64 or ARM64 system with running systemd 245 or later; the Linux user-linked version also requires systemd-logind and a working user service manager.

  **注意！Linux版已完成离线测试和两种架构的编译，但尚未在真实Linux设备上验证开机启动和校园网登录。**

  **Note! The Linux version has passed offline tests and has been built for both architectures, but startup at boot and campus-network login have not yet been verified on a real Linux device.**

- 可用的校园网账号和密码。

  A valid campus-network account and password are required.

- 网卡必须已通过有线网或 Wi-Fi 连接至校园网。但注意，**电脑处于睡眠、休眠及关机时工具无法正常工作**，如有需要请调整设备的自动睡眠、休眠及关机情况。

  The network adapter must already be connected to the campus network through Ethernet or Wi-Fi. **The tool cannot operate while the computer is asleep, hibernating or shut down.** Adjust automatic sleep, hibernation and shutdown settings if necessary.

- 注意: 当前版本针对已核对的 SRun V1.18/B2022·0802 门户，默认服务器为 `http://10.0.0.55`，参数为 `ac_id=8`、`n=200`、`type=1`、`double_stack=0`、`ignore=2`。它不是直接适用于其他学校的通用客户端；其他部署需先核对并调整对应参数。

  Note: This version targets the verified SRun V1.18/B2022·0802 portal at `http://10.0.0.55`, with `ac_id=8`, `n=200`, `type=1`, `double_stack=0` and `ignore=2`. It is not a universal client for other schools; verify and adjust the parameters before using it with another deployment.

### 版本选择 / Choosing a version

除操作系统外，该工具包含两个可独立安装的版本，请根据需求选择。

For each operating system, the tool offers two independently installable versions. Choose the one that fits your needs.

- **关联用户版**：设备由多人共享使用且只希望在自己的账户登录期间自动登录校园网。

  **User-linked version:** for a shared computer when you want automatic campus-network login only while your own account is signed in.

- **关联电脑版**：设备仅自己使用或不介意他人在使用时自动登录你的校园网；需要开机后无需操作也可以自动登录校园网。

  **PC-linked version:** for a computer you use alone, or when you accept automatic use of your campus account across users and want unattended login after boot.

具体区别如下：

The differences are listed below:


| 项目 / Item | 关联用户版 / User-linked | 关联电脑版 / PC-linked |
| --- | --- | --- |
| 启动条件 / Starts when | 配置任务的用户登录后 / After the configuring user signs in | Windows开机约 60 秒后；Linux网络初始化后等待约 60 秒 / Windows: about 60 seconds after boot; Linux: waits about 60 seconds after network initialization |
| 开机后尚未登录账户时 / Before the first sign-in after boot | 不进行校园网认证 / Does not authenticate to the campus network | 可以运行 / Can run |
| 执行账户 / Runs as | 配置任务的用户 / The configuring user | Windows为内置 SYSTEM 服务账户；Linux为 root / Windows: built-in SYSTEM account; Linux: root |
| 中止条件 / Stops when | Windows为创建任务的用户注销；Linux通常为该用户最后一个登录会话结束，可能有短暂延迟 / Windows: the configuring user signs out; Linux: normally after that user's last login session ends, possibly after a short delay | 设备关闭 / The PC is shut down |
| 安装权限 / Installation permissions | Windows通常无需管理员，系统策略可能限制；Linux不要使用 sudo / Windows: usually no administrator privileges required, subject to system policy; Linux: do not use sudo | Windows需要管理员权限；Linux使用 sudo / Windows: administrator privileges required; Linux: use sudo |
| 登录凭据 / Login credentials | 为配置任务的用户保存一组 / One set for the configuring user | 为整台电脑保存一组，不为每个用户分别保存 / One set for the entire PC, not a separate set for each user |
| 密码保护 / Password protection | Windows使用当前用户范围的 DPAPI；Linux使用 AES-256-GCM 和用户文件权限 / Windows: current-user DPAPI; Linux: AES-256-GCM and user file permissions | Windows使用当前电脑范围的 DPAPI，目录限 SYSTEM 和管理员访问；Linux使用 AES-256-GCM，目录限 root 访问 / Windows: local-machine DPAPI, with directories restricted to SYSTEM and administrators; Linux: AES-256-GCM, with directories restricted to root |

### 工具安装 / Installation

1.根据电脑操作系统及需要的版本，下载并打开对应文件夹，其中已包含该版本工具运行所需全部文件，可以删除除该文件夹外的其他文件。Windows系统选择`_win`，Linux系统选择`_linux`；关联用户版选择`_link2user`，关联电脑版打开`_link2PC`。

Download and open the folder matching your computer's operating system and preferred version. It contains all files needed to run that version, so you can delete the other files and folders. Choose `_win` for Windows or `_linux` for Linux, and `_link2user` for the user-linked version or `_link2PC` for the PC-linked version.

2.Windows系统的关联用户版双击运行`install_win_link2user.cmd`、关联电脑版右键`install_win_link2PC.cmd`并选择“以管理员身份运行”；Linux系统在对应文件夹的终端中运行，关联用户版运行`sh install_linux_link2user.sh`，关联电脑版运行`sudo sh install_linux_link2PC.sh`。

On Windows, double-click `install_win_link2user.cmd` for the user-linked version, or right-click `install_win_link2PC.cmd` and choose **Run as administrator** for the PC-linked version. On Linux, open a terminal in the corresponding folder and run `sh install_linux_link2user.sh` for the user-linked version, or `sudo sh install_linux_link2PC.sh` for the PC-linked version.

3.根据提示输入账号、密码和检查间隔等。

Enter your campus-network account, password, check interval and other requested information when prompted.

4.根据提示选择是否立即尝试一次登录来验证账号和密码的正确性。请注意，若验证时已登录，则会**注销**当前登录后再尝试新登录。

Follow the prompts to choose whether to attempt a login immediately to verify your account and password. If already authenticated, verification will **sign out** of the current campus-network session before attempting a new login.

5.如需修改账号、密码或检查间隔，重复第2步重新安装即可。

To change the account, password or check interval, repeat step 2 and reinstall.

6.如需验证是否安装成功或查看日志，Windows系统的关联用户版双击运行`status_win_link2user.cmd`、关联电脑版右键`status_win_link2PC.cmd`并选择“以管理员身份运行”；Linux系统在对应文件夹的终端中运行，关联用户版运行`sh status_linux_link2user.sh`，关联电脑版运行`sudo sh status_linux_link2PC.sh`。

To check installation status or view logs on Windows, double-click `status_win_link2user.cmd` for the user-linked version, or right-click `status_win_link2PC.cmd` and choose **Run as administrator** for the PC-linked version. On Linux, open a terminal in the corresponding folder and run `sh status_linux_link2user.sh` for the user-linked version, or `sudo sh status_linux_link2PC.sh` for the PC-linked version.

7.如需卸载该工具，Windows系统的关联用户版双击运行`uninstall_win_link2user.cmd`、关联电脑版右键`uninstall_win_link2PC.cmd`并选择“以管理员身份运行”；Linux系统在对应文件夹的终端中运行，关联用户版运行`sh uninstall_linux_link2user.sh`，关联电脑版运行`sudo sh uninstall_linux_link2PC.sh`，并根据提示清理对应路径残留文件。

To uninstall on Windows, double-click `uninstall_win_link2user.cmd` for the user-linked version, or right-click `uninstall_win_link2PC.cmd` and choose **Run as administrator** for the PC-linked version. On Linux, open a terminal in the corresponding folder and run `sh uninstall_linux_link2user.sh` for the user-linked version, or `sudo sh uninstall_linux_link2PC.sh` for the PC-linked version. Follow the prompts to remove retained files at the indicated paths.

## 安全性说明 / Security and privacy

本工具不向开发者或无关第三方上传个人数据，也不提供云端存储。为实现自动登录，仅在本机保存校园网账号（明文）、加密密码、配置及运行日志；日志可能包含 Windows 用户名或 Linux UID、IP、时间和状态码，不保存密码或原始认证请求。认证所需的信息会发送给配置的校园网门户。

The tool does not upload personal data to its developer or unrelated third parties, and provides no cloud storage. Automatic login requires local storage of the campus account (plaintext), an encrypted password, configuration and runtime logs. Logs may contain the Windows username or Linux UID, IP address, timestamps and status codes, but not passwords or raw authentication requests. Information required for authentication is sent to the configured campus-network portal.

- Windows关联用户版 / Windows link2user：配置和日志位于 `%LOCALAPPDATA%\SrunAutoLogin`，密码使用 Windows DPAPI 当前用户范围加密（CurrentUser）。

  Configuration and logs are stored in `%LOCALAPPDATA%\SrunAutoLogin`. The password uses current-user Windows DPAPI encryption (CurrentUser).

- Windows关联电脑版 / Windows link2PC：配置和日志位于 `%ProgramData%\SrunMachineLogin`，密码使用 Windows DPAPI 当前电脑范围加密（LocalMachine）。程序目录为 `%ProgramFiles%\SrunMachineLogin`；程序和数据目录仅限 SYSTEM 和管理员访问。

  Configuration and logs are stored in `%ProgramData%\SrunMachineLogin`. The password uses local-machine Windows DPAPI encryption (LocalMachine). Programs are installed in `%ProgramFiles%\SrunMachineLogin`; program and data directories are restricted to SYSTEM and administrators.

- Linux关联用户版 / Linux link2user：程序、配置、密钥和日志位于 `~/.local/share/LiteBIT_Net_linux_link2user`，属于安装时的用户。

  Programs, configuration, key and logs are stored in `~/.local/share/LiteBIT_Net_linux_link2user`, owned by the installing user.

- Linux关联电脑版 / Linux link2PC：配置、密钥和日志位于 `/var/lib/LiteBIT_Net_linux_link2PC`，程序位于 `/opt/LiteBIT_Net_linux_link2PC`，属于 root。

  Configuration, key and logs are stored in `/var/lib/LiteBIT_Net_linux_link2PC`; programs are installed in `/opt/LiteBIT_Net_linux_link2PC`. These directories are owned by root.

Linux两个版本均在程序内使用 AES-256-GCM 加密密码，并校验配置元数据；随机密钥保存在 `credential.key`。目录权限为 700，配置、密钥和日志为 600，不将密码或密钥写入命令行、环境变量或临时明文文件。这不是硬件绑定加密，同时获得配置和密钥的人可以解密密码。

Both Linux versions encrypt passwords inside the program with AES-256-GCM and authenticate configuration metadata. A random key is stored in `credential.key`. Directories use mode 700; configuration, key and logs use mode 600. Passwords and keys are not written to command arguments, environment variables or temporary plaintext files. This encryption is not hardware-bound; anyone who obtains both configuration and key can decrypt the password.

账号和日志不加密，本机管理员、root 或相应执行用户仍可能访问凭据；默认 HTTP 门户不提供 HTTPS 传输保护。卸载只移除后台任务或服务，不删除保存的程序、配置、密钥和日志，也不会注销已有的校园网登录。

Accounts and logs are not encrypted. Local administrators, root or the corresponding execution user may still access credentials, and the default HTTP portal does not provide HTTPS transport protection. Uninstalling removes only the background task or service; it retains program files, configuration, keys and logs, and does not sign out of the existing campus-network session.

- 不要上传或公开安装后生成的运行目录、`config.json`、`credential.key`、日志、账号密码、Cookie、HAR 或抓包文件；仅上传发布目录中的工具、源码和说明。不要认为 `.gitignore` 能保护 GitHub 网页手动上传。

  Do not upload or publish installed runtime directories, `config.json`, `credential.key`, logs, credentials, cookies, HAR files or packet captures. Publish only tools, source code and documentation from the release folder. Do not assume `.gitignore` protects GitHub's manual web uploads.

- 不要将账号密码写入源码、共享加密配置或密钥，或放宽任何版本的程序、数据目录权限。

  Do not hard-code credentials, share encrypted configuration or keys, or relax permissions on any version's program and data directories.

- 不要在无法现场处理断网的远程会话中贸然注销验证；跳过验证或结果未知后继续安装，不代表账号密码正确。

  Do not initiate sign-out verification remotely unless you can handle a lost connection at the computer. Skipping verification or continuing after an unknown result does not prove the credentials are correct.

## 其他 / Miscellaneous

### 联系方式 / Contact

如果你有任何问题、建议或反馈，欢迎通过以下方式联系我：

If you have any questions, suggestions or feedback, feel free to contact me:

- **电子邮箱 / Email**: [HarryZinc666@outlook.com](mailto:HarryZinc666@outlook.com)

### 碎碎念 / A few thoughts

本工具是之前自己为了自己远程控制电脑方便，vibe coding的一个小工具吧，希望能帮助到有需要的人。后来发现大家有需求，就加上了对Linux系统的支持。如果该工具对你有帮助，请不要忘记点亮收藏star，这对我很重要！

This is a little tool I vibe-coded to make remote access to my own computer easier. I hope it can help others who need it. Later, when I found that others wanted it too, I added Linux support. If this tool is helpful to you, please don't forget to star the project—it means a lot to me!

### 赞助支持 / Sponsor and Support

如果该工具对你有帮助，欢迎通过以下方式给作者补个🥚补充一下蛋白质。

If this tool has helped you, you're welcome to buy the author an egg 🥚 for a little extra protein:

- 点亮收藏⭐  / star⭐
- 推荐给朋友  /  recommend to friends
- [「微信赞赏码」 / 「WeChat donation code」](sponsor/wechat.png)

你的赞助支持将用于：

Your sponsor and support will be used to:

- 为AI会员续费  /  Renewal of AI membership
- 租用域名、服务器来支持接下来项目的开发  /  Renting domain names and servers to support the development of future projects
