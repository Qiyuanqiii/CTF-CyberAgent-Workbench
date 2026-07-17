# CyberAgent Workbench Desktop Plan

状态：路线图已确定；非 schema 的 D1-A 路径隔离 Skill 预览 Go 桥已完成，D0 Wails 壳、原生文件对话框、桌面二进制与安装包尚未实现。

## 目标

在保留 `cyberagent` CLI 为一等入口的同时，提供面向日常使用的 Windows 桌面端。桌面端参考 Codex 的项目/任务线程、活动流、Diff 审查、审批队列和多 Agent 观察体验，但使用 CyberAgent Workbench 自己的 Code/Cyber、Plan/Deliver、Run、Policy、Skill 和 Sandbox 模型，不复制其视觉资产或私有实现。

## 架构决定

- Go 仍是唯一控制平面；Policy、Scope、预算、SQLite、模型、工具、文件、Docker、Shell 和密钥均由 Go 管理。
- React/Vite 复用现有 `web/` 控制台，桌面外壳优先评估 Wails，并在技术验证后固定一个经过审计的稳定版本。
- TypeScript 业务操作继续使用 Go 拥有的 HTTP/SSE/OpenAPI 契约；桌面绑定只处理窗口、通知和受控文件选择等操作系统能力，不建立第二套业务 API。
- Electron 不作为默认方案，避免 Node 成为并行高权限控制面。Tauri 不作为默认方案，避免 Rust 从确定性分析工具变成桌面主控。
- 桌面端不直接读取 API key，不直接控制 Docker/Shell，不实现 Scope 或文件权限判断，也不在 Local Storage、前端日志或注册表保存凭证。
- CLI、TUI、Web 和 Desktop 必须投影同一 Run/Event/Approval 状态；关闭窗口不得隐式取消后台 Run。

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
- 该边界当前没有产品入口，不接入 HTTP/OpenAPI/React，也不创建数据库或运行事件；ADR 0033 记录了未来 Wails 只能绑定“打开原生对话框”的要求。

## 分阶段交付

### D0：桌面基础验证（现在可做）

- 建立 Wails 技术验证和 `cmd/cyberagent-desktop` 边界。
- 嵌入现有 React read-first 控制台，复用 Go 同源 API/SSE/OpenAPI；v64 档位选择仍只调用 Go control route。
- 固定窗口生命周期、单实例策略、端口/令牌传递和 CLI 并存时的 SQLite/Run lease 行为。
- 生产模式禁止远程导航和开发者工具，增加 CSP、资源完整性、窗口来源与外部链接边界测试。
- 只输出开发版/便携测试二进制；不做安装器、注册表、自启动、自动更新、协议关联或后台服务。
- 将 ADR 0033 的 Go selector 接到原生文件对话框；渲染层只能消费一次性句柄，不得提交路径、文件字节或 multipart upload。

### D1：日常工作台（产品可用度约 65-70%）

- 增加由 Go API 驱动的 Run 创建、Session 对话、Plan 选择、引导队列、审批、Diff 审查和 Skill 管理。
- 所有 mutation 使用独立 control token、Origin/Host 校验、幂等 operation key 和 typed errors。
- 完成 CLI/Desktop 并发、崩溃恢复、窗口重开、后台 Run 与断线续传测试。
- Code 与 Cyber 保持不同 Skill 目录和风险呈现；桌面切换不改变 Run 内不可变模式。

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

- Windows 10/11 x64 的启动、关闭、恢复、离线和 WebView2 缺失路径有自动化测试。
- 渲染进程无法绕过 Go API 访问 Shell、Docker、密钥或工作区外文件。
- CLI 与 Desktop 同时运行时，SQLite 不损坏、operation 重放不分叉、Run lease 不被窗口生命周期偷取。
- 安装、升级和卸载不会静默删除 Workspace、数据库、凭证或用户创建文件。
- 未签名开发产物不得伪装成正式发布；正式包必须有可核验签名和哈希。
