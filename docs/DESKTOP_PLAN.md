# CyberAgent Workbench Desktop Plan

状态：Desktop D0-A、D0-B 与 schema-v72 D1-R1 自动化核心已完成。Wails v2.13.0 Windows 壳、嵌入式 React bundle、进程内 Go API、同库恢复、高水位事件续传、WebView2 失败关闭、内存令牌、原生 `.zip` 对话框、路径隔离 Skill 预览和显式 capability 下的受控 Run 创建已经通过完整本地发布门；Windows 10 实机矩阵、Session/Plan/审批/Diff/Skill mutation、安装包、正式便携发行、注册表、自启动、更新和高权限执行仍未实现。

## 目标

在保留 `cyberagent` CLI 为一等入口的同时，提供面向日常使用的 Windows 桌面端。桌面端参考 Codex 的项目/任务线程、活动流、Diff 审查、审批队列和多 Agent 观察体验，但使用 CyberAgent Workbench 自己的 Code/Cyber、Plan/Deliver、Run、Policy、Skill 和 Sandbox 模型，不复制其视觉资产或私有实现。

## 架构决定

- Go 仍是唯一控制平面；Policy、Scope、预算、SQLite、模型、工具、文件、Docker、Shell 和密钥均由 Go 管理。
- React/Vite 复用现有 `web/` 控制台；桌面外壳已固定 Wails v2.13.0 稳定版。v3 仍处于 Alpha，不进入当前主线。
- TypeScript 业务操作继续使用 Go 拥有的 HTTP/OpenAPI 契约；普通 Web 保留 SSE，Windows Desktop 因 Wails v2 AssetServer 不支持流式响应而使用同一事件表、同一 Run-bound 高水位 cursor 的 `run-event-poll.v1`。桌面绑定只处理内存启动材料和受控文件选择，不建立第二套业务 API。
- Electron 不作为默认方案，避免 Node 成为并行高权限控制面。Tauri 不作为默认方案，避免 Rust 从确定性分析工具变成桌面主控。
- 桌面端不直接读取 API key，不直接控制 Docker/Shell，不实现 Scope 或文件权限判断，也不在 Local Storage、前端日志或注册表保存凭证。
- CLI、TUI、Web 和 Desktop 必须投影同一 Run/Event/Approval 状态；关闭窗口不得隐式取消后台 Run。D0-A/D0-B 只读取已有状态，D1-R1 只创建关闭的 Run 图；两者都不取得 Run execution lease。

## 目标布局

- 左栏：Project、Workspace、Run、Session 与任务状态。
- 中栏：对话、执行进度、事件流、排队输入和交付结果。
- 右栏：Plan、WorkItems、Diff、Agents、Tools、Approvals、Findings 与 Artifacts。
- 底部：输入区、Code/Cyber 工作面、Plan/Deliver 阶段、Provider/Model 和预算摘要。
- 独立页面：Skills、Providers、Policy、Workspace、审计记录和桌面设置。

所有高风险操作必须显示 Go 返回的结构化候选、Scope、风险代码和审批要求；前端文案不能自行推导授权结果。

## 已完成的前置边界

- `skills.ReadPackageFile` 已成为 CLI 与未来 Desktop 共用的有界普通文件读取器；它拒绝 symlink、目录、空/超限包和首尾空白改写路径，并在读取前后复核身份且不回显路径。
- `desktop.NewSkillPackagePreviewBoundary` 把未来原生选择器和渲染桥拆成两个值：只有 Go selector 接收路径，renderer bridge 只接收 256-bit 不透明句柄。
- Go 在发放句柄前完成严格包校验并立即丢弃路径/正文；内存最多保留 16 份投影，五分钟过期且单次消费。
- `desktop_skill_package_preview.v1` 只返回有界风险元数据，排除路径、文件名、正文、Manifest description/content path/content digest，并固定安装、命令、网络、Provider、工具和能力授权为 false。
- D0-A 已把该边界接入 Wails 原生对话框和 React 只读预览。渲染层仍不能提交路径、文件字节或安装请求，也不会因预览创建数据库事实或 Run 事件；ADR 0033 与 ADR 0034 分别记录路径隔离和桌面壳边界。

## D0-A/D0-B/D1-R1 当前实现

- `cmd/cyberagent-desktop` 只在 Windows `desktop,wv2runtime.error` build tags 下编译，production 构建再增加 `production`；默认 read-only。显式 `--enable-profile-control` 只开放 schema v64 非授权档位选择，显式 `--enable-run-creation` 只开放 schema v72 受控 Run 创建；两项 capability 独立，creation-only 不能访问取消/档位 route。
- `web/dist` 以 compile-time embed 进入二进制；Go 在启动前验证 index、内容哈希资源、类型、数量、单项/总大小并复制为不可变内存快照。
- Wails AssetServer 直接调用现有 `httpapi.API` Handler，不监听 TCP 端口；同一 Go 层继续负责 Bearer、Host、CSP、Policy、SQLite 和 DTO。
- Renderer 绑定面只有 `Bootstrap`、`SelectSkillPackage`、`PreviewSkillPackage` 三个方法。进程、Shell、Docker、Skill 安装和 renderer path input 权限全部固定为 false。
- read/control token 每次进程启动随机生成，只驻留内存，不写 Local Storage、SQLite、日志、命令输出或注册表；两者不得相同。只有至少一个显式 control capability 开启时才生成 control token。
- 单实例、窗口恢复、禁用 WebView 文件拖放/默认右键菜单、renderer code integrity、路径隔离错误和 bounded startup dialog 已接入。
- `desktop.ControlPlane` 与 `desktop.Lifecycle` 固定同库 API 所有权、幂等关闭、崩溃重开、第二实例让位和停止后永久静默；第二实例参数与工作目录不会进入主实例。
- Desktop 通过 `GET /api/v1/runs/{run_id}/events/poll` 消费与 SSE 相同的真实事件 frame/cursor；React 最多在内存保留 16 个 Run、每个 500 帧，不写浏览器存储。
- WebView2 `94.0.992.31` 以上只读预检发生在 bundle/数据库之前；失败时不下载、不安装、不打开 URL。进程内适配器只接受精确 `http://wails.localhost`，外部链接、表单和 popup 在 Desktop renderer 中被阻止。
- secure production-tag 二进制已经在隔离数据目录通过 Windows 11 强制结束/重开与第二实例实机烟测；主工作台、Skill modal 与原生 `.zip` 对话框也已通过视觉复核。D1-R1 的 Run 创建 route、能力分离和 React 对话框由自动化覆盖，正式发布前仍需随最终二进制复跑完整 Windows 10/11 人工矩阵。

本地构建：

```powershell
powershell -ExecutionPolicy Bypass -File scripts/build-desktop.ps1
```

输出位于 `build/desktop/cyberagent-desktop.exe`，默认不开放 control token。该文件是未签名开发/便携测试产物，不是正式发行包。机器需要 Windows 10/11 和 WebView2 Evergreen Runtime `94.0.992.31` 或更新版本；缺失时应用只给出本机指导，不会隐式安装。测试其他数据目录时，可在启动前设置 `CYBERAGENT_HOME`；桌面渲染层无法读取或更改这个路径。

显式启用受控 Run 创建：

```powershell
.\build\desktop\cyberagent-desktop.exe --enable-run-creation

# 可与非授权档位选择共同启用。
.\build\desktop\cyberagent-desktop.exe --enable-run-creation --enable-profile-control
```

创建只接受已注册 Workspace，并固定默认预算、禁用网络/目标和 `preview/noop` 执行档位；它不会自动发送 Session 消息、调用模型、取得 execution lease 或启动进程。

## 分阶段交付

### D0：桌面基础验证（自动化核心完成，Windows 10 实机待补）

- [x] 固定 Wails v2.13.0 和 `cmd/cyberagent-desktop` Windows build-tag 边界。
- [x] 嵌入现有 React read-first 控制台，以进程内 Handler 复用 Go API/OpenAPI；v64 档位选择仍只调用 Go control route。
- [x] 建立随机内存令牌、单实例、窗口恢复、生产 bundle、CSP 与原生 `.zip` 对话框；渲染层只消费一次性句柄。
- [x] 输出未签名开发/便携测试二进制，不创建安装器、注册表、自启动、更新、协议关联或后台服务。
- [x] D0-B 完成 CLI/Desktop 同库并发、关闭/崩溃重开、单实例恢复、poll/SSE cursor 互续与 WebView2 缺失/过旧/探测失败诊断。
- [x] D0-B 增加精确 renderer origin、规范 `RequestURI`、外部 navigation/form/popup 阻断、secure build-tag 门禁、Windows CI 和 Windows 11 实机恢复记录；仍不增加业务 mutation。
- [ ] 在正式便携或签名发行前补齐 Windows 10 x64 实机启动、第二实例、强制结束/重开和 WebView2 缺失路径矩阵。

### D1：日常工作台（产品可用度约 65-70%）

- [x] D1-R1 / schema v72：Go API 受控创建 Mission/Run/Session，严格注册 Workspace、Scope、默认预算、幂等 operation、事务事件和关闭 execution profile；React 可选择 Workspace/Profile/Surface/Phase 并在成功后刷新、选中新 Run。
- [x] D1-R1 capability 与 `--enable-profile-control` 独立；creation-only token 不能访问旧控制 route，Wails native bridge 不增加方法。
- [x] D1-R1 完整门禁通过全仓普通/race、普通/secure-Desktop 检查、45 项前端测试、确定性契约、生产构建、依赖/隐私/Markdown 扫描和隔离 CLI smoke；没有已知未解决高/中风险。
- [ ] D1-S1：增加 Run-bound Session 对话与操作者引导提交，复用 RunSupervisor、队列、预算、Policy、脱敏和事件语义。
- [ ] 随后的独立切片增加 Plan 选择/阶段切换、审批、Diff 审查和 Skill 管理；每类 mutation 分别做状态、幂等、权限与重启审计。
- [ ] 所有 mutation 使用独立 control token、Origin/Host 校验、幂等 operation key 和 typed errors；CLI/Desktop 并发、窗口重开、后台 Run、重放与断线续传不得只沿用 D0 结论。
- [ ] Code 与 Cyber 保持不同 Skill 目录和风险呈现；桌面切换不改变 Run 内不可变模式。

### D2：Windows Beta 分发（产品可用度约 75-80%）

- 提供便携 ZIP 和签名 MSIX；检测 WebView2 Evergreen Runtime，缺失时使用受控引导或依赖安装。
- 固定 per-user 安装、升级、降级、卸载、用户数据保留/删除和崩溃日志策略。
- 代码签名材料只进入受保护 CI secret/签名服务，不进入仓库或普通构建日志。
- 发布产物生成 SBOM、哈希、签名和可复现版本元数据；自动更新仍需独立安全审查。

### D3：按需扩展

- 企业 MSI、Microsoft Store、远程环境、`cyberagent://` 协议、文件关联、开机启动和后台服务分别立项。
- 任何自动更新必须验证签名、版本通道、防降级策略、原子替换和失败回滚。
- macOS/Linux 打包只在 Windows Beta 稳定后评估，不让跨平台包装阻塞 Go 核心能力。

## 安装与注册表边界

- 开发和便携版不要求安装包，也不要求应用自定义注册表键。
- MSIX/MSI 可由系统/安装器登记应用身份和卸载信息；业务状态、凭证和 Policy 不存注册表。
- 文件关联、自定义协议、自启动、服务和右键菜单会扩大系统权限面，D0-D2 默认不启用。
- 安装器不得请求管理员权限，除非未来某个独立企业场景给出不可替代的理由并通过审计。

## 发布门禁

- Windows 自动化覆盖启动前置条件、关闭/恢复、同库续传和 renderer 边界；Windows 11 x64 已实机验证，Windows 10 x64 仍是正式发行前必过矩阵。
- 渲染进程无法绕过 Go API 访问 Shell、Docker、密钥或工作区外文件。
- CLI 与 Desktop 同时运行时，SQLite 不损坏、operation 重放不分叉、Run lease 不被窗口生命周期或 Run 创建偷取。
- 安装、升级和卸载不会静默删除 Workspace、数据库、凭证或用户创建文件。
- 未签名开发产物不得伪装成正式发布；正式包必须有可核验签名和哈希。

ADR 0034、ADR 0035 与 ADR 0036 分别记录 D0-A 可见壳、D0-B 生命周期/事件续传加固和 D1-R1 受控 Run 创建。Wails 使用 MIT 许可证；D2 生成任何可分发 ZIP/MSIX 前必须把 Wails 及其他运行时依赖的许可证/notice、SBOM 和哈希一起打包。
